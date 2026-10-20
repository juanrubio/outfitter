package engine

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/juanrubio/outfitter/internal/domain"
)

func lookPathFromMap(paths map[string]string) func(string) (string, error) {
	return func(name string) (string, error) {
		if path, ok := paths[name]; ok {
			return path, nil
		}
		return "", errors.New("executable not found: " + name)
	}
}

func symlinkOrFatal(t *testing.T, target, link string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatalf("mkdir link parent: %v", err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("symlink %s -> %s: %v", link, target, err)
	}
}

func TestInspectLocalAllBackends(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	writeTestFile(t, filepath.Join(home, ".local", "bin", "muse"), "launcher", 0o755)
	writeTestFile(t, filepath.Join(home, ".local", "bin", ".muse-version"), "v1.4.10-R6427.1\n", 0o644)
	writeTestFile(t, filepath.Join(home, ".local", "bin", "quotary"), "binary", 0o755)

	uvTarget := filepath.Join(home, ".local", "share", "uv", "tools", "browser-use", "bin", "browser-use")
	writeTestFile(t, uvTarget, "binary", 0o755)
	symlinkOrFatal(t, uvTarget, filepath.Join(home, ".local", "bin", "browser-use"))

	codexbarTarget := filepath.Join(home, ".local", "share", "outfitter", "codexbar", "codexbar")
	writeTestFile(t, codexbarTarget, "binary", 0o755)
	symlinkOrFatal(t, codexbarTarget, filepath.Join(home, ".local", "bin", "codexbar"))

	miseJSON := strings.ReplaceAll(`{
		"npm:@openai/codex": [{"version": "0.163.0", "install_path": "$HOME/.local/share/mise/installs/node/24/lib/node_modules/@openai/codex", "installed": true, "active": true}],
		"aqua:jesseduffield/lazygit": [{"version": "0.40.2", "install_path": "$HOME/.local/share/mise/installs/aqua/jesseduffield/lazygit", "installed": true}],
		"ubi:juanrubio/boardwalk": [{"version": "v0.4.2", "install_path": "$HOME/.local/share/mise/installs/ubi/juanrubio/boardwalk", "installed": true}],
		"github:example/gh-tool": [{"version": "2.0.0", "install_path": "$HOME/.local/share/mise/installs/github/example/gh-tool", "installed": true}]
	}`, "$HOME", home)

	runner := &fakeRunner{respond: func(_ context.Context, name string, args ...string) ([]byte, error) {
		if filepath.Base(name) == "mise" && len(args) == 3 && args[0] == "ls" {
			return []byte(miseJSON), nil
		}
		if name == "dpkg-query" && len(args) == 3 {
			switch args[2] {
			case "tmux":
				return []byte("3.4-1ubuntu1\n"), nil
			default:
				return nil, errors.New("dpkg-query: package not installed")
			}
		}
		switch filepath.Base(name) {
		case "browser-use":
			return []byte("browser-use 0.5.1\n"), nil
		case "codexbar":
			return []byte("codexbar version 0.77.0\n"), nil
		case "quotary":
			return []byte("quotary 0.1.0-dev\n"), nil
		case "ext":
			return []byte("ext 9.9.9\n"), nil
		default:
			return nil, fmt.Errorf("unexpected command %s %v", name, args)
		}
	}}

	lookPath := lookPathFromMap(map[string]string{
		"mise":        "/usr/local/bin/mise",
		"codex":       filepath.Join(home, ".local", "share", "mise", "shims", "codex"),
		"lazygit":     filepath.Join(home, ".local", "share", "mise", "shims", "lazygit"),
		"boardwalk":   filepath.Join(home, ".local", "bin", "boardwalk"),
		"browser-use": filepath.Join(home, ".local", "bin", "browser-use"),
		"tmux":        "/usr/bin/tmux",
		"muse":        filepath.Join(home, ".local", "bin", "muse"),
		"codexbar":    filepath.Join(home, ".local", "bin", "codexbar"),
		"quotary":     filepath.Join(home, ".local", "bin", "quotary"),
		"ext":         "/usr/local/bin/ext",
	})

	cat := testCatalog(
		testTool("codex", "codex", domain.BackendMiseNPM, "@openai/codex"),
		testTool("lazygit", "lazygit", domain.BackendMiseAqua, "jesseduffield/lazygit"),
		testTool("boardwalk", "boardwalk", domain.BackendMiseUbi, "juanrubio/boardwalk[exe=boardwalk]"),
		testTool("github-tool", "gh-tool", domain.BackendMiseGitHub, "example/gh-tool"),
		testTool("browser-use", "browser-use", domain.BackendUV, "browser-use"),
		testTool("tmux", "tmux", domain.BackendAPT, "tmux"),
		testTool("muse", "muse", domain.BackendVendor, "api.meta.ai/muse-code (muse-stable)"),
		testTool("codexbar", "codexbar", domain.BackendVendor, "steipete/CodexBar"),
		testTool("quotary", "quotary", domain.BackendSource, "juanrubio/quotary"),
		testTool("ext", "ext", domain.BackendMiseAqua, "example/ext"),
		testTool("missing", "missing", domain.BackendMiseNPM, "@missing/pkg"),
	)

	inspector := &Inspector{HomeDir: home, Runner: runner, LookPath: lookPath}
	states := inspector.InspectLocal(context.Background(), cat)
	if len(states) != len(cat.Tools) {
		t.Fatalf("states = %d, want %d", len(states), len(cat.Tools))
	}

	tests := []struct {
		id         string
		provenance domain.Provenance
		version    string
		detail     string
	}{
		{id: "codex", provenance: domain.ProvenanceMise, version: "0.163.0", detail: "mise"},
		{id: "lazygit", provenance: domain.ProvenanceMise, version: "0.40.2", detail: "mise"},
		{id: "boardwalk", provenance: domain.ProvenanceMise, version: "0.4.2", detail: "mise"},
		{id: "github-tool", provenance: domain.ProvenanceMise, version: "2.0.0", detail: "mise"},
		{id: "browser-use", provenance: domain.ProvenanceUV, version: "0.5.1", detail: "uv"},
		{id: "tmux", provenance: domain.ProvenanceAPT, version: "3.4", detail: "APT"},
		{id: "muse", provenance: domain.ProvenanceVendor, version: "1.4.10-R6427.1", detail: "muse-version"},
		{id: "codexbar", provenance: domain.ProvenanceVendor, version: "0.77.0", detail: "Vendor bundle"},
		{id: "quotary", provenance: domain.ProvenanceSource, version: "0.1.0-dev", detail: "source"},
		{id: "ext", provenance: domain.ProvenanceExternal, version: "9.9.9", detail: "External"},
	}
	for _, test := range tests {
		state, ok := states[test.id]
		if !ok {
			t.Errorf("%s: missing state", test.id)
			continue
		}
		if state.Provenance != test.provenance {
			t.Errorf("%s: provenance = %q, want %q (detail %q)", test.id, state.Provenance, test.provenance, state.ProvenanceDetail)
		}
		if state.InstalledVersion != test.version {
			t.Errorf("%s: installed version = %q, want %q", test.id, state.InstalledVersion, test.version)
		}
		if !strings.Contains(state.ProvenanceDetail, test.detail) {
			t.Errorf("%s: detail = %q, want containing %q", test.id, state.ProvenanceDetail, test.detail)
		}
		if state.CheckStatus != domain.StatusUnknown {
			t.Errorf("%s: check status = %q, want unknown", test.id, state.CheckStatus)
		}
		if state.BinaryPath == "" {
			t.Errorf("%s: binary path is empty", test.id)
		}
	}

	missing := states["missing"]
	if missing.Provenance != domain.ProvenanceNone || missing.InstalledVersion != "" {
		t.Fatalf("missing tool state = %+v", missing)
	}
	if !runner.called("/usr/local/bin/mise", "ls", "--installed", "--json") {
		t.Fatalf("mise ls was not called: %v", runner.callsSnapshot())
	}
}

// TestInspectLocalUnmanagedSystemBinariesAreExternal covers binaries that
// live in system paths (or whose catalog backend is apt) but are not owned
// by dpkg: a stray /usr/bin copy of a mise tool, a manually installed
// /usr/local/bin binary, and a self-built copy of an apt tool. None of
// them may be reported as APT packages, because apt cannot update them.
func TestInspectLocalUnmanagedSystemBinariesAreExternal(t *testing.T) {
	t.Parallel()
	home := t.TempDir()

	runner := &fakeRunner{respond: func(_ context.Context, name string, args ...string) ([]byte, error) {
		if filepath.Base(name) == "mise" && len(args) == 3 && args[0] == "ls" {
			return []byte(`{}`), nil
		}
		if name == "dpkg-query" {
			return nil, errors.New("dpkg-query: package not installed")
		}
		switch filepath.Base(name) {
		case "dust":
			return []byte("Dust 1.2.4\n"), nil
		case "broot":
			return []byte("broot 1.2.4\n"), nil
		case "tig":
			return []byte("tig version 2.5.2\n"), nil
		default:
			return nil, fmt.Errorf("unexpected command %s %v", name, args)
		}
	}}

	lookPath := lookPathFromMap(map[string]string{
		"mise":  "/usr/local/bin/mise",
		"dust":  "/usr/bin/dust",
		"broot": "/usr/local/bin/broot",
		"tig":   "/usr/local/bin/tig",
	})

	cat := testCatalog(
		testTool("dust", "dust", domain.BackendMiseAqua, "bootandy/dust"),
		testTool("broot", "broot", domain.BackendMiseUbi, "Canop/broot"),
		testTool("tig", "tig", domain.BackendAPT, "tig"),
	)

	inspector := &Inspector{HomeDir: home, Runner: runner, LookPath: lookPath}
	states := inspector.InspectLocal(context.Background(), cat)

	wantVersions := map[string]string{"dust": "1.2.4", "broot": "1.2.4", "tig": "2.5.2"}
	for id, version := range wantVersions {
		state, ok := states[id]
		if !ok {
			t.Errorf("%s: missing state", id)
			continue
		}
		if state.Provenance != domain.ProvenanceExternal {
			t.Errorf("%s: provenance = %q (detail %q), want %q",
				id, state.Provenance, state.ProvenanceDetail, domain.ProvenanceExternal)
		}
		if state.InstalledVersion != version {
			t.Errorf("%s: installed version = %q, want %q", id, state.InstalledVersion, version)
		}
	}
}

func TestInspectLocalShadowedManagedInstalls(t *testing.T) {
	t.Parallel()
	home := t.TempDir()

	miseJSON := strings.ReplaceAll(`{
		"aqua:bootandy/dust": [{"version": "1.2.6", "install_path": "$HOME/.local/share/mise/installs/aqua/bootandy/dust/1.2.6", "installed": true, "active": true}],
		"npm:@openai/codex": [{"version": "0.163.0", "install_path": "$HOME/.local/share/mise/installs/node/24/lib/node_modules/@openai/codex", "installed": true, "active": true}]
	}`, "$HOME", home)

	runner := &fakeRunner{respond: func(_ context.Context, name string, args ...string) ([]byte, error) {
		if filepath.Base(name) == "mise" && len(args) == 3 && args[0] == "ls" {
			return []byte(miseJSON), nil
		}
		if name == "dpkg-query" && len(args) == 3 {
			if args[2] == "tig" {
				return []byte("2.6.0-2\n"), nil
			}
			return nil, errors.New("dpkg-query: package not installed")
		}
		switch filepath.Base(name) {
		case "dust":
			return []byte("Dust 1.2.4\n"), nil
		case "tig":
			return []byte("tig version 2.5.2\n"), nil
		case "codex":
			return []byte("codex 0.163.0\n"), nil
		default:
			return nil, fmt.Errorf("unexpected command %s %v", name, args)
		}
	}}

	lookPath := lookPathFromMap(map[string]string{
		"mise":  "/usr/local/bin/mise",
		"dust":  "/usr/bin/dust",
		"tig":   "/usr/local/bin/tig",
		"codex": filepath.Join(home, ".local", "share", "mise", "shims", "codex"),
	})

	cat := testCatalog(
		testTool("dust", "dust", domain.BackendMiseAqua, "bootandy/dust"),
		testTool("tig", "tig", domain.BackendAPT, "tig"),
		testTool("codex", "codex", domain.BackendMiseNPM, "@openai/codex"),
	)

	inspector := &Inspector{HomeDir: home, Runner: runner, LookPath: lookPath}
	states := inspector.InspectLocal(context.Background(), cat)

	dust := states["dust"]
	if dust.InstalledVersion != "1.2.6" {
		t.Errorf("dust: InstalledVersion = %q, want %q", dust.InstalledVersion, "1.2.6")
	}
	if dust.Provenance != domain.ProvenanceMise {
		t.Errorf("dust: Provenance = %q, want %q", dust.Provenance, domain.ProvenanceMise)
	}
	if dust.ShadowedBy != "/usr/bin/dust" {
		t.Errorf("dust: ShadowedBy = %q, want %q", dust.ShadowedBy, "/usr/bin/dust")
	}
	if dust.ShadowVersion != "1.2.4" {
		t.Errorf("dust: ShadowVersion = %q, want %q", dust.ShadowVersion, "1.2.4")
	}

	tig := states["tig"]
	if tig.InstalledVersion != "2.6.0" {
		t.Errorf("tig: InstalledVersion = %q, want %q", tig.InstalledVersion, "2.6.0")
	}
	if tig.Provenance != domain.ProvenanceAPT {
		t.Errorf("tig: Provenance = %q, want %q", tig.Provenance, domain.ProvenanceAPT)
	}
	if tig.ShadowedBy != "/usr/local/bin/tig" {
		t.Errorf("tig: ShadowedBy = %q, want %q", tig.ShadowedBy, "/usr/local/bin/tig")
	}
	if tig.ShadowVersion != "2.5.2" {
		t.Errorf("tig: ShadowVersion = %q, want %q", tig.ShadowVersion, "2.5.2")
	}

	codex := states["codex"]
	if codex.ShadowedBy != "" {
		t.Errorf("codex: ShadowedBy = %q, want empty", codex.ShadowedBy)
	}
	if codex.ShadowVersion != "" {
		t.Errorf("codex: ShadowVersion = %q, want empty", codex.ShadowVersion)
	}
}

func TestInspectLocalMiseSliceFormat(t *testing.T) {
	t.Parallel()
	runner := &fakeRunner{respond: func(_ context.Context, name string, args ...string) ([]byte, error) {
		if filepath.Base(name) == "mise" && len(args) == 3 {
			return []byte(`[{"name":"npm:foo","version":"v1.2.3","install_path":"/mise/installs/foo"}]`), nil
		}
		return nil, errors.New("unexpected command")
	}}
	cat := testCatalog(testTool("foo-tool", "foo", domain.BackendMiseNPM, "foo"))
	inspector := &Inspector{
		HomeDir:  t.TempDir(),
		Runner:   runner,
		LookPath: lookPathFromMap(map[string]string{"mise": "/usr/local/bin/mise"}),
	}
	states := inspector.InspectLocal(context.Background(), cat)
	state := states["foo-tool"]
	if state.Provenance != domain.ProvenanceMise || state.InstalledVersion != "1.2.3" {
		t.Fatalf("state = %+v", state)
	}
}

func TestInspectLocalFallsBackToLocalBin(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	writeTestFile(t, filepath.Join(home, ".local", "bin", "tool"), "binary", 0o755)
	runner := &fakeRunner{respond: func(_ context.Context, name string, args ...string) ([]byte, error) {
		if name == filepath.Join(home, ".local", "bin", "tool") {
			return []byte("tool version v2.1.0"), nil
		}
		return nil, errors.New("unexpected command")
	}}
	cat := testCatalog(testTool("tool", "tool", domain.BackendVendor, "vendor/tool"))
	inspector := &Inspector{HomeDir: home, Runner: runner, LookPath: lookPathFromMap(nil)}
	state := inspector.InspectLocal(context.Background(), cat)["tool"]
	if state.InstalledVersion != "2.1.0" || state.Provenance != domain.ProvenanceVendor {
		t.Fatalf("state = %+v", state)
	}
}

func TestNormalizeVersion(t *testing.T) {
	t.Parallel()
	tests := map[string]string{
		"v1.2.3":         "1.2.3",
		"V2.0.1":         "2.0.1",
		"1:3.4-1ubuntu1": "3.4",
		"3.4-1":          "3.4",
		"1.4.10-R6427.1": "1.4.10-R6427.1",
		" 2026.9.6\n":    "2026.9.6",
		"1.2.3-beta.1":   "1.2.3-beta.1",
		"0.1.0-dev":      "0.1.0-dev",
		"jq-1.8.2":       "1.8.2",
		"tmux-3.5a":      "3.5a",
		"release-v2.4.0": "2.4.0",
	}
	for input, want := range tests {
		if got := NormalizeVersion(input); got != want {
			t.Errorf("NormalizeVersion(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestInspectUpstreamGitHubTokenAndCallbacks(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	authHeaders := make([]string, 0, 2)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		authHeaders = append(authHeaders, r.Header.Get("Authorization"))
		mu.Unlock()
		switch r.URL.Path {
		case "/repos/juanrubio/boardwalk/releases/latest":
			_, _ = w.Write([]byte(`{"tag_name":"v0.4.2"}`))
		case "/repos/steipete/CodexBar/releases/latest":
			_, _ = w.Write([]byte(`{"tag_name":"0.77.0"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	runner := &fakeRunner{respond: func(_ context.Context, name string, args ...string) ([]byte, error) {
		if name == "gh" && len(args) == 2 {
			return []byte("secret-token\n"), nil
		}
		if name == "mise" {
			return nil, errors.New("mise latest unavailable")
		}
		return nil, fmt.Errorf("unexpected command %s", name)
	}}
	cat := testCatalog(
		testTool("boardwalk", "boardwalk", domain.BackendMiseUbi, "juanrubio/boardwalk"),
		testTool("codexbar", "codexbar", domain.BackendVendor, "steipete/CodexBar"),
	)
	cat.Settings.GitHubTokenFromGH = true

	current := map[string]domain.ToolState{
		"boardwalk": {InstalledVersion: "0.4.1", Provenance: domain.ProvenanceMise},
		"codexbar":  {InstalledVersion: "0.76.0", Provenance: domain.ProvenanceVendor},
	}
	var updates []string
	inspector := &Inspector{
		HomeDir:          t.TempDir(),
		Runner:           runner,
		LookPath:         lookPathFromMap(map[string]string{"gh": "/usr/bin/gh"}),
		HTTPClient:       server.Client(),
		GitHubAPIBaseURL: server.URL,
		Concurrency:      2,
	}
	states := inspector.InspectUpstream(context.Background(), cat, current, func(id string, state domain.ToolState) {
		updates = append(updates, id)
		if state.CheckStatus != domain.StatusReady {
			t.Errorf("%s: callback status = %q", id, state.CheckStatus)
		}
	})

	if states["boardwalk"].LatestVersion != "0.4.2" || states["boardwalk"].CheckStatus != domain.StatusReady {
		t.Fatalf("boardwalk state = %+v", states["boardwalk"])
	}
	if states["codexbar"].LatestVersion != "0.77.0" {
		t.Fatalf("codexbar state = %+v", states["codexbar"])
	}
	if len(updates) != 2 {
		t.Fatalf("updates = %v, want 2 callbacks", updates)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(authHeaders) != 2 {
		t.Fatalf("GitHub requests = %d, want 2", len(authHeaders))
	}
	for _, header := range authHeaders {
		if header != "Bearer secret-token" {
			t.Fatalf("Authorization header = %q", header)
		}
	}
}

func TestInspectUpstreamGitHubTokenDisabled(t *testing.T) {
	t.Parallel()
	var authHeader string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"tag_name":"v1.0.0"}`))
	}))
	defer server.Close()

	runner := &fakeRunner{respond: func(_ context.Context, name string, args ...string) ([]byte, error) {
		if name == "gh" {
			return []byte("should-not-be-used"), nil
		}
		return nil, errors.New("mise unavailable")
	}}
	cat := testCatalog(testTool("boardwalk", "boardwalk", domain.BackendMiseUbi, "juanrubio/boardwalk"))
	cat.Settings.GitHubTokenFromGH = false

	inspector := &Inspector{
		HomeDir:          t.TempDir(),
		Runner:           runner,
		LookPath:         lookPathFromMap(map[string]string{"gh": "/usr/bin/gh", "mise": "/usr/local/bin/mise"}),
		GitHubAPIBaseURL: server.URL,
	}
	state := inspector.InspectUpstream(context.Background(), cat, nil, nil)["boardwalk"]
	if state.LatestVersion != "1.0.0" {
		t.Fatalf("state = %+v", state)
	}
	if authHeader != "" {
		t.Fatalf("Authorization header = %q, want empty", authHeader)
	}
	if runner.called("gh", "auth", "token") {
		t.Fatal("gh auth token was called while disabled")
	}
}

func TestInspectUpstreamRegistryFallbacks(t *testing.T) {
	t.Parallel()

	t.Run("npm", func(t *testing.T) {
		t.Parallel()
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !strings.Contains(r.URL.Path, "openai") || !strings.HasSuffix(r.URL.Path, "/latest") {
				http.NotFound(w, r)
				return
			}
			_, _ = w.Write([]byte(`{"version":"0.165.0"}`))
		}))
		defer server.Close()

		cat := testCatalog(testTool("codex", "codex", domain.BackendMiseNPM, "@openai/codex"))
		inspector := &Inspector{
			HomeDir:            t.TempDir(),
			Runner:             &fakeRunner{respond: func(context.Context, string, ...string) ([]byte, error) { return nil, errors.New("mise unavailable") }},
			LookPath:           lookPathFromMap(nil),
			NPMRegistryBaseURL: server.URL,
		}
		state := inspector.InspectUpstream(context.Background(), cat, nil, nil)["codex"]
		if state.LatestVersion != "0.165.0" || state.CheckStatus != domain.StatusReady {
			t.Fatalf("state = %+v", state)
		}
	})

	t.Run("pypi", func(t *testing.T) {
		t.Parallel()
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/pypi/browser-use/json" {
				http.NotFound(w, r)
				return
			}
			_, _ = w.Write([]byte(`{"info":{"version":"0.5.1"}}`))
		}))
		defer server.Close()

		cat := testCatalog(testTool("browser-use", "browser-use", domain.BackendUV, "browser-use"))
		inspector := &Inspector{
			HomeDir:     t.TempDir(),
			Runner:      &fakeRunner{},
			LookPath:    lookPathFromMap(nil),
			PyPIBaseURL: server.URL,
		}
		state := inspector.InspectUpstream(context.Background(), cat, nil, nil)["browser-use"]
		if state.LatestVersion != "0.5.1" || state.CheckStatus != domain.StatusReady {
			t.Fatalf("state = %+v", state)
		}
	})

	t.Run("apt", func(t *testing.T) {
		t.Parallel()
		runner := &fakeRunner{respond: func(_ context.Context, name string, args ...string) ([]byte, error) {
			if name == "apt-cache" && len(args) == 2 && args[0] == "policy" {
				return []byte("tmux:\n  Installed: 3.3a-3\n  Candidate: 1:3.4-1ubuntu1\n  Version table:\n"), nil
			}
			return nil, errors.New("unexpected command")
		}}
		cat := testCatalog(testTool("tmux", "tmux", domain.BackendAPT, "tmux"))
		inspector := &Inspector{HomeDir: t.TempDir(), Runner: runner, LookPath: lookPathFromMap(nil)}
		state := inspector.InspectUpstream(context.Background(), cat, nil, nil)["tmux"]
		if state.LatestVersion != "3.4" || state.CheckStatus != domain.StatusReady {
			t.Fatalf("state = %+v", state)
		}
		if !runner.called("apt-cache", "policy", "tmux") {
			t.Fatalf("apt-cache policy was not called: %v", runner.callsSnapshot())
		}
	})

	t.Run("mise latest", func(t *testing.T) {
		t.Parallel()
		runner := &fakeRunner{respond: func(_ context.Context, name string, args ...string) ([]byte, error) {
			if name == "/usr/local/bin/mise" && len(args) == 2 && args[0] == "latest" {
				return []byte("mise WARN using 2024.1.0 cache\n0.170.0\n"), nil
			}
			return nil, errors.New("unexpected command")
		}}
		cat := testCatalog(testTool("codex", "codex", domain.BackendMiseNPM, "@openai/codex"))
		inspector := &Inspector{
			HomeDir:  t.TempDir(),
			Runner:   runner,
			LookPath: lookPathFromMap(map[string]string{"mise": "/usr/local/bin/mise"}),
		}
		state := inspector.InspectUpstream(context.Background(), cat, nil, nil)["codex"]
		if state.LatestVersion != "0.170.0" || state.CheckStatus != domain.StatusReady {
			t.Fatalf("state = %+v", state)
		}
	})

	t.Run("mise latest falls back to registry", func(t *testing.T) {
		t.Parallel()
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"version":"1.1.1"}`))
		}))
		defer server.Close()
		runner := &fakeRunner{respond: func(_ context.Context, name string, args ...string) ([]byte, error) {
			if name == "/usr/local/bin/mise" {
				return nil, errors.New("mise failed")
			}
			return nil, errors.New("unexpected command")
		}}
		cat := testCatalog(testTool("codex", "codex", domain.BackendMiseNPM, "@openai/codex"))
		inspector := &Inspector{
			HomeDir:            t.TempDir(),
			Runner:             runner,
			LookPath:           lookPathFromMap(map[string]string{"mise": "/usr/local/bin/mise"}),
			NPMRegistryBaseURL: server.URL,
		}
		state := inspector.InspectUpstream(context.Background(), cat, nil, nil)["codex"]
		if state.LatestVersion != "1.1.1" || state.CheckStatus != domain.StatusReady {
			t.Fatalf("state = %+v", state)
		}
	})
}

func TestInspectUpstreamVendorAndSourcePreserveKnownState(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	writeTestFile(t, filepath.Join(home, ".local", "bin", ".muse-release-info.json"),
		`{"latest":{"version":"v1.4.12-R6501.0"}}`, 0o644)

	runner := &fakeRunner{respond: func(_ context.Context, name string, args ...string) ([]byte, error) {
		return nil, errors.New("mise unavailable")
	}}
	cat := testCatalog(
		testTool("muse", "muse", domain.BackendVendor, "api.meta.ai/muse-code (muse-stable)"),
		testTool("quotary", "quotary", domain.BackendSource, "juanrubio/quotary"),
	)
	current := map[string]domain.ToolState{
		"muse":    {InstalledVersion: "1.4.10-R6427.1", LatestVersion: "1.4.12-R6501.0", Provenance: domain.ProvenanceVendor},
		"quotary": {InstalledVersion: "0.1.0-dev", LatestVersion: "0.1.0-dev", Provenance: domain.ProvenanceSource},
	}
	inspector := &Inspector{HomeDir: home, Runner: runner, LookPath: lookPathFromMap(nil)}
	states := inspector.InspectUpstream(context.Background(), cat, current, nil)

	if states["muse"].LatestVersion != "1.4.12-R6501.0" || states["muse"].CheckStatus != domain.StatusReady {
		t.Fatalf("muse state = %+v", states["muse"])
	}
	if states["quotary"].LatestVersion != "0.1.0-dev" || states["quotary"].CheckStatus != domain.StatusReady {
		t.Fatalf("quotary state = %+v", states["quotary"])
	}
	if states["quotary"].LastError != "" {
		t.Fatalf("source tool reported an error: %+v", states["quotary"])
	}
}

func TestInspectUpstreamVendorPreservesStateOnReachabilityError(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer server.Close()

	cat := testCatalog(testTool("codexbar", "codexbar", domain.BackendVendor, "steipete/CodexBar"))
	current := map[string]domain.ToolState{
		"codexbar": {InstalledVersion: "0.76.0", LatestVersion: "0.77.0", Provenance: domain.ProvenanceVendor},
	}
	inspector := &Inspector{
		HomeDir:          t.TempDir(),
		Runner:           &fakeRunner{},
		LookPath:         lookPathFromMap(nil),
		GitHubAPIBaseURL: server.URL,
	}
	state := inspector.InspectUpstream(context.Background(), cat, current, nil)["codexbar"]
	if state.LatestVersion != "0.77.0" || state.CheckStatus != domain.StatusReady || state.LastError != "" {
		t.Fatalf("state = %+v", state)
	}
}

func TestInspectUpstreamReportsErrors(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "registry down", http.StatusInternalServerError)
	}))
	defer server.Close()

	cat := testCatalog(testTool("codex", "codex", domain.BackendMiseNPM, "@openai/codex"))
	updates := 0
	inspector := &Inspector{
		HomeDir:            t.TempDir(),
		Runner:             &fakeRunner{},
		LookPath:           lookPathFromMap(nil),
		NPMRegistryBaseURL: server.URL,
	}
	states := inspector.InspectUpstream(context.Background(), cat, nil, func(string, domain.ToolState) {
		updates++
	})
	state := states["codex"]
	if state.CheckStatus != domain.StatusError {
		t.Fatalf("check status = %q, want error", state.CheckStatus)
	}
	if !strings.Contains(state.LastError, "500") {
		t.Fatalf("last error = %q, want status detail", state.LastError)
	}
	if updates != 1 {
		t.Fatalf("updates = %d, want 1", updates)
	}
}

func TestInspectUpstreamBoundedConcurrency(t *testing.T) {
	t.Parallel()
	var inFlight int32
	var maximum int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		current := atomic.AddInt32(&inFlight, 1)
		for {
			observed := atomic.LoadInt32(&maximum)
			if current <= observed || atomic.CompareAndSwapInt32(&maximum, observed, current) {
				break
			}
		}
		time.Sleep(15 * time.Millisecond)
		atomic.AddInt32(&inFlight, -1)
		_, _ = w.Write([]byte(`{"version":"1.0.0"}`))
	}))
	defer server.Close()

	tools := make([]domain.Tool, 0, 8)
	for index := 0; index < 8; index++ {
		tools = append(tools, testTool(fmt.Sprintf("tool-%d", index), fmt.Sprintf("tool-%d", index),
			domain.BackendMiseNPM, fmt.Sprintf("@scope/tool-%d", index)))
	}
	cat := testCatalog(tools...)
	inspector := &Inspector{
		HomeDir:            t.TempDir(),
		Runner:             &fakeRunner{},
		LookPath:           lookPathFromMap(nil),
		NPMRegistryBaseURL: server.URL,
		Concurrency:        2,
	}
	states := inspector.InspectUpstream(context.Background(), cat, nil, nil)
	if len(states) != 8 {
		t.Fatalf("states = %d, want 8", len(states))
	}
	if maximum > 2 {
		t.Fatalf("maximum in-flight requests = %d, want <= 2", maximum)
	}
	if maximum < 2 {
		t.Fatalf("maximum in-flight requests = %d, want 2", maximum)
	}
	for id, state := range states {
		if state.LatestVersion != "1.0.0" || state.CheckStatus != domain.StatusReady {
			t.Fatalf("%s state = %+v", id, state)
		}
	}
}

func TestGitHubRepoParsing(t *testing.T) {
	t.Parallel()
	tests := []struct {
		pkg       string
		wantOwner string
		wantRepo  string
		wantOK    bool
	}{
		{pkg: "juanrubio/boardwalk[exe=boardwalk]", wantOwner: "juanrubio", wantRepo: "boardwalk", wantOK: true},
		{pkg: "ubi:juanrubio/boardwalk", wantOwner: "juanrubio", wantRepo: "boardwalk", wantOK: true},
		{pkg: "api.meta.ai/muse-code (muse-stable)", wantOK: false},
		{pkg: "browser-use", wantOK: false},
		{pkg: "a/b/c", wantOK: false},
	}
	for _, test := range tests {
		owner, repo, ok := githubRepo(test.pkg)
		if ok != test.wantOK || owner != test.wantOwner || repo != test.wantRepo {
			t.Errorf("githubRepo(%q) = %q, %q, %v", test.pkg, owner, repo, ok)
		}
	}
}

func TestExtractVersion(t *testing.T) {
	t.Parallel()
	tests := []struct {
		output  string
		pattern string
		want    string
	}{
		{output: "lazygit version v0.40.2", want: "0.40.2"},
		{output: "tmux 3.4", pattern: `tmux ([0-9][0-9A-Za-z.~+-]*)`, want: "3.4"},
		{output: "gh version 2.62.0 (2025-01-01)", pattern: `gh version ([0-9]+\.[0-9]+\.[0-9]+)`, want: "2.62.0"},
		{output: "no version here", want: ""},
		{output: "v1.2.3-beta.1", want: "1.2.3-beta.1"},
	}
	for _, test := range tests {
		if got := extractVersion(test.output, test.pattern); got != test.want {
			t.Errorf("extractVersion(%q, %q) = %q, want %q", test.output, test.pattern, got, test.want)
		}
	}
}

func TestInspectorDefaults(t *testing.T) {
	t.Parallel()
	inspector := (&Inspector{}).withDefaults()
	if inspector.Concurrency != defaultConcurrency {
		t.Fatalf("concurrency = %d", inspector.Concurrency)
	}
	if inspector.GitHubAPIBaseURL != defaultGitHubAPIURL ||
		inspector.NPMRegistryBaseURL != defaultNPMRegistry ||
		inspector.PyPIBaseURL != defaultPyPIURL {
		t.Fatalf("base URLs not defaulted: %+v", inspector)
	}
	if inspector.Runner == nil || inspector.LookPath == nil || inspector.ReadLink == nil ||
		inspector.ReadFile == nil || inspector.Stat == nil || inspector.HTTPClient == nil {
		t.Fatalf("callbacks not defaulted: %+v", inspector)
	}
}

func TestInspectLocalNilCatalog(t *testing.T) {
	t.Parallel()
	states := NewInspector(t.TempDir()).InspectLocal(context.Background(), nil)
	if len(states) != 0 {
		t.Fatalf("states = %v", states)
	}
}

func TestResolveSymlinkLoopIsBounded(t *testing.T) {
	t.Parallel()
	inspector := &Inspector{
		ReadLink: func(path string) (string, error) {
			return path, nil
		},
	}
	if got := inspector.resolveSymlink("/tmp/loop"); got != "/tmp/loop" {
		t.Fatalf("resolveSymlink = %q", got)
	}
}

func TestMuseInstalledPathIsPreferred(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	musePath := filepath.Join(home, ".local", "bin", "muse")
	writeTestFile(t, musePath, "launcher", 0o755)
	writeTestFile(t, filepath.Join(home, ".local", "bin", ".muse-version"), "1.0.0", 0o644)

	cat := testCatalog(testTool("muse", "muse", domain.BackendVendor, "api.meta.ai/muse-code"))
	inspector := &Inspector{
		HomeDir:  home,
		Runner:   &fakeRunner{},
		LookPath: lookPathFromMap(map[string]string{"muse": "/opt/muse"}), // must be ignored
	}
	state := inspector.InspectLocal(context.Background(), cat)["muse"]
	if state.BinaryPath != musePath || state.InstalledVersion != "1.0.0" ||
		state.Provenance != domain.ProvenanceVendor {
		t.Fatalf("state = %+v", state)
	}
}
