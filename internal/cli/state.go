package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/juanrubio/outfitter/internal/catalog"
	"github.com/juanrubio/outfitter/internal/demo"
	"github.com/juanrubio/outfitter/internal/domain"
	"github.com/juanrubio/outfitter/internal/engine"
)

type statusEntry struct {
	ID               string             `json:"id"`
	Name             string             `json:"name"`
	Category         string             `json:"category"`
	Backend          domain.BackendType `json:"backend"`
	InstalledVersion string             `json:"installed_version"`
	LatestVersion    string             `json:"latest_version"`
	Installed        bool               `json:"installed"`
	Outdated         bool               `json:"outdated"`
	BinaryPath       string             `json:"binary_path"`
	Provenance       domain.Provenance  `json:"provenance"`
}

func (opts *options) loadCatalog() (*catalog.Catalog, error) {
	if opts.demo {
		snapshot, err := demo.Load()
		if err != nil {
			return nil, err
		}
		return snapshot.Catalog, nil
	}
	if opts.catalog != "" {
		return catalog.Load(opts.catalog)
	}
	if opts.deps.HomeDir == "" {
		return catalog.LoadDefault()
	}
	path := filepath.Join(opts.deps.HomeDir, ".config", "outfitter", "catalog.toml")
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return catalog.LoadDefault()
		}
		return nil, fmt.Errorf("read user catalog %s: %w", path, err)
	}
	return catalog.Load(path)
}

func (opts *options) localStates(
	ctx context.Context,
	cat *catalog.Catalog,
) (map[string]domain.ToolState, error) {
	if opts.demo {
		snapshot, err := demo.Load()
		if err != nil {
			return nil, err
		}
		return snapshot.States, nil
	}
	return opts.deps.InspectLocal(ctx, cat), nil
}

func (opts *options) upstreamStates(
	ctx context.Context,
	cat *catalog.Catalog,
	states map[string]domain.ToolState,
) map[string]domain.ToolState {
	if opts.demo {
		return states
	}
	return opts.deps.InspectUpstream(ctx, cat, states, nil)
}

func (opts *options) miseStatus() (bool, string) {
	if opts.demo {
		snapshot, err := demo.Load()
		if err != nil {
			return false, ""
		}
		return snapshot.MiseInstalled, snapshot.MiseVersion
	}
	_, installed := opts.deps.MiseInstalled()
	return installed, ""
}

func (opts *options) mergeCachedUpstream(states map[string]domain.ToolState) map[string]domain.ToolState {
	if opts.demo {
		return states
	}
	path := engine.DefaultCachePath(opts.deps.HomeDir)
	if path == "" {
		return states
	}
	snapshot, err := engine.LoadCache(path)
	if err != nil {
		return states
	}
	merged := cloneStateMap(states)
	for id, state := range merged {
		cached, ok := snapshot.States[id]
		if !ok {
			continue
		}
		if state.LatestVersion == "" {
			state.LatestVersion = cached.LatestVersion
		}
		if state.CheckStatus == domain.StatusUnknown && cached.CheckStatus != domain.StatusUnknown {
			state.CheckStatus = cached.CheckStatus
		}
		merged[id] = state
	}
	return merged
}

func (opts *options) saveCache(states map[string]domain.ToolState) error {
	if opts.demo {
		return nil
	}
	path := engine.DefaultCachePath(opts.deps.HomeDir)
	if path == "" {
		return errors.New("state cache path is empty")
	}
	return engine.SaveCache(path, engine.CacheSnapshot{States: states})
}

func statusEntries(cat *catalog.Catalog, states map[string]domain.ToolState) []statusEntry {
	entries := make([]statusEntry, 0, len(cat.Tools))
	for _, tool := range cat.Tools {
		state := states[tool.ID]
		entries = append(entries, statusEntry{
			ID:               tool.ID,
			Name:             tool.Name,
			Category:         tool.CategoryID,
			Backend:          tool.Backend,
			InstalledVersion: state.InstalledVersion,
			LatestVersion:    state.LatestVersion,
			Installed:        state.IsInstalled(),
			Outdated:         state.IsOutdated(),
			BinaryPath:       state.BinaryPath,
			Provenance:       state.Provenance,
		})
	}
	return entries
}

func statusLabel(state domain.ToolState) string {
	switch {
	case state.IsOutdated():
		return "update available"
	case state.IsInstalled():
		return "installed"
	default:
		return "not installed"
	}
}

func cloneStateMap(states map[string]domain.ToolState) map[string]domain.ToolState {
	cloned := make(map[string]domain.ToolState, len(states))
	for id, state := range states {
		cloned[id] = state
	}
	return cloned
}
