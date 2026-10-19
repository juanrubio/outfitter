package engine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func checksumLine(data []byte, name string) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]) + "  " + name
}

func TestBootstrapMiseSuccess(t *testing.T) {
	t.Parallel()
	archive := makeTarGz(t, []tarFile{
		{Name: ".", Content: ""},
		{Name: "mise/bin/mise", Content: "#!/bin/sh\necho mise 2024.1.0\n"},
	})
	sums := checksumLine(archive, "mise-latest-linux-x64.tar.gz") + "\n"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/SHASUMS256.txt":
			_, _ = w.Write([]byte(sums))
		case "/mise-latest-linux-x64.tar.gz":
			_, _ = w.Write(archive)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	installDir := t.TempDir()
	runner := &fakeRunner{respond: func(context.Context, string, ...string) ([]byte, error) {
		return []byte("mise 2024.1.0\n"), nil
	}}
	path, err := BootstrapMise(context.Background(), BootstrapOptions{
		BaseURL:    server.URL,
		InstallDir: installDir,
		OS:         "linux",
		Arch:       "amd64",
		Runner:     runner,
	})
	if err != nil {
		t.Fatalf("BootstrapMise: %v", err)
	}
	expected := filepath.Join(installDir, "mise")
	if path != expected {
		t.Fatalf("path = %q, want %q", path, expected)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat installed binary: %v", err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Fatalf("mode = %o, want 0755", info.Mode().Perm())
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read installed binary: %v", err)
	}
	if !strings.Contains(string(content), "mise 2024.1.0") {
		t.Fatalf("unexpected binary content %q", content)
	}
	if !runner.called(path, "--version") {
		t.Fatalf("verification runner not called with %s --version: %v", path, runner.callsSnapshot())
	}
}

func TestBootstrapMiseVersionedArchiveName(t *testing.T) {
	t.Parallel()
	archive := makeTarGz(t, []tarFile{
		{Name: ".", Content: ""},
		{Name: "mise/bin/mise", Content: "#!/bin/sh\necho mise 2026.10.15\n"},
	})
	sums := strings.Join([]string{
		strings.Repeat("a", 64) + "  ./mise-v2026.10.15-linux-x64-musl.tar.gz",
		checksumLine(archive, "./mise-v2026.10.15-linux-x64.tar.gz"),
		"",
	}, "\n")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/SHASUMS256.txt":
			_, _ = w.Write([]byte(sums))
		case "/mise-v2026.10.15-linux-x64.tar.gz":
			_, _ = w.Write(archive)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	installDir := t.TempDir()
	path, err := BootstrapMise(context.Background(), BootstrapOptions{
		BaseURL:    server.URL,
		InstallDir: installDir,
		OS:         "linux",
		Arch:       "amd64",
	})
	if err != nil {
		t.Fatalf("BootstrapMise: %v", err)
	}
	if path != filepath.Join(installDir, "mise") {
		t.Fatalf("path = %q", path)
	}
}

func TestBootstrapMiseChecksumMismatchFailsClosed(t *testing.T) {
	t.Parallel()
	archive := makeTarGz(t, []tarFile{{Name: "mise/bin/mise", Content: "binary"}})
	sums := strings.Repeat("0", 64) + "  mise-latest-linux-x64.tar.gz\n"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/SHASUMS256.txt":
			_, _ = w.Write([]byte(sums))
		case "/mise-latest-linux-x64.tar.gz":
			_, _ = w.Write(archive)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	installDir := t.TempDir()
	_, err := BootstrapMise(context.Background(), BootstrapOptions{
		BaseURL: server.URL, InstallDir: installDir, OS: "linux", Arch: "amd64",
	})
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("error = %v, want checksum mismatch", err)
	}
	if _, statErr := os.Stat(filepath.Join(installDir, "mise")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("mise binary was left behind: %v", statErr)
	}
	entries, readErr := os.ReadDir(installDir)
	if readErr != nil {
		t.Fatalf("read install dir: %v", readErr)
	}
	if len(entries) != 0 {
		t.Fatalf("install dir is not clean: %v", entries)
	}
}

func TestBootstrapMiseMissingChecksumEntry(t *testing.T) {
	t.Parallel()
	archive := makeTarGz(t, []tarFile{{Name: "mise/bin/mise", Content: "binary"}})
	sums := checksumLine(archive, "mise-latest-linux-arm64.tar.gz") + "\n"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/SHASUMS256.txt" {
			_, _ = w.Write([]byte(sums))
			return
		}
		_, _ = w.Write(archive)
	}))
	defer server.Close()

	installDir := t.TempDir()
	_, err := BootstrapMise(context.Background(), BootstrapOptions{
		BaseURL: server.URL, InstallDir: installDir, OS: "linux", Arch: "amd64",
	})
	if err == nil || !strings.Contains(err.Error(), "not found in SHASUMS256.txt") {
		t.Fatalf("error = %v, want missing checksum entry", err)
	}
}

func TestBootstrapMiseHTTPFailures(t *testing.T) {
	t.Parallel()
	archive := makeTarGz(t, []tarFile{{Name: "mise/bin/mise", Content: "binary"}})
	tests := []struct {
		name          string
		checksumCode  int
		archiveCode   int
		wantSubstring string
	}{
		{name: "checksum 404", checksumCode: http.StatusNotFound, archiveCode: http.StatusOK, wantSubstring: "404"},
		{name: "checksum 500", checksumCode: http.StatusInternalServerError, archiveCode: http.StatusOK, wantSubstring: "500"},
		{name: "archive 404", checksumCode: http.StatusOK, archiveCode: http.StatusNotFound, wantSubstring: "404"},
		{name: "archive 500", checksumCode: http.StatusOK, archiveCode: http.StatusInternalServerError, wantSubstring: "500"},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/SHASUMS256.txt":
					if test.checksumCode != http.StatusOK {
						w.WriteHeader(test.checksumCode)
						return
					}
					_, _ = w.Write([]byte(checksumLine(archive, "mise-latest-linux-x64.tar.gz") + "\n"))
				case "/mise-latest-linux-x64.tar.gz":
					if test.archiveCode != http.StatusOK {
						w.WriteHeader(test.archiveCode)
						return
					}
					_, _ = w.Write(archive)
				default:
					http.NotFound(w, r)
				}
			}))
			defer server.Close()

			_, err := BootstrapMise(context.Background(), BootstrapOptions{
				BaseURL: server.URL, InstallDir: t.TempDir(), OS: "linux", Arch: "amd64",
			})
			if err == nil || !strings.Contains(err.Error(), test.wantSubstring) {
				t.Fatalf("error = %v, want containing %q", err, test.wantSubstring)
			}
		})
	}
}

func TestBootstrapMiseRejectsUnsupportedPlatforms(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		opts BootstrapOptions
	}{
		{name: "arch", opts: BootstrapOptions{InstallDir: t.TempDir(), OS: "linux", Arch: "riscv64"}},
		{name: "os", opts: BootstrapOptions{InstallDir: t.TempDir(), OS: "darwin", Arch: "amd64"}},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, err := BootstrapMise(context.Background(), test.opts)
			if !errors.Is(err, ErrUnsupportedPlatform) {
				t.Fatalf("error = %v, want ErrUnsupportedPlatform", err)
			}
		})
	}
}

func TestBootstrapMiseRejectsPathTraversal(t *testing.T) {
	t.Parallel()
	archive := makeTarGz(t, []tarFile{
		{Name: "../escape", Content: "nope"},
		{Name: "mise/bin/mise", Content: "binary"},
	})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/SHASUMS256.txt":
			_, _ = w.Write([]byte(checksumLine(archive, "mise-latest-linux-x64.tar.gz") + "\n"))
		case "/mise-latest-linux-x64.tar.gz":
			_, _ = w.Write(archive)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	installDir := t.TempDir()
	_, err := BootstrapMise(context.Background(), BootstrapOptions{
		BaseURL: server.URL, InstallDir: installDir, OS: "linux", Arch: "amd64",
	})
	if err == nil || !strings.Contains(err.Error(), "unsafe path") {
		t.Fatalf("error = %v, want unsafe path", err)
	}
	if _, statErr := os.Stat(filepath.Join(installDir, "mise")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("mise binary was left behind: %v", statErr)
	}
	if _, statErr := os.Stat(filepath.Join(filepath.Dir(installDir), "escape")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("archive entry escaped the install directory: %v", statErr)
	}
}

func TestBootstrapMiseRejectsCorruptArchive(t *testing.T) {
	t.Parallel()
	archive := []byte("this is not a gzip archive")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/SHASUMS256.txt":
			_, _ = w.Write([]byte(checksumLine(archive, "mise-latest-linux-x64.tar.gz") + "\n"))
		case "/mise-latest-linux-x64.tar.gz":
			_, _ = w.Write(archive)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	installDir := t.TempDir()
	_, err := BootstrapMise(context.Background(), BootstrapOptions{
		BaseURL: server.URL, InstallDir: installDir, OS: "linux", Arch: "amd64",
	})
	if err == nil {
		t.Fatal("expected corrupt archive error")
	}
	if _, statErr := os.Stat(filepath.Join(installDir, "mise")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("mise binary was left behind: %v", statErr)
	}
}

func TestBootstrapMiseFallsBackToRootBinaryEntry(t *testing.T) {
	t.Parallel()
	archive := makeTarGz(t, []tarFile{{Name: "bin/mise", Content: "binary"}})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/SHASUMS256.txt":
			_, _ = w.Write([]byte(checksumLine(archive, "mise-latest-linux-arm64.tar.gz") + "\n"))
		case "/mise-latest-linux-arm64.tar.gz":
			_, _ = w.Write(archive)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	installDir := t.TempDir()
	path, err := BootstrapMise(context.Background(), BootstrapOptions{
		BaseURL: server.URL, InstallDir: installDir, OS: "linux", Arch: "arm64",
	})
	if err != nil {
		t.Fatalf("BootstrapMise: %v", err)
	}
	if path != filepath.Join(installDir, "mise") {
		t.Fatalf("path = %q", path)
	}
}

func TestFindMise(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	fallback := filepath.Join(home, ".local", "bin", "mise")
	writeTestFile(t, fallback, "binary", 0o755)

	path, found := FindMise(func(string) (string, error) {
		return "/usr/local/bin/mise", nil
	}, home)
	if !found || path != "/usr/local/bin/mise" {
		t.Fatalf("FindMise PATH = %q, %v", path, found)
	}

	path, found = FindMise(func(string) (string, error) {
		return "", errors.New("not found")
	}, home)
	if !found || path != fallback {
		t.Fatalf("FindMise fallback = %q, %v", path, found)
	}

	writeTestFile(t, fallback, "binary", 0o644)
	if _, found := FindMise(func(string) (string, error) {
		return "", errors.New("not found")
	}, home); found {
		t.Fatal("non-executable fallback was accepted")
	}

	if _, found := FindMise(func(string) (string, error) {
		return "", errors.New("not found")
	}, ""); found {
		t.Fatal("empty home directory produced a fallback")
	}
}

func TestMiseEnv(t *testing.T) {
	t.Parallel()
	home := "/home/tester"
	bin := filepath.Join(home, ".local", "bin")
	shims := filepath.Join(home, ".local", "share", "mise", "shims")

	base := []string{"HOME=" + home, "PATH=/usr/bin:/bin"}
	env := MiseEnv(base, home)
	wantPath := "PATH=" + bin + string(os.PathListSeparator) + shims +
		string(os.PathListSeparator) + "/usr/bin:/bin"
	if len(env) != 2 || env[1] != wantPath {
		t.Fatalf("MiseEnv = %v, want PATH %q", env, wantPath)
	}
	if base[1] != "PATH=/usr/bin:/bin" {
		t.Fatalf("MiseEnv mutated its input: %v", base)
	}

	env = MiseEnv([]string{"PATH=" + bin + ":/usr/bin"}, home)
	if !strings.HasPrefix(env[0], "PATH="+shims+string(os.PathListSeparator)) {
		t.Fatalf("missing shims were not prepended: %v", env)
	}

	env = MiseEnv([]string{"HOME=" + home}, home)
	if len(env) != 2 || env[1] != "PATH="+bin+string(os.PathListSeparator)+shims {
		t.Fatalf("missing PATH was not appended: %v", env)
	}

	already := []string{"PATH=" + bin + string(os.PathListSeparator) + shims + ":/usr/bin"}
	if got := MiseEnv(already, home); got[0] != already[0] {
		t.Fatalf("already-present directories were reordered: %v", got)
	}

	if got := MiseEnv([]string{"PATH=/usr/bin"}, ""); len(got) != 1 || got[0] != "PATH=/usr/bin" {
		t.Fatalf("empty home changed the environment: %v", got)
	}
}
