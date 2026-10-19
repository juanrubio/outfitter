package engine

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/juanrubio/outfitter/internal/domain"
)

// CacheSnapshot is the persisted state used to paint the TUI immediately on
// startup before a fresh discovery pass completes.
type CacheSnapshot struct {
	UpdatedAt time.Time                   `json:"updated_at"`
	States    map[string]domain.ToolState `json:"states"`
}

// DefaultCachePath returns <homeDir>/.cache/outfitter/state.json. An empty
// homeDir falls back to the current user's home directory.
func DefaultCachePath(homeDir string) string {
	if homeDir == "" {
		home, err := os.UserHomeDir()
		if err != nil || home == "" {
			return ""
		}
		homeDir = home
	}
	return filepath.Join(homeDir, ".cache", "outfitter", "state.json")
}

// LoadCache reads a snapshot. A missing file returns an empty snapshot together
// with os.ErrNotExist so callers can distinguish a cold start.
func LoadCache(path string) (CacheSnapshot, error) {
	empty := CacheSnapshot{States: make(map[string]domain.ToolState)}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return empty, os.ErrNotExist
		}
		return empty, fmt.Errorf("read state cache %s: %w", path, err)
	}

	var snapshot CacheSnapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return empty, fmt.Errorf("parse state cache %s: %w", path, err)
	}
	if snapshot.States == nil {
		snapshot.States = make(map[string]domain.ToolState)
	}
	return snapshot, nil
}

// SaveCache atomically writes a snapshot by creating a temporary file beside
// the target and renaming it into place with 0644 permissions.
func SaveCache(path string, snapshot CacheSnapshot) error {
	if path == "" {
		return errors.New("state cache path is empty")
	}
	if snapshot.UpdatedAt.IsZero() {
		snapshot.UpdatedAt = time.Now().UTC()
	}
	if snapshot.States == nil {
		snapshot.States = make(map[string]domain.ToolState)
	}

	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return fmt.Errorf("create state cache directory %s: %w", directory, err)
	}

	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return fmt.Errorf("encode state cache: %w", err)
	}
	data = append(data, '\n')

	temporary, err := os.CreateTemp(directory, ".state-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary state cache: %w", err)
	}
	temporaryPath := temporary.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(temporaryPath)
		}
	}()

	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write state cache: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("flush state cache: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close state cache: %w", err)
	}
	if err := os.Chmod(temporaryPath, 0o644); err != nil {
		return fmt.Errorf("set state cache permissions: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("commit state cache: %w", err)
	}
	cleanup = false
	return nil
}
