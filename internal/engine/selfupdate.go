package engine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/juanrubio/outfitter/internal/catalog"
	"github.com/juanrubio/outfitter/internal/domain"
)

const (
	// SelfRepo is the GitHub repository that publishes Outfitter releases.
	SelfRepo = "juanrubio/outfitter"
	// DefaultSelfCheckInterval bounds how often background self-checks hit GitHub.
	DefaultSelfCheckInterval = 24 * time.Hour
	// NoUpdateCheckEnv disables background self-update checks when set.
	NoUpdateCheckEnv = "OUTFITTER_NO_UPDATE_CHECK"

	selfReleaseURL = "https://github.com/juanrubio/outfitter/releases/latest"
	selfMiseSpec   = "ubi:juanrubio/outfitter[exe=outfitter]"
)

// ErrSelfUpdateUnsupported reports an installation Outfitter cannot safely
// replace itself (development build, read-only location, unsupported OS).
var ErrSelfUpdateUnsupported = errors.New("self-update is not available for this installation")

// SelfInstallKind describes how the running Outfitter binary was installed,
// which decides how (or whether) it can update itself.
type SelfInstallKind string

const (
	SelfInstallMise        SelfInstallKind = "mise"
	SelfInstallStandalone  SelfInstallKind = "standalone"
	SelfInstallManual      SelfInstallKind = "manual"
	SelfInstallDevelopment SelfInstallKind = "development"
	SelfInstallUnknown     SelfInstallKind = "unknown"
)

// SelfState is the observed update state of the running Outfitter binary. It
// is deliberately separate from catalog tool states: Outfitter itself is not
// a catalog tool and must be visible under every catalog, including full
// replacement catalogs.
type SelfState struct {
	CurrentVersion string          `json:"current_version"`
	LatestVersion  string          `json:"latest_version,omitempty"`
	Outdated       bool            `json:"outdated"`
	CheckedAt      time.Time       `json:"checked_at,omitempty"`
	ExecutablePath string          `json:"executable_path,omitempty"`
	ResolvedPath   string          `json:"resolved_path,omitempty"`
	InstallKind    SelfInstallKind `json:"install_kind,omitempty"`
	CanSelfUpdate  bool            `json:"can_self_update"`
	ManualHint     string          `json:"manual_hint,omitempty"`
	ReleaseURL     string          `json:"release_url,omitempty"`
	UpdateCommand  string          `json:"update_command,omitempty"`
	LastError      string          `json:"last_error,omitempty"`
}

// SelfUpdateOptions controls a self-update attempt.
type SelfUpdateOptions struct {
	DryRun bool
}

// SelfUpdateResult reports the outcome of Update.
type SelfUpdateResult struct {
	State           SelfState `json:"state"`
	Updated         bool      `json:"updated"`
	DryRun          bool      `json:"dry_run"`
	Command         string    `json:"command,omitempty"`
	BackupPath      string    `json:"backup_path,omitempty"`
	RequiresRestart bool      `json:"requires_restart"`
	Message         string    `json:"message,omitempty"`
}

// SelfUpdater checks GitHub for newer Outfitter releases and replaces the
// running binary when its install provenance makes that safe. Side effects
// are injectable so tests stay offline.
type SelfUpdater struct {
	CurrentVersion   string
	HomeDir          string
	ExecutablePath   string
	HTTPClient       *http.Client
	GitHubAPIBaseURL string
	Runner           CommandRunner
	LookPath         func(string) (string, error)
	EvalSymlinks     func(string) (string, error)
	Getenv           func(string) string
	Now              func() time.Time
	GOOS             string
	GOARCH           string
	CachePath        string
	CheckInterval    time.Duration
}

// NewSelfUpdater returns an updater for the given build version.
func NewSelfUpdater(currentVersion, homeDir string) *SelfUpdater {
	return (&SelfUpdater{CurrentVersion: currentVersion, HomeDir: homeDir}).withDefaults()
}

func (u *SelfUpdater) withDefaults() *SelfUpdater {
	copy := &SelfUpdater{}
	if u != nil {
		*copy = *u
	}
	if copy.HomeDir == "" {
		if home, err := os.UserHomeDir(); err == nil {
			copy.HomeDir = home
		}
	}
	if copy.ExecutablePath == "" {
		if exe, err := os.Executable(); err == nil {
			copy.ExecutablePath = exe
		}
	}
	if copy.HTTPClient == nil {
		copy.HTTPClient = &http.Client{Timeout: defaultHTTPTimeout}
	}
	if copy.GitHubAPIBaseURL == "" {
		copy.GitHubAPIBaseURL = defaultGitHubAPIURL
	}
	if copy.Runner == nil {
		copy.Runner = ExecRunner{}
	}
	if copy.LookPath == nil {
		copy.LookPath = exec.LookPath
	}
	if copy.EvalSymlinks == nil {
		copy.EvalSymlinks = filepath.EvalSymlinks
	}
	if copy.Getenv == nil {
		copy.Getenv = os.Getenv
	}
	if copy.Now == nil {
		copy.Now = time.Now
	}
	if copy.GOOS == "" {
		copy.GOOS = runtime.GOOS
	}
	if copy.GOARCH == "" {
		copy.GOARCH = runtime.GOARCH
	}
	if copy.CheckInterval <= 0 {
		copy.CheckInterval = DefaultSelfCheckInterval
	}
	return copy
}

// DefaultSelfCheckCachePath returns <homeDir>/.cache/outfitter/self-check.json.
func DefaultSelfCheckCachePath(homeDir string) string {
	if homeDir == "" {
		home, err := os.UserHomeDir()
		if err != nil || home == "" {
			return ""
		}
		homeDir = home
	}
	return filepath.Join(homeDir, ".cache", "outfitter", "self-check.json")
}

func (u *SelfUpdater) cachePath() string {
	if u.CachePath != "" {
		return u.CachePath
	}
	return DefaultSelfCheckCachePath(u.HomeDir)
}

type selfCheckRecord struct {
	LatestVersion string    `json:"latest_version,omitempty"`
	ReleaseURL    string    `json:"release_url,omitempty"`
	CheckedAt     time.Time `json:"checked_at,omitempty"`
	LastError     string    `json:"last_error,omitempty"`
}

func (u *SelfUpdater) checksEnabled(cat *catalog.Catalog) bool {
	if strings.TrimSpace(u.Getenv(NoUpdateCheckEnv)) != "" {
		return false
	}
	return cat == nil || cat.Settings.SelfUpdateCheck
}

func (u *SelfUpdater) baseState() SelfState {
	current := NormalizeVersion(u.CurrentVersion)
	state := SelfState{
		CurrentVersion: current,
		ExecutablePath: u.ExecutablePath,
		InstallKind:    SelfInstallUnknown,
		ReleaseURL:     selfReleaseURL,
	}
	resolved := u.ExecutablePath
	if resolved != "" {
		if absolute, err := filepath.Abs(resolved); err == nil {
			resolved = absolute
		}
		if evaluated, err := u.EvalSymlinks(resolved); err == nil && evaluated != "" {
			resolved = evaluated
		}
	}
	state.ResolvedPath = resolved
	if current == "" || strings.EqualFold(strings.TrimSpace(u.CurrentVersion), "dev") {
		state.InstallKind = SelfInstallDevelopment
		state.ManualHint = "development build; self-update is disabled"
		return state
	}
	state.InstallKind, state.CanSelfUpdate, state.ManualHint = u.classifyInstall(state.ExecutablePath, resolved)
	state.UpdateCommand = u.updateCommand(state, "")
	return state
}

func (u *SelfUpdater) classifyInstall(original, resolved string) (SelfInstallKind, bool, string) {
	if isMiseManagedPath(original) || isMiseManagedPath(resolved) {
		if _, found := FindMise(u.LookPath, u.HomeDir); found {
			return SelfInstallMise, true, ""
		}
		return SelfInstallMise, false, "mise was not found; reinstall from " + selfReleaseURL
	}
	if resolved == "" {
		return SelfInstallUnknown, false, "download the latest release from " + selfReleaseURL
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.Mode().IsRegular() {
		return SelfInstallUnknown, false, "download the latest release from " + selfReleaseURL
	}
	if !supportedSelfPlatform(u.GOOS, u.GOARCH) {
		return SelfInstallManual, false, fmt.Sprintf("no release binary for %s/%s; download manually from %s", u.GOOS, u.GOARCH, selfReleaseURL)
	}
	if directoryWritable(filepath.Dir(resolved)) {
		return SelfInstallStandalone, true, ""
	}
	return SelfInstallManual, false, "binary directory is not writable; download the latest release from " + selfReleaseURL
}

func isMiseManagedPath(path string) bool {
	slash := filepath.ToSlash(path)
	return strings.Contains(slash, "mise/shims/") ||
		strings.Contains(slash, "mise/installs/ubi-juanrubio-outfitter/") ||
		strings.Contains(slash, "mise/installs/ubi/juanrubio/outfitter/")
}

func supportedSelfPlatform(goos, goarch string) bool {
	return (goos == "linux" || goos == "darwin") && (goarch == "amd64" || goarch == "arm64")
}

func directoryWritable(dir string) bool {
	if dir == "" {
		return false
	}
	probe, err := os.CreateTemp(dir, ".outfitter-write-*")
	if err != nil {
		return false
	}
	name := probe.Name()
	_ = probe.Close()
	_ = os.Remove(name)
	return true
}

func (u *SelfUpdater) applyLatest(state *SelfState, latest, releaseURL string) {
	state.LatestVersion = NormalizeVersion(latest)
	if releaseURL != "" {
		state.ReleaseURL = releaseURL
	}
	state.Outdated = state.LatestVersion != "" && state.CurrentVersion != "" &&
		domain.CompareVersions(state.LatestVersion, state.CurrentVersion) > 0
	state.UpdateCommand = u.updateCommand(*state, state.LatestVersion)
}

func (u *SelfUpdater) updateCommand(state SelfState, latest string) string {
	if state.InstallKind == SelfInstallMise {
		if latest != "" {
			return "mise use --global " + selfMiseSpec + "@" + NormalizeVersion(latest)
		}
		return "mise use --global " + selfMiseSpec + "@latest"
	}
	if state.InstallKind == SelfInstallStandalone && state.ResolvedPath != "" {
		return "download, verify, and replace " + state.ResolvedPath
	}
	return ""
}

// CachedState returns the last background check without any network I/O. It
// recomputes outdatedness against the running binary, so a just-updated
// binary never reports itself outdated from a stale cache entry.
func (u *SelfUpdater) CachedState(cat *catalog.Catalog) SelfState {
	u = u.withDefaults()
	state := u.baseState()
	if state.InstallKind == SelfInstallDevelopment || !u.checksEnabled(cat) {
		return state
	}
	record, err := u.loadRecord()
	if err != nil {
		return state
	}
	state.CheckedAt = record.CheckedAt
	state.LastError = record.LastError
	u.applyLatest(&state, record.LatestVersion, record.ReleaseURL)
	return state
}

// Check reports whether a newer Outfitter release exists. Unless force is
// set, fresh cached results are returned without contacting GitHub.
// Development builds and opt-out settings short-circuit without network I/O.
func (u *SelfUpdater) Check(ctx context.Context, cat *catalog.Catalog, force bool) (SelfState, error) {
	return u.check(ctx, cat, force, false)
}

// CheckExplicit is Check for an explicit user request (`outfitter
// self-update --check`): it always contacts GitHub and ignores the
// background-check opt-out, because asking is itself the request to check.
func (u *SelfUpdater) CheckExplicit(ctx context.Context, cat *catalog.Catalog) (SelfState, error) {
	return u.check(ctx, cat, true, true)
}

func (u *SelfUpdater) check(ctx context.Context, cat *catalog.Catalog, force, explicit bool) (SelfState, error) {
	u = u.withDefaults()
	state := u.baseState()
	if state.InstallKind == SelfInstallDevelopment || (!explicit && !u.checksEnabled(cat)) {
		return state, nil
	}
	previous, _ := u.loadRecord()
	if !force && !previous.CheckedAt.IsZero() && u.Now().Sub(previous.CheckedAt) < u.CheckInterval {
		state.CheckedAt = previous.CheckedAt
		state.LastError = previous.LastError
		u.applyLatest(&state, previous.LatestVersion, previous.ReleaseURL)
		return state, nil
	}
	release, err := u.latestRelease(ctx, cat)
	now := u.Now().UTC()
	state.CheckedAt = now
	if err != nil {
		state.LastError = err.Error()
		// A failed refresh must not erase the last known release.
		u.applyLatest(&state, previous.LatestVersion, previous.ReleaseURL)
		_ = u.saveRecord(selfCheckRecord{
			LatestVersion: previous.LatestVersion,
			ReleaseURL:    previous.ReleaseURL,
			CheckedAt:     now,
			LastError:     state.LastError,
		})
		return state, err
	}
	u.applyLatest(&state, release.TagName, release.HTMLURL)
	_ = u.saveRecord(selfCheckRecord{
		LatestVersion: state.LatestVersion,
		ReleaseURL:    state.ReleaseURL,
		CheckedAt:     now,
	})
	return state, nil
}

func (u *SelfUpdater) loadRecord() (selfCheckRecord, error) {
	path := u.cachePath()
	if path == "" {
		return selfCheckRecord{}, os.ErrNotExist
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return selfCheckRecord{}, err
	}
	var record selfCheckRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return selfCheckRecord{}, fmt.Errorf("parse self-check cache %s: %w", path, err)
	}
	return record, nil
}

func (u *SelfUpdater) saveRecord(record selfCheckRecord) error {
	path := u.cachePath()
	if path == "" {
		return errors.New("self-check cache path is empty")
	}
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return fmt.Errorf("create self-check cache directory: %w", err)
	}
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	temporary, err := os.CreateTemp(directory, ".self-check-*.tmp")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer func() { _ = os.Remove(temporaryPath) }()
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Chmod(temporaryPath, 0o644); err != nil {
		return err
	}
	return os.Rename(temporaryPath, path)
}

type selfReleaseAsset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
}

type selfRelease struct {
	TagName    string             `json:"tag_name"`
	HTMLURL    string             `json:"html_url"`
	Draft      bool               `json:"draft"`
	Prerelease bool               `json:"prerelease"`
	Assets     []selfReleaseAsset `json:"assets"`
}

func (r selfRelease) asset(name string) (selfReleaseAsset, bool) {
	for _, asset := range r.Assets {
		if asset.Name == name && asset.URL != "" {
			return asset, true
		}
	}
	return selfReleaseAsset{}, false
}

func (u *SelfUpdater) latestRelease(ctx context.Context, cat *catalog.Catalog) (selfRelease, error) {
	inspector := &Inspector{
		Runner:           u.Runner,
		LookPath:         u.LookPath,
		HomeDir:          u.HomeDir,
		HTTPClient:       u.HTTPClient,
		GitHubAPIBaseURL: u.GitHubAPIBaseURL,
	}
	inspector = inspector.withDefaults()
	token := ""
	if cat != nil {
		token = inspector.githubToken(ctx, cat)
	}
	owner, repo, _ := strings.Cut(SelfRepo, "/")
	endpoint := strings.TrimRight(u.GitHubAPIBaseURL, "/") + "/repos/" +
		url.PathEscape(owner) + "/" + url.PathEscape(repo) + "/releases/latest"
	headers := map[string]string{"Accept": "application/vnd.github+json"}
	if token != "" {
		headers["Authorization"] = "Bearer " + token
	}
	var release selfRelease
	if err := inspector.getJSON(ctx, endpoint, headers, &release); err != nil {
		return selfRelease{}, err
	}
	if release.Draft || release.Prerelease {
		return selfRelease{}, errors.New("GitHub returned a draft or prerelease as latest")
	}
	if NormalizeVersion(release.TagName) == "" {
		return selfRelease{}, fmt.Errorf("GitHub returned no release tag for %s", SelfRepo)
	}
	return release, nil
}

// Update installs the latest release when one is available. An explicit
// update ignores the background-check opt-out and cache interval: invoking it
// is itself the user's request to check now.
func (u *SelfUpdater) Update(ctx context.Context, cat *catalog.Catalog, opts SelfUpdateOptions) (SelfUpdateResult, error) {
	u = u.withDefaults()
	state := u.baseState()
	result := SelfUpdateResult{DryRun: opts.DryRun}
	if state.InstallKind == SelfInstallDevelopment {
		result.State = state
		result.Message = state.ManualHint
		return result, fmt.Errorf("%w: %s", ErrSelfUpdateUnsupported, state.ManualHint)
	}
	release, err := u.latestRelease(ctx, cat)
	now := u.Now().UTC()
	state.CheckedAt = now
	if err != nil {
		state.LastError = err.Error()
		result.State = state
		return result, err
	}
	u.applyLatest(&state, release.TagName, release.HTMLURL)
	_ = u.saveRecord(selfCheckRecord{
		LatestVersion: state.LatestVersion,
		ReleaseURL:    state.ReleaseURL,
		CheckedAt:     now,
	})
	result.State = state
	result.Command = state.UpdateCommand
	if !state.Outdated {
		result.Message = fmt.Sprintf("Outfitter %s is already up to date.", state.CurrentVersion)
		return result, nil
	}
	if !state.CanSelfUpdate {
		hint := state.ManualHint
		if hint == "" {
			hint = "download the latest release from " + state.ReleaseURL
		}
		result.Message = hint
		return result, fmt.Errorf("%w: %s", ErrSelfUpdateUnsupported, hint)
	}
	if opts.DryRun {
		result.Message = fmt.Sprintf("Would update Outfitter %s to %s: %s", state.CurrentVersion, state.LatestVersion, result.Command)
		return result, nil
	}

	switch state.InstallKind {
	case SelfInstallMise:
		err = u.updateViaMise(ctx, state)
	case SelfInstallStandalone:
		result.BackupPath, err = u.updateStandalone(ctx, state, release)
	default:
		err = fmt.Errorf("%w: install kind %q", ErrSelfUpdateUnsupported, state.InstallKind)
	}
	if err != nil {
		return result, err
	}
	result.Updated = true
	result.RequiresRestart = true
	result.Message = fmt.Sprintf("Outfitter updated to %s. Restart Outfitter to run the new version.", state.LatestVersion)
	return result, nil
}

func (u *SelfUpdater) updateViaMise(ctx context.Context, state SelfState) error {
	misePath, found := FindMise(u.LookPath, u.HomeDir)
	if !found {
		return fmt.Errorf("%w: mise was not found", ErrSelfUpdateUnsupported)
	}
	spec := selfMiseSpec + "@" + state.LatestVersion
	if _, err := u.Runner.Run(ctx, misePath, "use", "--global", spec); err != nil {
		return fmt.Errorf("update Outfitter with mise: %w", err)
	}
	out, err := u.Runner.Run(ctx, misePath, "which", "outfitter")
	if err != nil {
		return fmt.Errorf("locate updated Outfitter: %w", err)
	}
	newPath := strings.TrimSpace(string(out))
	if newPath == "" {
		return errors.New("mise did not report an Outfitter binary after updating")
	}
	return u.verifyBinaryVersion(ctx, newPath, state.LatestVersion)
}

func (u *SelfUpdater) updateStandalone(ctx context.Context, state SelfState, release selfRelease) (string, error) {
	assetName := fmt.Sprintf("outfitter_%s_%s_%s", state.LatestVersion, u.GOOS, u.GOARCH)
	binaryAsset, ok := release.asset(assetName)
	if !ok {
		return "", fmt.Errorf("release %s has no asset %q", release.TagName, assetName)
	}
	sumsAsset, ok := release.asset("SHA256SUMS")
	if !ok {
		return "", fmt.Errorf("release %s has no SHA256SUMS asset", release.TagName)
	}
	sums, err := downloadBytes(ctx, u.HTTPClient, sumsAsset.URL, maxChecksumBytes)
	if err != nil {
		return "", fmt.Errorf("download Outfitter checksums: %w", err)
	}
	expected, err := parseChecksum(sums, assetName)
	if err != nil {
		return "", err
	}

	target := state.ResolvedPath
	directory := filepath.Dir(target)
	temporary, err := os.CreateTemp(directory, ".outfitter-update-*")
	if err != nil {
		return "", fmt.Errorf("create temporary Outfitter binary: %w", err)
	}
	temporaryPath := temporary.Name()
	defer func() { _ = os.Remove(temporaryPath) }()

	hasher := sha256.New()
	written, err := downloadTo(ctx, u.HTTPClient, binaryAsset.URL, io.MultiWriter(temporary, hasher), maxBinaryBytes)
	if closeErr := temporary.Close(); err == nil && closeErr != nil {
		err = closeErr
	}
	if err != nil {
		return "", fmt.Errorf("download Outfitter %s: %w", state.LatestVersion, err)
	}
	if written == 0 {
		return "", errors.New("download Outfitter: empty response")
	}
	if actual := hex.EncodeToString(hasher.Sum(nil)); !strings.EqualFold(actual, expected) {
		return "", fmt.Errorf("outfitter binary checksum mismatch: expected %s, got %s", expected, actual)
	}
	if err := os.Chmod(temporaryPath, 0o755); err != nil {
		return "", fmt.Errorf("make downloaded Outfitter executable: %w", err)
	}
	if err := u.verifyBinaryVersion(ctx, temporaryPath, state.LatestVersion); err != nil {
		return "", err
	}

	backupPath := target + ".previous"
	if err := copyExecutable(target, backupPath); err != nil {
		return "", fmt.Errorf("back up current Outfitter binary: %w", err)
	}
	if err := os.Rename(temporaryPath, target); err != nil {
		return backupPath, fmt.Errorf("replace Outfitter binary: %w", err)
	}
	if err := u.verifyBinaryVersion(ctx, target, state.LatestVersion); err != nil {
		if rollbackErr := os.Rename(backupPath, target); rollbackErr == nil {
			return "", fmt.Errorf("verify updated Outfitter (rolled back): %w", err)
		}
		return backupPath, fmt.Errorf("verify updated Outfitter: %w", err)
	}
	return backupPath, nil
}

func (u *SelfUpdater) verifyBinaryVersion(ctx context.Context, path, want string) error {
	out, err := u.Runner.Run(ctx, path, "--version")
	if err != nil {
		return fmt.Errorf("verify Outfitter %s: %w", want, err)
	}
	got := NormalizeVersion(extractVersion(string(out), `([0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?)`))
	if got != NormalizeVersion(want) {
		return fmt.Errorf("verify Outfitter: binary reports version %q, want %q", got, NormalizeVersion(want))
	}
	return nil
}

func parseChecksum(data []byte, asset string) (string, error) {
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		if strings.TrimPrefix(fields[1], "*") == asset {
			if len(fields[0]) != sha256.Size*2 {
				return "", fmt.Errorf("checksum for %s is malformed", asset)
			}
			if _, err := hex.DecodeString(fields[0]); err != nil {
				return "", fmt.Errorf("checksum for %s is malformed: %w", asset, err)
			}
			return fields[0], nil
		}
	}
	return "", fmt.Errorf("checksum for %s not found", asset)
}

func copyExecutable(source, target string) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer func() { _ = input.Close() }()
	temporary, err := os.CreateTemp(filepath.Dir(target), ".outfitter-backup-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer func() { _ = os.Remove(temporaryPath) }()
	if _, err := io.Copy(temporary, input); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Chmod(temporaryPath, 0o755); err != nil {
		return err
	}
	return os.Rename(temporaryPath, target)
}
