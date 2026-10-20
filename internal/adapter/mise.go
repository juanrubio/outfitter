package adapter

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/juanrubio/outfitter/internal/domain"
)

// Runtime prerequisites installed into the global mise configuration before a
// tool that depends on them is changed.
const (
	miseNodePrerequisite = "node@lts"
	miseUVPrerequisite   = "uv@latest"
)

func (a *Adapter) miseCommands(step domain.PlanStep) ([]CommandSpec, error) {
	mise := a.resolveMise()
	specs := make([]CommandSpec, 0, 2)

	if step.Action == domain.ActionInstall || step.Action == domain.ActionUpdate {
		switch step.Tool.Backend {
		case domain.BackendMiseNPM:
			if a.ensureOnce("node") {
				specs = append(specs, miseSpecCommand(mise, "use", "--global", miseNodePrerequisite))
			}
		case domain.BackendUV:
			if a.ensureOnce("uv") {
				specs = append(specs, miseSpecCommand(mise, "use", "--global", miseUVPrerequisite))
			}
		}
	}

	if step.Tool.Backend == domain.BackendUV {
		pkg := strings.TrimSpace(step.Tool.Package)
		if pkg == "" {
			return nil, fmt.Errorf("tool %q: uv package name is empty", step.Tool.ID)
		}
		var verb string
		switch step.Action {
		case domain.ActionInstall:
			verb = "install"
		case domain.ActionUpdate:
			// A migrating update adopts a binary uv does not own yet;
			// "uv tool upgrade" only works on uv-installed tools.
			if step.Migrate {
				verb = "install"
			} else {
				verb = "upgrade"
			}
		case domain.ActionRemove:
			verb = "uninstall"
		default:
			return nil, fmt.Errorf("tool %q: unsupported action %q", step.Tool.ID, step.Action)
		}
		specs = append(specs, miseSpecCommand(mise, "x", "uv", "--", "uv", "tool", verb, pkg))
		return specs, nil
	}

	spec := miseSpec(step.Tool)
	if spec == "" {
		return nil, fmt.Errorf("tool %q: mise package name is empty", step.Tool.ID)
	}

	switch step.Action {
	case domain.ActionInstall:
		specs = append(specs, miseSpecCommand(mise, "use", "--global", spec+"@latest"))
	case domain.ActionUpdate:
		// A migrating update adopts a binary mise does not own yet;
		// "mise upgrade" on a tool absent from the mise config reports
		// "All tools are up to date" and changes nothing.
		if step.Migrate {
			specs = append(specs, miseSpecCommand(mise, "use", "--global", spec+"@latest"))
		} else {
			specs = append(specs, miseSpecCommand(mise, "upgrade", spec))
		}
	case domain.ActionRemove:
		specs = append(specs, miseSpecCommand(mise, "unuse", "--global", spec))
	default:
		return nil, fmt.Errorf("tool %q: unsupported action %q", step.Tool.ID, step.Action)
	}
	return specs, nil
}

func (a *Adapter) resolveMise() string {
	if a.lookPath != nil {
		if resolved, err := a.lookPath("mise"); err == nil && strings.TrimSpace(resolved) != "" {
			return resolved
		}
	}
	if a.homeDir != "" {
		candidate := filepath.Join(a.homeDir, ".local", "bin", "mise")
		if info, err := os.Stat(candidate); err == nil && info.Mode().IsRegular() &&
			info.Mode().Perm()&0o111 != 0 {
			return candidate
		}
	}
	return "mise"
}

func miseSpecCommand(program string, args ...string) CommandSpec {
	return CommandSpec{
		Program: program,
		Args:    args,
		Display: displayCommand("mise", args...),
	}
}

// miseSpec derives the install/upgrade identifier from a tool package,
// preserving an explicit registry prefix and otherwise selecting the registry
// implied by the backend.
func miseSpec(tool domain.Tool) string {
	pkg := strings.TrimSpace(tool.Package)
	if pkg == "" {
		return ""
	}
	for _, prefix := range []string{"aqua:", "ubi:", "github:", "npm:"} {
		if strings.HasPrefix(pkg, prefix) {
			return pkg
		}
	}

	switch tool.Backend {
	case domain.BackendMiseAqua:
		return "aqua:" + pkg
	case domain.BackendMiseUbi, domain.BackendMiseGitHub:
		return "ubi:" + pkg
	case domain.BackendMiseNPM:
		return "npm:" + pkg
	default:
		return pkg
	}
}
