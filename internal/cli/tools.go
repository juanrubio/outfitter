package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/juanrubio/outfitter/internal/catalog"
	"github.com/juanrubio/outfitter/internal/domain"
)

func newInstallCmd(opts *options) *cobra.Command {
	var dryRun, yes, jsonOut, removeStrays bool
	cmd := &cobra.Command{
		Use:   "install <tool-id>...",
		Short: "Install specific tools",
		Args: func(_ *cobra.Command, args []string) error {
			if len(args) == 0 {
				return &ExitError{Code: 2, Err: fmt.Errorf("install requires at least one tool id")}
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			cat, err := opts.loadCatalog()
			if err != nil {
				return err
			}
			tools, err := validateTools(cat, args)
			if err != nil {
				return err
			}
			states, err := opts.localStates(cmd.Context(), cat)
			if err != nil {
				return err
			}
			states = opts.upstreamStates(cmd.Context(), cat, states)

			actions := make(map[string]domain.DesiredAction, len(tools))
			for _, tool := range tools {
				state := states[tool.ID]
				switch {
				case !state.IsInstalled():
					actions[tool.ID] = domain.ActionInstall
				case state.IsOutdated():
					actions[tool.ID] = domain.ActionUpdate
				}
			}
			return executeMutation(cmd, opts, cat, states, actions, mutationOptions{
				DryRun:       dryRun,
				Yes:          yes,
				JSON:         jsonOut,
				RemoveStrays: removeStrays,
			})
		},
	}
	addMutationFlags(cmd, &dryRun, &yes, &jsonOut, &removeStrays)
	return cmd
}

func newUpdateCmd(opts *options) *cobra.Command {
	var dryRun, yes, jsonOut, all, removeStrays bool
	cmd := &cobra.Command{
		Use:   "update [<tool-id>...]",
		Short: "Update outdated tools",
		RunE: func(cmd *cobra.Command, args []string) error {
			cat, err := opts.loadCatalog()
			if err != nil {
				return err
			}
			var tools []domain.Tool
			if len(args) > 0 {
				tools, err = validateTools(cat, args)
				if err != nil {
					return err
				}
			}
			states, err := opts.localStates(cmd.Context(), cat)
			if err != nil {
				return err
			}
			states = opts.upstreamStates(cmd.Context(), cat, states)

			actions := make(map[string]domain.DesiredAction)
			switch {
			case all || len(args) == 0:
				for _, tool := range cat.Tools {
					if states[tool.ID].IsOutdated() {
						actions[tool.ID] = domain.ActionUpdate
					}
				}
			default:
				for _, tool := range tools {
					actions[tool.ID] = domain.ActionUpdate
				}
			}
			return executeMutation(cmd, opts, cat, states, actions, mutationOptions{
				DryRun:       dryRun,
				Yes:          yes,
				JSON:         jsonOut,
				RemoveStrays: removeStrays,
			})
		},
	}
	addMutationFlags(cmd, &dryRun, &yes, &jsonOut, &removeStrays)
	cmd.Flags().BoolVar(&all, "all", false, "update every installed and outdated tool")
	return cmd
}

func newRemoveCmd(opts *options) *cobra.Command {
	var dryRun, yes, jsonOut, removeStrays bool
	cmd := &cobra.Command{
		Use:   "remove <tool-id>...",
		Short: "Uninstall specific tools",
		Args: func(_ *cobra.Command, args []string) error {
			if len(args) == 0 {
				return &ExitError{Code: 2, Err: fmt.Errorf("remove requires at least one tool id")}
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			cat, err := opts.loadCatalog()
			if err != nil {
				return err
			}
			tools, err := validateTools(cat, args)
			if err != nil {
				return err
			}
			states, err := opts.localStates(cmd.Context(), cat)
			if err != nil {
				return err
			}

			actions := make(map[string]domain.DesiredAction, len(tools))
			for _, tool := range tools {
				actions[tool.ID] = domain.ActionRemove
			}
			return executeMutation(cmd, opts, cat, states, actions, mutationOptions{
				DryRun:       dryRun,
				Yes:          yes,
				JSON:         jsonOut,
				RemoveStrays: removeStrays,
			})
		},
	}
	addMutationFlags(cmd, &dryRun, &yes, &jsonOut, &removeStrays)
	return cmd
}

func addMutationFlags(cmd *cobra.Command, dryRun, yes, jsonOut, removeStrays *bool) {
	cmd.Flags().BoolVar(dryRun, "dry-run", false, "print the execution plan without running it")
	cmd.Flags().BoolVar(yes, "yes", false, "confirm execution without prompting")
	cmd.Flags().BoolVar(jsonOut, "json", false, "emit machine-readable JSON")
	cmd.Flags().BoolVar(removeStrays, "remove-strays", false, "also delete stray binaries left behind by migrating updates")
}

func validateTools(cat *catalog.Catalog, ids []string) ([]domain.Tool, error) {
	tools := make([]domain.Tool, 0, len(ids))
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		tool, ok := cat.ToolByID(id)
		if !ok {
			return nil, &ExitError{Code: 2, Err: fmt.Errorf("unknown tool id %q", id)}
		}
		if _, duplicate := seen[id]; duplicate {
			continue
		}
		seen[id] = struct{}{}
		tools = append(tools, tool)
	}
	return tools, nil
}
