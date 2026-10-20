package adapter

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	// DefaultCodexBarAPIBaseURL is the GitHub API endpoint used to resolve the
	// latest CodexBar Linux release when no archive URL is supplied.
	DefaultCodexBarAPIBaseURL = "https://api.github.com"

	codexBarRepository     = "steipete/CodexBar"
	codexBarPrimaryBinary  = "CodexBarCLI"
	codexBarFallbackBinary = "codexbar"
	codexBarHTTPTimeout    = 60 * time.Second
	maxCodexBarArchiveSize = 512 << 20
	maxCodexBarFiles       = 10000
)

// CodexBarOptions controls the native CodexBar bundle installer. Every network
// input is overridable so tests can use an httptest server without touching the
// public internet.
type CodexBarOptions struct {
	HomeDir          string
	Version          string
	ArchiveURL       string
	GitHubAPIBaseURL string
	HTTPClient       *http.Client
	OS               string
	Arch             string
}

func (o CodexBarOptions) resolved() (CodexBarOptions, error) {
	if o.HomeDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return o, fmt.Errorf("resolve home directory: %w", err)
		}
		o.HomeDir = home
	}
	if o.GitHubAPIBaseURL == "" {
		o.GitHubAPIBaseURL = DefaultCodexBarAPIBaseURL
	}
	o.GitHubAPIBaseURL = strings.TrimRight(o.GitHubAPIBaseURL, "/")
	if o.HTTPClient == nil {
		o.HTTPClient = &http.Client{Timeout: codexBarHTTPTimeout}
	}
	if o.OS == "" {
		o.OS = runtime.GOOS
	}
	if o.Arch == "" {
		o.Arch = runtime.GOARCH
	}
	return o, nil
}

// InstallCodexBar downloads the Linux CodexBar release archive and installs the
// executable and its companion Swift resource bundle into
// <home>/.local/share/outfitter/codexbar. <home>/.local/bin/codexbar is then
// repointed at the executable.
func InstallCodexBar(ctx context.Context, opts CodexBarOptions, onLine func(string)) error {
	opts, err := opts.resolved()
	if err != nil {
		return err
	}

	archiveURL := strings.TrimSpace(opts.ArchiveURL)
	version := ""
	if archiveURL == "" {
		archiveURL, version, err = resolveCodexBarArchiveURL(ctx, opts)
		if err != nil {
			return err
		}
	} else if v := strings.TrimSpace(opts.Version); v != "" && !strings.EqualFold(v, "latest") {
		version = normalizeCodexBarVersion(v)
	}

	emitLine(onLine, "Downloading CodexBar release from "+archiveURL)
	archivePath, err := downloadCodexBarArchive(ctx, opts.HTTPClient, archiveURL)
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(archivePath) }()

	shareDir := filepath.Join(opts.HomeDir, ".local", "share", "outfitter")
	if err := os.MkdirAll(shareDir, 0o755); err != nil {
		return fmt.Errorf("create CodexBar share directory: %w", err)
	}
	stageDir, err := os.MkdirTemp(shareDir, ".codexbar-stage-*")
	if err != nil {
		return fmt.Errorf("create CodexBar staging directory: %w", err)
	}
	installed := false
	defer func() {
		if !installed {
			_ = os.RemoveAll(stageDir)
		}
	}()

	binaryName, err := extractCodexBarBundle(archivePath, stageDir)
	if err != nil {
		return err
	}
	emitLine(onLine, "Extracted "+binaryName+" and companion resource bundle")

	targetDir := filepath.Join(shareDir, "codexbar")
	targetBinary := filepath.Join(targetDir, binaryName)
	if err := os.Chmod(filepath.Join(stageDir, binaryName), 0o755); err != nil {
		return fmt.Errorf("set CodexBar executable permissions: %w", err)
	}
	// CodexBarCLI cannot report its own version (`--version` prints only
	// its name), so record the resolved release version for discovery.
	if version != "" {
		if err := os.WriteFile(filepath.Join(stageDir, "VERSION"), []byte(version+"\n"), 0o644); err != nil {
			return fmt.Errorf("write CodexBar VERSION sidecar: %w", err)
		}
	}
	if err := os.RemoveAll(targetDir); err != nil {
		return fmt.Errorf("replace CodexBar installation: %w", err)
	}
	if err := os.Rename(stageDir, targetDir); err != nil {
		return fmt.Errorf("install CodexBar bundle: %w", err)
	}
	installed = true

	binDir := filepath.Join(opts.HomeDir, ".local", "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return fmt.Errorf("create binary directory: %w", err)
	}
	linkPath := filepath.Join(binDir, codexBarFallbackBinary)
	if err := os.RemoveAll(linkPath); err != nil {
		return fmt.Errorf("replace CodexBar symlink: %w", err)
	}
	if err := os.Symlink(targetBinary, linkPath); err != nil {
		return fmt.Errorf("link CodexBar executable: %w", err)
	}
	emitLine(onLine, "Linked "+linkPath+" -> "+targetBinary)
	return nil
}

// RemoveCodexBar removes the user-space symlink and bundle directory.
func RemoveCodexBar(homeDir string, onLine func(string)) error {
	if homeDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("resolve home directory: %w", err)
		}
		homeDir = home
	}

	linkPath := filepath.Join(homeDir, ".local", "bin", codexBarFallbackBinary)
	if err := os.RemoveAll(linkPath); err != nil {
		return fmt.Errorf("remove CodexBar symlink: %w", err)
	}
	emitLine(onLine, "Removed "+linkPath)

	targetDir := filepath.Join(homeDir, ".local", "share", "outfitter", "codexbar")
	if err := os.RemoveAll(targetDir); err != nil {
		return fmt.Errorf("remove CodexBar bundle: %w", err)
	}
	emitLine(onLine, "Removed "+targetDir)
	return nil
}

type codexBarRelease struct {
	TagName string `json:"tag_name"`
	Assets  []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

func resolveCodexBarArchiveURL(ctx context.Context, opts CodexBarOptions) (string, string, error) {
	endpoint := opts.GitHubAPIBaseURL + "/repos/" + codexBarRepository + "/releases/latest"
	version := strings.TrimSpace(opts.Version)
	if version != "" && !strings.EqualFold(version, "latest") {
		endpoint = opts.GitHubAPIBaseURL + "/repos/" + codexBarRepository +
			"/releases/tags/" + url.PathEscape(version)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, http.NoBody)
	if err != nil {
		return "", "", fmt.Errorf("build CodexBar release request: %w", err)
	}
	req.Header.Set("User-Agent", "outfitter")
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := opts.HTTPClient.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("request CodexBar release: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return "", "", fmt.Errorf("request CodexBar release: unexpected status %s", resp.Status)
	}

	var release codexBarRelease
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&release); err != nil {
		return "", "", fmt.Errorf("decode CodexBar release: %w", err)
	}
	if chosen, ok := selectCodexBarAsset(release, opts.Arch); ok {
		return chosen, normalizeCodexBarVersion(release.TagName), nil
	}
	return "", "", errors.New("CodexBar release has no matching Linux .tar.gz asset")
}

func selectCodexBarAsset(release codexBarRelease, arch string) (string, bool) {
	archTokens := codexBarArchTokens(arch)
	bestScore := -1
	bestURL := ""
	for _, asset := range release.Assets {
		name := strings.ToLower(asset.Name)
		if !strings.HasSuffix(name, ".tar.gz") && !strings.HasSuffix(name, ".tgz") {
			continue
		}
		if strings.Contains(name, "sha256") || strings.Contains(name, "checksums") {
			continue
		}
		if !strings.Contains(name, "linux") {
			continue
		}
		score := 2
		for _, token := range archTokens {
			if strings.Contains(name, token) {
				score += 4
				break
			}
		}
		if strings.TrimSpace(asset.BrowserDownloadURL) == "" {
			continue
		}
		if score > bestScore {
			bestScore = score
			bestURL = asset.BrowserDownloadURL
		}
	}
	if bestURL == "" || bestScore < 2 {
		return "", false
	}
	return bestURL, true
}

func codexBarArchTokens(arch string) []string {
	switch arch {
	case "amd64":
		return []string{"x86_64", "amd64", "x64"}
	case "arm64":
		return []string{"arm64", "aarch64"}
	default:
		return []string{arch}
	}
}

func downloadCodexBarArchive(ctx context.Context, client *http.Client, rawURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, http.NoBody)
	if err != nil {
		return "", fmt.Errorf("build CodexBar download request: %w", err)
	}
	req.Header.Set("User-Agent", "outfitter")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("download CodexBar archive: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return "", fmt.Errorf("download CodexBar archive: unexpected status %s", resp.Status)
	}

	archive, err := os.CreateTemp("", "outfitter-codexbar-*.tar.gz")
	if err != nil {
		return "", fmt.Errorf("create CodexBar archive file: %w", err)
	}
	archivePath := archive.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(archivePath)
		}
	}()

	written, copyErr := io.Copy(archive, io.LimitReader(resp.Body, maxCodexBarArchiveSize+1))
	closeErr := archive.Close()
	if copyErr != nil {
		return "", fmt.Errorf("write CodexBar archive: %w", copyErr)
	}
	if closeErr != nil {
		return "", fmt.Errorf("close CodexBar archive: %w", closeErr)
	}
	if written > maxCodexBarArchiveSize {
		return "", fmt.Errorf("CodexBar archive exceeds %d bytes", maxCodexBarArchiveSize)
	}
	if written == 0 {
		return "", errors.New("CodexBar archive is empty")
	}
	cleanup = false
	return archivePath, nil
}

func extractCodexBarBundle(archivePath, stageDir string) (string, error) {
	archive, err := os.Open(archivePath)
	if err != nil {
		return "", fmt.Errorf("open CodexBar archive: %w", err)
	}
	defer func() { _ = archive.Close() }()

	gzipped, err := gzip.NewReader(archive)
	if err != nil {
		return "", fmt.Errorf("read CodexBar archive: %w", err)
	}
	defer func() { _ = gzipped.Close() }()

	reader := tar.NewReader(gzipped)
	files := 0
	var total int64
	foundExecutable := false

	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", fmt.Errorf("read CodexBar archive: %w", err)
		}
		files++
		if files > maxCodexBarFiles {
			return "", fmt.Errorf("CodexBar archive contains more than %d entries", maxCodexBarFiles)
		}

		clean, err := safeCodexBarPath(header.Name)
		if err != nil {
			return "", err
		}
		if clean == "." {
			continue
		}
		relative, executable := codexBarBundleEntry(clean)
		if relative == "" {
			continue
		}

		destination := filepath.Join(stageDir, filepath.FromSlash(relative))
		if header.Typeflag == tar.TypeDir {
			if err := os.MkdirAll(destination, 0o755); err != nil {
				return "", fmt.Errorf("create CodexBar bundle directory: %w", err)
			}
			continue
		}
		if header.Typeflag != tar.TypeReg && header.Typeflag != 0 {
			continue
		}

		if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
			return "", fmt.Errorf("create CodexBar bundle directory: %w", err)
		}
		mode := os.FileMode(header.Mode) & 0o777
		if executable {
			mode = 0o755
			foundExecutable = true
		} else if mode == 0 {
			mode = 0o644
		}

		output, err := os.OpenFile(destination, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
		if err != nil {
			return "", fmt.Errorf("create CodexBar bundle file: %w", err)
		}
		written, copyErr := io.Copy(output, io.LimitReader(reader, maxCodexBarArchiveSize-total+1))
		closeErr := output.Close()
		if copyErr != nil {
			return "", fmt.Errorf("extract CodexBar bundle file: %w", copyErr)
		}
		if closeErr != nil {
			return "", fmt.Errorf("close CodexBar bundle file: %w", closeErr)
		}
		total += written
		if total > maxCodexBarArchiveSize {
			return "", fmt.Errorf("CodexBar bundle exceeds %d bytes", maxCodexBarArchiveSize)
		}
	}

	if !foundExecutable {
		return "", errors.New("CodexBar archive does not contain CodexBarCLI or codexbar")
	}

	for _, candidate := range []string{codexBarPrimaryBinary, codexBarFallbackBinary} {
		info, err := os.Stat(filepath.Join(stageDir, candidate))
		if err == nil && info.Mode().IsRegular() {
			if err := os.Chmod(filepath.Join(stageDir, candidate), 0o755); err != nil {
				return "", fmt.Errorf("set CodexBar executable permissions: %w", err)
			}
			return candidate, nil
		}
	}
	return "", errors.New("CodexBar executable was not extracted")
}

func safeCodexBarPath(name string) (string, error) {
	trimmed := strings.TrimPrefix(strings.TrimSpace(name), "./")
	cleaned := path.Clean(trimmed)
	if strings.ContainsRune(cleaned, '\x00') || strings.ContainsRune(cleaned, '\\') ||
		path.IsAbs(cleaned) || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", fmt.Errorf("CodexBar archive contains unsafe path %q", name)
	}
	return cleaned, nil
}

func codexBarBundleEntry(clean string) (relative string, executable bool) {
	parts := strings.Split(clean, "/")
	base := parts[len(parts)-1]
	if strings.EqualFold(base, codexBarPrimaryBinary) || strings.EqualFold(base, codexBarFallbackBinary) {
		return base, true
	}
	for index, part := range parts {
		if strings.HasSuffix(part, ".bundle") || strings.HasSuffix(part, ".resources") {
			return strings.Join(parts[index:], "/"), false
		}
	}
	return "", false
}

// normalizeCodexBarVersion reduces a release tag like "v0.78.0" to the
// bare version recorded in the VERSION sidecar.
func normalizeCodexBarVersion(version string) string {
	return strings.TrimPrefix(strings.TrimSpace(version), "v")
}

func emitLine(onLine func(string), line string) {
	if onLine != nil {
		onLine(line)
	}
}
