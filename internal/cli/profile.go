package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/juanrubio/outfitter/internal/catalog"
	"github.com/juanrubio/outfitter/internal/domain"
)

type profileEntry struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Short       string   `json:"short"`
	Description string   `json:"description"`
	ToolCount   int      `json:"tool_count"`
	Tools       []string `json:"tools"`
}

func newProfileCmd(opts *options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "profile",
		Short: "List and apply built-in and custom profiles",
	}
	cmd.AddCommand(newProfileListCmd(opts), newProfileApplyCmd(opts))
	return cmd
}

func newProfileListCmd(opts *options) *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List available profiles",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cat, err := opts.loadCatalog()
			if err != nil {
				return err
			}
			entries := make([]profileEntry, 0, len(cat.Profiles))
			for _, profile := range cat.Profiles {
				entries = append(entries, profileEntry{
					ID:          profile.ID,
					Name:        profile.Name,
					Short:       profile.Short,
					Description: profile.Description,
					ToolCount:   len(profile.Tools),
					Tools:       profile.Tools,
				})
			}
			if jsonOut {
				return writeJSON(cmd.OutOrStdout(), entries)
			}
			out := cmd.OutOrStdout()
			for _, entry := range entries {
				if _, err := fmt.Fprintf(out, "%s\t%s\t%d tools\t%s\n",
					entry.ID, entry.Name, entry.ToolCount, entry.Description); err != nil {
					return err
				}
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit machine-readable JSON")
	return cmd
}

func newProfileApplyCmd(opts *options) *cobra.Command {
	var dryRun, yes, jsonOut bool
	cmd := &cobra.Command{
		Use:   "apply <name>",
		Short: "Install missing tools and update outdated tools in a profile",
		Args: func(_ *cobra.Command, args []string) error {
			if len(args) != 1 {
				return &ExitError{Code: 2, Err: fmt.Errorf("profile apply requires exactly one profile name")}
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			cat, err := opts.loadCatalog()
			if err != nil {
				return err
			}
			profile, ok := resolveProfile(cat, args[0])
			if !ok {
				return &ExitError{Code: 2, Err: fmt.Errorf("unknown profile %q", args[0])}
			}
			states, err := opts.localStates(cmd.Context(), cat)
			if err != nil {
				return err
			}
			states = opts.upstreamStates(cmd.Context(), cat, states)

			actions := make(map[string]domain.DesiredAction)
			for _, id := range profile.Tools {
				state := states[id]
				switch {
				case !state.IsInstalled():
					actions[id] = domain.ActionInstall
				case state.IsOutdated():
					actions[id] = domain.ActionUpdate
				}
			}
			return executeMutation(cmd, opts, cat, states, actions, mutationOptions{
				DryRun: dryRun,
				Yes:    yes,
				JSON:   jsonOut,
			})
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print the execution plan without running it")
	cmd.Flags().BoolVar(&yes, "yes", false, "confirm execution without prompting")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit machine-readable JSON")
	return cmd
}

func resolveProfile(cat *catalog.Catalog, name string) (domain.Profile, bool) {
	if profile, ok := cat.ProfileByID(name); ok {
		return profile, true
	}
	wanted := strings.ToLower(strings.TrimSpace(name))
	if wanted == "" {
		return domain.Profile{}, false
	}
	for _, profile := range cat.Profiles {
		if strings.ToLower(profile.Short) == wanted || strings.ToLower(profile.Name) == wanted {
			return profile, true
		}
	}
	return domain.Profile{}, false
}
