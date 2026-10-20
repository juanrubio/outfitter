package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/juanrubio/outfitter/internal/catalog"
	"github.com/juanrubio/outfitter/internal/domain"
	"github.com/juanrubio/outfitter/internal/engine"
)

type mutationOptions struct {
	DryRun       bool
	Yes          bool
	JSON         bool
	RemoveStrays bool
}

type jsonStepResult struct {
	Tool     string               `json:"tool"`
	Action   domain.DesiredAction `json:"action"`
	Command  string               `json:"command"`
	Success  bool                 `json:"success"`
	ExitCode int                  `json:"exit_code"`
	Duration time.Duration        `json:"duration"`
	Error    string               `json:"error,omitempty"`
}

type jsonRunResult struct {
	Plan      domain.ActionPlan `json:"plan"`
	Steps     []jsonStepResult  `json:"steps"`
	Succeeded int               `json:"succeeded"`
	Failed    int               `json:"failed"`
	LogPath   string            `json:"log_path,omitempty"`
}

func executeMutation(
	cmd *cobra.Command,
	opts *options,
	cat *catalog.Catalog,
	states map[string]domain.ToolState,
	actions map[string]domain.DesiredAction,
	mutation mutationOptions,
) error {
	miseInstalled, miseVersion := opts.miseStatus()
	plan := domain.BuildActionPlan(cat.Tools, states, actions, miseInstalled, miseVersion)
	plan.RemoveStrays = mutation.RemoveStrays
	out := cmd.OutOrStdout()

	if mutation.DryRun {
		if mutation.JSON {
			return writeJSON(out, plan)
		}
		return printPlan(out, plan)
	}
	if plan.IsEmpty() {
		if mutation.JSON {
			return writeJSON(out, jsonRunResult{Plan: plan, Steps: []jsonStepResult{}})
		}
		_, err := fmt.Fprintln(out, "Nothing to do; all requested tools are already current.")
		return err
	}

	if !mutation.Yes {
		if mutation.JSON {
			if err := writeJSON(out, plan); err != nil {
				return err
			}
		} else if err := printPlan(out, plan); err != nil {
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

	if !mutation.JSON {
		if err := printPlan(out, plan); err != nil {
			return err
		}
	}

	var result engine.PlanResult
	if opts.demo {
		result = runDemoPlan(cmd.Context(), plan, mutation.JSON, out)
	} else {
		result = opts.deps.RunPlan(cmd.Context(), plan, progressHandler(mutation.JSON, out))
	}

	if mutation.JSON {
		payload := jsonRunResult{
			Plan:      plan,
			Steps:     make([]jsonStepResult, 0, len(result.Steps)),
			Succeeded: result.Succeeded(),
			Failed:    result.Failed(),
			LogPath:   result.LogPath,
		}
		for _, step := range result.Steps {
			entry := jsonStepResult{
				Tool:     step.Step.Tool.ID,
				Action:   step.Step.Action,
				Command:  step.Step.Command,
				Success:  step.Success,
				ExitCode: step.ExitCode,
				Duration: step.Duration,
			}
			if step.Err != nil {
				entry.Error = step.Err.Error()
			}
			payload.Steps = append(payload.Steps, entry)
		}
		if err := writeJSON(out, payload); err != nil {
			return err
		}
	} else if err := printResult(out, result); err != nil {
		return err
	}

	if result.HasFailures() {
		return &ExitError{
			Code: 1,
			Err:  fmt.Errorf("%d of %d steps failed", result.Failed(), len(result.Steps)),
		}
	}
	return nil
}

func printPlan(w io.Writer, plan domain.ActionPlan) error {
	if plan.IsEmpty() {
		_, err := fmt.Fprintln(w, "Plan is empty.")
		return err
	}
	if _, err := fmt.Fprintf(w, "Plan: %d step(s)\n", plan.TotalSteps()); err != nil {
		return err
	}
	for index, step := range plan.Steps() {
		prefix := ""
		if step.Sudo {
			prefix = "[sudo] "
		}
		line := fmt.Sprintf("  %d. %s%s %s", index+1, prefix, step.Action, step.Tool.ID)
		switch step.Action {
		case domain.ActionUpdate:
			if step.FromVersion != "" || step.ToVersion != "" {
				line += fmt.Sprintf(" (%s → %s)", displayVersion(step.FromVersion), displayVersion(step.ToVersion))
			}
		case domain.ActionInstall:
			if step.ToVersion != "" {
				line += " (→ " + step.ToVersion + ")"
			}
		}
		if _, err := fmt.Fprintln(w, line); err != nil {
			return err
		}
		if step.Blocked != "" {
			if _, err := fmt.Fprintf(w, "       warning: cannot %s — %s\n", step.Action, step.Blocked); err != nil {
				return err
			}
			continue
		}
		if _, err := fmt.Fprintf(w, "       $ %s\n", step.Command); err != nil {
			return err
		}
		if step.StrayBinaryPath != "" {
			if plan.RemoveStrays {
				if _, err := fmt.Fprintf(w, "       will remove stray after update: $ %s\n", step.StrayRemovalCommand()); err != nil {
					return err
				}
			} else {
				if _, err := fmt.Fprintf(w, "       warning: unmanaged binary %s stays on disk and may shadow the managed %s install\n",
					step.StrayBinaryPath, step.Tool.Backend); err != nil {
					return err
				}
				if _, err := fmt.Fprintf(w, "       remove it after updating: $ %s\n", step.StrayRemovalCommand()); err != nil {
					return err
				}
			}
		}
	}
	_, err := fmt.Fprintln(w)
	return err
}

func printResult(w io.Writer, result engine.PlanResult) error {
	if _, err := fmt.Fprintf(w, "Summary: %d succeeded, %d failed\n",
		result.Succeeded(), result.Failed()); err != nil {
		return err
	}
	for _, step := range result.Steps {
		if step.Success {
			if _, err := fmt.Fprintf(w, "  ✓ %s %s\n", step.Step.Action, step.Step.Tool.ID); err != nil {
				return err
			}
			continue
		}
		if _, err := fmt.Fprintf(w, "  ✗ %s %s: %v\n", step.Step.Action, step.Step.Tool.ID, step.Err); err != nil {
			return err
		}
	}
	if result.LogPath != "" {
		if _, err := fmt.Fprintf(w, "Log: %s\n", result.LogPath); err != nil {
			return err
		}
	}
	if len(result.Strays) > 0 {
		if _, err := fmt.Fprintln(w, "Stray binaries:"); err != nil {
			return err
		}
		for _, stray := range result.Strays {
			if stray.Removed {
				if _, err := fmt.Fprintf(w, "  ✓ removed %s (%s)\n", stray.Path, stray.ToolID); err != nil {
					return err
				}
			} else if _, err := fmt.Fprintf(w, "  ✗ %s (%s): %v\n", stray.Path, stray.ToolID, stray.Err); err != nil {
				return err
			}
		}
		return nil
	}
	strays := make([]domain.PlanStep, 0, len(result.Steps))
	for _, step := range result.Steps {
		if step.Success && step.Step.StrayBinaryPath != "" {
			strays = append(strays, step.Step)
		}
	}
	if len(strays) > 0 {
		if _, err := fmt.Fprintln(w, "Stray binaries left behind (unmanaged copies that may shadow the updated tools):"); err != nil {
			return err
		}
		for _, step := range strays {
			if _, err := fmt.Fprintf(w, "  %s: $ %s\n", step.StrayBinaryPath, step.StrayRemovalCommand()); err != nil {
				return err
			}
		}
	}
	return nil
}

func displayVersion(version string) string {
	if version == "" {
		return "?"
	}
	return version
}

func promptForConfirmation(cmd *cobra.Command, out io.Writer) (bool, error) {
	if _, err := fmt.Fprint(out, "Proceed? [y/N]: "); err != nil {
		return false, err
	}
	answer, err := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return false, err
	}
	switch strings.ToLower(strings.TrimSpace(answer)) {
	case "y", "yes":
		return true, nil
	default:
		return false, nil
	}
}

func progressHandler(jsonOut bool, out io.Writer) func(engine.StepEvent) {
	if jsonOut {
		return nil
	}
	return func(event engine.StepEvent) {
		if event.Line != "" {
			_, _ = fmt.Fprintf(out, "       %s\n", event.Line)
			return
		}
		switch event.Status {
		case string(engine.StepStatusRunning):
			_, _ = fmt.Fprintf(out, "[%d/%d] %s %s…\n",
				event.StepIndex+1, event.TotalSteps, event.Step.Action, event.Step.Tool.ID)
		case string(engine.StepStatusDone):
			_, _ = fmt.Fprintf(out, "[%d/%d] ✓ %s %s\n",
				event.StepIndex+1, event.TotalSteps, event.Step.Action, event.Step.Tool.ID)
		case string(engine.StepStatusFailed):
			_, _ = fmt.Fprintf(out, "[%d/%d] ✗ %s %s: %v\n",
				event.StepIndex+1, event.TotalSteps, event.Step.Action, event.Step.Tool.ID, event.Err)
		}
	}
}

func runDemoPlan(
	ctx context.Context,
	plan domain.ActionPlan,
	jsonOut bool,
	out io.Writer,
) engine.PlanResult {
	started := time.Now()
	result := engine.PlanResult{StartedAt: started}
	steps := plan.Steps()
	result.Steps = make([]engine.StepResult, 0, len(steps))
	handler := progressHandler(jsonOut, out)

	for index, step := range steps {
		if ctx.Err() != nil {
			break
		}
		if handler != nil {
			handler(engine.StepEvent{
				StepIndex:  index,
				TotalSteps: len(steps),
				Step:       step,
				Status:     string(engine.StepStatusRunning),
				Line:       "$ " + step.Command,
			})
		}
		result.Steps = append(result.Steps, engine.StepResult{
			Step:    step,
			Success: true,
		})
		if handler != nil {
			handler(engine.StepEvent{
				StepIndex:  index,
				TotalSteps: len(steps),
				Step:       step,
				Status:     string(engine.StepStatusDone),
			})
		}
	}
	result.FinishedAt = time.Now()
	result.Duration = result.FinishedAt.Sub(result.StartedAt)
	return result
}
