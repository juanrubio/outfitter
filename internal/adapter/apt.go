package adapter

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/juanrubio/outfitter/internal/domain"
)

func (a *Adapter) aptCommands(step domain.PlanStep) ([]CommandSpec, error) {
	pkg := strings.TrimSpace(step.Tool.Package)
	if pkg == "" {
		return nil, fmt.Errorf("tool %q: apt package name is empty", step.Tool.ID)
	}

	args := []string{"apt-get"}
	switch step.Action {
	case domain.ActionInstall:
		args = append(args, "install", "-y", pkg)
	case domain.ActionUpdate:
		// A migrating update adopts a binary dpkg does not own yet;
		// --only-upgrade would skip it as "not installed" and succeed
		// without changing anything.
		if step.Migrate {
			args = append(args, "install", "-y", pkg)
		} else {
			args = append(args, "install", "--only-upgrade", "-y", pkg)
		}
	case domain.ActionRemove:
		args = append(args, "remove", "-y", pkg)
	default:
		return nil, fmt.Errorf("tool %q: unsupported action %q", step.Tool.ID, step.Action)
	}

	return []CommandSpec{{
		Program: "sudo",
		Args:    args,
		Display: displayCommand("sudo", args...),
	}}, nil
}

// VerifySudoCommand returns the interactive pre-flight command used by the TUI
// through tea.ExecProcess (or by a CLI confirmation prompt) before a plan that
// needs elevated privileges starts.
func VerifySudoCommand() *exec.Cmd {
	return exec.Command("sudo", "-v")
}
