package adapter

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/juanrubio/outfitter/internal/domain"
)

func testAdapter(t *testing.T) (*Adapter, string) {
	t.Helper()
	home := t.TempDir()
	return New(Options{
		HomeDir: home,
		LookPath: func(string) (string, error) {
			return "/opt/mise", nil
		},
	}), home
}

func planStep(tool domain.Tool, action domain.DesiredAction) domain.PlanStep {
	return domain.PlanStep{Tool: tool, Action: action, Command: tool.CommandFor(action)}
}

func displays(specs []CommandSpec) []string {
	result := make([]string, 0, len(specs))
	for _, spec := range specs {
		result = append(result, spec.Display)
	}
	return result
}

func assertDisplays(t *testing.T, specs []CommandSpec, expected ...string) {
	t.Helper()
	got := displays(specs)
	if len(got) != len(expected) {
		t.Fatalf("commands = %v, want %v", got, expected)
	}
	for index := range expected {
		if got[index] != expected[index] {
			t.Fatalf("command %d = %q, want %q (all: %v)", index, got[index], expected[index], got)
		}
	}
}

func TestMiseCommandsAcrossBackendsAndActions(t *testing.T) {
	cases := []struct {
		name     string
		tool     domain.Tool
		action   domain.DesiredAction
		commands []string
	}{
		{
			name: "aqua install",
			tool: domain.Tool{
				ID: "lazygit", Binary: "lazygit", Backend: domain.BackendMiseAqua,
				Package: "jesseduffield/lazygit",
			},
			action:   domain.ActionInstall,
			commands: []string{"mise use --global aqua:jesseduffield/lazygit@latest"},
		},
		{
			name: "aqua update",
			tool: domain.Tool{
				ID: "lazygit", Binary: "lazygit", Backend: domain.BackendMiseAqua,
				Package: "jesseduffield/lazygit",
			},
			action:   domain.ActionUpdate,
			commands: []string{"mise upgrade aqua:jesseduffield/lazygit"},
		},
		{
			name: "aqua remove",
			tool: domain.Tool{
				ID: "lazygit", Binary: "lazygit", Backend: domain.BackendMiseAqua,
				Package: "jesseduffield/lazygit",
			},
			action:   domain.ActionRemove,
			commands: []string{"mise unuse --global aqua:jesseduffield/lazygit"},
		},
		{
			name: "ubi install",
			tool: domain.Tool{
				ID: "boardwalk", Binary: "boardwalk", Backend: domain.BackendMiseUbi,
				Package: "juanrubio/boardwalk",
			},
			action:   domain.ActionInstall,
			commands: []string{"mise use --global ubi:juanrubio/boardwalk@latest"},
		},
		{
			name: "github install",
			tool: domain.Tool{
				ID: "mise", Binary: "mise", Backend: domain.BackendMiseGitHub,
				Package: "jdx/mise",
			},
			action:   domain.ActionInstall,
			commands: []string{"mise use --global github:jdx/mise@latest"},
		},
		{
			name: "npm install prepends node lts",
			tool: domain.Tool{
				ID: "codex", Binary: "codex", Backend: domain.BackendMiseNPM,
				Package: "@openai/codex",
			},
			action: domain.ActionInstall,
			commands: []string{
				"mise use --global node@lts",
				"mise use --global npm:@openai/codex@latest",
			},
		},
		{
			name: "npm update prepends node lts",
			tool: domain.Tool{
				ID: "codex", Binary: "codex", Backend: domain.BackendMiseNPM,
				Package: "@openai/codex",
			},
			action: domain.ActionUpdate,
			commands: []string{
				"mise use --global node@lts",
				"mise upgrade npm:@openai/codex",
			},
		},
		{
			name: "npm remove has no prerequisite",
			tool: domain.Tool{
				ID: "codex", Binary: "codex", Backend: domain.BackendMiseNPM,
				Package: "@openai/codex",
			},
			action:   domain.ActionRemove,
			commands: []string{"mise unuse --global npm:@openai/codex"},
		},
		{
			name: "uv install prepends uv runtime",
			tool: domain.Tool{
				ID: "browser-use", Binary: "browser-use", Backend: domain.BackendUV,
				Package: "browser-use",
			},
			action: domain.ActionInstall,
			commands: []string{
				"mise use --global uv@latest",
				"mise x uv -- uv tool install browser-use",
			},
		},
		{
			name: "uv update prepends uv runtime",
			tool: domain.Tool{
				ID: "browser-use", Binary: "browser-use", Backend: domain.BackendUV,
				Package: "browser-use",
			},
			action: domain.ActionUpdate,
			commands: []string{
				"mise use --global uv@latest",
				"mise x uv -- uv tool upgrade browser-use",
			},
		},
		{
			name: "uv remove has no prerequisite",
			tool: domain.Tool{
				ID: "browser-use", Binary: "browser-use", Backend: domain.BackendUV,
				Package: "browser-use",
			},
			action:   domain.ActionRemove,
			commands: []string{"mise x uv -- uv tool uninstall browser-use"},
		},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			a, _ := testAdapter(t)
			specs, err := a.Commands(planStep(test.tool, test.action))
			if err != nil {
				t.Fatalf("Commands: %v", err)
			}
			assertDisplays(t, specs, test.commands...)
			for _, spec := range specs {
				if spec.Program != "/opt/mise" && spec.Program != "mise" {
					t.Fatalf("program = %q, want resolved mise", spec.Program)
				}
			}
		})
	}
}

func TestMiseRuntimePrerequisitesAreEnsuredOncePerRun(t *testing.T) {
	a, _ := testAdapter(t)
	codex := domain.Tool{
		ID: "codex", Binary: "codex", Backend: domain.BackendMiseNPM, Package: "@openai/codex",
	}
	pi := domain.Tool{
		ID: "pi", Binary: "pi", Backend: domain.BackendMiseNPM,
		Package: "@earendil-works/pi-coding-agent",
	}

	first, err := a.Commands(planStep(codex, domain.ActionInstall))
	if err != nil {
		t.Fatal(err)
	}
	assertDisplays(t, first,
		"mise use --global node@lts",
		"mise use --global npm:@openai/codex@latest",
	)

	second, err := a.Commands(planStep(pi, domain.ActionInstall))
	if err != nil {
		t.Fatal(err)
	}
	assertDisplays(t, second, "mise use --global npm:@earendil-works/pi-coding-agent@latest")

	a.Reset()
	afterReset, err := a.Commands(planStep(pi, domain.ActionInstall))
	if err != nil {
		t.Fatal(err)
	}
	assertDisplays(t, afterReset,
		"mise use --global node@lts",
		"mise use --global npm:@earendil-works/pi-coding-agent@latest",
	)
}

func TestMiseResolutionFallsBackToHomeAndPath(t *testing.T) {
	home := t.TempDir()
	bin := filepath.Join(home, ".local", "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	misePath := filepath.Join(bin, "mise")
	if err := os.WriteFile(misePath, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	a := New(Options{
		HomeDir: home,
		LookPath: func(string) (string, error) {
			return "", os.ErrNotExist
		},
	})
	tool := domain.Tool{
		ID: "fzf", Binary: "fzf", Backend: domain.BackendMiseAqua, Package: "junegunn/fzf",
	}
	specs, err := a.Commands(planStep(tool, domain.ActionInstall))
	if err != nil {
		t.Fatal(err)
	}
	if specs[0].Program != misePath {
		t.Fatalf("program = %q, want %q", specs[0].Program, misePath)
	}

	missing := New(Options{
		HomeDir: t.TempDir(),
		LookPath: func(string) (string, error) {
			return "", os.ErrNotExist
		},
	})
	specs, err = missing.Commands(planStep(tool, domain.ActionInstall))
	if err != nil {
		t.Fatal(err)
	}
	if specs[0].Program != "mise" {
		t.Fatalf("fallback program = %q, want mise", specs[0].Program)
	}
}

func TestAPTCommands(t *testing.T) {
	cases := []struct {
		action   domain.DesiredAction
		commands []string
	}{
		{domain.ActionInstall, []string{"sudo apt-get install -y tmux"}},
		{domain.ActionUpdate, []string{"sudo apt-get install --only-upgrade -y tmux"}},
		{domain.ActionRemove, []string{"sudo apt-get remove -y tmux"}},
	}
	for _, test := range cases {
		a, _ := testAdapter(t)
		tool := domain.Tool{
			ID: "tmux", Binary: "tmux", Backend: domain.BackendAPT, Package: "tmux", Sudo: true,
		}
		specs, err := a.Commands(planStep(tool, test.action))
		if err != nil {
			t.Fatalf("%s: %v", test.action, err)
		}
		assertDisplays(t, specs, test.commands...)
		if specs[0].Program != "sudo" {
			t.Fatalf("%s program = %q, want sudo", test.action, specs[0].Program)
		}
		if got := specs[0].Args; len(got) < 2 || got[0] != "apt-get" {
			t.Fatalf("%s args = %v", test.action, got)
		}
	}

	cmd := VerifySudoCommand()
	if filepath.Base(cmd.Path) != "sudo" {
		t.Fatalf("VerifySudoCommand path = %q", cmd.Path)
	}
	if len(cmd.Args) != 2 || cmd.Args[1] != "-v" {
		t.Fatalf("VerifySudoCommand args = %v", cmd.Args)
	}
}

// TestMigrateUpdateCommandsUseInstallSemantics verifies that an update
// step flagged Migrate (the backend does not own the discovered install)
// runs the backend's install command instead of its upgrade-only command,
// which would silently do nothing.
func TestMigrateUpdateCommandsUseInstallSemantics(t *testing.T) {
	cases := []struct {
		name     string
		tool     domain.Tool
		commands []string
	}{
		{
			name: "mise aqua migrate uses use --global @latest",
			tool: domain.Tool{
				ID: "dust", Binary: "dust", Backend: domain.BackendMiseAqua,
				Package: "bootandy/dust",
			},
			commands: []string{"mise use --global aqua:bootandy/dust@latest"},
		},
		{
			name: "mise ubi migrate uses use --global @latest",
			tool: domain.Tool{
				ID: "broot", Binary: "broot", Backend: domain.BackendMiseUbi,
				Package: "Canop/broot",
			},
			commands: []string{"mise use --global ubi:Canop/broot@latest"},
		},
		{
			name: "mise npm migrate keeps node prerequisite",
			tool: domain.Tool{
				ID: "codex", Binary: "codex", Backend: domain.BackendMiseNPM,
				Package: "@openai/codex",
			},
			commands: []string{
				"mise use --global node@lts",
				"mise use --global npm:@openai/codex@latest",
			},
		},
		{
			name: "uv migrate uses tool install",
			tool: domain.Tool{
				ID: "browser-use", Binary: "browser-use", Backend: domain.BackendUV,
				Package: "browser-use",
			},
			commands: []string{
				"mise use --global uv@latest",
				"mise x uv -- uv tool install browser-use",
			},
		},
		{
			name: "apt migrate drops --only-upgrade",
			tool: domain.Tool{
				ID: "tig", Binary: "tig", Backend: domain.BackendAPT,
				Package: "tig", Sudo: true,
			},
			commands: []string{"sudo apt-get install -y tig"},
		},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			a, _ := testAdapter(t)
			step := planStep(test.tool, domain.ActionUpdate)
			step.Migrate = true
			specs, err := a.Commands(step)
			if err != nil {
				t.Fatalf("Commands: %v", err)
			}
			assertDisplays(t, specs, test.commands...)
		})
	}
}

func TestVendorCommandsUseConfiguredShell(t *testing.T) {
	a, _ := testAdapter(t)
	tool := domain.Tool{
		ID: "claude-code", Binary: "claude", Backend: domain.BackendVendor,
		InstallCmd: "curl -fsSL https://claude.ai/install.sh | sh",
		UpdateCmd:  "claude update",
		RemoveCmd:  "rm -f ~/.local/bin/claude",
	}
	for action, command := range map[domain.DesiredAction]string{
		domain.ActionInstall: "curl -fsSL https://claude.ai/install.sh | sh",
		domain.ActionUpdate:  "claude update",
		domain.ActionRemove:  "rm -f ~/.local/bin/claude",
	} {
		specs, err := a.Commands(planStep(tool, action))
		if err != nil {
			t.Fatalf("%s: %v", action, err)
		}
		if len(specs) != 1 {
			t.Fatalf("%s specs = %v", action, specs)
		}
		assertDisplays(t, specs, command)
		if specs[0].Program != "bash" {
			t.Fatalf("%s program = %q", action, specs[0].Program)
		}
		expectedArgs := []string{"-o", "pipefail", "-c", command}
		if strings.Join(specs[0].Args, "\x00") != strings.Join(expectedArgs, "\x00") {
			t.Fatalf("%s args = %v, want %v", action, specs[0].Args, expectedArgs)
		}
	}
}

func TestSourceCommandsExpandHomeAndBuild(t *testing.T) {
	a, home := testAdapter(t)
	tool := domain.Tool{
		ID: "quotary", Binary: "quotary", Backend: domain.BackendSource,
		SourceDir: "~/repos/quotary",
	}
	expectedSource := filepath.Join(home, "repos", "quotary")
	expectedBinary := filepath.Join(home, ".local", "bin", "quotary")

	for _, action := range []domain.DesiredAction{domain.ActionInstall, domain.ActionUpdate} {
		specs, err := a.Commands(planStep(tool, action))
		if err != nil {
			t.Fatalf("%s: %v", action, err)
		}
		if len(specs) != 1 {
			t.Fatalf("%s specs = %v", action, specs)
		}
		spec := specs[0]
		if spec.Program != "go" {
			t.Fatalf("%s program = %q, want go", action, spec.Program)
		}
		if spec.Dir != expectedSource {
			t.Fatalf("%s dir = %q, want %q", action, spec.Dir, expectedSource)
		}
		expectedArgs := []string{"build", "-o", expectedBinary, "./cmd/quotary"}
		if strings.Join(spec.Args, "\x00") != strings.Join(expectedArgs, "\x00") {
			t.Fatalf("%s args = %v, want %v", action, spec.Args, expectedArgs)
		}
	}

	specs, err := a.Commands(planStep(tool, domain.ActionRemove))
	if err != nil {
		t.Fatal(err)
	}
	assertDisplays(t, specs, "rm -f "+expectedBinary)
}

func TestSourceCustomBuildCommand(t *testing.T) {
	a, home := testAdapter(t)
	tool := domain.Tool{
		ID: "quotary", Binary: "quotary", Backend: domain.BackendSource,
		SourceDir: "~/repos/quotary",
		BuildCmd:  "go build -o ~/.local/bin/quotary ./cmd/quotary",
	}
	specs, err := a.Commands(planStep(tool, domain.ActionInstall))
	if err != nil {
		t.Fatal(err)
	}
	if len(specs) != 1 || specs[0].Program != "bash" {
		t.Fatalf("specs = %v", specs)
	}
	if specs[0].Dir != filepath.Join(home, "repos", "quotary") {
		t.Fatalf("dir = %q", specs[0].Dir)
	}
	assertDisplays(t, specs, "go build -o ~/.local/bin/quotary ./cmd/quotary")
}

func tarGz(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buffer bytes.Buffer
	gzipWriter := gzip.NewWriter(&buffer)
	tarWriter := tar.NewWriter(gzipWriter)
	for name, content := range files {
		header := &tar.Header{
			Name:     name,
			Mode:     0o755,
			Size:     int64(len(content)),
			Typeflag: tar.TypeReg,
			ModTime:  time.Unix(0, 0),
		}
		if err := tarWriter.WriteHeader(header); err != nil {
			t.Fatalf("write tar header: %v", err)
		}
		if _, err := tarWriter.Write([]byte(content)); err != nil {
			t.Fatalf("write tar content: %v", err)
		}
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func codexBarFixture(t *testing.T) []byte {
	t.Helper()
	return tarGz(t, map[string]string{
		"CodexBarCLI-linux/CodexBarCLI":                                        "#!/bin/sh\necho codexbar\n",
		"CodexBarCLI-linux/CodexBarCLI_CodexBarCore.bundle/Info.plist":         "<plist>fixture</plist>",
		"CodexBarCLI-linux/CodexBarCLI_CodexBarCore.bundle/Resources/icon.bin": "icon",
		"CodexBarCLI-linux/README.md":                                          "ignored",
	})
}

func TestInstallAndRemoveCodexBar(t *testing.T) {
	archive := codexBarFixture(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/codexbar.tar.gz" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(archive)
	}))
	defer server.Close()

	home := t.TempDir()
	var lines []string
	err := InstallCodexBar(context.Background(), CodexBarOptions{
		HomeDir:    home,
		ArchiveURL: server.URL + "/codexbar.tar.gz",
	}, func(line string) {
		lines = append(lines, line)
	})
	if err != nil {
		t.Fatalf("InstallCodexBar: %v", err)
	}

	bundleDir := filepath.Join(home, ".local", "share", "outfitter", "codexbar")
	binaryPath := filepath.Join(bundleDir, "CodexBarCLI")
	info, err := os.Stat(binaryPath)
	if err != nil {
		t.Fatalf("stat executable: %v", err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Fatalf("executable mode = %o, want 0755", info.Mode().Perm())
	}
	bundleInfo := filepath.Join(bundleDir, "CodexBarCLI_CodexBarCore.bundle", "Info.plist")
	if _, err := os.Stat(bundleInfo); err != nil {
		t.Fatalf("bundle Info.plist not extracted: %v", err)
	}
	if _, err := os.Stat(filepath.Join(bundleDir, "README.md")); !os.IsNotExist(err) {
		t.Fatalf("unrelated archive file should not be extracted: %v", err)
	}

	linkPath := filepath.Join(home, ".local", "bin", "codexbar")
	target, err := os.Readlink(linkPath)
	if err != nil {
		t.Fatalf("read symlink: %v", err)
	}
	if target != binaryPath {
		t.Fatalf("symlink target = %q, want %q", target, binaryPath)
	}
	if len(lines) == 0 || !strings.Contains(strings.Join(lines, "\n"), "Linked") {
		t.Fatalf("expected progress lines, got %v", lines)
	}

	if err := RemoveCodexBar(home, nil); err != nil {
		t.Fatalf("RemoveCodexBar: %v", err)
	}
	if _, err := os.Lstat(linkPath); !os.IsNotExist(err) {
		t.Fatalf("symlink still exists: %v", err)
	}
	if _, err := os.Stat(bundleDir); !os.IsNotExist(err) {
		t.Fatalf("bundle directory still exists: %v", err)
	}
}

func TestInstallCodexBarResolvesGitHubReleaseAsset(t *testing.T) {
	archive := codexBarFixture(t)
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/steipete/CodexBar/releases/latest":
			payload := map[string]any{
				"tag_name": "v0.77.0",
				"assets": []map[string]string{
					{
						"name":                 "CodexBarCLI-linux-x86_64.tar.gz",
						"browser_download_url": server.URL + "/download.tar.gz",
					},
				},
			}
			if err := json.NewEncoder(w).Encode(payload); err != nil {
				t.Errorf("encode release: %v", err)
			}
		case "/download.tar.gz":
			_, _ = w.Write(archive)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	home := t.TempDir()
	err := InstallCodexBar(context.Background(), CodexBarOptions{
		HomeDir:          home,
		GitHubAPIBaseURL: server.URL,
		OS:               "linux",
		Arch:             "amd64",
	}, nil)
	if err != nil {
		t.Fatalf("InstallCodexBar: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".local", "bin", "codexbar")); err != nil {
		t.Fatalf("codexbar symlink missing: %v", err)
	}
}

func TestCodexBarRejectsUnsafeArchivePaths(t *testing.T) {
	archive := tarGz(t, map[string]string{
		"CodexBarCLI": "binary",
		"../escape":   "bad",
	})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(archive)
	}))
	defer server.Close()

	home := t.TempDir()
	err := InstallCodexBar(context.Background(), CodexBarOptions{
		HomeDir:    home,
		ArchiveURL: server.URL,
	}, nil)
	if err == nil || !strings.Contains(err.Error(), "unsafe path") {
		t.Fatalf("error = %v, want unsafe path", err)
	}
}
