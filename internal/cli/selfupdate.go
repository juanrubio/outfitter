package cli

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/juanrubio/outfitter/internal/catalog"
	"github.com/juanrubio/outfitter/internal/engine"
)

type selfUpdateReport struct {
	State           engine.SelfState `json:"self"`
	Updated         bool             `json:"updated"`
	DryRun          bool             `json:"dry_run"`
	Command         string           `json:"command,omitempty"`
	BackupPath      string           `json:"backup_path,omitempty"`
	RequiresRestart bool             `json:"requires_restart"`
	Message         string           `json:"message,omitempty"`
	Error           string           `json:"error,omitempty"`
}

func reportFromSelfResult(result engine.SelfUpdateResult) selfUpdateReport {
	return selfUpdateReport{
		State:           result.State,
		Updated:         result.Updated,
		DryRun:          result.DryRun,
		Command:         result.Command,
		BackupPath:      result.BackupPath,
		RequiresRestart: result.RequiresRestart,
		Message:         result.Message,
	}
}

func selfUpdateLine(state engine.SelfState) string {
	if !state.Outdated {
		return ""
	}
	return fmt.Sprintf("Outfitter update available: %s → %s (run: outfitter self-update)",
		state.CurrentVersion, state.LatestVersion)
}

func writeSelfUpdateLine(out io.Writer, state engine.SelfState) error {
	line := selfUpdateLine(state)
	if line == "" {
		return nil
	}
	if _, err := fmt.Fprintln(out); err != nil {
		return err
	}
	_, err := fmt.Fprintln(out, line)
	return err
}

// selfCheck serves explicit self-update invocations, so it bypasses the
// background-check opt-out. Tool `check` and the TUI use the regular
// (opt-out-respecting) SelfCheck dependency instead.
func (opts *options) selfCheck(ctx context.Context, cat *catalog.Catalog, _ bool) (engine.SelfState, error) {
	if opts.deps.SelfCheckExplicit != nil {
		return opts.deps.SelfCheckExplicit(ctx, cat)
	}
	if opts.deps.SelfCheck != nil {
		return opts.deps.SelfCheck(ctx, cat, true)
	}
	return engine.NewSelfUpdater(Version, opts.deps.HomeDir).CheckExplicit(ctx, cat)
}

func (opts *options) runSelfUpdate(ctx context.Context, cat *catalog.Catalog, updateOpts engine.SelfUpdateOptions) (engine.SelfUpdateResult, error) {
	if opts.deps.SelfUpdate != nil {
		return opts.deps.SelfUpdate(ctx, cat, updateOpts)
	}
	return engine.NewSelfUpdater(Version, opts.deps.HomeDir).Update(ctx, cat, updateOpts)
}

func newSelfUpdateCmd(opts *options) *cobra.Command {
	var checkOnly, dryRun, yes, jsonOut bool
	cmd := &cobra.Command{
		Use:   "self-update",
		Short: "Check for and install a newer Outfitter release",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runSelfUpdateCommand(cmd, opts, checkOnly, dryRun, yes, jsonOut)
		},
	}
	cmd.Flags().BoolVar(&checkOnly, "check", false, "only report whether an Outfitter update is available")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print the self-update plan without changing the binary")
	cmd.Flags().BoolVar(&yes, "yes", false, "confirm the update without prompting")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit machine-readable JSON")
	return cmd
}

func runSelfUpdateCommand(cmd *cobra.Command, opts *options, checkOnly, dryRun, yes, jsonOut bool) error {
	out := cmd.OutOrStdout()
	if opts.demo {
		return &ExitError{Code: 2, Err: errors.New("self-update is disabled in demo mode")}
	}
	if checkOnly && dryRun {
		return &ExitError{Code: 2, Err: errors.New("--check and --dry-run cannot be used together")}
	}
	// A broken user catalog must not block updating Outfitter itself; the
	// catalog is used only for settings and the optional gh token.
	cat, _ := opts.loadRealCatalog()
	ctx := cmd.Context()

	if checkOnly {
		state, err := opts.selfCheck(ctx, cat, true)
		report := selfUpdateReport{State: state}
		if err != nil {
			report.Error = err.Error()
			if jsonOut {
				_ = writeJSON(out, report)
			}
			return err
		}
		report.Message = selfStateMessage(state)
		if jsonOut {
			return writeJSON(out, report)
		}
		_, err = fmt.Fprintln(out, report.Message)
		return err
	}

	if dryRun {
		result, err := opts.runSelfUpdate(ctx, cat, engine.SelfUpdateOptions{DryRun: true})
		report := reportFromSelfResult(result)
		if err != nil {
			report.Error = err.Error()
			if jsonOut {
				_ = writeJSON(out, report)
			}
			return err
		}
		if jsonOut {
			return writeJSON(out, report)
		}
		return printSelfUpdatePlan(out, report, true)
	}

	state, err := opts.selfCheck(ctx, cat, true)
	if err != nil {
		report := selfUpdateReport{State: state, Error: err.Error()}
		if jsonOut {
			_ = writeJSON(out, report)
		}
		return err
	}
	report := selfUpdateReport{State: state, Command: state.UpdateCommand}
	if !state.Outdated {
		report.Message = selfStateMessage(state)
		if jsonOut {
			return writeJSON(out, report)
		}
		_, err := fmt.Fprintln(out, report.Message)
		return err
	}
	if !state.CanSelfUpdate {
		hint := state.ManualHint
		if hint == "" {
			hint = "download the latest release from " + state.ReleaseURL
		}
		report.Message = hint
		err := fmt.Errorf("%w: %s", engine.ErrSelfUpdateUnsupported, hint)
		report.Error = err.Error()
		if jsonOut {
			_ = writeJSON(out, report)
		}
		return err
	}

	if !yes {
		if jsonOut {
			report.DryRun = true
			report.Message = "self-update requires --yes when --json is used"
			report.Error = report.Message
			_ = writeJSON(out, report)
			return &ExitError{Code: 2, Err: errors.New(report.Message)}
		}
		if err := printSelfUpdatePlan(out, report, false); err != nil {
			return err
		}
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

	result, err := opts.runSelfUpdate(ctx, cat, engine.SelfUpdateOptions{})
	report = reportFromSelfResult(result)
	if err != nil {
		report.Error = err.Error()
		if jsonOut {
			_ = writeJSON(out, report)
		}
		return err
	}
	if jsonOut {
		return writeJSON(out, report)
	}
	if report.Message != "" {
		if _, err := fmt.Fprintln(out, report.Message); err != nil {
			return err
		}
	}
	if report.BackupPath != "" {
		if _, err := fmt.Fprintf(out, "Previous binary kept at %s\n", report.BackupPath); err != nil {
			return err
		}
	}
	return nil
}

func selfStateMessage(state engine.SelfState) string {
	switch {
	case state.Outdated && state.CanSelfUpdate:
		return selfUpdateLine(state)
	case state.Outdated:
		hint := state.ManualHint
		if hint == "" {
			hint = "download it from " + state.ReleaseURL
		}
		return fmt.Sprintf("Outfitter update available: %s → %s; %s", state.CurrentVersion, state.LatestVersion, hint)
	case state.LatestVersion != "":
		return fmt.Sprintf("Outfitter %s is up to date.", state.CurrentVersion)
	case state.InstallKind == engine.SelfInstallDevelopment:
		return "Outfitter development build; self-update checks are disabled."
	case state.LastError != "":
		return "Could not check for an Outfitter update: " + state.LastError
	default:
		return fmt.Sprintf("Outfitter %s; no update check result is available.", state.CurrentVersion)
	}
}

func printSelfUpdatePlan(out io.Writer, report selfUpdateReport, dryRun bool) error {
	state := report.State
	prefix := "Self-update plan"
	if dryRun {
		prefix = "Self-update dry run"
	}
	if _, err := fmt.Fprintf(out, "%s: Outfitter %s → %s\n", prefix, state.CurrentVersion, state.LatestVersion); err != nil {
		return err
	}
	if report.Command != "" {
		if _, err := fmt.Fprintf(out, "Method: %s\n", report.Command); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintln(out, "The running process keeps the old version; restart Outfitter after updating.")
	return err
}
