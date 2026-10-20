package engine

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

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

	t.Run("empty input is a no-op", func(t *testing.T) {
		removed, err := RemoveShadowedBinaries(nil)
		if err != nil || len(removed) != 0 {
			t.Fatalf("removed = %v, err = %v, want empty/nil", removed, err)
		}
	})
}
