package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func shellSetupForTest(t *testing.T, env map[string]string) (*ShellSetup, string) {
	t.Helper()
	home := t.TempDir()
	if env == nil {
		env = map[string]string{}
	}
	if _, ok := env["PATH"]; !ok {
		env["PATH"] = "/usr/local/bin:/usr/bin:/bin"
	}
	return &ShellSetup{
		HomeDir: home,
		Getenv:  func(key string) string { return env[key] },
	}, home
}

func TestDetectShellIntegration(t *testing.T) {
	tests := []struct {
		name       string
		env        map[string]string
		rcName     string
		rcContent  string
		wantShell  ShellKind
		wantRC     string // basename or relative path suffix; "" means empty
		wantInteg  bool
		wantFixabl bool
	}{
		{
			name:       "zsh without integration",
			env:        map[string]string{"SHELL": "/usr/bin/zsh"},
			wantShell:  ShellZsh,
			wantRC:     ".zshrc",
			wantInteg:  false,
			wantFixabl: true,
		},
		{
			name:       "bash with MISE_SHELL set",
			env:        map[string]string{"SHELL": "/bin/bash", "MISE_SHELL": "bash"},
			wantShell:  ShellBash,
			wantRC:     ".bashrc",
			wantInteg:  true,
			wantFixabl: true,
		},
		{
			name:       "zsh with shims on PATH",
			env:        map[string]string{"SHELL": "/bin/zsh", "PATH": "SHIMS:/usr/bin:/bin"},
			wantShell:  ShellZsh,
			wantRC:     ".zshrc",
			wantInteg:  true,
			wantFixabl: true,
		},
		{
			name:       "bash with activate line in rc",
			env:        map[string]string{"SHELL": "/bin/bash"},
			rcName:     ".bashrc",
			rcContent:  "eval \"$(~/.local/bin/mise activate bash)\"\n",
			wantShell:  ShellBash,
			wantRC:     ".bashrc",
			wantInteg:  true,
			wantFixabl: true,
		},
		{
			name:       "fish is warn-only",
			env:        map[string]string{"SHELL": "/usr/bin/fish"},
			wantShell:  ShellFish,
			wantRC:     filepath.Join(".config", "fish", "config.fish"),
			wantInteg:  false,
			wantFixabl: false,
		},
		{
			name:       "unknown shell",
			env:        map[string]string{"SHELL": "/bin/sh"},
			wantShell:  ShellOther,
			wantRC:     "",
			wantInteg:  false,
			wantFixabl: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			setup, home := shellSetupForTest(t, test.env)
			if test.env["PATH"] != "" && strings.Contains(test.env["PATH"], "SHIMS") {
				setup.Getenv = func(key string) string {
					if key == "PATH" {
						return strings.Replace(test.env["PATH"], "SHIMS",
							filepath.Join(home, ".local", "share", "mise", "shims"), 1)
					}
					return test.env[key]
				}
			}
			if test.rcName != "" {
				writeTestFile(t, filepath.Join(home, test.rcName), test.rcContent, 0o644)
			}

			got := setup.Detect("")
			if got.Shell != test.wantShell {
				t.Errorf("Shell = %q, want %q", got.Shell, test.wantShell)
			}
			wantRC := ""
			if test.wantRC != "" {
				wantRC = filepath.Join(home, test.wantRC)
			}
			if got.RCFile != wantRC {
				t.Errorf("RCFile = %q, want %q", got.RCFile, wantRC)
			}
			if got.Integrated != test.wantInteg {
				t.Errorf("Integrated = %v, want %v", got.Integrated, test.wantInteg)
			}
			if got.Fixable != test.wantFixabl {
				t.Errorf("Fixable = %v, want %v", got.Fixable, test.wantFixabl)
			}
		})
	}
}

func TestApplyShellIntegration(t *testing.T) {
	const misePath = "/home/u/.local/bin/mise"

	t.Run("creates rc with activate block and is idempotent", func(t *testing.T) {
		setup, home := shellSetupForTest(t, map[string]string{"SHELL": "/bin/zsh"})
		changed, err := setup.Apply("", misePath, false)
		if err != nil {
			t.Fatal(err)
		}
		if !changed {
			t.Fatal("changed = false, want true")
		}
		data, err := os.ReadFile(filepath.Join(home, ".zshrc"))
		if err != nil {
			t.Fatal(err)
		}
		content := string(data)
		for _, want := range []string{
			"# >>> outfitter mise >>>",
			`eval "$(` + misePath + ` activate zsh)"`,
			"# <<< outfitter mise <<<",
		} {
			if !strings.Contains(content, want) {
				t.Errorf("rc missing %q:\n%s", want, content)
			}
		}
		if got := setup.Detect(""); !got.Integrated {
			t.Errorf("after apply, Integrated = false, want true (%+v)", got)
		}

		changed, err = setup.Apply("", misePath, false)
		if err != nil {
			t.Fatal(err)
		}
		if changed {
			t.Error("second apply changed = true, want false")
		}
		again, _ := os.ReadFile(filepath.Join(home, ".zshrc"))
		if string(again) != content {
			t.Errorf("second apply rewrote rc:\n%s", again)
		}
	})

	t.Run("switching to shims replaces the marked block once", func(t *testing.T) {
		setup, home := shellSetupForTest(t, map[string]string{"SHELL": "/bin/bash"})
		if _, err := setup.Apply("", misePath, false); err != nil {
			t.Fatal(err)
		}
		changed, err := setup.Apply("", misePath, true)
		if err != nil {
			t.Fatal(err)
		}
		if !changed {
			t.Fatal("changed = false, want true when switching mechanism")
		}
		data, _ := os.ReadFile(filepath.Join(home, ".bashrc"))
		content := string(data)
		if strings.Count(content, "# >>> outfitter mise >>>") != 1 {
			t.Errorf("marker block duplicated:\n%s", content)
		}
		if !strings.Contains(content, `export PATH="$HOME/.local/share/mise/shims:$PATH"`) {
			t.Errorf("shims line missing:\n%s", content)
		}
		if strings.Contains(content, "mise activate") {
			t.Errorf("activate line still present:\n%s", content)
		}
	})

	t.Run("existing user activate line is left untouched", func(t *testing.T) {
		setup, home := shellSetupForTest(t, map[string]string{"SHELL": "/bin/bash"})
		original := "# my rc\neval \"$(mise activate bash)\"\n"
		writeTestFile(t, filepath.Join(home, ".bashrc"), original, 0o644)
		changed, err := setup.Apply("", misePath, false)
		if err != nil {
			t.Fatal(err)
		}
		if changed {
			t.Error("changed = true, want false for pre-integrated rc")
		}
		data, _ := os.ReadFile(filepath.Join(home, ".bashrc"))
		if string(data) != original {
			t.Errorf("rc modified:\n%s", data)
		}
	})

	t.Run("writes through a symlinked rc without replacing the link", func(t *testing.T) {
		setup, home := shellSetupForTest(t, map[string]string{"SHELL": "/bin/zsh"})
		target := filepath.Join(home, "dotfiles", ".zshrc")
		writeTestFile(t, target, "# dotfiles rc\n", 0o644)
		link := filepath.Join(home, ".zshrc")
		if err := os.Symlink(target, link); err != nil {
			t.Fatal(err)
		}
		if _, err := setup.Apply("", misePath, false); err != nil {
			t.Fatal(err)
		}
		info, err := os.Lstat(link)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode()&os.ModeSymlink == 0 {
			t.Fatal("rc symlink was replaced by a regular file")
		}
		data, _ := os.ReadFile(target)
		if !strings.Contains(string(data), "# >>> outfitter mise >>>") {
			t.Errorf("symlink target missing block:\n%s", data)
		}
	})

	t.Run("fish cannot be auto-fixed", func(t *testing.T) {
		setup, home := shellSetupForTest(t, map[string]string{"SHELL": "/usr/bin/fish"})
		changed, err := setup.Apply("", misePath, false)
		if err == nil {
			t.Fatal("err = nil, want unsupported-shell error")
		}
		if changed {
			t.Error("changed = true, want false")
		}
		if !strings.Contains(err.Error(), "activate fish") {
			t.Errorf("error should carry the manual fish line, got: %v", err)
		}
		if _, statErr := os.Stat(filepath.Join(home, ".config", "fish", "config.fish")); !os.IsNotExist(statErr) {
			t.Errorf("fish rc should not be created, stat err = %v", statErr)
		}
	})
}
