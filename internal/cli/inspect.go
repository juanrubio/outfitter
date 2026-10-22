package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/juanrubio/outfitter/internal/engine"
)

func newStatusCmd(opts *options) *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "status",
		Short: "List catalog tools, installed versions, and cached updates",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cat, err := opts.loadCatalog()
			if err != nil {
				return err
			}
			states, err := opts.localStates(cmd.Context(), cat)
			if err != nil {
				return err
			}
			states = opts.mergeCachedUpstream(states)
			self := opts.cachedSelfState(cat)
			entries := statusEntries(cat, states)
			if jsonOut {
				return writeJSON(cmd.OutOrStdout(), entries)
			}
			if err := writeStatusTable(cmd.OutOrStdout(), cat.Tools, states); err != nil {
				return err
			}
			if err := writeSelfUpdateLine(cmd.OutOrStdout(), self); err != nil {
				return err
			}
			return opts.writeShellWarning(cmd.OutOrStdout(), states)
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit machine-readable JSON")
	return cmd
}

func newCheckCmd(opts *options) *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "check",
		Short: "Check upstream registries for available updates",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cat, err := opts.loadCatalog()
			if err != nil {
				return err
			}
			states, err := opts.localStates(cmd.Context(), cat)
			if err != nil {
				return err
			}
			states = opts.upstreamStates(cmd.Context(), cat, states)
			self := opts.checkedSelfState(cmd.Context(), cat)
			if err := opts.saveCache(states); err != nil {
				return err
			}

			entries := statusEntries(cat, states)
			updates := make([]string, 0)
			upToDate := 0
			notInstalled := 0
			for _, entry := range entries {
				switch {
				case entry.Outdated:
					updates = append(updates, entry.ID)
				case entry.Installed:
					upToDate++
				default:
					notInstalled++
				}
			}
			payload := struct {
				Tools         []statusEntry    `json:"tools"`
				OutdatedCount int              `json:"outdated_count"`
				Updates       []string         `json:"updates"`
				Self          engine.SelfState `json:"self"`
			}{
				Tools:         entries,
				OutdatedCount: len(updates),
				Updates:       updates,
				Self:          self,
			}
			if jsonOut {
				return writeJSON(cmd.OutOrStdout(), payload)
			}

			out := cmd.OutOrStdout()
			if line := selfUpdateLine(self); line != "" {
				if _, err := fmt.Fprintln(out, line); err != nil {
					return err
				}
			}
			if len(updates) == 0 {
				if _, err := fmt.Fprintf(out, "All %d installed tools are up to date.\n", upToDate); err != nil {
					return err
				}
			} else {
				if _, err := fmt.Fprintln(out, "Updates available:"); err != nil {
					return err
				}
				for _, entry := range entries {
					if !entry.Outdated {
						continue
					}
					if _, err := fmt.Fprintf(out, "  %s  %s → %s\n",
						entry.ID, entry.InstalledVersion, entry.LatestVersion); err != nil {
						return err
					}
				}
			}
			_, err = fmt.Fprintf(out, "%d up to date · %d outdated · %d not installed\n",
				upToDate, len(updates), notInstalled)
			return err
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit machine-readable JSON")
	return cmd
}
