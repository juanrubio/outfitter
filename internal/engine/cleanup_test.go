package engine

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

// npmCopilotFixture builds a fake npm global install layout under dir:
// bin/copilot -> ../lib/node_modules/@github/copilot/index.js, plus bin/npm.
func npmCopilotFixture(t *testing.T, dir string) (binPath, npmPath string) {
	t.Helper()
	target := filepath.Join(dir, "lib", "node_modules", "@github", "copilot", "index.js")
	writeTestFile(t, target, "binary", 0o755)
	npmPath = filepath.Join(dir, "bin", "npm")
	writeTestFile(t, npmPath, "binary", 0o755)
	binPath = filepath.Join(dir, "bin", "copilot")
	rel, err := filepath.Rel(filepath.Dir(binPath), target)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(rel, binPath); err != nil {
		t.Fatal(err)
	}
	return binPath, npmPath
}

func TestRemovalCommandForNPMGlobal(t *testing.T) {
	dir := t.TempDir()
	binPath, npmPath := npmCopilotFixture(t, dir)
	resolved, err := filepath.EvalSymlinks(binPath)
	if err != nil {
		t.Fatal(err)
	}
	got := removalCommandFor(binPath, resolved, os.Stat)
	want := npmPath + " uninstall -g @github/copilot"
	if got != want {
		t.Errorf("removalCommandFor(scoped) = %q, want %q", got, want)
	}

	plain := filepath.Join(dir, "dust")
	writeTestFile(t, plain, "binary", 0o755)
	if got := removalCommandFor(plain, plain, os.Stat); got != "rm "+plain {
		t.Errorf("removalCommandFor(plain) = %q, want rm fallback", got)
	}
}

func TestRemoveShadowedBinaries(t *testing.T) {
	t.Run("removes user-owned files and tolerates missing ones", func(t *testing.T) {
		dir := t.TempDir()
		one := filepath.Join(dir, "dust")
		two := filepath.Join(dir, "broot")
		writeTestFile(t, one, "binary", 0o755)
		writeTestFile(t, two, "binary", 0o755)
		missing := filepath.Join(dir, "already-gone")

		removed, err := RemoveShadowedBinaries([]string{one, missing, two})
		if err != nil {
			t.Fatal(err)
		}
		if len(removed) != 3 {
			t.Fatalf("removed = %v, want all three paths", removed)
		}
		for _, path := range []string{one, two} {
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Errorf("%s still exists (stat err = %v)", path, err)
			}
		}
	})

	t.Run("permission errors fall back to sudo", func(t *testing.T) {
		originalRemove, originalSudo := removeFile, sudoRemoveFile
		t.Cleanup(func() { removeFile, sudoRemoveFile = originalRemove, originalSudo })

		removeFile = func(string) error { return fs.ErrPermission }
		var sudoPaths []string
		sudoRemoveFile = func(path string) error {
			sudoPaths = append(sudoPaths, path)
			return nil
		}

		removed, err := RemoveShadowedBinaries([]string{"/usr/bin/dust"})
		if err != nil {
			t.Fatal(err)
		}
		if len(removed) != 1 || removed[0] != "/usr/bin/dust" {
			t.Fatalf("removed = %v, want [/usr/bin/dust]", removed)
		}
		if len(sudoPaths) != 1 || sudoPaths[0] != "/usr/bin/dust" {
			t.Fatalf("sudo calls = %v, want [/usr/bin/dust]", sudoPaths)
		}
	})

	t.Run("sudo failure is reported and excludes the path", func(t *testing.T) {
		originalRemove, originalSudo := removeFile, sudoRemoveFile
		t.Cleanup(func() { removeFile, sudoRemoveFile = originalRemove, originalSudo })

		removeFile = func(string) error { return fs.ErrPermission }
		sudoRemoveFile = func(string) error { return errors.New("sudo: a password is required") }

		removed, err := RemoveShadowedBinaries([]string{"/usr/bin/dust"})
		if err == nil {
			t.Fatal("err = nil, want sudo failure")
		}
		if len(removed) != 0 {
			t.Fatalf("removed = %v, want none", removed)
		}
	})

	t.Run("npm global installs uninstall via npm", func(t *testing.T) {
		originalNPM := npmUninstallFile
		t.Cleanup(func() { npmUninstallFile = originalNPM })

		dir := t.TempDir()
		binPath, npmPath := npmCopilotFixture(t, dir)
		var gotNPM, gotPkg string
		npmUninstallFile = func(npm, pkg string) error {
			gotNPM, gotPkg = npm, pkg
			return nil
		}

		removed, err := RemoveShadowedBinaries([]string{binPath})
		if err != nil {
			t.Fatal(err)
		}
		if len(removed) != 1 || removed[0] != binPath {
			t.Fatalf("removed = %v, want [%s]", removed, binPath)
		}
		if gotNPM != npmPath || gotPkg != "@github/copilot" {
			t.Fatalf("npm uninstall called with (%q, %q), want (%q, @github/copilot)", gotNPM, gotPkg, npmPath)
		}
	})

	t.Run("npm failure falls back to direct removal", func(t *testing.T) {
		originalNPM := npmUninstallFile
		t.Cleanup(func() { npmUninstallFile = originalNPM })

		dir := t.TempDir()
		binPath, _ := npmCopilotFixture(t, dir)
		npmUninstallFile = func(string, string) error { return errors.New("npm failed") }

		removed, err := RemoveShadowedBinaries([]string{binPath})
		if err != nil {
			t.Fatal(err)
		}
		if len(removed) != 1 {
			t.Fatalf("removed = %v, want the symlink gone via fallback", removed)
		}
		if _, err := os.Lstat(binPath); !os.IsNotExist(err) {
			t.Fatalf("symlink still exists (stat err = %v)", err)
		}
	})

	t.Run("empty input is a no-op", func(t *testing.T) {
		removed, err := RemoveShadowedBinaries(nil)
		if err != nil || len(removed) != 0 {
			t.Fatalf("removed = %v, err = %v, want empty/nil", removed, err)
		}
	})
}
