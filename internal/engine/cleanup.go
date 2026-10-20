package engine

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/juanrubio/outfitter/internal/domain"
)

// Indirections so tests can exercise the sudo fallback without root.
var (
	removeFile     = os.Remove
	sudoRemoveFile = func(path string) error {
		return exec.Command("sudo", "rm", "--", path).Run() //nolint:gosec // path comes from discovered tool state, shown to the user before confirming
	}
	npmUninstallFile = func(npmPath, prefix, pkg string) error {
		return exec.Command(npmPath, "uninstall", "-g", "--prefix", prefix, pkg).Run() //nolint:gosec // npm path, prefix, and package are derived from the discovered install layout
	}
)

// npmGlobalInstall reports whether resolvedPath lives under an npm
// global tree (lib/node_modules), returning the sibling npm executable,
// the tree's prefix (the directory containing lib/), and the package
// name. Deleting such a binary with rm would orphan the package; npm
// uninstall removes both. The prefix must travel with the command: an
// npm shim derives its global prefix from whichever node the
// environment resolves, so unpinned uninstalls can silently target a
// different tree and still exit 0.
func npmGlobalInstall(binaryPath, resolvedPath string) (npmPath, prefix, pkg string, ok bool) {
	const marker = "/lib/node_modules/"
	slashResolved := filepath.ToSlash(resolvedPath)
	idx := strings.Index(slashResolved, marker)
	if idx < 0 {
		return "", "", "", false
	}
	parts := strings.Split(slashResolved[idx+len(marker):], "/")
	if len(parts) == 0 || parts[0] == "" {
		return "", "", "", false
	}
	if strings.HasPrefix(parts[0], "@") {
		if len(parts) < 2 || parts[1] == "" {
			return "", "", "", false
		}
		pkg = parts[0] + "/" + parts[1]
	} else {
		pkg = parts[0]
	}
	prefix = filepath.Clean(filepath.FromSlash(slashResolved[:idx]))
	return filepath.Join(filepath.Dir(binaryPath), "npm"), prefix, pkg, true
}

// removalCommandFor returns the shell command that removes the binary
// at binaryPath: npm uninstall for npm-global installs (when the
// sibling npm exists), plain rm otherwise.
func removalCommandFor(binaryPath, resolvedPath string, stat func(string) (os.FileInfo, error)) string {
	if npmPath, prefix, pkg, ok := npmGlobalInstall(binaryPath, resolvedPath); ok {
		if stat == nil {
			stat = os.Stat
		}
		if _, err := stat(npmPath); err == nil {
			return domain.ShellQuotePath(npmPath) + " uninstall -g --prefix " + domain.ShellQuotePath(prefix) + " " + pkg
		}
	}
	return domain.RemovalCommand(binaryPath)
}

// resolvePathLinks follows symlinks from path as far as possible.
func resolvePathLinks(path string) string {
	current := path
	for depth := 0; depth < 8; depth++ {
		target, err := os.Readlink(current)
		if err != nil || strings.TrimSpace(target) == "" {
			return current
		}
		if !filepath.IsAbs(target) {
			target = filepath.Join(filepath.Dir(current), target)
		}
		current = filepath.Clean(target)
	}
	return current
}

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
		if npmPath, prefix, pkg, ok := npmGlobalInstall(path, resolvePathLinks(path)); ok {
			if err := npmUninstallFile(npmPath, prefix, pkg); err == nil {
				if _, statErr := os.Lstat(path); statErr != nil {
					removed = append(removed, path)
					continue
				}
				// npm exited 0 but the binary is still there (silent
				// no-op against the wrong prefix): fall through to
				// direct removal so progress is still made.
			}
			// Fall through to direct removal so progress is still made.
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
