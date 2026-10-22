package engine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/juanrubio/outfitter/internal/catalog"
	"github.com/juanrubio/outfitter/internal/domain"
)

const (
	selfAssetName = "outfitter_0.6.0_linux_amd64"
	selfNewScript = "#!/bin/sh\necho 'outfitter version 0.6.0'\n"
	selfOldScript = "#!/bin/sh\necho 'outfitter version 0.5.0'\n"
)

func selfTestCatalog() *catalog.Catalog {
	settings := domain.DefaultSettings()
	settings.GitHubTokenFromGH = false
	return &catalog.Catalog{Settings: settings}
}

func selfReleaseServer(t *testing.T, requests *atomic.Int32, downloads *atomic.Int32, corrupt bool) *httptest.Server {
	t.Helper()
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/juanrubio/outfitter/releases/latest":
			requests.Add(1)
			payload := map[string]any{
				"tag_name": "v0.6.0",
				"html_url": server.URL + "/release/v0.6.0",
				"assets": []map[string]any{
					{"name": selfAssetName, "browser_download_url": server.URL + "/download/" + selfAssetName},
					{"name": "SHA256SUMS", "browser_download_url": server.URL + "/download/SHA256SUMS"},
				},
			}
			_ = json.NewEncoder(w).Encode(payload)
		case "/download/" + selfAssetName:
			downloads.Add(1)
			_, _ = w.Write([]byte(selfNewScript))
		case "/download/SHA256SUMS":
			downloads.Add(1)
			sum := sha256.Sum256([]byte(selfNewScript))
			digest := hex.EncodeToString(sum[:])
			if corrupt {
				digest = strings.Repeat("0", 64)
			}
			_, _ = w.Write([]byte(digest + "  " + selfAssetName + "\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func selfUpdaterForTest(home, exe, api string) *SelfUpdater {
	updater := NewSelfUpdater("0.5.0", home)
	updater.ExecutablePath = exe
	updater.GitHubAPIBaseURL = api
	updater.GOOS = "linux"
	updater.GOARCH = "amd64"
	return updater
}

func TestSelfUpdaterCheckOutdatedAndCaches(t *testing.T) {
	home := t.TempDir()
	exe := filepath.Join(home, ".local", "bin", "outfitter")
	writeTestFile(t, exe, selfOldScript, 0o755)
	var requests, downloads atomic.Int32
	server := selfReleaseServer(t, &requests, &downloads, false)
	updater := selfUpdaterForTest(home, exe, server.URL)

	state, err := updater.Check(context.Background(), selfTestCatalog(), false)
	if err != nil {
		t.Fatal(err)
	}
	if state.CurrentVersion != "0.5.0" || state.LatestVersion != "0.6.0" || !state.Outdated {
		t.Fatalf("state = %#v", state)
	}
	if state.InstallKind != SelfInstallStandalone || !state.CanSelfUpdate {
		t.Fatalf("install classification = %q can=%v", state.InstallKind, state.CanSelfUpdate)
	}
	if _, err := updater.Check(context.Background(), selfTestCatalog(), false); err != nil {
		t.Fatal(err)
	}
	if requests.Load() != 1 {
		t.Fatalf("release requests = %d, want cached single request", requests.Load())
	}
	if _, err := updater.Check(context.Background(), selfTestCatalog(), true); err != nil {
		t.Fatal(err)
	}
	if requests.Load() != 2 {
		t.Fatalf("forced release requests = %d, want 2", requests.Load())
	}
	cached := updater.CachedState(selfTestCatalog())
	if !cached.Outdated || cached.LatestVersion != "0.6.0" {
		t.Fatalf("cached state = %#v", cached)
	}
}

func TestSelfUpdaterCheckSkipsDevAndOptOut(t *testing.T) {
	var requests, downloads atomic.Int32
	server := selfReleaseServer(t, &requests, &downloads, false)

	dev := NewSelfUpdater("dev", t.TempDir())
	dev.ExecutablePath = filepath.Join(t.TempDir(), "outfitter")
	dev.GitHubAPIBaseURL = server.URL
	state, err := dev.Check(context.Background(), selfTestCatalog(), true)
	if err != nil {
		t.Fatal(err)
	}
	if state.Outdated || state.CanSelfUpdate || state.InstallKind != SelfInstallDevelopment {
		t.Fatalf("dev state = %#v", state)
	}

	cat := selfTestCatalog()
	cat.Settings.SelfUpdateCheck = false
	home := t.TempDir()
	exe := filepath.Join(home, "outfitter")
	writeTestFile(t, exe, selfOldScript, 0o755)
	updater := selfUpdaterForTest(home, exe, server.URL)
	state, err = updater.Check(context.Background(), cat, false)
	if err != nil {
		t.Fatal(err)
	}
	if state.Outdated || state.LatestVersion != "" {
		t.Fatalf("opt-out state = %#v", state)
	}
	if requests.Load() != 0 {
		t.Fatalf("opt-out made %d network requests", requests.Load())
	}

	// An explicit user request bypasses the background opt-out.
	state, err = updater.CheckExplicit(context.Background(), cat)
	if err != nil {
		t.Fatal(err)
	}
	if !state.Outdated || state.LatestVersion != "0.6.0" || requests.Load() != 1 {
		t.Fatalf("explicit state = %#v requests=%d", state, requests.Load())
	}
}

func TestSelfUpdaterFailedRefreshKeepsLastKnownRelease(t *testing.T) {
	home := t.TempDir()
	exe := filepath.Join(home, "outfitter")
	writeTestFile(t, exe, selfOldScript, 0o755)
	var calls atomic.Int32
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) > 1 {
			http.Error(w, "boom", http.StatusInternalServerError)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"tag_name": "v0.6.0",
			"html_url": server.URL + "/release/v0.6.0",
			"assets":   []map[string]any{},
		})
	}))
	defer server.Close()
	updater := selfUpdaterForTest(home, exe, server.URL)

	if _, err := updater.Check(context.Background(), selfTestCatalog(), true); err != nil {
		t.Fatal(err)
	}
	state, err := updater.Check(context.Background(), selfTestCatalog(), true)
	if err == nil {
		t.Fatal("second check should report the network failure")
	}
	if !state.Outdated || state.LatestVersion != "0.6.0" || state.LastError == "" {
		t.Fatalf("failed-refresh state = %#v", state)
	}
	if cached := updater.CachedState(selfTestCatalog()); !cached.Outdated {
		t.Fatalf("cached state after failed refresh = %#v", cached)
	}
}

func TestSelfUpdaterDryRunAndStandaloneUpdate(t *testing.T) {
	home := t.TempDir()
	exe := filepath.Join(home, ".local", "bin", "outfitter")
	writeTestFile(t, exe, selfOldScript, 0o755)
	var requests, downloads atomic.Int32
	server := selfReleaseServer(t, &requests, &downloads, false)
	updater := selfUpdaterForTest(home, exe, server.URL)

	result, err := updater.Update(context.Background(), selfTestCatalog(), SelfUpdateOptions{DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if result.Updated || !result.DryRun || !result.State.Outdated {
		t.Fatalf("dry-run result = %#v", result)
	}
	if downloads.Load() != 0 {
		t.Fatalf("dry run downloaded %d assets", downloads.Load())
	}
	if got, _ := os.ReadFile(exe); string(got) != selfOldScript {
		t.Fatalf("dry run changed binary: %q", got)
	}

	result, err = updater.Update(context.Background(), selfTestCatalog(), SelfUpdateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Updated || !result.RequiresRestart || result.BackupPath == "" {
		t.Fatalf("update result = %#v", result)
	}
	if got, _ := os.ReadFile(exe); string(got) != selfNewScript {
		t.Fatalf("updated binary = %q", got)
	}
	if got, _ := os.ReadFile(result.BackupPath); string(got) != selfOldScript {
		t.Fatalf("backup = %q", got)
	}
}

func TestSelfUpdaterBadChecksumLeavesBinary(t *testing.T) {
	home := t.TempDir()
	exe := filepath.Join(home, "outfitter")
	writeTestFile(t, exe, selfOldScript, 0o755)
	var requests, downloads atomic.Int32
	server := selfReleaseServer(t, &requests, &downloads, true)
	updater := selfUpdaterForTest(home, exe, server.URL)

	if _, err := updater.Update(context.Background(), selfTestCatalog(), SelfUpdateOptions{}); err == nil ||
		!strings.Contains(err.Error(), "checksum") {
		t.Fatalf("update error = %v", err)
	}
	if got, _ := os.ReadFile(exe); string(got) != selfOldScript {
		t.Fatalf("corrupt update changed binary: %q", got)
	}
}

func TestSelfUpdaterMiseManagedUpdate(t *testing.T) {
	home := t.TempDir()
	exe := filepath.Join(home, ".local", "share", "mise", "shims", "outfitter")
	newExe := filepath.Join(home, ".local", "share", "mise", "installs", "ubi-juanrubio-outfitter", "0.6.0", "outfitter")
	writeTestFile(t, exe, selfOldScript, 0o755)
	writeTestFile(t, newExe, selfNewScript, 0o755)
	var requests, downloads atomic.Int32
	server := selfReleaseServer(t, &requests, &downloads, false)
	updater := selfUpdaterForTest(home, exe, server.URL)
	runner := &fakeRunner{respond: func(_ context.Context, name string, args ...string) ([]byte, error) {
		switch {
		case filepath.Base(name) == "mise" && len(args) == 3 && args[0] == "use":
			return []byte("installed\n"), nil
		case filepath.Base(name) == "mise" && len(args) == 2 && args[0] == "which":
			return []byte(newExe + "\n"), nil
		case name == newExe && len(args) == 1 && args[0] == "--version":
			return []byte("outfitter version 0.6.0\n"), nil
		default:
			return nil, nil
		}
	}}
	updater.Runner = runner
	updater.LookPath = func(name string) (string, error) {
		if name == "mise" {
			return "/fake/mise", nil
		}
		return "", os.ErrNotExist
	}

	result, err := updater.Update(context.Background(), selfTestCatalog(), SelfUpdateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Updated || result.State.InstallKind != SelfInstallMise {
		t.Fatalf("mise result = %#v", result)
	}
	if !runner.called("/fake/mise", "use", "--global", "ubi:juanrubio/outfitter[exe=outfitter]@0.6.0") {
		t.Fatalf("runner calls = %#v", runner.callsSnapshot())
	}
	if downloads.Load() != 0 {
		t.Fatalf("mise update downloaded %d direct assets", downloads.Load())
	}
}

func TestSelfUpdaterReadOnlyStandaloneIsManual(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can write to read-only directories")
	}
	home := t.TempDir()
	dir := filepath.Join(home, "readonly")
	exe := filepath.Join(dir, "outfitter")
	writeTestFile(t, exe, selfOldScript, 0o755)
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
	var requests, downloads atomic.Int32
	server := selfReleaseServer(t, &requests, &downloads, false)
	updater := selfUpdaterForTest(home, exe, server.URL)

	state, err := updater.Check(context.Background(), selfTestCatalog(), false)
	if err != nil {
		t.Fatal(err)
	}
	if !state.Outdated || state.CanSelfUpdate || state.InstallKind != SelfInstallManual || state.ManualHint == "" {
		t.Fatalf("manual state = %#v", state)
	}
	if _, err := updater.Update(context.Background(), selfTestCatalog(), SelfUpdateOptions{}); err == nil {
		t.Fatal("manual install unexpectedly updated")
	}
}
