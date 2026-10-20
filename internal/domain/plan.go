package domain

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

// BootstrapMiseCommand is the internal command used by the Plan & Apply view
// for the verified mise bootstrapper.
const BootstrapMiseCommand = "outfitter internal-bootstrap mise"

// PlanStep is one executable action in an ActionPlan.
type PlanStep struct {
	Tool        Tool          `json:"tool"`
	Action      DesiredAction `json:"action"`
	FromVersion string        `json:"from_version,omitempty"`
	ToVersion   string        `json:"to_version,omitempty"`
	Command     string        `json:"command"`
	Sudo        bool          `json:"sudo"`
	// Migrate marks an update step whose discovered installation is not
	// owned by the tool's catalog backend (for example an external PATH
	// binary). Such a step executes the backend's install command instead
	// of its upgrade-only command, which would silently change nothing.
	Migrate bool `json:"migrate,omitempty"`
	// StrayBinaryPath is the unmanaged binary that remains on disk after
	// a migrating update installs the managed copy elsewhere. It is
	// empty when the install overwrites the discovered path (or the step
	// is not a migration), so no cleanup warning is needed.
	StrayBinaryPath string `json:"stray_binary_path,omitempty"`
}

// StrayRemovalCommand returns the shell command that deletes the step's
// stray binary, ready for the user to run after the plan completes. It
// returns "" when the step leaves no stray binary behind.
func (s PlanStep) StrayRemovalCommand() string {
	path := strings.TrimSpace(s.StrayBinaryPath)
	if path == "" {
		return ""
	}
	prefix := "rm "
	for _, systemPrefix := range []string{"/usr/", "/bin/", "/sbin/", "/opt/"} {
		if strings.HasPrefix(path, systemPrefix) {
			prefix = "sudo rm "
			break
		}
	}
	return prefix + shellQuotePath(path)
}

// shellQuotePath returns the path unchanged when it needs no quoting, or
// single-quoted (with embedded quotes escaped) otherwise.
func shellQuotePath(path string) string {
	if strings.IndexFunc(path, func(r rune) bool {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			return false
		}
		switch r {
		case '/', '.', '_', '-', '@', '%', '+', '=', ':', ',':
			return false
		}
		return true
	}) < 0 {
		return path
	}
	return "'" + strings.ReplaceAll(path, "'", `'\''`) + "'"
}

// strayBinaryPath returns the discovered binary path that a migrating
// install leaves behind, or "" when the backend's install overwrites the
// discovered path.
func strayBinaryPath(tool Tool, state ToolState) string {
	path := strings.TrimSpace(state.BinaryPath)
	if path == "" {
		return ""
	}
	switch tool.Backend {
	case BackendAPT:
		// apt installs into the system directories, overwriting a stray
		// binary discovered there.
		switch filepath.Dir(path) {
		case "/usr/bin", "/bin", "/usr/sbin", "/sbin":
			return ""
		}
		return path
	case BackendVendor, BackendSource:
		// Vendor installers and source builds place the binary in the
		// user install directory, overwriting a stray discovered there.
		if strings.Contains(filepath.ToSlash(path), "/.local/bin/") {
			return ""
		}
		return path
	default:
		// mise and uv keep managed copies under their own directories,
		// never at the discovered external path.
		return path
	}
}

// ActionPlan is the ordered, dry-run representation of a pending plan.
type ActionPlan struct {
	BootstrapStep *PlanStep  `json:"bootstrap,omitempty"`
	Installations []PlanStep `json:"installations"`
	Updates       []PlanStep `json:"updates"`
	Removals      []PlanStep `json:"removals"`
}

// NeedsSudo reports whether any step requires elevated privileges.
func (p ActionPlan) NeedsSudo() bool {
	for _, step := range p.Steps() {
		if step.Sudo {
			return true
		}
	}
	return false
}

// TotalSteps returns the number of executable steps, including a bootstrap.
func (p ActionPlan) TotalSteps() int {
	total := len(p.Installations) + len(p.Updates) + len(p.Removals)
	if p.BootstrapStep != nil {
		total++
	}
	return total
}

// Counts returns the number of queued installs, updates, and removals. The
// mise bootstrap step is reported separately and is not counted as a tool
// installation.
func (p ActionPlan) Counts() (installs, updates, removals int) {
	return len(p.Installations), len(p.Updates), len(p.Removals)
}

// IsEmpty reports whether the plan has no tool actions.
func (p ActionPlan) IsEmpty() bool {
	return p.TotalSteps() == 0
}

// Steps returns every step in execution order.
func (p ActionPlan) Steps() []PlanStep {
	steps := make([]PlanStep, 0, p.TotalSteps())
	if p.BootstrapStep != nil {
		steps = append(steps, *p.BootstrapStep)
	}
	steps = append(steps, p.Installations...)
	steps = append(steps, p.Updates...)
	steps = append(steps, p.Removals...)
	return steps
}

// InstallCommand returns the configured install command or a canonical command
// derived from the tool's backend and package.
func (t Tool) InstallCommand() string {
	if t.InstallCmd != "" {
		return t.InstallCmd
	}
	return DefaultInstallCommand(t)
}

// UpdateCommand returns the configured update command or a canonical command
// derived from the tool's backend and package.
func (t Tool) UpdateCommand() string {
	if t.UpdateCmd != "" {
		return t.UpdateCmd
	}
	return DefaultUpdateCommand(t)
}

// RemoveCommand returns the configured remove command or a canonical command
// derived from the tool's backend and package.
func (t Tool) RemoveCommand() string {
	if t.RemoveCmd != "" {
		return t.RemoveCmd
	}
	return DefaultRemoveCommand(t)
}

// CommandFor returns the command for a desired action.
func (t Tool) CommandFor(action DesiredAction) string {
	switch action {
	case ActionInstall:
		return t.InstallCommand()
	case ActionUpdate:
		return t.UpdateCommand()
	case ActionRemove:
		return t.RemoveCommand()
	default:
		return ""
	}
}

// DefaultInstallCommand synthesizes the canonical install command for a
// backend when the catalog does not provide a custom command.
func DefaultInstallCommand(t Tool) string {
	switch t.Backend {
	case BackendMiseAqua, BackendMiseUbi, BackendMiseGitHub, BackendMiseNPM:
		spec := miseSpec(t)
		if spec == "" {
			return ""
		}
		return "mise use --global " + spec + "@latest"
	case BackendUV:
		if t.Package == "" {
			return ""
		}
		return "mise x uv -- uv tool install " + t.Package
	case BackendAPT:
		if t.Package == "" {
			return ""
		}
		return "sudo apt-get install -y " + t.Package
	case BackendSource:
		return sourceBuildCommand(t)
	case BackendVendor:
		return ""
	default:
		return ""
	}
}

// DefaultUpdateCommand synthesizes the canonical update command for a backend.
func DefaultUpdateCommand(t Tool) string {
	switch t.Backend {
	case BackendMiseAqua, BackendMiseUbi, BackendMiseGitHub, BackendMiseNPM:
		spec := miseSpec(t)
		if spec == "" {
			return ""
		}
		return "mise upgrade " + spec
	case BackendUV:
		if t.Package == "" {
			return ""
		}
		return "mise x uv -- uv tool upgrade " + t.Package
	case BackendAPT:
		if t.Package == "" {
			return ""
		}
		return "sudo apt-get install --only-upgrade -y " + t.Package
	case BackendSource:
		return sourceBuildCommand(t)
	case BackendVendor:
		return ""
	default:
		return ""
	}
}

// DefaultRemoveCommand synthesizes the canonical removal command for a
// backend. Vendor installers are deliberately left to an explicit command.
func DefaultRemoveCommand(t Tool) string {
	switch t.Backend {
	case BackendMiseAqua, BackendMiseUbi, BackendMiseGitHub, BackendMiseNPM:
		spec := miseSpec(t)
		if spec == "" {
			return ""
		}
		return "mise unuse --global " + spec
	case BackendUV:
		if t.Package == "" {
			return ""
		}
		return "mise x uv -- uv tool uninstall " + t.Package
	case BackendAPT:
		if t.Package == "" {
			return ""
		}
		return "sudo apt-get remove -y " + t.Package
	case BackendSource:
		if t.Binary == "" {
			return ""
		}
		return fmt.Sprintf("rm -f %s/%s", DefaultInstallDir, t.Binary)
	case BackendVendor:
		return ""
	default:
		return ""
	}
}

func miseSpec(t Tool) string {
	pkg := strings.TrimSpace(t.Package)
	if pkg == "" {
		return ""
	}
	for _, prefix := range []string{"aqua:", "ubi:", "github:", "npm:"} {
		if strings.HasPrefix(pkg, prefix) {
			return pkg
		}
	}

	switch t.Backend {
	case BackendMiseAqua:
		return "aqua:" + pkg
	case BackendMiseUbi, BackendMiseGitHub:
		return "ubi:" + pkg
	case BackendMiseNPM:
		return "npm:" + pkg
	default:
		return pkg
	}
}

func sourceBuildCommand(t Tool) string {
	if t.BuildCmd != "" {
		return t.BuildCmd
	}
	if t.SourceDir != "" {
		target := "./cmd/" + t.Binary
		return fmt.Sprintf("go -C %s build -o %s/%s %s", t.SourceDir, DefaultInstallDir, t.Binary, target)
	}
	if t.Binary == "" {
		return ""
	}
	return fmt.Sprintf("go build -o %s/%s .", DefaultInstallDir, t.Binary)
}

// BuildActionPlan orders queued actions into bootstrap, installation, update,
// and removal phases. System packages are placed before user-space installers
// within each phase so a single sudo verification covers all apt work.
func BuildActionPlan(
	tools []Tool,
	states map[string]ToolState,
	actions map[string]DesiredAction,
	miseInstalled bool,
	miseVersion string,
) ActionPlan {
	plan := ActionPlan{}
	requiresMise := false

	var installs, updates, removals []PlanStep
	for _, tool := range tools {
		action := actions[tool.ID]
		if action == "" || action == ActionNone {
			continue
		}

		state := states[tool.ID]
		step := PlanStep{
			Tool:   tool,
			Action: action,
			Sudo:   tool.Sudo || tool.Backend == BackendAPT,
		}

		switch action {
		case ActionInstall:
			step.ToVersion = state.LatestVersion
			step.Command = tool.InstallCommand()
			installs = append(installs, step)
		case ActionUpdate:
			step.FromVersion = state.InstalledVersion
			step.ToVersion = state.LatestVersion
			step.Command = tool.UpdateCommand()
			if !state.ManagedByBackend(tool.Backend) {
				// The backend does not own the discovered binary, so its
				// upgrade-only command would change nothing. Adopt the
				// tool by running the install command instead.
				step.Migrate = true
				step.Command = tool.InstallCommand()
				step.StrayBinaryPath = strayBinaryPath(tool, state)
			}
			updates = append(updates, step)
		case ActionRemove:
			step.FromVersion = state.InstalledVersion
			step.Command = tool.RemoveCommand()
			removals = append(removals, step)
		default:
			continue
		}

		if tool.Backend.RequiresMise() {
			requiresMise = true
		}
	}

	plan.Installations = sortPlanSteps(installs)
	plan.Updates = sortPlanSteps(updates)
	plan.Removals = sortPlanSteps(removals)

	bootstrapRequested := actions["mise"] == ActionInstall || actions["mise"] == ActionUpdate
	if !miseInstalled && (requiresMise || bootstrapRequested) {
		plan.BootstrapStep = &PlanStep{
			Tool: Tool{
				ID:          "mise",
				Name:        "mise",
				Binary:      "mise",
				CategoryID:  "utilities",
				Backend:     BackendMiseGitHub,
				Package:     "jdx/mise",
				Description: "Development environment tool manager",
			},
			Action:    ActionInstall,
			ToVersion: miseVersion,
			Command:   BootstrapMiseCommand,
		}
	}

	return plan
}

func sortPlanSteps(steps []PlanStep) []PlanStep {
	sort.SliceStable(steps, func(i, j int) bool {
		return backendRank(steps[i].Tool.Backend) < backendRank(steps[j].Tool.Backend)
	})
	return steps
}

func backendRank(backend BackendType) int {
	switch backend {
	case BackendAPT:
		return 0
	case BackendMiseAqua, BackendMiseUbi, BackendMiseGitHub, BackendMiseNPM:
		return 1
	case BackendUV:
		return 2
	case BackendVendor:
		return 3
	case BackendSource:
		return 4
	default:
		return 5
	}
}
