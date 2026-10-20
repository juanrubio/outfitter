package cli

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/juanrubio/outfitter/internal/domain"
	"github.com/juanrubio/outfitter/internal/engine"
)

type doctorOptions struct {
	Fix     bool
	Shims   bool
	Shell   string
	DryRun  bool
	Yes     bool
	JSONOut bool
}

type doctorReport struct {
	MiseInstalled bool                    `json:"mise_installed"`
	MisePath      string                  `json:"mise_path,omitempty"`
	Integration   engine.ShellIntegration `json:"integration"`
	SuggestedLine string                  `json:"suggested_line,omitempty"`
	Changed       bool                    `json:"changed,omitempty"`
}

func newDoctorCmd(opts *options) *cobra.Command {
	var doctor doctorOptions
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Diagnose and fix shell integration for mise-managed tools",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runDoctor(cmd, opts, doctor)
		},
	}
	cmd.Flags().BoolVar(&doctor.Fix, "fix", false, "add the mise integration block to your shell rc file")
	cmd.Flags().BoolVar(&doctor.Shims, "shims", false, "put the mise shims directory on PATH instead of using mise activate")
	cmd.Flags().StringVar(&doctor.Shell, "shell", "", "shell to configure (bash, zsh, fish); defaults to $SHELL")
	cmd.Flags().BoolVar(&doctor.DryRun, "dry-run", false, "print the rc block without writing it")
	cmd.Flags().BoolVar(&doctor.Yes, "yes", false, "confirm the rc change without prompting")
	cmd.Flags().BoolVar(&doctor.JSONOut, "json", false, "emit machine-readable JSON")
	return cmd
}

func runDoctor(cmd *cobra.Command, opts *options, doctor doctorOptions) error {
	out := cmd.OutOrStdout()
	if opts.demo {
		if doctor.JSONOut {
			return writeJSON(out, doctorReport{})
		}
		_, err := fmt.Fprintln(out, "Demo mode: shell integration checks are disabled.")
		return err
	}

	setup := opts.deps.ShellSetup()
	misePath, miseInstalled := opts.deps.MiseInstalled()
	integration := setup.Detect(doctor.Shell)
	report := doctorReport{
		MiseInstalled: miseInstalled,
		MisePath:      misePath,
		Integration:   integration,
		SuggestedLine: setup.IntegrationLine(integration.Shell, misePath, doctor.Shims),
	}

	if !doctor.JSONOut {
		if err := printDoctorDiagnosis(out, report); err != nil {
			return err
		}
	}

	if !miseInstalled {
		if doctor.JSONOut {
			return writeJSON(out, report)
		}
		_, err := fmt.Fprintln(out, "mise is not installed yet; Outfitter bootstraps it on the first apply that needs it.")
		return err
	}
	if integration.Integrated {
		if doctor.JSONOut {
			return writeJSON(out, report)
		}
		_, err := fmt.Fprintln(out, "Shell integration OK: mise-managed tools resolve in your shell.")
		return err
	}

	if doctor.DryRun {
		if doctor.JSONOut {
			return writeJSON(out, report)
		}
		return printDoctorBlock(out, setup, integration, misePath, doctor.Shims, true)
	}
	if !doctor.Fix {
		if doctor.JSONOut {
			return writeJSON(out, report)
		}
		if _, err := fmt.Fprintf(out, "Add this line to %s:\n  %s\n", displayRC(integration.RCFile), report.SuggestedLine); err != nil {
			return err
		}
		if integration.Fixable {
			_, err := fmt.Fprintln(out, "Or run `outfitter doctor --fix` to add it automatically.")
			return err
		}
		return nil
	}

	// --fix: preview the block, confirm, then apply.
	if !doctor.JSONOut {
		if err := printDoctorBlock(out, setup, integration, misePath, doctor.Shims, false); err != nil {
			return err
		}
	}
	if !doctor.Yes {
		if !opts.deps.IsTTY(cmd.InOrStdin(), cmd.OutOrStdout()) {
			return &ExitError{
				Code: 2,
				Err:  errors.New("refusing to execute without --yes in a non-interactive environment"),
			}
		}
		confirmed, err := promptForConfirmation(cmd, out)
		if err != nil {
			return err
		}
		if !confirmed {
			return &ExitError{Code: 2, Err: errors.New("confirmation refused")}
		}
	}

	changed, err := setup.Apply(doctor.Shell, misePath, doctor.Shims)
	if err != nil {
		return &ExitError{Code: 2, Err: err}
	}
	report.Changed = changed
	report.Integration = setup.Detect(doctor.Shell)

	if doctor.JSONOut {
		return writeJSON(out, report)
	}
	if !changed {
		_, err := fmt.Fprintln(out, "Already integrated; nothing changed.")
		return err
	}
	if _, err := fmt.Fprintf(out, "Added mise integration to %s.\n", displayRC(integration.RCFile)); err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "Open a new terminal, or run this in your current shell:\n  %s\n", report.SuggestedLine)
	return err
}

func printDoctorDiagnosis(out io.Writer, report doctorReport) error {
	miseLine := "not installed"
	if report.MiseInstalled {
		miseLine = report.MisePath + " (installed)"
	}
	integLine := "missing — mise-managed tools will not resolve in your shell"
	if report.Integration.Integrated {
		integLine = "active"
	}
	lines := []string{
		fmt.Sprintf("Shell: %s", report.Integration.Shell),
		fmt.Sprintf("RC file: %s", displayRC(report.Integration.RCFile)),
		fmt.Sprintf("mise: %s", miseLine),
		fmt.Sprintf("Integration: %s", integLine),
	}
	for _, line := range lines {
		if _, err := fmt.Fprintln(out, line); err != nil {
			return err
		}
	}
	return nil
}

func printDoctorBlock(out io.Writer, setup *engine.ShellSetup, integration engine.ShellIntegration, misePath string, shims, dryRun bool) error {
	if !integration.Fixable {
		_, err := fmt.Fprintf(out, "Automatic fix supports bash and zsh. For %s, add this line to %s:\n  %s\n",
			integration.Shell, displayRC(integration.RCFile), setup.IntegrationLine(integration.Shell, misePath, shims))
		return err
	}
	verb := "Append to"
	if dryRun {
		verb = "Would append to"
	}
	if _, err := fmt.Fprintf(out, "%s %s:\n", verb, displayRC(integration.RCFile)); err != nil {
		return err
	}
	for _, line := range setup.IntegrationBlock(integration.Shell, misePath, shims) {
		if _, err := fmt.Fprintf(out, "  %s\n", line); err != nil {
			return err
		}
	}
	return nil
}

// displayRC renders an rc path, annotating the resolved target when the
// file is a symlink (common with managed dotfiles).
func displayRC(path string) string {
	if path == "" {
		return "(none)"
	}
	if resolved, err := filepath.EvalSymlinks(path); err == nil && resolved != path {
		return fmt.Sprintf("%s (→ %s)", path, resolved)
	}
	return path
}

// shellIntegrationWarning reports the integration state when a warning is
// warranted: mise installed, at least one mise-managed tool present, and
// no shell integration. Shared by status and doctor-adjacent flows.
func (opts *options) shellIntegrationWarning(states map[string]domain.ToolState) (engine.ShellIntegration, string, bool) {
	if opts.demo {
		return engine.ShellIntegration{}, "", false
	}
	misePath, installed := opts.deps.MiseInstalled()
	if !installed {
		return engine.ShellIntegration{}, "", false
	}
	anyMise := false
	for _, state := range states {
		if state.Provenance == domain.ProvenanceMise {
			anyMise = true
			break
		}
	}
	if !anyMise {
		return engine.ShellIntegration{}, "", false
	}
	integration := opts.deps.ShellSetup().Detect("")
	if integration.Integrated {
		return engine.ShellIntegration{}, "", false
	}
	return integration, misePath, true
}

// writeShellWarning prints the persistent shell-integration warning used
// after the status table.
func (opts *options) writeShellWarning(out io.Writer, states map[string]domain.ToolState) error {
	integration, misePath, warn := opts.shellIntegrationWarning(states)
	if !warn {
		return nil
	}
	setup := opts.deps.ShellSetup()
	line := setup.IntegrationLine(integration.Shell, misePath, false)
	if _, err := fmt.Fprintln(out); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "Shell integration: mise is not active in your %s shell — mise-managed tools won't resolve in your terminal.\n", integration.Shell); err != nil {
		return err
	}
	if integration.Fixable {
		_, err := fmt.Fprintf(out, "  Fix: outfitter doctor --fix   (manual: add `%s` to %s)\n", line, displayRC(integration.RCFile))
		return err
	}
	_, err := fmt.Fprintf(out, "  Add to %s:\n    %s\n", displayRC(integration.RCFile), line)
	return err
}
