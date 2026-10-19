package adapter

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/juanrubio/outfitter/internal/domain"
)

func (a *Adapter) sourceCommands(step domain.PlanStep) ([]CommandSpec, error) {
	binary := strings.TrimSpace(step.Tool.Binary)
	if binary == "" {
		return nil, fmt.Errorf("tool %q: source binary name is empty", step.Tool.ID)
	}
	installDir := filepath.Join(a.homeDir, ".local", "bin")
	target := filepath.Join(installDir, binary)

	if step.Action == domain.ActionRemove {
		args := []string{"-f", target}
		return []CommandSpec{{
			Program: "rm",
			Args:    args,
			Display: displayCommand("rm", args...),
		}}, nil
	}
	if step.Action != domain.ActionInstall && step.Action != domain.ActionUpdate {
		return nil, fmt.Errorf("tool %q: unsupported action %q", step.Tool.ID, step.Action)
	}

	sourceDir := a.expandHome(step.Tool.SourceDir)
	buildCmd := strings.TrimSpace(step.Tool.BuildCmd)
	if buildCmd != "" {
		spec := shellSpec(buildCmd)
		spec.Dir = sourceDir
		return []CommandSpec{spec}, nil
	}
	if sourceDir == "" {
		return nil, fmt.Errorf("tool %q: %w", step.Tool.ID, errNoSourceDir)
	}

	args := []string{"build", "-o", target, "./cmd/" + binary}
	return []CommandSpec{{
		Program: "go",
		Args:    args,
		Dir:     sourceDir,
		Display: displayCommand("go", args...),
	}}, nil
}
