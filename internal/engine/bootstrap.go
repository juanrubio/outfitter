// Package engine discovers local tool installations, checks upstream versions,
// bootstraps mise, and caches state snapshots. It isolates every external side
// effect behind injectable callbacks so the behavior is fully testable offline.
package engine

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	// DefaultMiseReleaseURL is the canonical GitHub release download prefix.
	DefaultMiseReleaseURL = "https://github.com/jdx/mise/releases/latest/download"

	defaultHTTPTimeout = 60 * time.Second

	maxChecksumBytes = 1 << 20
	maxArchiveBytes  = 512 << 20
	maxBinaryBytes   = 256 << 20
	maxArchiveFiles  = 10000
)

// ErrUnsupportedPlatform reports a host that has no official mise archive.
var ErrUnsupportedPlatform = errors.New("unsupported platform for mise")

// FindMise locates mise on PATH, falling back to <homeDir>/.local/bin/mise.
// A nil lookPath uses exec.LookPath. The fallback must be an executable regular
// file.
func FindMise(lookPath func(string) (string, error), homeDir string) (string, bool) {
	if lookPath == nil {
		lookPath = exec.LookPath
	}
	if resolved, err := lookPath("mise"); err == nil && strings.TrimSpace(resolved) != "" {
		return resolved, true
	}
	if homeDir == "" {
		return "", false
	}

	candidate := filepath.Join(homeDir, ".local", "bin", "mise")
	info, err := os.Stat(candidate)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
		return "", false
	}
	return candidate, true
}

// MiseEnv returns baseEnv with ~/.local/bin and the mise shims directory
// prepended to PATH when they are not already present.
func MiseEnv(baseEnv []string, homeDir string) []string {
	env := append([]string(nil), baseEnv...)
	if homeDir == "" {
		return env
	}

	dirs := []string{
		filepath.Join(homeDir, ".local", "bin"),
		filepath.Join(homeDir, ".local", "share", "mise", "shims"),
	}

	pathIndex := -1
	current := ""
	for index, entry := range env {
		if strings.HasPrefix(entry, "PATH=") {
			pathIndex = index
			current = strings.TrimPrefix(entry, "PATH=")
			break
		}
	}

	existing := splitPath(current)
	prefix := make([]string, 0, len(dirs))
	for _, dir := range dirs {
		if dir == "" || containsString(existing, dir) || containsString(prefix, dir) {
			continue
		}
		prefix = append(prefix, dir)
	}
	if len(prefix) == 0 {
		return env
	}

	combined := append(prefix, existing...)
	value := strings.Join(combined, string(os.PathListSeparator))
	if pathIndex >= 0 {
		env[pathIndex] = "PATH=" + value
	} else {
		env = append(env, "PATH="+value)
	}
	return env
}

// BootstrapOptions controls BootstrapMise. Zero values select safe defaults.
type BootstrapOptions struct {
	// HomeDir is used to derive the default install directory.
	HomeDir string
	// BaseURL is the release download prefix. It defaults to
	// DefaultMiseReleaseURL so tests can override it with an httptest server.
	BaseURL string
	// InstallDir receives the mise binary. It defaults to
	// <HomeDir>/.local/bin.
	InstallDir string
	// Arch overrides runtime.GOARCH. amd64 maps to x64 and arm64 to arm64.
	Arch string
	// OS overrides runtime.GOOS. Only linux is supported.
	OS string
	// HTTPClient overrides the default 60 second timeout client.
	HTTPClient *http.Client
	// Runner optionally verifies the installed binary with `mise --version`.
	Runner CommandRunner
}

func (o BootstrapOptions) resolved() (BootstrapOptions, error) {
	if o.HomeDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return o, fmt.Errorf("resolve home directory: %w", err)
		}
		o.HomeDir = home
	}
	if o.BaseURL == "" {
		o.BaseURL = DefaultMiseReleaseURL
	}
	o.BaseURL = strings.TrimRight(o.BaseURL, "/")
	if o.InstallDir == "" {
		o.InstallDir = filepath.Join(o.HomeDir, ".local", "bin")
	}
	if o.Arch == "" {
		o.Arch = runtime.GOARCH
	}
	if o.OS == "" {
		o.OS = runtime.GOOS
	}
	if o.HTTPClient == nil {
		o.HTTPClient = &http.Client{Timeout: defaultHTTPTimeout}
	}
	return o, nil
}

// BootstrapMise downloads the official mise archive, verifies its SHA-256
// digest against SHASUMS256.txt, and atomically installs <InstallDir>/mise with
// 0755 permissions. Corrupt archives, missing checksums, and checksum
// mismatches fail closed and leave no binary behind.
func BootstrapMise(ctx context.Context, opts BootstrapOptions) (string, error) {
	opts, err := opts.resolved()
	if err != nil {
		return "", err
	}

	arch, err := miseArchiveArch(opts.Arch)
	if err != nil {
		return "", err
	}
	if opts.OS != "linux" {
		return "", fmt.Errorf("%w: operating system %q (only linux is supported)", ErrUnsupportedPlatform, opts.OS)
	}

	checksums, err := downloadBytes(ctx, opts.HTTPClient, opts.BaseURL+"/SHASUMS256.txt", maxChecksumBytes)
	if err != nil {
		return "", fmt.Errorf("download mise checksums: %w", err)
	}
	archiveName, expected, err := checksumForPlatform(checksums, opts.OS, arch)
	if err != nil {
		return "", err
	}

	if err := os.MkdirAll(opts.InstallDir, 0o755); err != nil {
		return "", fmt.Errorf("create install directory %s: %w", opts.InstallDir, err)
	}

	archive, err := os.CreateTemp(opts.InstallDir, ".mise-archive-*")
	if err != nil {
		return "", fmt.Errorf("create temporary archive: %w", err)
	}
	archivePath := archive.Name()
	defer func() { _ = os.Remove(archivePath) }()

	hasher := sha256.New()
	written, err := downloadTo(ctx, opts.HTTPClient, opts.BaseURL+"/"+archiveName, io.MultiWriter(archive, hasher), maxArchiveBytes)
	if closeErr := archive.Close(); err == nil && closeErr != nil {
		err = closeErr
	}
	if err != nil {
		return "", fmt.Errorf("download mise archive: %w", err)
	}
	if written == 0 {
		return "", errors.New("download mise archive: empty response")
	}

	actual := hex.EncodeToString(hasher.Sum(nil))
	if !strings.EqualFold(actual, expected) {
		return "", fmt.Errorf("mise archive checksum mismatch: expected %s, got %s", expected, actual)
	}

	target, err := extractMiseBinary(archivePath, opts.InstallDir)
	if err != nil {
		return "", err
	}

	if opts.Runner != nil {
		if _, err := opts.Runner.Run(ctx, target, "--version"); err != nil {
			return target, fmt.Errorf("verify installed mise: %w", err)
		}
	}
	return target, nil
}

func miseArchiveArch(arch string) (string, error) {
	switch arch {
	case "amd64":
		return "x64", nil
	case "arm64":
		return "arm64", nil
	default:
		return "", fmt.Errorf("%w: architecture %q (supported: amd64, arm64)", ErrUnsupportedPlatform, arch)
	}
}

func downloadBytes(ctx context.Context, client *http.Client, rawURL string, limit int64) ([]byte, error) {
	var buffer bytes.Buffer
	if _, err := downloadTo(ctx, client, rawURL, &buffer, limit); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func downloadTo(ctx context.Context, client *http.Client, rawURL string, dst io.Writer, limit int64) (int64, error) {
	if client == nil {
		client = &http.Client{Timeout: defaultHTTPTimeout}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, http.NoBody)
	if err != nil {
		return 0, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("User-Agent", "outfitter")

	resp, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("request %s: %w", rawURL, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return 0, fmt.Errorf("request %s: unexpected status %s", rawURL, resp.Status)
	}

	written, err := io.Copy(dst, io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return written, fmt.Errorf("read %s: %w", rawURL, err)
	}
	if written > limit {
		return written, fmt.Errorf("response from %s exceeds %d bytes", rawURL, limit)
	}
	return written, nil
}

func checksumForPlatform(shasums []byte, goos, arch string) (string, string, error) {
	suffix := fmt.Sprintf("-%s-%s.tar.gz", goos, arch)
	latestName := "mise-latest" + suffix
	for _, line := range strings.Split(string(shasums), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		name := strings.TrimPrefix(fields[1], "*")
		name = strings.TrimPrefix(name, "./")
		if !matchesMiseArchive(name, latestName, suffix) {
			continue
		}
		digest := strings.ToLower(fields[0])
		if len(digest) != sha256.Size*2 {
			return "", "", fmt.Errorf("invalid SHA-256 digest for %s in SHASUMS256.txt", name)
		}
		if _, err := hex.DecodeString(digest); err != nil {
			return "", "", fmt.Errorf("invalid SHA-256 digest for %s in SHASUMS256.txt: %w", name, err)
		}
		return name, digest, nil
	}
	return "", "", fmt.Errorf("checksum for %s not found in SHASUMS256.txt", latestName)
}

func matchesMiseArchive(name, latestName, suffix string) bool {
	if name == latestName {
		return true
	}
	return strings.HasPrefix(name, "mise-v") && strings.HasSuffix(name, suffix)
}

func extractMiseBinary(archivePath, installDir string) (string, error) {
	archive, err := os.Open(archivePath)
	if err != nil {
		return "", fmt.Errorf("open mise archive: %w", err)
	}
	defer func() { _ = archive.Close() }()

	gzipped, err := gzip.NewReader(archive)
	if err != nil {
		return "", fmt.Errorf("read mise archive: %w", err)
	}
	defer func() { _ = gzipped.Close() }()

	target := filepath.Join(installDir, "mise")
	output, err := os.CreateTemp(installDir, ".mise-binary-*")
	if err != nil {
		return "", fmt.Errorf("create temporary binary: %w", err)
	}
	outputPath := output.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(outputPath)
		}
	}()

	reader := tar.NewReader(gzipped)
	files := 0
	found := false

	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", fmt.Errorf("read mise archive: %w", err)
		}
		files++
		if files > maxArchiveFiles {
			return "", fmt.Errorf("mise archive contains more than %d entries", maxArchiveFiles)
		}

		name, err := safeArchivePath(header.Name)
		if err != nil {
			return "", err
		}
		if name == "." {
			continue
		}
		// TypeReg is '0'; legacy archives may use the deprecated zero byte.
		if header.Typeflag != tar.TypeReg && header.Typeflag != 0 {
			continue
		}
		if name != "mise/bin/mise" && name != "bin/mise" && name != "mise" {
			continue
		}

		written, err := io.Copy(output, io.LimitReader(reader, maxBinaryBytes+1))
		if err != nil {
			return "", fmt.Errorf("extract mise binary: %w", err)
		}
		if written > maxBinaryBytes {
			return "", fmt.Errorf("mise binary exceeds %d bytes", maxBinaryBytes)
		}
		if written == 0 {
			return "", errors.New("mise archive contains an empty binary")
		}
		found = true
		break
	}

	if !found {
		return "", errors.New("mise archive does not contain a mise binary")
	}
	if err := output.Sync(); err != nil {
		return "", fmt.Errorf("flush mise binary: %w", err)
	}
	if err := output.Close(); err != nil {
		return "", fmt.Errorf("close mise binary: %w", err)
	}
	if err := os.Chmod(outputPath, 0o755); err != nil {
		return "", fmt.Errorf("set mise permissions: %w", err)
	}
	if err := os.Rename(outputPath, target); err != nil {
		return "", fmt.Errorf("install mise binary: %w", err)
	}
	cleanup = false
	return target, nil
}

func safeArchivePath(name string) (string, error) {
	trimmed := strings.TrimPrefix(strings.TrimSpace(name), "./")
	cleaned := path.Clean(trimmed)
	if strings.ContainsRune(cleaned, '\x00') || strings.ContainsRune(cleaned, '\\') ||
		path.IsAbs(cleaned) || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", fmt.Errorf("mise archive contains unsafe path %q", name)
	}
	return cleaned, nil
}

func splitPath(value string) []string {
	if value == "" {
		return nil
	}
	parts := strings.Split(value, string(os.PathListSeparator))
	filtered := make([]string, 0, len(parts))
	for _, part := range parts {
		if part != "" {
			filtered = append(filtered, part)
		}
	}
	return filtered
}

func containsString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
