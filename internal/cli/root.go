// Package cli implements Outfitter's Cobra command tree.
package cli

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/juanrubio/outfitter/internal/tui"
)

// Version is set at release time via -ldflags.
var Version = "dev"

// ExitError carries the deterministic process exit code for a CLI failure.
type ExitError struct {
	Code int
	Err  error
}

// Error implements error.
func (e *ExitError) Error() string {
	if e == nil {
		return ""
	}
	if e.Err == nil {
		return fmt.Sprintf("exit status %d", e.Code)
	}
	return e.Err.Error()
}

// Unwrap exposes the underlying failure to errors.Is/errors.As.
func (e *ExitError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// ExitCode maps an error to the CLI contract: 0 for success, the wrapped
// ExitError code for usage/confirmation/execution failures, and 1 otherwise.
func ExitCode(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *ExitError
	if errors.As(err, &exitErr) {
		return exitErr.Code
	}
	return 1
}

type options struct {
	version bool
	demo    bool
	catalog string
	deps    Deps
}

// NewRootCmd builds the root command with production dependencies.
func NewRootCmd() *cobra.Command {
	return NewRootCmdWithDeps(DefaultDeps())
}

// NewRootCmdWithDeps builds the root command with injectable dependencies.
func NewRootCmdWithDeps(deps Deps) *cobra.Command {
	opts := &options{deps: deps.withDefaults()}
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
			return opts.deps.RunTUI(cmd.Context(), tui.Options{
				Demo:        opts.demo,
				CatalogPath: opts.catalog,
			})
		},
	}
	cmd.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return &ExitError{Code: 2, Err: err}
	})
	cmd.PersistentFlags().BoolVar(&opts.demo, "demo", false, "run the offline fictional demonstration")
	cmd.PersistentFlags().StringVar(&opts.catalog, "catalog", "", "path to a user catalog.toml")
	cmd.Flags().BoolVar(&opts.version, "version", false, "print version information")

	cmd.AddCommand(
		newStatusCmd(opts),
		newCheckCmd(opts),
		newDoctorCmd(opts),
		newProfileCmd(opts),
		newInstallCmd(opts),
		newUpdateCmd(opts),
		newRemoveCmd(opts),
		newCatalogCmd(opts),
		newInternalBootstrapCmd(opts),
	)
	return cmd
}

// Execute runs the root command and returns its error.
func Execute() error {
	return normalizeExecuteError(NewRootCmd().Execute())
}

func normalizeExecuteError(err error) error {
	if err == nil {
		return nil
	}
	var exitErr *ExitError
	if errors.As(err, &exitErr) {
		return err
	}
	if strings.HasPrefix(err.Error(), "unknown command") {
		return &ExitError{Code: 2, Err: err}
	}
	return err
}
