package tui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/juanrubio/outfitter/internal/adapter"
	"github.com/juanrubio/outfitter/internal/catalog"
	"github.com/juanrubio/outfitter/internal/domain"
	"github.com/juanrubio/outfitter/internal/engine"
)

func drainCommands(t *testing.T, m *Model, commands ...tea.Cmd) {
	t.Helper()
	queue := append([]tea.Cmd(nil), commands...)
	processed := 0
	for len(queue) > 0 {
		processed++
		if processed > 1000 {
			t.Fatal("command queue did not settle")
		}
		cmd := queue[0]
		queue = queue[1:]
		if cmd == nil {
			continue
		}
		msg := cmd()
		if batch, ok := msg.(tea.BatchMsg); ok {
			queue = append(queue, batch...)
			continue
		}
		_, next := m.Update(msg)
		if next != nil {
			queue = append(queue, next)
		}
	}
}

func TestLiveExecutionUsesInjectedRunner(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	cat, err := catalog.LoadDefault()
	if err != nil {
		t.Fatal(err)
	}
	executed := false
	executor := func(
		_ context.Context,
		plan domain.ActionPlan,
		onEvent func(engine.StepEvent),
	) engine.PlanResult {
		executed = true
		steps := plan.Steps()
		results := make([]engine.StepResult, 0, len(steps))
		for index, step := range steps {
			onEvent(engine.StepEvent{
				StepIndex: index, TotalSteps: len(steps), Step: step,
				Status: string(engine.StepStatusRunning),
			})
			onEvent(engine.StepEvent{
				StepIndex: index, TotalSteps: len(steps), Step: step,
				Status: string(engine.StepStatusRunning), Line: "fake adapter output",
			})
			onEvent(engine.StepEvent{
				StepIndex: index, TotalSteps: len(steps), Step: step,
				Status: string(engine.StepStatusDone), Duration: 25 * time.Millisecond,
			})
			results = append(results, engine.StepResult{Step: step, Success: true})
		}
		return engine.PlanResult{
			StartedAt: time.Now(), FinishedAt: time.Now(),
			Duration: time.Millisecond, Steps: results, LogPath: "/tmp/outfitter-run.log",
		}
	}

	m, err := NewModel(Options{
		Catalog: cat,
		Width:   120,
		Height:  40,
		States: map[string]domain.ToolState{
			"claude-code": {InstalledVersion: "1.0.0", LatestVersion: "2.0.0"},
		},
		Actions:       map[string]domain.DesiredAction{"claude-code": domain.ActionUpdate},
		MiseInstalled: true,
		OnExecute:     executor,
	})
	if err != nil {
		t.Fatal(err)
	}
	if m.autoRefresh {
		t.Fatal("explicit states should disable automatic refresh")
	}

	model, cmd := m.beginExecution()
	updated, ok := model.(*Model)
	if !ok {
		t.Fatalf("beginExecution returned %T", model)
	}
	if updated.view != "applying" {
		t.Fatalf("view = %q, want applying", updated.view)
	}
	if cmd == nil {
		t.Fatal("live execution did not return a command")
	}
	drainCommands(t, updated, cmd)

	if !executed {
		t.Fatal("injected OnExecute was not called")
	}
	done, total, failed := updated.executionCounts()
	if done != 1 || total != 1 || failed != 0 {
		t.Fatalf("counts = (%d, %d, %d), want (1, 1, 0)", done, total, failed)
	}
	if updated.lastLogPath != "/tmp/outfitter-run.log" {
		t.Fatalf("last log path = %q", updated.lastLogPath)
	}
	if !strings.Contains(updated.notice, "1 succeeded") {
		t.Fatalf("notice = %q", updated.notice)
	}
}

// TestPlanStrayRemovalToggle verifies [s] in the plan review opts in to
// stray removal: the plan handed to the executor carries RemoveStrays,
// and the finish notice reports the removed strays.
func TestPlanStrayRemovalToggle(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	cat, err := catalog.LoadDefault()
	if err != nil {
		t.Fatal(err)
	}
	var executedPlan domain.ActionPlan
	executor := func(
		_ context.Context, plan domain.ActionPlan, _ func(engine.StepEvent),
	) engine.PlanResult {
		executedPlan = plan
		steps := plan.Steps()
		results := make([]engine.StepResult, 0, len(steps))
		for _, step := range steps {
			results = append(results, engine.StepResult{Step: step, Success: true})
		}
		return engine.PlanResult{
			StartedAt: time.Now(), FinishedAt: time.Now(),
			Steps: results,
			Strays: []engine.StrayResult{
				{ToolID: "lazygit", Path: "/home/test/old/lazygit", Removed: true},
			},
			LogPath: "/tmp/outfitter-run.log",
		}
	}

	m, err := NewModel(Options{
		Catalog: cat,
		Width:   120,
		Height:  40,
		States: map[string]domain.ToolState{
			"lazygit": {
				InstalledVersion: "0.66.1", LatestVersion: "0.66.2",
				BinaryPath: "/home/test/old/lazygit", Provenance: domain.ProvenanceExternal,
			},
		},
		Actions:       map[string]domain.DesiredAction{"lazygit": domain.ActionUpdate},
		MiseInstalled: true,
		OnExecute:     executor,
	})
	if err != nil {
		t.Fatal(err)
	}
	if m.plan.RemoveStrays {
		t.Fatal("RemoveStrays must default to off")
	}

	pressKey(m, 'a', "a")
	if m.view != "plan" {
		t.Fatalf("view = %q, want plan", m.view)
	}
	for _, marker := range []string{"s Remove strays: off", "STRAY BINARIES (⚠1) — left in place"} {
		if !strings.Contains(m.View().Content, marker) {
			t.Fatalf("plan screen missing %q:\n%s", marker, m.View().Content)
		}
	}
	if strings.Contains(m.View().Content, "[s] also remove") {
		t.Fatalf("plan screen still shows inline [s] hint outside footer:\n%s", m.View().Content)
	}

	pressKey(m, 's', "s")
	if !m.plan.RemoveStrays {
		t.Fatal("plan.RemoveStrays = false after [s]")
	}
	for _, marker := range []string{"s Remove strays: on", "STRAY BINARIES (⚠1) — will remove after applying"} {
		if !strings.Contains(m.View().Content, marker) {
			t.Fatalf("plan screen missing %q:\n%s", marker, m.View().Content)
		}
	}

	model, cmd := m.beginExecution()
	updated, ok := model.(*Model)
	if !ok {
		t.Fatalf("beginExecution returned %T", model)
	}
	drainCommands(t, updated, cmd)
	if !executedPlan.RemoveStrays {
		t.Fatal("executed plan did not carry RemoveStrays")
	}
	if !strings.Contains(updated.notice, "removed 1 stray") {
		t.Fatalf("notice = %q, want stray removal summary", updated.notice)
	}
	if updated.removeStrays {
		t.Fatal("removeStrays toggle must reset after execution")
	}
}

// TestLiveEventNeverRegressesTerminalStep covers the stray phase emitting
// line events for a step that already finished: the step must stay done
// so overall progress can reach 100%.
func TestLiveEventNeverRegressesTerminalStep(t *testing.T) {
	step := domain.PlanStep{Tool: domain.Tool{Name: "Lazygit"}}
	m := &Model{execSteps: []executionStep{{step: step, status: StepRunning}}}

	m.applyLiveEvent(engine.StepEvent{
		StepIndex: 0, Step: step,
		Status: string(engine.StepStatusDone), Duration: 1300 * time.Millisecond,
	})
	m.applyLiveEvent(engine.StepEvent{
		StepIndex: 0, Step: step,
		Status: string(engine.StepStatusRunning),
		Line:   "Removed stray binary /home/test/old/lazygit",
	})

	got := m.execSteps[0]
	if got.status != StepDone {
		t.Fatalf("status = %q, want done after late running line event", got.status)
	}
	if got.elapsed == "" {
		t.Fatal("elapsed must survive the late line event")
	}
	found := false
	for _, line := range got.logs {
		if strings.Contains(line, "Removed stray binary") {
			found = true
		}
	}
	if !found {
		t.Fatalf("stray line missing from step logs: %v", got.logs)
	}
	done, total, _ := m.executionCounts()
	if done != 1 || total != 1 {
		t.Fatalf("executionCounts = %d/%d, want 1/1", done, total)
	}
}

// TestLiveStrayRunReachesFullProgress drives the real engine Runner
// (fake command executor, real stray file) through the TUI: after the
// stray phase every step must read done so progress reaches 100%.
func TestLiveStrayRunReachesFullProgress(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	cat, err := catalog.LoadDefault()
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	strayPath := filepath.Join(home, "old", "lazygit")
	if err := os.MkdirAll(filepath.Dir(strayPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(strayPath, []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}

	runner := engine.NewRunner(engine.RunnerOptions{
		HomeDir: home,
		LogDir:  t.TempDir(),
		CommandExecutor: engine.CommandExecutorFunc(func(
			_ context.Context, spec adapter.CommandSpec, onLine func(string),
		) (int, error) {
			onLine("installed " + spec.Display)
			return 0, nil
		}),
		LookPath: func(string) (string, error) { return "/usr/bin/mise", nil },
	})

	m, err := NewModel(Options{
		Catalog: cat,
		Width:   120,
		Height:  40,
		States: map[string]domain.ToolState{
			"lazygit": {
				InstalledVersion: "0.66.1", LatestVersion: "0.66.2",
				BinaryPath: strayPath, Provenance: domain.ProvenanceExternal,
			},
		},
		Actions:       map[string]domain.DesiredAction{"lazygit": domain.ActionUpdate},
		MiseInstalled: true,
		OnExecute: func(ctx context.Context, plan domain.ActionPlan, onEvent func(engine.StepEvent)) engine.PlanResult {
			return runner.Run(ctx, plan, onEvent)
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	pressKey(m, 'a', "a")
	pressKey(m, 's', "s")
	model, cmd := m.beginExecution()
	updated, ok := model.(*Model)
	if !ok {
		t.Fatalf("beginExecution returned %T", model)
	}
	drainCommands(t, updated, cmd)

	done, total, failed := updated.executionCounts()
	if total == 0 || done != total || failed != 0 {
		t.Fatalf("counts = (%d, %d, %d), want all done", done, total, failed)
	}
	for _, step := range updated.execSteps {
		if step.status != StepDone {
			t.Fatalf("step %s status = %q, want done", step.step.Tool.ID, step.status)
		}
	}
	if _, err := os.Stat(strayPath); !os.IsNotExist(err) {
		t.Fatalf("stray still exists after run (stat err = %v)", err)
	}
	if !strings.Contains(updated.notice, "removed 1 stray") {
		t.Fatalf("notice = %q, want stray removal summary", updated.notice)
	}
}

func TestLiveRefreshUsesInjectedInspector(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	cat, err := catalog.LoadDefault()
	if err != nil {
		t.Fatal(err)
	}
	refreshed := false
	m, err := NewModel(Options{
		Catalog: cat,
		Width:   120,
		Height:  40,
		States:  map[string]domain.ToolState{},
		OnRefresh: func(context.Context) (map[string]domain.ToolState, error) {
			refreshed = true
			return map[string]domain.ToolState{
				"fzf": {
					InstalledVersion: "0.62.0",
					LatestVersion:    "0.62.0",
					Provenance:       domain.ProvenanceMise,
				},
			}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	cmd := pressKey(m, 'r', "r")
	if cmd == nil {
		t.Fatal("live refresh did not return a command")
	}
	drainCommands(t, m, cmd)

	if !refreshed {
		t.Fatal("injected OnRefresh was not called")
	}
	if got := m.states["fzf"].InstalledVersion; got != "0.62.0" {
		t.Fatalf("refreshed fzf version = %q", got)
	}
	if !strings.Contains(m.notice, "refreshed") {
		t.Fatalf("notice = %q", m.notice)
	}
}

func TestStartupShowsRefreshProgressAndSpinner(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	t.Setenv("HOME", t.TempDir())
	cat, err := catalog.LoadDefault()
	if err != nil {
		t.Fatal(err)
	}

	m, err := NewModel(Options{
		Catalog: cat,
		Width:   120,
		Height:  40,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !m.refreshing {
		t.Fatal("expected live model without explicit states to start in refreshing state")
	}

	initialView := m.View().Content
	if !strings.Contains(initialView, "Scanning local tools…") {
		t.Fatalf("initial view missing scanning progress bar:\n%s", initialView)
	}
	if !strings.Contains(initialView, "checking…") {
		t.Fatalf("initial view missing per-row checking spinner:\n%s", initialView)
	}

	// Simulate local scan completion and incremental upstream check progress.
	m.Update(refreshProgressMsg{
		phase: "Checking upstream versions…",
		states: map[string]domain.ToolState{
			"claude-code": {
				InstalledVersion: "2.1.289",
				CheckStatus:      domain.StatusChecking,
			},
		},
		total: len(cat.Tools),
	})
	m.Update(refreshProgressMsg{
		phase:  "Checking upstream versions…",
		toolID: "claude-code",
		state: domain.ToolState{
			InstalledVersion: "2.1.289",
			LatestVersion:    "2.1.290",
			CheckStatus:      domain.StatusReady,
		},
		checked: 1,
		total:   len(cat.Tools),
	})
	m.Update(spinnerTickMsg{})

	midView := m.View().Content
	if !strings.Contains(midView, "Checking upstream versions…") || !strings.Contains(midView, "1/48 checked") {
		t.Fatalf("mid-refresh view missing progress counter:\n%s", midView)
	}
	if !strings.Contains(midView, "2.1.289") || !strings.Contains(midView, "→ 2.1.290") {
		t.Fatalf("mid-refresh view did not update claude-code row incrementally:\n%s", midView)
	}

	m.Update(refreshDoneMsg{
		states: map[string]domain.ToolState{
			"claude-code": {
				InstalledVersion: "2.1.289",
				LatestVersion:    "2.1.290",
				CheckStatus:      domain.StatusReady,
			},
		},
	})
	if m.refreshing {
		t.Fatal("expected refreshing to be false after refreshDoneMsg")
	}
}
