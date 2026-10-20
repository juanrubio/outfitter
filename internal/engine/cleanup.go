package engine

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"strings"
)

// Indirections so tests can exercise the sudo fallback without root.
var (
	removeFile     = os.Remove
	sudoRemoveFile = func(path string) error {
		return exec.Command("sudo", "rm", "--", path).Run() //nolint:gosec // path comes from discovered tool state, shown to the user before confirming
	}
)

// RemoveShadowedBinaries deletes each path, falling back to sudo when a
// plain remove is denied (system-owned strays). Missing files count as
// removed. It returns the paths that are gone and a joined error for
// the rest. Callers are expected to have verified sudo credentials
// first when any path needs them.
func RemoveShadowedBinaries(paths []string) (removed []string, err error) {
	var failures []error
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		removeErr := removeFile(path)
		switch {
		case removeErr == nil || errors.Is(removeErr, fs.ErrNotExist):
			removed = append(removed, path)
		case errors.Is(removeErr, fs.ErrPermission):
			if sudoErr := sudoRemoveFile(path); sudoErr != nil {
				failures = append(failures, fmt.Errorf("%s: %w", path, sudoErr))
			} else {
				removed = append(removed, path)
			}
		default:
			failures = append(failures, fmt.Errorf("%s: %w", path, removeErr))
		}
	}
	return removed, errors.Join(failures...)
}
