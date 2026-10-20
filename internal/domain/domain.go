// Package domain contains Outfitter's backend-agnostic catalog, state, and
// action-plan values. It has no dependency on the terminal, CLI, or execution
// adapters.
package domain

import "strings"

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
	ShadowedBy       string      `json:"shadowed_by,omitempty"`
	ShadowVersion    string      `json:"shadow_version,omitempty"`
	// RemovalCommand is a manager-specific removal command discovered
	// for the installed binary (for example npm uninstall for an
	// npm-global install). Empty means plain rm of BinaryPath.
	RemovalCommand string `json:"removal_command,omitempty"`
}

// ShadowRemovalCommand returns the shell command that deletes the
// shadowing binary, or "" when there is no shadow.
func (s ToolState) ShadowRemovalCommand() string {
	if s.ShadowedBy == "" {
		return ""
	}
	if s.RemovalCommand != "" {
		return s.RemovalCommand
	}
	return RemovalCommand(s.ShadowedBy)
}

// IsInstalled reports whether a version was discovered on disk.
func (s ToolState) IsInstalled() bool {
	return s.InstalledVersion != ""
}

// ManagedByBackend reports whether the discovered installation is owned by
// the given catalog backend, so the backend's upgrade-only command can
// update it in place. An unknown provenance (for example a state built
// without discovery) is treated as managed to preserve the classic update
// path; only a concretely different channel counts as unmanaged.
func (s ToolState) ManagedByBackend(backend BackendType) bool {
	switch s.Provenance {
	case "", ProvenanceNone:
		return true
	}
	switch backend {
	case BackendMiseAqua, BackendMiseUbi, BackendMiseGitHub, BackendMiseNPM:
		return s.Provenance == ProvenanceMise
	case BackendUV:
		return s.Provenance == ProvenanceUV
	case BackendAPT:
		return s.Provenance == ProvenanceAPT
	case BackendVendor:
		return s.Provenance == ProvenanceVendor
	case BackendSource:
		return s.Provenance == ProvenanceSource
	default:
		return false
	}
}

// IsOutdated reports whether both versions are known and LatestVersion is
// strictly newer than InstalledVersion.
func (s ToolState) IsOutdated() bool {
	if s.InstalledVersion == "" || s.LatestVersion == "" {
		return false
	}
	return CompareVersions(s.LatestVersion, s.InstalledVersion) > 0
}

// CompareVersions compares two version strings. It returns >0 when left is
// newer than right, <0 when left is older than right, and 0 when equal.
// Non-numeric versions fall back to string inequality only when both have no
// numeric components.
func CompareVersions(left, right string) int {
	left = strings.TrimSpace(left)
	right = strings.TrimSpace(right)
	if left == right {
		return 0
	}
	lNums, lRest := splitVersionParts(left)
	rNums, rRest := splitVersionParts(right)
	if len(lNums) == 0 || len(rNums) == 0 {
		return 0
	}
	maxLen := len(lNums)
	if len(rNums) > maxLen {
		maxLen = len(rNums)
	}
	for i := 0; i < maxLen; i++ {
		var lv, rv int
		if i < len(lNums) {
			lv = lNums[i]
		}
		if i < len(rNums) {
			rv = rNums[i]
		}
		if lv != rv {
			if lv > rv {
				return 1
			}
			return -1
		}
	}
	if lRest == rRest {
		return 0
	}
	if lRest > rRest {
		return 1
	}
	return -1
}

func splitVersionParts(v string) ([]int, string) {
	v = strings.TrimPrefix(strings.TrimPrefix(v, "v"), "V")
	var nums []int
	rest := ""
	parts := strings.Split(v, ".")
	for idx, part := range parts {
		num := 0
		digits := 0
		for digits < len(part) && part[digits] >= '0' && part[digits] <= '9' {
			num = num*10 + int(part[digits]-'0')
			digits++
		}
		if digits == 0 {
			rest = strings.Join(parts[idx:], ".")
			break
		}
		nums = append(nums, num)
		if digits < len(part) {
			suffix := part[digits:]
			if idx+1 < len(parts) {
				suffix += "." + strings.Join(parts[idx+1:], ".")
			}
			rest = suffix
			break
		}
	}
	return nums, rest
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
	if current == "" {
		current = ActionNone
	}
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
