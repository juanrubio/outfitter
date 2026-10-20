package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/juanrubio/outfitter/internal/catalog"
	"github.com/juanrubio/outfitter/internal/domain"
)

const (
	defaultConcurrency   = 4
	defaultGitHubAPIURL  = "https://api.github.com"
	defaultNPMRegistry   = "https://registry.npmjs.org"
	defaultPyPIURL       = "https://pypi.org"
	maxUpstreamBodyBytes = 4 << 20
)

// CommandRunner executes an external command and returns its combined output.
type CommandRunner interface {
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
}

// ExecRunner is the production CommandRunner. It never invokes a shell.
type ExecRunner struct{}

// Run executes the command and returns its combined stdout and stderr.
func (ExecRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return out, fmt.Errorf("run %s %s: %w", name, strings.Join(args, " "), err)
	}
	return out, nil
}

// Inspector discovers local state and checks upstream registries. Every side
// effect is injectable so tests never depend on the host environment.
type Inspector struct {
	Runner             CommandRunner
	LookPath           func(string) (string, error)
	ReadLink           func(string) (string, error)
	ReadFile           func(string) ([]byte, error)
	Stat               func(string) (os.FileInfo, error)
	HomeDir            string
	HTTPClient         *http.Client
	GitHubAPIBaseURL   string
	NPMRegistryBaseURL string
	PyPIBaseURL        string
	Concurrency        int
}

// InspectorOptions is an alias for Inspector, provided for callers that prefer
// an options-style constructor name.
type InspectorOptions = Inspector

// NewInspector returns an Inspector with production defaults and the supplied
// home directory.
func NewInspector(homeDir string) *Inspector {
	return (&Inspector{HomeDir: homeDir}).withDefaults()
}

// InspectLocal discovers local installations using production defaults.
func InspectLocal(ctx context.Context, cat *catalog.Catalog) map[string]domain.ToolState {
	return NewInspector("").InspectLocal(ctx, cat)
}

// InspectUpstream checks upstream versions using production defaults.
func InspectUpstream(
	ctx context.Context,
	cat *catalog.Catalog,
	current map[string]domain.ToolState,
	onUpdate func(toolID string, state domain.ToolState),
) map[string]domain.ToolState {
	return NewInspector("").InspectUpstream(ctx, cat, current, onUpdate)
}

func (i *Inspector) withDefaults() *Inspector {
	copy := &Inspector{}
	if i != nil {
		*copy = *i
	}
	if copy.Runner == nil {
		copy.Runner = ExecRunner{}
	}
	if copy.LookPath == nil {
		copy.LookPath = exec.LookPath
	}
	if copy.ReadLink == nil {
		copy.ReadLink = os.Readlink
	}
	if copy.ReadFile == nil {
		copy.ReadFile = os.ReadFile
	}
	if copy.Stat == nil {
		copy.Stat = os.Stat
	}
	if copy.HomeDir == "" {
		if home, err := os.UserHomeDir(); err == nil {
			copy.HomeDir = home
		}
	}
	if copy.HTTPClient == nil {
		copy.HTTPClient = &http.Client{Timeout: 30 * time.Second}
	}
	if copy.GitHubAPIBaseURL == "" {
		copy.GitHubAPIBaseURL = defaultGitHubAPIURL
	}
	if copy.NPMRegistryBaseURL == "" {
		copy.NPMRegistryBaseURL = defaultNPMRegistry
	}
	if copy.PyPIBaseURL == "" {
		copy.PyPIBaseURL = defaultPyPIURL
	}
	if copy.Concurrency <= 0 {
		copy.Concurrency = defaultConcurrency
	}
	return copy
}

// InspectLocal performs a fast local scan and returns one state per catalog
// tool. CheckStatus is Unknown until InspectUpstream completes.
func (i *Inspector) InspectLocal(ctx context.Context, cat *catalog.Catalog) map[string]domain.ToolState {
	i = i.withDefaults()
	states := make(map[string]domain.ToolState)
	if cat == nil {
		return states
	}

	misePath, miseFound := FindMise(i.LookPath, i.HomeDir)
	installedByMise := make(map[string]miseInstalled)
	if miseFound {
		installedByMise = i.readMiseInstalled(ctx, misePath)
	}

	for _, tool := range cat.Tools {
		states[tool.ID] = i.inspectTool(ctx, tool, installedByMise)
	}
	return states
}

func (i *Inspector) inspectTool(
	ctx context.Context,
	tool domain.Tool,
	installedByMise map[string]miseInstalled,
) domain.ToolState {
	state := domain.ToolState{
		Provenance:       domain.ProvenanceNone,
		ProvenanceDetail: "Not installed",
		CheckStatus:      domain.StatusUnknown,
	}

	binDir := filepath.Join(i.HomeDir, ".local", "bin")
	musePath := filepath.Join(binDir, tool.Binary)
	museInstall := false
	if tool.Backend == domain.BackendVendor && tool.Binary == "muse" && i.HomeDir != "" {
		if info, err := i.Stat(musePath); err == nil && info.Mode().IsRegular() {
			state.BinaryPath = musePath
			museInstall = true
		}
	}

	foundOnPath := false
	if state.BinaryPath == "" {
		if resolved, err := i.LookPath(tool.Binary); err == nil && strings.TrimSpace(resolved) != "" {
			state.BinaryPath = resolved
			foundOnPath = true
		} else if i.HomeDir != "" {
			candidate := filepath.Join(binDir, tool.Binary)
			if info, err := i.Stat(candidate); err == nil && info.Mode().IsRegular() {
				state.BinaryPath = candidate
			}
		}
	}

	entry, miseManaged := lookupMise(installedByMise, tool)
	if miseManaged {
		if state.BinaryPath == "" {
			state.BinaryPath = entry.InstallPath
		}
	}

	if state.BinaryPath == "" && !miseManaged {
		return state
	}

	var installedVersion string
	if miseManaged {
		installedVersion = entry.Version
	}
	if museInstall {
		if version := i.readMuseVersion(); version != "" {
			installedVersion = version
		}
	}

	resolvedPath := i.resolveSymlink(state.BinaryPath)
	provenance, detail := i.classify(tool, state.BinaryPath, resolvedPath, miseManaged, museInstall)
	state.Provenance = provenance
	state.ProvenanceDetail = detail

	if provenance == domain.ProvenanceAPT {
		// classify only infers APT from the catalog backend or a system
		// binary path. Require dpkg to actually own the package: a stray
		// or self-built binary in /usr/bin or /usr/local/bin is an
		// unmanaged external install that apt cannot update in place.
		if version, err := i.dpkgVersion(ctx, tool.Package); err == nil {
			if installedVersion == "" {
				installedVersion = version
			}
		} else {
			state.Provenance = domain.ProvenanceExternal
			state.ProvenanceDetail = fmt.Sprintf(
				"External installation (%s)", displayPath(i.HomeDir, state.BinaryPath))
		}
	}
	if installedVersion == "" {
		installedVersion = i.binaryVersion(ctx, tool, state.BinaryPath)
	}
	state.InstalledVersion = NormalizeVersion(installedVersion)
	i.detectShadow(ctx, tool, &state, foundOnPath, miseManaged, resolvedPath)
	return state
}

func (i *Inspector) detectShadow(ctx context.Context, tool domain.Tool, state *domain.ToolState, foundOnPath, miseManaged bool, resolvedPath string) {
	if !foundOnPath || state.InstalledVersion == "" || (!miseManaged && state.Provenance != domain.ProvenanceAPT) {
		return
	}
	if miseManaged {
		binarySlash := filepath.ToSlash(state.BinaryPath)
		resolvedSlash := filepath.ToSlash(resolvedPath)
		if strings.Contains(binarySlash, "mise/shims/") || strings.Contains(resolvedSlash, "mise/installs/") || strings.Contains(resolvedSlash, "mise/shims/") {
			return
		}
	}
	if state.Provenance == domain.ProvenanceAPT {
		switch filepath.Dir(state.BinaryPath) {
		case "/usr/bin", "/bin", "/usr/sbin", "/sbin":
			return
		}
	}
	probe := NormalizeVersion(i.binaryVersion(ctx, tool, state.BinaryPath))
	if probe != "" && probe != state.InstalledVersion {
		state.ShadowedBy = state.BinaryPath
		state.ShadowVersion = probe
	}
}

func (i *Inspector) classify(
	tool domain.Tool,
	binaryPath, resolvedPath string,
	miseManaged, museInstall bool,
) (domain.Provenance, string) {
	binDir := filepath.Join(i.HomeDir, ".local", "bin")
	miseDetail := fmt.Sprintf("Managed by mise (%s:%s)", tool.Backend, cleanPackage(tool.Package))

	if museInstall {
		return domain.ProvenanceVendor, "Vendor OCI launcher + .muse-version"
	}
	if miseManaged {
		return domain.ProvenanceMise, miseDetail
	}

	normalized := filepath.ToSlash(resolvedPath)
	switch {
	case strings.Contains(normalized, "mise/installs/") || strings.Contains(normalized, "mise/shims/"):
		return domain.ProvenanceMise, miseDetail
	case strings.Contains(normalized, "uv/tools/") || strings.Contains(normalized, "/.local/share/uv/tools"):
		return domain.ProvenanceUV, fmt.Sprintf("Installed by uv (%s)", displayPath(i.HomeDir, resolvedPath))
	case strings.Contains(normalized, "/.local/share/outfitter/"):
		return domain.ProvenanceVendor, fmt.Sprintf(
			"Vendor bundle (%s)", displayPath(i.HomeDir, resolvedPath))
	}

	if tool.Backend == domain.BackendVendor && pathWithin(binaryPath, binDir) {
		return domain.ProvenanceVendor, fmt.Sprintf(
			"Vendor binary (%s)", displayPath(i.HomeDir, binaryPath))
	}
	if tool.Backend == domain.BackendAPT || isSystemPath(binaryPath) {
		return domain.ProvenanceAPT, fmt.Sprintf("APT package (%s)", tool.Package)
	}
	if tool.Backend == domain.BackendSource && pathWithin(binaryPath, binDir) {
		return domain.ProvenanceSource, fmt.Sprintf(
			"Local source build (%s)", displayPath(i.HomeDir, binaryPath))
	}
	return domain.ProvenanceExternal, fmt.Sprintf(
		"External installation (%s)", displayPath(i.HomeDir, binaryPath))
}

func (i *Inspector) readMuseVersion() string {
	path := filepath.Join(i.HomeDir, ".local", "bin", ".muse-version")
	data, err := i.ReadFile(path)
	if err != nil {
		return ""
	}
	content := strings.TrimSpace(string(data))
	if strings.HasPrefix(content, "{") || strings.HasPrefix(content, "[") {
		var payload any
		if err := json.Unmarshal([]byte(content), &payload); err == nil {
			return NormalizeVersion(findVersionString(payload))
		}
	}
	if line, _, ok := strings.Cut(content, "\n"); ok {
		content = line
	}
	return NormalizeVersion(content)
}

func (i *Inspector) dpkgVersion(ctx context.Context, pkg string) (string, error) {
	pkg = cleanPackage(pkg)
	if pkg == "" {
		return "", errors.New("apt package name is empty")
	}
	out, err := i.Runner.Run(ctx, "dpkg-query", "-W", "-f=${Version}", pkg)
	if err != nil {
		return "", err
	}
	version := NormalizeVersion(string(out))
	if version == "" {
		return "", fmt.Errorf("dpkg-query returned no version for %s", pkg)
	}
	return version, nil
}

func (i *Inspector) binaryVersion(ctx context.Context, tool domain.Tool, binaryPath string) string {
	if binaryPath == "" {
		return ""
	}
	args := tool.VersionArgs
	if len(args) == 0 {
		args = []string{"--version"}
	}
	out, err := i.Runner.Run(ctx, binaryPath, args...)
	if err != nil && len(bytes.TrimSpace(out)) == 0 {
		return ""
	}
	return extractVersion(string(out), tool.VersionRegex)
}

func (i *Inspector) resolveSymlink(path string) string {
	if path == "" {
		return ""
	}
	current := path
	for depth := 0; depth < 8; depth++ {
		target, err := i.ReadLink(current)
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

type miseEntry struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	InstallPath string `json:"install_path"`
	Installed   *bool  `json:"installed"`
	Active      *bool  `json:"active"`
}

type miseInstalled struct {
	Version     string
	InstallPath string
	Active      bool
}

func (i *Inspector) readMiseInstalled(ctx context.Context, misePath string) map[string]miseInstalled {
	index := make(map[string]miseInstalled)
	out, err := i.Runner.Run(ctx, misePath, "ls", "--installed", "--json")
	if err != nil || len(bytes.TrimSpace(out)) == 0 {
		return index
	}

	var grouped map[string][]miseEntry
	if err := json.Unmarshal(out, &grouped); err == nil {
		for key, entries := range grouped {
			addMiseEntries(index, key, entries)
		}
		return index
	}

	var single map[string]miseEntry
	if err := json.Unmarshal(out, &single); err == nil {
		for key, entry := range single {
			addMiseEntries(index, key, []miseEntry{entry})
		}
		return index
	}

	var list []miseEntry
	if err := json.Unmarshal(out, &list); err == nil {
		for _, entry := range list {
			addMiseEntries(index, entry.Name, []miseEntry{entry})
		}
	}
	return index
}

func addMiseEntries(index map[string]miseInstalled, groupKey string, entries []miseEntry) {
	for _, entry := range entries {
		if entry.Installed != nil && !*entry.Installed {
			continue
		}
		version := NormalizeVersion(entry.Version)
		if version == "" {
			continue
		}
		installed := miseInstalled{
			Version:     version,
			InstallPath: entry.InstallPath,
			Active:      entry.Active != nil && *entry.Active,
		}
		key := entry.Name
		if key == "" {
			key = groupKey
		}
		addMiseIndex(index, key, installed)
		if entry.Name != "" && groupKey != "" {
			addMiseIndex(index, groupKey, installed)
		}
	}
}

func addMiseIndex(index map[string]miseInstalled, key string, installed miseInstalled) {
	for _, candidate := range miseKeyVariants(key) {
		if previous, ok := index[candidate]; ok {
			if previous.Active || !installed.Active {
				continue
			}
		}
		index[candidate] = installed
	}
}

func lookupMise(index map[string]miseInstalled, tool domain.Tool) (miseInstalled, bool) {
	for _, key := range miseLookupKeys(tool) {
		if installed, ok := index[key]; ok {
			return installed, true
		}
	}
	return miseInstalled{}, false
}

func miseLookupKeys(tool domain.Tool) []string {
	pkg := cleanPackage(tool.Package)
	candidates := []string{tool.Binary}
	if pkg != "" {
		candidates = append(candidates, pkg, string(tool.Backend)+":"+pkg)
	}
	// Only the tool's own backend spec counts as managed. Cross-matching
	// aqua/ubi/github installs would mask a broken or wrong-platform
	// artifact installed by a different backend (e.g. ubi picking the
	// macOS binary out of broot's multi-platform zip) behind this
	// backend's version.
	switch tool.Backend {
	case domain.BackendMiseAqua:
		candidates = append(candidates, "aqua:"+pkg)
	case domain.BackendMiseUbi:
		candidates = append(candidates, "ubi:"+pkg)
	case domain.BackendMiseGitHub:
		candidates = append(candidates, "github:"+pkg)
	case domain.BackendMiseNPM:
		candidates = append(candidates, "npm:"+pkg)
	case domain.BackendUV:
		candidates = append(candidates, "pipx:"+pkg, "uv:"+pkg)
	}

	seen := make(map[string]struct{}, len(candidates)*2)
	keys := make([]string, 0, len(candidates)*2)
	for _, candidate := range candidates {
		for _, variant := range miseKeyVariants(candidate) {
			if variant == "" {
				continue
			}
			if _, ok := seen[variant]; ok {
				continue
			}
			seen[variant] = struct{}{}
			keys = append(keys, variant)
		}
	}
	return keys
}

func miseKeyVariants(key string) []string {
	key = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(key, "./")))
	if key == "" {
		return nil
	}
	variants := []string{key}
	for _, prefix := range []string{"aqua:", "ubi:", "github:", "npm:", "pipx:", "uv:"} {
		if strings.HasPrefix(key, prefix) {
			trimmed := strings.TrimPrefix(key, prefix)
			if trimmed != "" && trimmed != key {
				variants = append(variants, trimmed)
			}
			break
		}
	}
	return variants
}

// InspectUpstream resolves LatestVersion for every catalog tool with a bounded
// worker pool. onUpdate, when non-nil, is invoked once per tool from a single
// goroutine as results arrive.
func (i *Inspector) InspectUpstream(
	ctx context.Context,
	cat *catalog.Catalog,
	current map[string]domain.ToolState,
	onUpdate func(toolID string, state domain.ToolState),
) map[string]domain.ToolState {
	i = i.withDefaults()
	states := cloneStates(current)
	if cat == nil {
		return states
	}

	// Workers only read the snapshot; the main goroutine owns result writes.
	snapshot := cloneStates(current)
	misePath, miseFound := FindMise(i.LookPath, i.HomeDir)
	token := i.githubToken(ctx, cat)

	type job struct {
		tool domain.Tool
	}
	type result struct {
		toolID string
		state  domain.ToolState
	}

	workers := i.Concurrency
	if workers > len(cat.Tools) {
		workers = len(cat.Tools)
	}
	jobs := make(chan job)
	results := make(chan result)
	var wait sync.WaitGroup

	for worker := 0; worker < workers; worker++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			for work := range jobs {
				state := snapshot[work.tool.ID]
				state = i.applyUpstream(ctx, work.tool, state, misePath, miseFound, token)
				results <- result{toolID: work.tool.ID, state: state}
			}
		}()
	}

	go func() {
		for _, tool := range cat.Tools {
			jobs <- job{tool: tool}
		}
		close(jobs)
		wait.Wait()
		close(results)
	}()

	for item := range results {
		states[item.toolID] = item.state
		if onUpdate != nil {
			onUpdate(item.toolID, item.state)
		}
	}
	return states
}

func (i *Inspector) applyUpstream(
	ctx context.Context,
	tool domain.Tool,
	state domain.ToolState,
	misePath string,
	miseFound bool,
	token string,
) domain.ToolState {
	latest, err := i.latestVersion(ctx, tool, misePath, miseFound, token)
	switch {
	case err == nil && latest != "":
		normalized := NormalizeVersion(latest)
		if (tool.Backend == domain.BackendVendor || tool.Backend == domain.BackendSource) &&
			state.InstalledVersion != "" &&
			domain.CompareVersions(normalized, state.InstalledVersion) < 0 {
			normalized = state.InstalledVersion
		}
		state.LatestVersion = normalized
		state.CheckStatus = domain.StatusReady
		state.LastError = ""
	case err == nil || tool.Backend == domain.BackendVendor || tool.Backend == domain.BackendSource:
		// Vendor and source installs have no universal upstream. Keep whatever
		// was already known and never surface a false error.
		if state.LatestVersion == "" && state.InstalledVersion != "" {
			state.LatestVersion = state.InstalledVersion
		}
		state.CheckStatus = domain.StatusReady
		state.LastError = ""
	default:
		state.CheckStatus = domain.StatusError
		state.LastError = err.Error()
	}
	return state
}

func (i *Inspector) latestVersion(
	ctx context.Context,
	tool domain.Tool,
	misePath string,
	miseFound bool,
	token string,
) (string, error) {
	var miseErr error
	if miseFound {
		if spec := miseSpec(tool); spec != "" {
			out, err := i.Runner.Run(ctx, misePath, "latest", spec)
			if err != nil {
				miseErr = err
			} else if version := parseLatestOutput(out); version != "" {
				return version, nil
			}
		}
	}

	var latest string
	var err error
	switch tool.Backend {
	case domain.BackendMiseNPM:
		latest, err = i.npmLatest(ctx, tool.Package)
	case domain.BackendMiseAqua, domain.BackendMiseUbi:
		latest, err = i.githubLatest(ctx, tool.Package, token)
	case domain.BackendMiseGitHub:
		// Registry short names have no owner/repo fallback; mise latest
		// above is their only upstream source.
		if _, _, ok := githubRepo(tool.Package); ok {
			latest, err = i.githubLatest(ctx, tool.Package, token)
		}
	case domain.BackendUV:
		latest, err = i.pypiLatest(ctx, tool.Package)
	case domain.BackendAPT:
		latest, err = i.aptLatest(ctx, tool.Package)
	case domain.BackendVendor:
		latest, err = i.vendorLatest(ctx, tool, token)
	case domain.BackendSource:
		return "", errNoUpstream
	default:
		return "", fmt.Errorf("unsupported backend %q", tool.Backend)
	}

	if err != nil {
		if miseErr != nil {
			return "", fmt.Errorf("mise latest failed (%v) and %s fallback failed: %w", miseErr, tool.Backend, err)
		}
		return "", err
	}
	if latest == "" && miseErr != nil {
		return "", miseErr
	}
	return latest, nil
}

var errNoUpstream = errors.New("no upstream version source")

func miseSpec(tool domain.Tool) string {
	pkg := strings.TrimPrefix(cleanPackage(tool.Package), string(tool.Backend)+":")
	if pkg == "" {
		return ""
	}
	switch tool.Backend {
	case domain.BackendMiseAqua:
		return "aqua:" + pkg
	case domain.BackendMiseUbi:
		return "ubi:" + pkg
	case domain.BackendMiseGitHub:
		// A package without a slash is a mise registry short name: the
		// registry carries per-platform options (e.g. broot's bin_path
		// template) that a full github: spec cannot express.
		if !strings.Contains(pkg, "/") {
			return pkg
		}
		return "github:" + pkg
	case domain.BackendMiseNPM:
		return "npm:" + pkg
	case domain.BackendUV:
		return "pipx:" + pkg
	default:
		return ""
	}
}

func (i *Inspector) npmLatest(ctx context.Context, pkg string) (string, error) {
	pkg = strings.TrimPrefix(cleanPackage(pkg), "npm:")
	if pkg == "" {
		return "", errors.New("npm package name is empty")
	}
	encoded := strings.ReplaceAll(url.PathEscape(pkg), "%40", "@")
	endpoint := strings.TrimRight(i.NPMRegistryBaseURL, "/") + "/" + encoded + "/latest"
	var payload struct {
		Version string `json:"version"`
	}
	if err := i.getJSON(ctx, endpoint, nil, &payload); err != nil {
		return "", err
	}
	if NormalizeVersion(payload.Version) == "" {
		return "", fmt.Errorf("npm registry returned no version for %s", pkg)
	}
	return payload.Version, nil
}

func (i *Inspector) pypiLatest(ctx context.Context, pkg string) (string, error) {
	pkg = cleanPackage(pkg)
	if pkg == "" {
		return "", errors.New("PyPI package name is empty")
	}
	endpoint := strings.TrimRight(i.PyPIBaseURL, "/") + "/pypi/" + url.PathEscape(pkg) + "/json"
	var payload struct {
		Info struct {
			Version string `json:"version"`
		} `json:"info"`
	}
	if err := i.getJSON(ctx, endpoint, nil, &payload); err != nil {
		return "", err
	}
	if NormalizeVersion(payload.Info.Version) == "" {
		return "", fmt.Errorf("PyPI returned no version for %s", pkg)
	}
	return payload.Info.Version, nil
}

func (i *Inspector) githubLatest(ctx context.Context, pkg, token string) (string, error) {
	owner, repo, ok := githubRepo(pkg)
	if !ok {
		return "", fmt.Errorf("package %q is not a GitHub owner/repo pair", pkg)
	}
	endpoint := strings.TrimRight(i.GitHubAPIBaseURL, "/") + "/repos/" +
		url.PathEscape(owner) + "/" + url.PathEscape(repo) + "/releases/latest"
	headers := map[string]string{"Accept": "application/vnd.github+json"}
	if token != "" {
		headers["Authorization"] = "Bearer " + token
	}
	var payload struct {
		TagName string `json:"tag_name"`
	}
	if err := i.getJSON(ctx, endpoint, headers, &payload); err != nil {
		return "", err
	}
	if NormalizeVersion(payload.TagName) == "" {
		return "", fmt.Errorf("GitHub returned no release tag for %s/%s", owner, repo)
	}
	return payload.TagName, nil
}

func (i *Inspector) aptLatest(ctx context.Context, pkg string) (string, error) {
	pkg = cleanPackage(pkg)
	if pkg == "" {
		return "", errors.New("apt package name is empty")
	}
	out, err := i.Runner.Run(ctx, "apt-cache", "policy", pkg)
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(out), "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "Candidate:") {
			continue
		}
		candidate := strings.TrimSpace(strings.TrimPrefix(trimmed, "Candidate:"))
		if candidate == "" || candidate == "(none)" {
			return "", fmt.Errorf("apt-cache reports no candidate for %s", pkg)
		}
		return NormalizeVersion(candidate), nil
	}
	return "", fmt.Errorf("apt-cache policy returned no candidate for %s", pkg)
}

func (i *Inspector) vendorLatest(ctx context.Context, tool domain.Tool, token string) (string, error) {
	if tool.Binary == "muse" {
		if version := i.museReleaseVersion(); version != "" {
			return version, nil
		}
	}
	if tool.Binary == "codexbar" {
		return i.githubLatest(ctx, "steipete/CodexBar", token)
	}
	if _, _, ok := githubRepo(tool.Package); ok {
		return i.githubLatest(ctx, tool.Package, token)
	}
	return "", nil
}

func (i *Inspector) museReleaseVersion() string {
	data, err := i.ReadFile(filepath.Join(i.HomeDir, ".local", "bin", ".muse-release-info.json"))
	if err != nil {
		return ""
	}
	var payload any
	if err := json.Unmarshal(data, &payload); err != nil {
		return ""
	}
	return NormalizeVersion(findVersionString(payload))
}

func (i *Inspector) githubToken(ctx context.Context, cat *catalog.Catalog) string {
	if cat == nil || !cat.Settings.GitHubTokenFromGH {
		return ""
	}
	if _, err := i.LookPath("gh"); err != nil {
		return ""
	}
	out, err := i.Runner.Run(ctx, "gh", "auth", "token")
	if err != nil {
		return ""
	}
	fields := strings.Fields(string(out))
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

func (i *Inspector) getJSON(ctx context.Context, rawURL string, headers map[string]string, dest any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, http.NoBody)
	if err != nil {
		return fmt.Errorf("build request for %s: %w", rawURL, err)
	}
	req.Header.Set("User-Agent", "outfitter")
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	resp, err := i.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("request %s: %w", rawURL, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		snippet := strings.TrimSpace(string(body))
		if snippet != "" {
			return fmt.Errorf("request %s: unexpected status %s: %s", rawURL, resp.Status, snippet)
		}
		return fmt.Errorf("request %s: unexpected status %s", rawURL, resp.Status)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxUpstreamBodyBytes+1))
	if err != nil {
		return fmt.Errorf("read %s: %w", rawURL, err)
	}
	if len(body) > maxUpstreamBodyBytes {
		return fmt.Errorf("response from %s exceeds %d bytes", rawURL, maxUpstreamBodyBytes)
	}
	if err := json.Unmarshal(body, dest); err != nil {
		return fmt.Errorf("decode %s: %w", rawURL, err)
	}
	return nil
}

var (
	defaultVersionRe  = regexp.MustCompile(`v?(\d+\.\d+(?:\.\d+)?(?:-[0-9A-Za-z.-]+)?)`)
	namedTagPrefixRe  = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_.-]*?-v?(\d+\.\d+.*)$`)
	debianRevisionRe  = regexp.MustCompile(`^(.+)-([0-9][0-9A-Za-z.+~]*)$`)
	digitPrefixSuffix = regexp.MustCompile(`^[0-9]+$`)
)

// NormalizeVersion removes a leading v, package-name tag prefixes (such as
// "jq-1.8.2"), Debian epochs, and Debian revision suffixes so installed and
// upstream versions compare consistently.
func NormalizeVersion(version string) string {
	normalized := strings.TrimSpace(version)
	normalized = strings.Trim(normalized, `"'`)
	normalized = strings.TrimSpace(normalized)
	if match := namedTagPrefixRe.FindStringSubmatch(normalized); match != nil {
		normalized = match[1]
	}
	if len(normalized) > 1 && (normalized[0] == 'v' || normalized[0] == 'V') &&
		normalized[1] >= '0' && normalized[1] <= '9' {
		normalized = normalized[1:]
	}
	if index := strings.Index(normalized, ":"); index > 0 &&
		digitPrefixSuffix.MatchString(normalized[:index]) {
		normalized = normalized[index+1:]
	}
	if match := debianRevisionRe.FindStringSubmatch(normalized); match != nil {
		normalized = match[1]
	}
	return strings.TrimSpace(normalized)
}

func extractVersion(output, pattern string) string {
	re := defaultVersionRe
	if strings.TrimSpace(pattern) != "" {
		if compiled, err := regexp.Compile(pattern); err == nil {
			re = compiled
		}
	}
	match := re.FindStringSubmatch(output)
	if match == nil {
		return ""
	}
	for _, capture := range match[1:] {
		if strings.TrimSpace(capture) != "" {
			return NormalizeVersion(capture)
		}
	}
	return NormalizeVersion(match[0])
}

func parseLatestOutput(out []byte) string {
	text := strings.TrimSpace(string(out))
	if text == "" {
		return ""
	}
	// Prefer a line that is exactly a version so warnings that mention other
	// versions cannot mask the real answer.
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if match := defaultVersionRe.FindString(trimmed); match != "" &&
			NormalizeVersion(match) == NormalizeVersion(trimmed) {
			return NormalizeVersion(match)
		}
	}
	if version := extractVersion(text, ""); version != "" {
		return version
	}
	line, _, _ := strings.Cut(text, "\n")
	return NormalizeVersion(line)
}

func cleanPackage(pkg string) string {
	pkg = strings.TrimSpace(pkg)
	if index := strings.Index(pkg, "["); index >= 0 {
		pkg = pkg[:index]
	}
	if index := strings.Index(pkg, " ("); index >= 0 {
		pkg = pkg[:index]
	}
	if strings.HasSuffix(pkg, ")") {
		if index := strings.Index(pkg, "("); index > 0 {
			pkg = pkg[:index]
		}
	}
	for _, prefix := range []string{"aqua:", "ubi:", "github:", "npm:", "pipx:", "uv:"} {
		if strings.HasPrefix(pkg, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(pkg, prefix))
		}
	}
	return strings.TrimSpace(pkg)
}

func githubRepo(pkg string) (string, string, bool) {
	cleaned := cleanPackage(pkg)
	if cleaned == "" || strings.ContainsAny(cleaned, " \t:()[]") {
		return "", "", false
	}
	parts := strings.Split(cleaned, "/")
	if len(parts) != 2 {
		return "", "", false
	}
	owner, repo := parts[0], parts[1]
	if owner == "" || repo == "" || strings.Contains(owner, ".") {
		return "", "", false
	}
	return owner, repo, true
}

func findVersionString(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case map[string]any:
		for _, key := range []string{
			"latest_version", "latestVersion", "version", "latest", "tag_name", "tag",
		} {
			if candidate, ok := typed[key]; ok {
				if found := findVersionString(candidate); found != "" {
					return found
				}
			}
		}
		for _, candidate := range typed {
			if found := findVersionString(candidate); found != "" {
				return found
			}
		}
	case []any:
		for _, candidate := range typed {
			if found := findVersionString(candidate); found != "" {
				return found
			}
		}
	}
	return ""
}

func cloneStates(states map[string]domain.ToolState) map[string]domain.ToolState {
	cloned := make(map[string]domain.ToolState, len(states))
	for id, state := range states {
		cloned[id] = state
	}
	return cloned
}

func displayPath(homeDir, value string) string {
	if homeDir == "" || value == "" {
		return value
	}
	home := filepath.Clean(homeDir)
	cleaned := filepath.Clean(value)
	if cleaned == home {
		return "~"
	}
	if strings.HasPrefix(cleaned, home+string(filepath.Separator)) {
		return "~" + strings.TrimPrefix(cleaned, home)
	}
	return value
}

func pathWithin(value, dir string) bool {
	if value == "" || dir == "" {
		return false
	}
	cleaned := filepath.Clean(value)
	base := filepath.Clean(dir)
	return cleaned == base || strings.HasPrefix(cleaned, base+string(filepath.Separator))
}

func isSystemPath(value string) bool {
	cleaned := filepath.Clean(value)
	return strings.HasPrefix(cleaned, "/usr/bin/") || strings.HasPrefix(cleaned, "/bin/")
}
