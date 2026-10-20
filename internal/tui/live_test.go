package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

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
	if !strings.Contains(midView, "Checking upstream versions…") || !strings.Contains(midView, "1/45 checked") {
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
