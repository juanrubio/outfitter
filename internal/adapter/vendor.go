package adapter

import (
	"context"
	"fmt"
	"strings"

	"github.com/juanrubio/outfitter/internal/domain"
)

func (a *Adapter) vendorCommands(step domain.PlanStep) ([]CommandSpec, error) {
	if step.Tool.Binary == "codexbar" {
		opts := a.codexBar
		opts.HomeDir = a.homeDir
		action := step.Action
		display := strings.TrimSpace(step.Command)
		if display == "" {
			display = "outfitter codexbar bundle"
		}
		return []CommandSpec{{
			Display: display,
			CustomFunc: func(ctx context.Context, onLine func(string)) error {
				if action == domain.ActionRemove {
					return RemoveCodexBar(opts.HomeDir, onLine)
				}
				return InstallCodexBar(ctx, opts, onLine)
			},
		}}, nil
	}

	command := strings.TrimSpace(step.Command)
	if command == "" {
		command = strings.TrimSpace(step.Tool.CommandFor(step.Action))
	}
	if command == "" {
		return nil, fmt.Errorf("tool %q: no vendor command configured for %s", step.Tool.ID, step.Action)
	}
	return []CommandSpec{shellSpec(command)}, nil
}
