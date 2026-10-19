// Package cli implements Outfitter's Cobra command tree.
package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/juanrubio/outfitter/internal/tui"
)

// Version is set at release time via -ldflags.
var Version = "dev"

type options struct {
	version bool
	demo    bool
	catalog string
}

// NewRootCmd builds the root command. It is exported for tests.
func NewRootCmd() *cobra.Command {
	opts := &options{}
	cmd := &cobra.Command{
		Use:           "outfitter",
		Short:         "Install and keep your AI development toolkit current",
		Long:          "Outfitter is a keyboard-first TUI and scriptable CLI for installing, inspecting, selecting, and bulk-updating the command-line harnesses and terminal tools used for AI-assisted software development.",
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if opts.version {
				_, err := fmt.Fprintf(cmd.OutOrStdout(), "outfitter version %s\n", Version)
				return err
			}
			return tui.Run(cmd.Context(), tui.Options{
				Demo:        opts.demo,
				CatalogPath: opts.catalog,
			})
		},
	}
	cmd.Flags().BoolVar(&opts.version, "version", false, "print version information")
	cmd.Flags().BoolVar(&opts.demo, "demo", false, "run the offline fictional demonstration")
	cmd.Flags().StringVar(&opts.catalog, "catalog", "", "path to a user catalog.toml")
	return cmd
}

// Execute runs the root command and returns its error.
func Execute() error {
	return NewRootCmd().Execute()
}
