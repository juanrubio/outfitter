package engine

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/juanrubio/outfitter/internal/domain"
)

func TestSaveAndLoadCache(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	path := DefaultCachePath(home)
	if path != filepath.Join(home, ".cache", "outfitter", "state.json") {
		t.Fatalf("DefaultCachePath = %q", path)
	}

	updatedAt := time.Date(2026, 10, 19, 12, 30, 0, 0, time.UTC)
	snapshot := CacheSnapshot{
		UpdatedAt: updatedAt,
		States: map[string]domain.ToolState{
			"codex": {
				InstalledVersion: "0.163.0",
				LatestVersion:    "0.165.0",
				BinaryPath:       "/home/tester/.local/share/mise/shims/codex",
				Provenance:       domain.ProvenanceMise,
				CheckStatus:      domain.StatusReady,
			},
			"tmux": {
				InstalledVersion: "3.4",
				LatestVersion:    "3.4",
				Provenance:       domain.ProvenanceAPT,
				CheckStatus:      domain.StatusError,
				LastError:        "temporary failure",
			},
		},
	}
	if err := SaveCache(path, snapshot); err != nil {
		t.Fatalf("SaveCache: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat cache: %v", err)
	}
	if info.Mode().Perm() != 0o644 {
		t.Fatalf("cache mode = %o, want 0644", info.Mode().Perm())
	}
	if !info.Mode().IsRegular() {
		t.Fatalf("cache is not a regular file")
	}

	loaded, err := LoadCache(path)
	if err != nil {
		t.Fatalf("LoadCache: %v", err)
	}
	if !loaded.UpdatedAt.Equal(updatedAt) {
		t.Fatalf("UpdatedAt = %v, want %v", loaded.UpdatedAt, updatedAt)
	}
	if len(loaded.States) != len(snapshot.States) {
		t.Fatalf("states = %d, want %d", len(loaded.States), len(snapshot.States))
	}
	for id, want := range snapshot.States {
		got, ok := loaded.States[id]
		if !ok {
			t.Fatalf("missing state for %s", id)
		}
		if got != want {
			t.Fatalf("%s state = %+v, want %+v", id, got, want)
		}
	}

	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatalf("read cache directory: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "state.json" {
		t.Fatalf("temporary files left behind: %v", entries)
	}
}

func TestSaveCacheDefaultsTimestamp(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "nested", "state.json")
	before := time.Now().UTC().Add(-time.Second)
	if err := SaveCache(path, CacheSnapshot{}); err != nil {
		t.Fatalf("SaveCache: %v", err)
	}
	loaded, err := LoadCache(path)
	if err != nil {
		t.Fatalf("LoadCache: %v", err)
	}
	if loaded.UpdatedAt.Before(before) {
		t.Fatalf("UpdatedAt = %v, want a recent timestamp", loaded.UpdatedAt)
	}
	if loaded.States == nil {
		t.Fatal("States is nil")
	}
}

func TestLoadCacheMissingFile(t *testing.T) {
	t.Parallel()
	snapshot, err := LoadCache(filepath.Join(t.TempDir(), "missing.json"))
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("error = %v, want os.ErrNotExist", err)
	}
	if snapshot.States == nil || len(snapshot.States) != 0 {
		t.Fatalf("snapshot = %+v, want empty states", snapshot)
	}
}

func TestLoadCacheCorruptJSON(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "state.json")
	writeTestFile(t, path, "{not valid json", 0o644)

	snapshot, err := LoadCache(path)
	if err == nil {
		t.Fatal("expected corrupt JSON error")
	}
	if !strings.Contains(err.Error(), "parse state cache") {
		t.Fatalf("error = %v, want parse context", err)
	}
	if snapshot.States == nil || len(snapshot.States) != 0 {
		t.Fatalf("snapshot = %+v, want empty states", snapshot)
	}
}

func TestSaveCacheEmptyPath(t *testing.T) {
	t.Parallel()
	if err := SaveCache("", CacheSnapshot{}); err == nil {
		t.Fatal("expected error for empty cache path")
	}
}

func TestDefaultCachePathEmptyHome(t *testing.T) {
	t.Parallel()
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		t.Skip("no home directory available")
	}
	if got := DefaultCachePath(""); got != filepath.Join(home, ".cache", "outfitter", "state.json") {
		t.Fatalf("DefaultCachePath(\"\") = %q", got)
	}
}
