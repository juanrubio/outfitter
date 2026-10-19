// Package domain contains Outfitter's backend-agnostic catalog, state, and
// action-plan values. It has no dependency on the terminal, CLI, or execution
// adapters.
package domain

// BackendType identifies how Outfitter installs and updates a tool.
type BackendType string

// Supported backend types.
const (
	BackendMiseAqua   BackendType = "mise:aqua"
	BackendMiseUbi    BackendType = "mise:ubi"
	BackendMiseGitHub BackendType = "mise:github"
	BackendMiseNPM    BackendType = "mise:npm"
	BackendUV         BackendType = "uv"
	BackendAPT        BackendType = "apt"
	BackendVendor     BackendType = "vendor"
	BackendSource     BackendType = "source"
)

// IsValid reports whether the backend is one of the supported identifiers.
func (b BackendType) IsValid() bool {
	switch b {
	case BackendMiseAqua, BackendMiseUbi, BackendMiseGitHub, BackendMiseNPM,
		BackendUV, BackendAPT, BackendVendor, BackendSource:
		return true
	default:
		return false
	}
}

// RequiresMise reports whether the backend is executed through mise. The uv
// backend is included because Outfitter invokes uv through `mise x uv --`.
func (b BackendType) RequiresMise() bool {
	switch b {
	case BackendMiseAqua, BackendMiseUbi, BackendMiseGitHub, BackendMiseNPM, BackendUV:
		return true
	default:
		return false
	}
}

// Provenance identifies the installation channel that owns a discovered
// binary.
type Provenance string

// Supported provenance values.
const (
	ProvenanceMise     Provenance = "mise"
	ProvenanceAPT      Provenance = "apt"
	ProvenanceVendor   Provenance = "vendor"
	ProvenanceUV       Provenance = "uv"
	ProvenanceSource   Provenance = "source"
	ProvenanceExternal Provenance = "external"
	ProvenanceNone     Provenance = "none"
)

// CheckStatus describes the state of an asynchronous version check.
type CheckStatus string

// Supported check states.
const (
	StatusUnknown  CheckStatus = "unknown"
	StatusChecking CheckStatus = "checking"
	StatusReady    CheckStatus = "ready"
	StatusError    CheckStatus = "error"
)

// Category is a numbered group of related catalog tools.
type Category struct {
	ID    string `json:"id" toml:"id"`
	Num   int    `json:"num" toml:"num"`
	Title string `json:"title" toml:"title"`
}

// Profile is a named bundle of catalog tool IDs.
type Profile struct {
	ID          string   `json:"id" toml:"id"`
	Name        string   `json:"name" toml:"name"`
	Short       string   `json:"short" toml:"short"`
	Description string   `json:"description" toml:"description"`
	Tools       []string `json:"tools" toml:"tools"`
}

// Default settings values.
const (
	DefaultInstallDir = "~/.local/bin"
)

// Settings contains user-configurable catalog defaults.
type Settings struct {
	InstallDir        string `json:"install_dir" toml:"install_dir"`
	AutoBootstrapMise bool   `json:"auto_bootstrap_mise" toml:"auto_bootstrap_mise"`
	GitHubTokenFromGH bool   `json:"github_token_from_gh" toml:"github_token_from_gh"`
}

// DefaultSettings returns Outfitter's built-in settings defaults.
func DefaultSettings() Settings {
	return Settings{
		InstallDir:        DefaultInstallDir,
		AutoBootstrapMise: true,
		GitHubTokenFromGH: true,
	}
}

// Tool describes one catalog entry.
type Tool struct {
	ID           string      `json:"id" toml:"id"`
	Name         string      `json:"name" toml:"name"`
	Binary       string      `json:"binary" toml:"binary"`
	CategoryID   string      `json:"category" toml:"category"`
	Backend      BackendType `json:"backend" toml:"backend"`
	Package      string      `json:"package" toml:"package"`
	Sudo         bool        `json:"sudo" toml:"sudo"`
	Profiles     []string    `json:"profiles" toml:"profiles"`
	Description  string      `json:"description" toml:"description"`
	InstallCmd   string      `json:"install_cmd" toml:"install_cmd"`
	UpdateCmd    string      `json:"update_cmd" toml:"update_cmd"`
	RemoveCmd    string      `json:"remove_cmd" toml:"remove_cmd"`
	SourceDir    string      `json:"source_dir" toml:"source_dir"`
	BuildCmd     string      `json:"build_cmd" toml:"build_cmd"`
	VersionArgs  []string    `json:"version_args" toml:"version_args"`
	VersionRegex string      `json:"version_regex" toml:"version_regex"`
	Disabled     bool        `json:"disabled" toml:"disabled"`
}

// ToolState is the observed installation and upstream state for a tool.
type ToolState struct {
	InstalledVersion string      `json:"installed_version"`
	LatestVersion    string      `json:"latest_version"`
	BinaryPath       string      `json:"binary_path"`
	Provenance       Provenance  `json:"provenance"`
	ProvenanceDetail string      `json:"provenance_detail"`
	CheckStatus      CheckStatus `json:"check_status"`
	LastError        string      `json:"last_error"`
}

// IsInstalled reports whether a version was discovered on disk.
func (s ToolState) IsInstalled() bool {
	return s.InstalledVersion != ""
}

// IsOutdated reports whether both versions are known and differ.
func (s ToolState) IsOutdated() bool {
	return s.InstalledVersion != "" && s.LatestVersion != "" && s.InstalledVersion != s.LatestVersion
}

// DesiredAction is a queued mutation in the pending action plan.
type DesiredAction string

// Supported pending actions.
const (
	ActionNone    DesiredAction = "none"
	ActionInstall DesiredAction = "install"
	ActionUpdate  DesiredAction = "update"
	ActionRemove  DesiredAction = "remove"
)

// NextAction implements the Space-key checkbox state machine from the TUI
// specification.
func NextAction(current DesiredAction, installed, outdated bool) DesiredAction {
	if !installed {
		if current == ActionNone || current == ActionRemove {
			return ActionInstall
		}
		return ActionNone
	}

	if outdated {
		switch current {
		case ActionNone:
			return ActionUpdate
		case ActionUpdate:
			return ActionRemove
		case ActionRemove:
			return ActionNone
		default:
			return ActionUpdate
		}
	}

	switch current {
	case ActionNone, ActionInstall, ActionUpdate:
		return ActionRemove
	default:
		return ActionNone
	}
}

// Marker returns the visual marker, semantic color token, and accessible label
// for a tool row.
func Marker(action DesiredAction, installed, outdated bool) (mark, token, label string) {
	switch action {
	case ActionInstall:
		return "[+]", "accent", "Install queued"
	case ActionUpdate:
		return "[↑]", "accent", "Update queued"
	case ActionRemove:
		return "[-]", "error", "Remove queued"
	}

	switch {
	case outdated:
		return "[!]", "low", "Update available"
	case installed:
		return "[✓]", "good", "Installed (current)"
	default:
		return "[ ]", "muted", "Not installed"
	}
}
