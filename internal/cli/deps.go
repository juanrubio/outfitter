package cli

import (
	"context"
	"io"
	"os"

	"github.com/juanrubio/outfitter/internal/catalog"
	"github.com/juanrubio/outfitter/internal/domain"
	"github.com/juanrubio/outfitter/internal/engine"
	"github.com/juanrubio/outfitter/internal/tui"
)

// Deps contains the external side effects used by the CLI. Tests inject
// deterministic implementations so the command tree stays fully offline.
type Deps struct {
	HomeDir      string
	IsTTY        func(in io.Reader, out io.Writer) bool
	InspectLocal func(
		ctx context.Context,
		cat *catalog.Catalog,
	) map[string]domain.ToolState
	InspectUpstream func(
		ctx context.Context,
		cat *catalog.Catalog,
		current map[string]domain.ToolState,
		onUpdate func(string, domain.ToolState),
	) map[string]domain.ToolState
	RunPlan func(
		ctx context.Context,
		plan domain.ActionPlan,
		onEvent func(engine.StepEvent),
	) engine.PlanResult
	RunTUI        func(ctx context.Context, opts tui.Options) error
	MiseInstalled func() (string, bool)
	ShellSetup    func() *engine.ShellSetup
}

// DefaultDeps returns production implementations for every CLI dependency.
func DefaultDeps() Deps {
	home, _ := os.UserHomeDir()
	deps := Deps{HomeDir: home}
	deps.IsTTY = defaultIsTTY
	deps.InspectLocal = func(ctx context.Context, cat *catalog.Catalog) map[string]domain.ToolState {
		return engine.NewInspector(deps.HomeDir).InspectLocal(ctx, cat)
	}
	deps.InspectUpstream = func(
		ctx context.Context,
		cat *catalog.Catalog,
		current map[string]domain.ToolState,
		onUpdate func(string, domain.ToolState),
	) map[string]domain.ToolState {
		return engine.NewInspector(deps.HomeDir).InspectUpstream(ctx, cat, current, onUpdate)
	}
	deps.RunPlan = func(
		ctx context.Context,
		plan domain.ActionPlan,
		onEvent func(engine.StepEvent),
	) engine.PlanResult {
		return engine.NewRunner(engine.RunnerOptions{HomeDir: deps.HomeDir}).Run(ctx, plan, onEvent)
	}
	deps.RunTUI = tui.Run
	deps.MiseInstalled = func() (string, bool) {
		return engine.FindMise(nil, deps.HomeDir)
	}
	deps.ShellSetup = func() *engine.ShellSetup {
		return &engine.ShellSetup{HomeDir: deps.HomeDir}
	}
	return deps
}

func (d Deps) withDefaults() Deps {
	defaults := DefaultDeps()
	if d.HomeDir == "" {
		d.HomeDir = defaults.HomeDir
	}
	if d.IsTTY == nil {
		d.IsTTY = defaults.IsTTY
	}
	if d.InspectLocal == nil {
		d.InspectLocal = defaults.InspectLocal
	}
	if d.InspectUpstream == nil {
		d.InspectUpstream = defaults.InspectUpstream
	}
	if d.RunPlan == nil {
		d.RunPlan = defaults.RunPlan
	}
	if d.RunTUI == nil {
		d.RunTUI = defaults.RunTUI
	}
	if d.MiseInstalled == nil {
		d.MiseInstalled = defaults.MiseInstalled
	}
	if d.ShellSetup == nil {
		d.ShellSetup = defaults.ShellSetup
	}
	return d
}

func defaultIsTTY(in io.Reader, out io.Writer) bool {
	return isCharDevice(in) && isCharDevice(out)
}

func isCharDevice(value any) bool {
	file, ok := value.(*os.File)
	if !ok {
		return false
	}
	info, err := file.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}
