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

// TestExecutionReturnToCatalog covers leaving the execution view after a
// run: succeeded tools leave the plan, failed tools stay queued for a
// retry, and the catalog is re-inspected whenever anything succeeded so
// the main screen shows post-update state.
func TestExecutionReturnToCatalog(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	cat, err := catalog.LoadDefault()
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name          string
		outcomes      map[string]bool
		wantActions   map[string]domain.DesiredAction
		wantRefresh   bool
		wantInstalled map[string]string
	}{
		{
			name:        "success resets plan and refreshes",
			outcomes:    map[string]bool{"claude-code": true, "codex": true},
			wantActions: map[string]domain.DesiredAction{},
			wantRefresh: true,
			wantInstalled: map[string]string{
				"claude-code": "2.0.0", "codex": "2.0.0",
			},
		},
		{
			name:        "partial failure keeps failed action and refreshes",
			outcomes:    map[string]bool{"claude-code": true, "codex": false},
			wantActions: map[string]domain.DesiredAction{"codex": domain.ActionUpdate},
			wantRefresh: true,
			wantInstalled: map[string]string{
				"claude-code": "2.0.0", "codex": "1.0.0",
			},
		},
		{
			name:     "all failed keeps plan and skips refresh",
			outcomes: map[string]bool{"claude-code": false, "codex": false},
			wantActions: map[string]domain.DesiredAction{
				"claude-code": domain.ActionUpdate, "codex": domain.ActionUpdate,
			},
			wantRefresh: false,
			wantInstalled: map[string]string{
				"claude-code": "1.0.0", "codex": "1.0.0",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			refreshed := false
			executor := func(
				_ context.Context, plan domain.ActionPlan, onEvent func(engine.StepEvent),
			) engine.PlanResult {
				steps := plan.Steps()
				results := make([]engine.StepResult, 0, len(steps))
				for index, step := range steps {
					success := tc.outcomes[step.Tool.ID]
					status := string(engine.StepStatusDone)
					if !success {
						status = string(engine.StepStatusFailed)
					}
					onEvent(engine.StepEvent{
						StepIndex: index, TotalSteps: len(steps), Step: step,
						Status: status, Duration: time.Millisecond,
					})
					results = append(results, engine.StepResult{Step: step, Success: success})
				}
				return engine.PlanResult{
					StartedAt: time.Now(), FinishedAt: time.Now(), Steps: results,
				}
			}

			m, err := NewModel(Options{
				Catalog: cat,
				Width:   120,
				Height:  40,
				States: map[string]domain.ToolState{
					"claude-code": {InstalledVersion: "1.0.0", LatestVersion: "2.0.0"},
					"codex":       {InstalledVersion: "1.0.0", LatestVersion: "2.0.0"},
				},
				Actions: map[string]domain.DesiredAction{
					"claude-code": domain.ActionUpdate,
					"codex":       domain.ActionUpdate,
				},
				MiseInstalled: true,
				OnExecute:     executor,
				OnRefresh: func(context.Context) (map[string]domain.ToolState, error) {
					refreshed = true
					return map[string]domain.ToolState{
						"claude-code": {InstalledVersion: tc.wantInstalled["claude-code"], LatestVersion: "2.0.0"},
						"codex":       {InstalledVersion: tc.wantInstalled["codex"], LatestVersion: "2.0.0"},
					}, nil
				},
			})
			if err != nil {
				t.Fatal(err)
			}

			_, cmd := m.beginExecution()
			drainCommands(t, m, cmd)
			if refreshed {
				t.Fatal("refresh ran before returning to the catalog")
			}

			cmd = pressKey(m, tea.KeyEscape, "")
			if m.view != "catalog" {
				t.Fatalf("view = %q, want catalog", m.view)
			}
			drainCommands(t, m, cmd)

			if refreshed != tc.wantRefresh {
				t.Fatalf("refreshed = %v, want %v", refreshed, tc.wantRefresh)
			}
			if len(m.actions) != len(tc.wantActions) {
				t.Fatalf("actions = %v, want %v", m.actions, tc.wantActions)
			}
			for id, want := range tc.wantActions {
				if got := m.actions[id]; got != want {
					t.Fatalf("actions[%q] = %q, want %q (all: %v)", id, got, want, m.actions)
				}
			}
			if tc.wantRefresh {
				for id, want := range tc.wantInstalled {
					if got := m.states[id].InstalledVersion; got != want {
						t.Fatalf("states[%q].InstalledVersion = %q, want %q", id, got, want)
					}
				}
				if _, _, _, installs, updates, removals := m.counts(); installs+updates+removals != len(tc.wantActions) {
					t.Fatalf("plan counts = +%d ↑%d -%d, want %d queued actions",
						installs, updates, removals, len(tc.wantActions))
				}
			}
		})
	}
}

// TestExecutionReturnWhileRunningKeepsPlan verifies Esc during a live run
// only leaves the view: the plan stays queued, no refresh starts, and the
// background run keeps its state. The second run also proves a previous
// run's result does not leak into the next execution.
func TestExecutionReturnWhileRunningKeepsPlan(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	cat, err := catalog.LoadDefault()
	if err != nil {
		t.Fatal(err)
	}
	refreshes := 0
	m, err := NewModel(Options{
		Catalog: cat,
		Width:   120,
		Height:  40,
		States: map[string]domain.ToolState{
			"claude-code": {InstalledVersion: "1.0.0", LatestVersion: "2.0.0"},
		},
		Actions:       map[string]domain.DesiredAction{"claude-code": domain.ActionUpdate},
		MiseInstalled: true,
		OnExecute: func(
			_ context.Context, plan domain.ActionPlan, _ func(engine.StepEvent),
		) engine.PlanResult {
			steps := plan.Steps()
			results := make([]engine.StepResult, 0, len(steps))
			for _, step := range steps {
				results = append(results, engine.StepResult{Step: step, Success: true})
			}
			return engine.PlanResult{StartedAt: time.Now(), FinishedAt: time.Now(), Steps: results}
		},
		OnRefresh: func(context.Context) (map[string]domain.ToolState, error) {
			refreshes++
			return map[string]domain.ToolState{
				"claude-code": {InstalledVersion: "2.0.0", LatestVersion: "2.0.0"},
			}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	// First run completes and the user returns to the catalog.
	_, cmd := m.beginExecution()
	drainCommands(t, m, cmd)
	cmd = pressKey(m, tea.KeyEscape, "")
	drainCommands(t, m, cmd)
	if refreshes != 1 {
		t.Fatalf("refreshes after first return = %d, want 1", refreshes)
	}

	// Queue a new action for the same tool and start a second run.
	m.actions["claude-code"] = domain.ActionRemove
	m.rebuildPlan()
	_, cmd = m.beginExecution()
	if cmd == nil {
		t.Fatal("live execution did not return a command")
	}
	// Deliberately do not run cmd: the second execution is still in flight.
	pressKey(m, tea.KeyEscape, "")

	if m.view != "catalog" {
		t.Fatalf("view = %q, want catalog", m.view)
	}
	if got := m.actions["claude-code"]; got != domain.ActionRemove {
		t.Fatalf("actions[claude-code] = %q, want remove still queued", got)
	}
	if refreshes != 1 {
		t.Fatalf("refreshes = %d, want 1 (no refresh while running)", refreshes)
	}
}

// TestExecutionFinishPreservesActionChangedDuringRun verifies that a
// queued action edited while a run continues in the background is not
// deleted when the originally executed action succeeds.
func TestExecutionFinishPreservesActionChangedDuringRun(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	cat, err := catalog.LoadDefault()
	if err != nil {
		t.Fatal(err)
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
		OnExecute: func(
			_ context.Context, plan domain.ActionPlan, _ func(engine.StepEvent),
		) engine.PlanResult {
			steps := plan.Steps()
			results := make([]engine.StepResult, 0, len(steps))
			for _, step := range steps {
				results = append(results, engine.StepResult{Step: step, Success: true})
			}
			return engine.PlanResult{StartedAt: time.Now(), FinishedAt: time.Now(), Steps: results}
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	_, cmd := m.beginExecution()
	// The user queues a removal for the same tool while the update runs.
	m.actions["claude-code"] = domain.ActionRemove
	m.rebuildPlan()
	drainCommands(t, m, cmd)

	if got := m.actions["claude-code"]; got != domain.ActionRemove {
		t.Fatalf("actions[claude-code] = %q, want remove preserved", got)
	}
}

// TestExecutionReturnBeforeLiveResultKeepsPlan verifies that terminal
// step rows alone do not finish a live run: until the runner result
// arrives (post-step cleanup may still be running), Esc keeps the plan
// and starts no refresh.
func TestExecutionReturnBeforeLiveResultKeepsPlan(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	cat, err := catalog.LoadDefault()
	if err != nil {
		t.Fatal(err)
	}
	refreshes := 0
	m, err := NewModel(Options{
		Catalog: cat,
		Width:   120,
		Height:  40,
		States: map[string]domain.ToolState{
			"claude-code": {InstalledVersion: "1.0.0", LatestVersion: "2.0.0"},
		},
		Actions:       map[string]domain.DesiredAction{"claude-code": domain.ActionUpdate},
		MiseInstalled: true,
		OnRefresh: func(context.Context) (map[string]domain.ToolState, error) {
			refreshes++
			return nil, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	m.startExecution()
	m.view = "applying"
	m.applyStepProgress(StepProgressMsg{Index: 0, Status: StepDone, Elapsed: "1.0s"})
	pressKey(m, tea.KeyEscape, "")

	if m.view != "catalog" {
		t.Fatalf("view = %q, want catalog", m.view)
	}
	if got := m.actions["claude-code"]; got != domain.ActionUpdate {
		t.Fatalf("actions[claude-code] = %q, want update still queued", got)
	}
	if refreshes != 0 {
		t.Fatalf("refreshes = %d, want 0 before the live result arrives", refreshes)
	}
}

// TestExecutionReturnDefersRefreshUntilCurrentRefreshEnds verifies that
// returning while another refresh is in flight owes a fresh inspection:
// when that refresh lands with its pre-execution snapshot, a new refresh
// runs so the catalog ends on post-update state.
func TestExecutionReturnDefersRefreshUntilCurrentRefreshEnds(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	cat, err := catalog.LoadDefault()
	if err != nil {
		t.Fatal(err)
	}
	refreshes := 0
	m, err := NewModel(Options{
		Catalog: cat,
		Width:   120,
		Height:  40,
		States: map[string]domain.ToolState{
			"claude-code": {InstalledVersion: "1.0.0", LatestVersion: "2.0.0"},
		},
		Actions:       map[string]domain.DesiredAction{"claude-code": domain.ActionUpdate},
		MiseInstalled: true,
		OnExecute: func(
			_ context.Context, plan domain.ActionPlan, _ func(engine.StepEvent),
		) engine.PlanResult {
			steps := plan.Steps()
			results := make([]engine.StepResult, 0, len(steps))
			for _, step := range steps {
				results = append(results, engine.StepResult{Step: step, Success: true})
			}
			return engine.PlanResult{StartedAt: time.Now(), FinishedAt: time.Now(), Steps: results}
		},
		OnRefresh: func(context.Context) (map[string]domain.ToolState, error) {
			refreshes++
			return map[string]domain.ToolState{
				"claude-code": {InstalledVersion: "2.0.0", LatestVersion: "2.0.0"},
			}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	_, cmd := m.beginExecution()
	drainCommands(t, m, cmd)

	// A refresh started before the execution is still in flight.
	m.refreshing = true
	if cmd := pressKey(m, tea.KeyEscape, ""); cmd != nil {
		t.Fatal("return started a second refresh while one was in flight")
	}
	if refreshes != 0 {
		t.Fatalf("refreshes = %d, want 0 while the first refresh runs", refreshes)
	}

	// The in-flight refresh lands with the pre-execution snapshot; the
	// owed post-execution refresh must now run.
	_, cmd = m.Update(refreshDoneMsg{states: map[string]domain.ToolState{
		"claude-code": {InstalledVersion: "1.0.0", LatestVersion: "2.0.0"},
	}})
	drainCommands(t, m, cmd)

	if refreshes != 1 {
		t.Fatalf("refreshes = %d, want 1 owed refresh after the first lands", refreshes)
	}
	if got := m.states["claude-code"].InstalledVersion; got != "2.0.0" {
		t.Fatalf("installed version = %q, want post-update 2.0.0", got)
	}
}

// TestExecutionReturnWhileRefreshStreamDrains verifies the owed
// post-execution refresh waits for the previous refresh's event stream
// to close: the done message alone must not start it (a late
// stream-closed message would clear the new stream), and the
// stream-closed message must not lose it.
func TestExecutionReturnWhileRefreshStreamDrains(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	cat, err := catalog.LoadDefault()
	if err != nil {
		t.Fatal(err)
	}
	refreshes := 0
	m, err := NewModel(Options{
		Catalog: cat,
		Width:   120,
		Height:  40,
		States: map[string]domain.ToolState{
			"claude-code": {InstalledVersion: "1.0.0", LatestVersion: "2.0.0"},
		},
		Actions:       map[string]domain.DesiredAction{"claude-code": domain.ActionUpdate},
		MiseInstalled: true,
		OnExecute: func(
			_ context.Context, plan domain.ActionPlan, _ func(engine.StepEvent),
		) engine.PlanResult {
			steps := plan.Steps()
			results := make([]engine.StepResult, 0, len(steps))
			for _, step := range steps {
				results = append(results, engine.StepResult{Step: step, Success: true})
			}
			return engine.PlanResult{StartedAt: time.Now(), FinishedAt: time.Now(), Steps: results}
		},
		OnRefresh: func(context.Context) (map[string]domain.ToolState, error) {
			refreshes++
			return map[string]domain.ToolState{
				"claude-code": {InstalledVersion: "2.0.0", LatestVersion: "2.0.0"},
			}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	_, cmd := m.beginExecution()
	drainCommands(t, m, cmd)

	// The previous refresh has finished scanning but its event stream
	// has not closed yet.
	m.refreshEvents = make(chan refreshProgressMsg, 1)
	if cmd := pressKey(m, tea.KeyEscape, ""); cmd != nil {
		t.Fatal("return started a refresh while the old stream was draining")
	}

	// Its done message lands with the pre-execution snapshot; the owed
	// refresh still waits for the stream to close.
	_, cmd = m.Update(refreshDoneMsg{states: map[string]domain.ToolState{
		"claude-code": {InstalledVersion: "1.0.0", LatestVersion: "2.0.0"},
	}})
	if cmd != nil {
		t.Fatal("refresh done started the owed refresh before the stream closed")
	}
	if refreshes != 0 {
		t.Fatalf("refreshes = %d, want 0 until the stream closes", refreshes)
	}

	_, cmd = m.Update(refreshStreamClosedMsg{})
	drainCommands(t, m, cmd)
	if refreshes != 1 {
		t.Fatalf("refreshes = %d, want 1 owed refresh after the stream closes", refreshes)
	}
	if got := m.states["claude-code"].InstalledVersion; got != "2.0.0" {
		t.Fatalf("installed version = %q, want post-update 2.0.0", got)
	}
}

// TestExecutionFinishInBackgroundRefreshesCatalog verifies that a run
// finishing after the user already left the execution view refreshes
// the catalog at finish time: no return-to-catalog moment is left to
// trigger it, so without this the cleared plan would sit next to
// pre-update versions.
func TestExecutionFinishInBackgroundRefreshesCatalog(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	cat, err := catalog.LoadDefault()
	if err != nil {
		t.Fatal(err)
	}
	refreshes := 0
	m, err := NewModel(Options{
		Catalog: cat,
		Width:   120,
		Height:  40,
		States: map[string]domain.ToolState{
			"claude-code": {InstalledVersion: "1.0.0", LatestVersion: "2.0.0"},
		},
		Actions:       map[string]domain.DesiredAction{"claude-code": domain.ActionUpdate},
		MiseInstalled: true,
		OnExecute: func(
			_ context.Context, plan domain.ActionPlan, _ func(engine.StepEvent),
		) engine.PlanResult {
			steps := plan.Steps()
			results := make([]engine.StepResult, 0, len(steps))
			for _, step := range steps {
				results = append(results, engine.StepResult{Step: step, Success: true})
			}
			return engine.PlanResult{StartedAt: time.Now(), FinishedAt: time.Now(), Steps: results}
		},
		OnRefresh: func(context.Context) (map[string]domain.ToolState, error) {
			refreshes++
			return map[string]domain.ToolState{
				"claude-code": {InstalledVersion: "2.0.0", LatestVersion: "2.0.0"},
			}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	_, cmd := m.beginExecution()
	// Leave while the run is in flight, then let it finish.
	pressKey(m, tea.KeyEscape, "")
	if m.view != "catalog" {
		t.Fatalf("view = %q, want catalog", m.view)
	}
	drainCommands(t, m, cmd)

	if refreshes != 1 {
		t.Fatalf("refreshes = %d, want 1 when the background run finishes", refreshes)
	}
	if len(m.actions) != 0 {
		t.Fatalf("actions = %v, want plan reset", m.actions)
	}
	if got := m.states["claude-code"].InstalledVersion; got != "2.0.0" {
		t.Fatalf("installed version = %q, want post-update 2.0.0", got)
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
