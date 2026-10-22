// Package tui implements Outfitter's terminal interface.
package tui

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"
	"golang.org/x/term"

	"github.com/juanrubio/outfitter/internal/adapter"
	"github.com/juanrubio/outfitter/internal/catalog"
	"github.com/juanrubio/outfitter/internal/demo"
	"github.com/juanrubio/outfitter/internal/domain"
	"github.com/juanrubio/outfitter/internal/engine"
	"github.com/juanrubio/outfitter/internal/theme"
)

const (
	defaultWidth  = 120
	defaultHeight = 40
)

type filterMode string

const (
	filterAll         filterMode = "all"
	filterOutdated    filterMode = "outdated"
	filterPending     filterMode = "pending"
	filterInstalled   filterMode = "installed"
	filterUninstalled filterMode = "uninstalled"
)

// StepStatus is the lifecycle state of a live execution step.
type StepStatus string

// Supported execution step states.
const (
	StepQueued  StepStatus = "queued"
	StepRunning StepStatus = "running"
	StepDone    StepStatus = "done"
	StepFailed  StepStatus = "failed"
)

type executionStep struct {
	step    domain.PlanStep
	status  StepStatus
	elapsed string
	logs    []string
}

// StepProgressMsg streams a live execution update into the progress view. The
// same message shape is used by the deterministic demo and the Phase 5 runner.
type StepProgressMsg struct {
	Index   int
	Status  StepStatus
	Elapsed string
	Log     []string
}

type liveStepMsg struct {
	event engine.StepEvent
}

type livePlanDoneMsg struct {
	result engine.PlanResult
}

type liveStreamClosedMsg struct{}

type refreshProgressMsg struct {
	phase   string
	toolID  string
	state   domain.ToolState
	states  map[string]domain.ToolState
	checked int
	total   int
}

type refreshStreamClosedMsg struct{}

type refreshDoneMsg struct {
	states    map[string]domain.ToolState
	selfState *engine.SelfState
	err       error
}

type selfUpdateDoneMsg struct {
	result engine.SelfUpdateResult
	err    error
}

type spinnerTickMsg struct{}

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

type sudoVerifiedMsg struct {
	err error
}

type shadowSudoVerifiedMsg struct {
	err error
}

// Options configures a TUI session. Tests may supply a catalog and state
// directly and render the model without a real terminal. In live mode,
// OnRefresh and OnExecute replace the default engine Inspector and Runner.
type Options struct {
	Demo        bool
	CatalogPath string
	HomeDir     string
	// IgnoreUserCatalog loads only the built-in catalog.
	IgnoreUserCatalog bool
	Width             int
	Height            int
	ThemeMode         theme.Mode
	Catalog           *catalog.Catalog
	States            map[string]domain.ToolState
	Actions           map[string]domain.DesiredAction
	MiseInstalled     bool
	MiseVersion       string
	PlatformNotice    string
	OnRefresh         func(context.Context) (map[string]domain.ToolState, error)
	OnExecute         func(context.Context, domain.ActionPlan, func(engine.StepEvent)) engine.PlanResult
	// ShellWarning, when non-empty, is shown as a persistent header line
	// until the shell fix succeeds. Live launches compute it from the
	// host shell when unset.
	ShellWarning string
	// OnFixShell applies the mise shell-integration fix and returns a
	// human-readable result line. Live launches install a default.
	OnFixShell func() (string, error)
	// OnRemoveShadows deletes shadowed binaries and returns the paths
	// that are gone. Live launches install a default.
	OnRemoveShadows func(paths []string) (removed []string, err error)
	// AppVersion is the running Outfitter build version used for
	// self-update checks. Empty or "dev" disables live self-checks.
	AppVersion string
	// SelfState seeds the known self-update state (tests and cached state).
	SelfState engine.SelfState
	// OnSelfCheck refreshes self-update state during upstream refresh.
	OnSelfCheck func(context.Context) (engine.SelfState, error)
	// OnSelfUpdate installs the available Outfitter update after the
	// user confirms. Live launches install a default.
	OnSelfUpdate func(context.Context) (engine.SelfUpdateResult, error)
}

// Model is the Bubble Tea model for the catalog and inspector layouts.
type Model struct {
	catalog        *catalog.Catalog
	states         map[string]domain.ToolState
	actions        map[string]domain.DesiredAction
	miseInstalled  bool
	miseVersion    string
	platformNotice string
	demo           bool

	width  int
	height int
	mode   theme.Mode
	styles theme.Styles

	selected           string
	collapsed          map[string]bool
	collapseStack      []string
	collapseAnchor     string
	view               string
	scrollOffset       int
	plan               domain.ActionPlan
	notice             string
	filter             filterMode
	query              string
	editing            bool
	quitChoice         bool
	confirmShellFix    bool
	shellWarning       string
	onFixShell         func() (string, error)
	shadowCursor       int
	shadowSelected     map[string]bool
	removeStrays       bool
	pendingShadowSweep bool
	onRemoveShadows    func(paths []string) (removed []string, err error)
	appVersion         string
	homeDir            string
	selfState          engine.SelfState
	selfChoice         bool
	selfUpdating       bool
	selfUpdated        bool
	onSelfCheck        func(context.Context) (engine.SelfState, error)
	onSelfUpdate       func(context.Context) (engine.SelfUpdateResult, error)
	activeProfileID    string
	profileCursor      int
	execSteps          []executionStep

	baseCtx        context.Context
	autoRefresh    bool
	refreshing     bool
	refreshPhase   string
	refreshChecked int
	refreshTotal   int
	spinnerIdx     int
	refreshEvents  chan refreshProgressMsg
	onRefresh      func(context.Context) (map[string]domain.ToolState, error)
	onExecute      func(context.Context, domain.ActionPlan, func(engine.StepEvent)) engine.PlanResult
	execCancel     context.CancelFunc
	liveEvents     chan engine.StepEvent
	liveResult     *engine.PlanResult
	lastLogPath    string
}

// NewModel builds a model with defaults suitable for the offline demo or a
// live catalog. Width and height default to 120×40 when omitted.
func NewModel(opts Options) (*Model, error) {
	width := opts.Width
	if width <= 0 {
		width = defaultWidth
	}
	height := opts.Height
	if height <= 0 {
		height = defaultHeight
	}

	mode := theme.ResolveMode(opts.ThemeMode)
	m := &Model{
		width:           width,
		height:          height,
		mode:            mode,
		styles:          theme.NewStyles(mode),
		states:          cloneStates(opts.States),
		actions:         cloneActions(opts.Actions),
		collapsed:       make(map[string]bool),
		view:            "catalog",
		filter:          filterAll,
		miseInstalled:   opts.MiseInstalled,
		miseVersion:     opts.MiseVersion,
		demo:            opts.Demo,
		activeProfileID: "desktop",
		baseCtx:         context.Background(),
		autoRefresh:     !opts.Demo && opts.States == nil,
		onRefresh:       opts.OnRefresh,
		onExecute:       opts.OnExecute,
		shellWarning:    opts.ShellWarning,
		onFixShell:      opts.OnFixShell,
		onRemoveShadows: opts.OnRemoveShadows,
		appVersion:      opts.AppVersion,
		homeDir:         opts.HomeDir,
		selfState:       opts.SelfState,
		onSelfCheck:     opts.OnSelfCheck,
		onSelfUpdate:    opts.OnSelfUpdate,
	}

	if opts.Demo {
		snapshot, err := demo.Load()
		if err != nil {
			return nil, err
		}
		m.catalog = snapshot.Catalog
		m.states = cloneStates(snapshot.States)
		m.actions = cloneActions(snapshot.Actions)
		m.miseInstalled = snapshot.MiseInstalled
		m.miseVersion = snapshot.MiseVersion
		m.platformNotice = snapshot.PlatformNotice
	}

	if opts.Catalog != nil {
		m.catalog = opts.Catalog
	}
	if opts.States != nil {
		m.states = cloneStates(opts.States)
	}
	if opts.Actions != nil {
		m.actions = cloneActions(opts.Actions)
	}
	if !opts.Demo {
		m.platformNotice = opts.PlatformNotice
	}
	if opts.MiseVersion != "" {
		m.miseVersion = opts.MiseVersion
	}

	if m.catalog == nil {
		cat, err := catalog.LoadWithOptions(catalog.LoadOptions{
			Path:              opts.CatalogPath,
			HomeDir:           opts.HomeDir,
			IgnoreUserCatalog: opts.IgnoreUserCatalog,
		})
		if err != nil {
			return nil, err
		}
		m.catalog = cat
	}
	if m.states == nil {
		m.states = make(map[string]domain.ToolState)
		if m.autoRefresh && m.onRefresh == nil {
			if cachePath := engine.DefaultCachePath(""); cachePath != "" {
				if cached, err := engine.LoadCache(cachePath); err == nil && len(cached.States) > 0 {
					m.states = cloneStates(cached.States)
				}
			}
			_, miseFound := engine.FindMise(nil, "")
			if miseFound {
				m.miseInstalled = true
			}
		}
	}
	if m.actions == nil {
		m.actions = make(map[string]domain.DesiredAction)
	}
	if !opts.Demo && m.autoRefresh && m.onRefresh == nil {
		updater := engine.NewSelfUpdater(m.appVersion, m.homeDir)
		if m.selfState.CurrentVersion == "" {
			m.selfState = updater.CachedState(m.catalog)
		}
		if m.onSelfCheck == nil {
			cat := m.catalog
			m.onSelfCheck = func(ctx context.Context) (engine.SelfState, error) {
				return updater.Check(ctx, cat, false)
			}
		}
		if m.onSelfUpdate == nil {
			cat := m.catalog
			m.onSelfUpdate = func(ctx context.Context) (engine.SelfUpdateResult, error) {
				return updater.Update(ctx, cat, engine.SelfUpdateOptions{})
			}
		}
	}
	if !opts.Demo && m.autoRefresh && m.onRefresh == nil && m.onRemoveShadows == nil {
		m.onRemoveShadows = engine.RemoveShadowedBinaries
	}
	if !opts.Demo && m.autoRefresh && m.onRefresh == nil && m.miseInstalled && m.shellWarning == "" {
		setup := &engine.ShellSetup{}
		if integration := setup.Detect(""); !integration.Integrated {
			m.shellWarning = fmt.Sprintf("⚠ mise is not active in your %s shell — mise-managed tools won't resolve in your terminal · [B] fix shell integration", integration.Shell)
			if m.onFixShell == nil {
				m.onFixShell = func() (string, error) {
					misePath, _ := engine.FindMise(nil, "")
					changed, err := setup.Apply("", misePath, false)
					if err != nil {
						return "", err
					}
					if !changed {
						return "Shell already integrated; nothing changed", nil
					}
					return "Added mise activation to " + integration.RCFile + " — open a new terminal to use it", nil
				}
			}
		}
	}
	if m.miseVersion == "" {
		m.miseVersion = "2026.10.1"
	}
	if m.platformNotice == "" {
		m.platformNotice = engine.DetectPlatformNotice()
	}
	if len(m.catalog.Profiles) == 0 {
		m.activeProfileID = ""
	} else if _, ok := m.catalog.ProfileByID(m.activeProfileID); !ok {
		m.activeProfileID = m.catalog.Profiles[0].ID
	}

	if m.autoRefresh {
		m.beginRefreshState()
	}

	m.rebuildPlan()
	m.selected = m.firstVisibleToolID()
	return m, nil
}

// Run starts the interactive application and restores the terminal on exit.
func Run(ctx context.Context, opts Options) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !term.IsTerminal(int(os.Stdin.Fd())) || !term.IsTerminal(int(os.Stdout.Fd())) {
		return nil
	}
	if opts.Width <= 0 || opts.Height <= 0 {
		if w, h, err := term.GetSize(int(os.Stdout.Fd())); err == nil && w > 0 && h > 0 {
			if opts.Width <= 0 {
				opts.Width = w
			}
			if opts.Height <= 0 {
				opts.Height = h
			}
		}
	}
	m, err := NewModel(opts)
	if err != nil {
		return err
	}
	m.baseCtx = ctx
	env, kitty := programEnv(os.Environ())
	progOpts := []tea.ProgramOption{tea.WithContext(ctx), tea.WithEnvironment(env)}
	if kitty && m.mode != theme.ModeMono {
		progOpts = append(progOpts, tea.WithColorProfile(colorprofile.TrueColor))
	}
	p := tea.NewProgram(m, progOpts...)
	_, err = p.Run()
	return err
}

// programEnv returns a copy of env adapted for the Bubble Tea renderer,
// reporting whether the terminal is Kitty. Ultraviolet enables CSI b (REP,
// repeat previous character) purely from TERM=xterm-kitty, but Kitty 0.16 has
// no REP handler and silently drops the sequence: every REP-compressed run of
// padding spaces or box-drawing characters vanishes and subsequent columns and
// borders shift left. Presenting TERM=xterm-256color keeps the renderer on ECH
// erasure, which Kitty 0.16 implements correctly.
func programEnv(env []string) ([]string, bool) {
	out := make([]string, len(env))
	copy(out, env)
	for index, kv := range out {
		termVal, ok := strings.CutPrefix(kv, "TERM=")
		if !ok {
			continue
		}
		if strings.Contains(termVal, "kitty") {
			out[index] = "TERM=xterm-256color"
			return out, true
		}
		return out, false
	}
	return out, false
}

// Init implements tea.Model. Live sessions with no supplied snapshot start a
// local inspection immediately; the demo remains fully offline.
func (m *Model) Init() tea.Cmd {
	if m.demo || !m.autoRefresh {
		return nil
	}
	return m.startRefresh()
}

// View implements tea.Model.
func (m *Model) View() tea.View {
	content := m.render()
	view := tea.NewView(content)
	view.AltScreen = true
	if m.styles.CanvasColor != nil && m.mode != theme.ModeMono {
		view.BackgroundColor = m.styles.CanvasColor
	}
	return view
}

// Update implements tea.Model.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		if msg.Width > 0 {
			m.width = msg.Width
		}
		if msg.Height > 0 {
			m.height = msg.Height
		}
		return m, nil
	case StepProgressMsg:
		m.applyStepProgress(msg)
		return m, nil
	case liveStepMsg:
		m.applyLiveEvent(msg.event)
		return m, waitForLiveEvent(m.liveEvents)
	case livePlanDoneMsg:
		m.finishLiveExecution(msg.result)
		return m, nil
	case liveStreamClosedMsg:
		m.liveEvents = nil
		return m, nil
	case refreshProgressMsg:
		m.applyRefreshProgress(msg)
		return m, waitForRefreshEvent(m.refreshEvents)
	case refreshStreamClosedMsg:
		m.refreshEvents = nil
		return m, nil
	case refreshDoneMsg:
		m.applyRefresh(msg)
		return m, nil
	case selfUpdateDoneMsg:
		m.applySelfUpdate(msg)
		return m, nil
	case spinnerTickMsg:
		if !m.refreshing {
			return m, nil
		}
		m.spinnerIdx = (m.spinnerIdx + 1) % len(spinnerFrames)
		return m, spinnerTickCmd()
	case sudoVerifiedMsg:
		return m.handleSudoVerified(msg)
	case shadowSudoVerifiedMsg:
		return m.handleShadowSudoVerified(msg)
	case tea.KeyPressMsg:
		return m.updateKey(msg)
	default:
		return m, nil
	}
}

func (m *Model) updateKey(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	keystroke := key.String()
	if keystroke == "ctrl+c" {
		m.cancelExecution()
		return m, tea.Quit
	}
	if m.editing {
		return m.updateSearchKey(key)
	}
	if m.confirmShellFix {
		m.confirmShellFix = false
		if keystroke == "y" {
			m.applyShellFix()
		} else {
			m.notice = "Shell fix cancelled"
		}
		return m, nil
	}
	switch m.view {
	case "quit":
		return m.updateQuitKey(keystroke)
	case "shadows":
		return m.updateShadowsKey(key)
	case "selfupdate":
		return m.updateSelfUpdateKey(keystroke)
	case "profiles":
		return m.updateProfilesKey(key)
	case "plan":
		return m.updatePlanKey(key)
	case "applying":
		switch keystroke {
		case "esc", "q":
			m.view = "catalog"
		case "l":
			if m.lastLogPath != "" {
				m.notice = "Execution log: " + m.lastLogPath
			}
		}
		return m, nil
	case "help":
		if keystroke == "esc" || keystroke == "?" || keystroke == "q" {
			m.view = "catalog"
		}
		return m, nil
	case "detail":
		switch {
		case keystroke == "esc" || keystroke == "q":
			m.view = "catalog"
		case keystroke == " " || keystroke == "space" || key.Key().Code == tea.KeySpace:
			m.toggleSelected()
		case keystroke == "i" || keystroke == "+" || keystroke == "U":
			m.queueFocusedUpdate()
		}
		return m, nil
	}

	if key.Key().Code == tea.KeySpace {
		m.toggleSelected()
		return m, nil
	}

	switch keystroke {
	case "q":
		m.view = "quit"
		m.quitChoice = false
	case "j", "down":
		m.moveSelection(1)
	case "k", "up":
		m.moveSelection(-1)
	case "pgdown":
		m.moveSelection(10)
	case "pgup":
		m.moveSelection(-10)
	case "1", "2", "3", "4":
		m.jumpToCategory(int(keystroke[0] - '0'))
	case "tab":
		m.toggleFocusedCategory()
	case " ", "space":
		m.toggleSelected()
	case "u":
		m.queueAllUpdates()
	case "U", "i", "+":
		m.queueFocusedUpdate()
	case "g":
		m.toggleFocusedCategoryActions()
	case "x":
		m.resetPlan()
	case "f":
		m.cycleFilter()
	case "/":
		m.editing = true
		m.notice = ""
	case "esc":
		if m.query != "" {
			m.query = ""
			m.ensureSelection()
		}
	case "b":
		m.toggleBootstrap()
	case "B":
		if m.shellWarning == "" {
			m.notice = "Shell integration OK (mise active)"
		} else {
			m.confirmShellFix = true
			m.notice = "Add mise activation to your shell rc file? Press y to confirm, any other key to cancel."
		}
	case "S":
		m.openShadows()
	case "O":
		return m.openSelfUpdate()
	case "t":
		m.cycleTheme()
	case "r", "R":
		if m.demo {
			m.notice = "Upstream versions verified just now"
			return m, nil
		}
		return m, m.startRefresh()
	case "c":
		m.notice = m.catalogNotice()
	case "p":
		if len(m.catalog.Profiles) == 0 {
			m.notice = "This catalog defines no profiles; all tools are shown."
			break
		}
		m.view = "profiles"
		m.profileCursor = m.activeProfileIndex()
	case "]":
		m.cycleProfile()
	case "a":
		m.view = "plan"
	case "?":
		m.view = "help"
	case "enter":
		if m.width < 100 {
			m.view = "detail"
		} else {
			m.view = "plan"
		}
	}
	return m, nil
}

func (m *Model) updateSearchKey(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "esc":
		m.editing = false
		m.query = ""
	case "enter":
		m.editing = false
	case "backspace":
		runes := []rune(m.query)
		if len(runes) > 0 {
			m.query = string(runes[:len(runes)-1])
		}
	default:
		text := key.Key().Text
		if text == "" && key.Key().Code == tea.KeySpace {
			text = " "
		}
		if text != "" && !key.Key().Mod.Contains(tea.ModCtrl) && !key.Key().Mod.Contains(tea.ModAlt) {
			m.query += text
		}
	}
	m.ensureSelection()
	return m, nil
}

// updateQuitKey drives the Quit Outfitter? modal: arrows/Tab choose
// between Cancel (default) and Quit, Enter confirms, y quits at once,
// Esc/n stays. Mirrors the sibling Boardwalk/Trailmark quit modals.
func (m *Model) updateQuitKey(keystroke string) (tea.Model, tea.Cmd) {
	switch keystroke {
	case "esc", "n":
		m.view = "catalog"
	case "left", "right", "up", "down", "tab", "shift+tab":
		m.quitChoice = !m.quitChoice
	case "y":
		m.cancelExecution()
		return m, tea.Quit
	case "enter":
		if m.quitChoice {
			m.cancelExecution()
			return m, tea.Quit
		}
		m.view = "catalog"
	}
	return m, nil
}

// selfUpdateWarning is the persistent header line shown while a newer
// Outfitter release is known and not yet installed by this session.
func (m *Model) selfUpdateWarning() string {
	if m.demo || m.selfUpdated || !m.selfState.Outdated {
		return ""
	}
	if m.selfUpdating {
		return fmt.Sprintf("⬆ Updating Outfitter to %s…", m.selfState.LatestVersion)
	}
	return fmt.Sprintf("⬆ Outfitter %s available (you have %s) · [O] update Outfitter",
		m.selfState.LatestVersion, m.selfState.CurrentVersion)
}

func (m *Model) openSelfUpdate() (tea.Model, tea.Cmd) {
	if m.selfUpdating {
		m.notice = "Outfitter update already running…"
		return m, nil
	}
	if m.selfUpdated {
		m.notice = "Outfitter updated — restart Outfitter to run the new version"
		return m, nil
	}
	if !m.selfState.Outdated {
		if m.selfState.CurrentVersion != "" {
			m.notice = "Outfitter " + m.selfState.CurrentVersion + " is up to date"
		} else {
			m.notice = "No Outfitter update available"
		}
		return m, nil
	}
	if !m.selfState.CanSelfUpdate {
		hint := m.selfState.ManualHint
		if hint == "" {
			hint = "download the latest release manually"
		}
		m.notice = fmt.Sprintf("Outfitter %s is available; %s", m.selfState.LatestVersion, hint)
		return m, nil
	}
	m.view = "selfupdate"
	m.selfChoice = false
	return m, nil
}

// updateSelfUpdateKey drives the Update Outfitter? modal, mirroring the
// quit modal: Cancel is the safe default.
func (m *Model) updateSelfUpdateKey(keystroke string) (tea.Model, tea.Cmd) {
	switch keystroke {
	case "esc", "n":
		m.view = "catalog"
		m.notice = "Outfitter update cancelled"
	case "left", "right", "up", "down", "tab", "shift+tab":
		m.selfChoice = !m.selfChoice
	case "y":
		return m.beginSelfUpdate()
	case "enter":
		if m.selfChoice {
			return m.beginSelfUpdate()
		}
		m.view = "catalog"
		m.notice = "Outfitter update cancelled"
	}
	return m, nil
}

func (m *Model) beginSelfUpdate() (tea.Model, tea.Cmd) {
	m.view = "catalog"
	if m.onSelfUpdate == nil {
		m.notice = "Self-update is unavailable in this session; run: outfitter self-update"
		return m, nil
	}
	m.selfUpdating = true
	m.notice = "Updating Outfitter…"
	ctx := m.baseCtx
	if ctx == nil {
		ctx = context.Background()
	}
	update := m.onSelfUpdate
	return m, func() tea.Msg {
		result, err := update(ctx)
		return selfUpdateDoneMsg{result: result, err: err}
	}
}

func (m *Model) applySelfUpdate(msg selfUpdateDoneMsg) {
	m.selfUpdating = false
	if msg.err != nil {
		m.notice = "Outfitter update failed: " + msg.err.Error()
		return
	}
	if msg.result.State.CurrentVersion != "" {
		m.selfState = msg.result.State
	}
	if msg.result.Updated {
		m.selfUpdated = true
	}
	if msg.result.Message != "" {
		m.notice = msg.result.Message
	} else {
		m.notice = "Outfitter update completed"
	}
}

func (m *Model) updateProfilesKey(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	keystroke := key.String()
	switch {
	case keystroke == "esc" || keystroke == "p" || keystroke == "q":
		m.view = "catalog"
	case keystroke == "j" || keystroke == "down":
		if len(m.catalog.Profiles) > 0 {
			m.profileCursor = (m.profileCursor + 1) % len(m.catalog.Profiles)
		}
	case keystroke == "k" || keystroke == "up":
		if len(m.catalog.Profiles) > 0 {
			m.profileCursor = (m.profileCursor + len(m.catalog.Profiles) - 1) % len(m.catalog.Profiles)
		}
	case keystroke == "enter":
		m.applySelectedProfile()
	case len(keystroke) == 1 && keystroke[0] >= '1' && keystroke[0] <= '9':
		index := int(keystroke[0] - '1')
		if index < len(m.catalog.Profiles) {
			m.profileCursor = index
			m.applySelectedProfile()
		}
	}
	return m, nil
}

func (m *Model) updatePlanKey(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "esc", "n", "q":
		m.view = "catalog"
	case "enter", "y":
		return m.beginExecution()
	case "s":
		if len(m.plan.StraySteps()) == 0 {
			m.notice = "No stray binaries in this plan"
		} else {
			m.removeStrays = !m.removeStrays
			m.rebuildPlan()
			if m.removeStrays {
				m.notice = "Stray binaries will be removed after applying"
			} else {
				m.notice = "Stray binaries will be kept"
			}
		}
	case "u":
		m.queueAllUpdates()
	}
	return m, nil
}

func (m *Model) firstVisibleToolID() string {
	ids := m.visibleToolIDs()
	if len(ids) == 0 {
		return ""
	}
	return ids[0]
}

func (m *Model) selectedTool() (domain.Tool, bool) {
	if m.selected == "" {
		return domain.Tool{}, false
	}
	return m.catalog.ToolByID(m.selected)
}

func (m *Model) visibleToolIDs() []string {
	ids := make([]string, 0, len(m.catalog.Tools))
	for _, category := range m.catalog.Categories {
		if m.collapsed[category.ID] {
			continue
		}
		for _, tool := range m.catalog.ToolsByCategory(category.ID) {
			if m.toolMatches(tool) {
				ids = append(ids, tool.ID)
			}
		}
	}
	return ids
}

func (m *Model) toolMatches(tool domain.Tool) bool {
	state := m.states[tool.ID]
	action := m.actions[tool.ID]
	switch m.filter {
	case filterOutdated:
		if !state.IsOutdated() {
			return false
		}
	case filterPending:
		if action == "" || action == domain.ActionNone {
			return false
		}
	case filterInstalled:
		if !state.IsInstalled() {
			return false
		}
	case filterUninstalled:
		if state.IsInstalled() {
			return false
		}
	case filterAll:
	}
	if m.query == "" {
		return true
	}
	query := strings.ToLower(m.query)
	haystack := strings.ToLower(strings.Join([]string{
		tool.Name,
		tool.Binary,
		string(tool.Backend),
		tool.Package,
		tool.Description,
	}, " "))
	return strings.Contains(haystack, query)
}

func (m *Model) ensureSelection() {
	ids := m.visibleToolIDs()
	if len(ids) == 0 {
		m.selected = ""
		return
	}
	for _, id := range ids {
		if id == m.selected {
			return
		}
	}
	m.selected = ids[0]
}

func (m *Model) moveSelection(delta int) {
	ids := m.visibleToolIDs()
	if len(ids) == 0 {
		m.selected = ""
		return
	}
	index := 0
	for i, id := range ids {
		if id == m.selected {
			index = i
			break
		}
	}
	index += delta
	if index < 0 {
		index = 0
	}
	if index >= len(ids) {
		index = len(ids) - 1
	}
	m.selected = ids[index]
}

// expandCategory reopens a collapsed category, drops it from the Tab
// stack, and selects its first visible tool when one matches.
func (m *Model) expandCategory(id string) {
	m.collapsed[id] = false
	for index, stacked := range m.collapseStack {
		if stacked == id {
			m.collapseStack = append(m.collapseStack[:index], m.collapseStack[index+1:]...)
			break
		}
	}
	for _, category := range m.catalog.Categories {
		if category.ID != id {
			continue
		}
		for _, tool := range m.catalog.ToolsByCategory(category.ID) {
			if m.toolMatches(tool) {
				m.selected = tool.ID
				return
			}
		}
	}
}

func (m *Model) jumpToCategory(num int) {
	m.collapseAnchor = ""
	for _, category := range m.catalog.Categories {
		if category.Num == num {
			m.expandCategory(category.ID)
			return
		}
	}
}

func (m *Model) toggleSelected() {
	tool, ok := m.selectedTool()
	if !ok {
		return
	}
	state := m.states[tool.ID]
	current := m.actions[tool.ID]
	next := domain.NextAction(current, state.IsInstalled(), state.IsOutdated())
	if next == domain.ActionNone {
		delete(m.actions, tool.ID)
	} else {
		m.actions[tool.ID] = next
	}
	_, _, label := domain.Marker(next, state.IsInstalled(), state.IsOutdated())
	m.notice = tool.Name + ": " + label
	m.rebuildPlan()
	m.ensureSelection()
}

// toggleFocusedCategory implements the Tab stack: Tab collapses the
// focused category and pushes it on a stack; pressing Tab again before
// navigating away pops the stack and reopens the most recently
// collapsed category (LIFO). Navigating away (selection moved off the
// collapse anchor) disarms the pop, so Tab collapses the newly focused
// category instead.
func (m *Model) toggleFocusedCategory() {
	if top := len(m.collapseStack) - 1; top >= 0 && m.collapseAnchor != "" && m.selected == m.collapseAnchor {
		id := m.collapseStack[top]
		m.collapseStack = m.collapseStack[:top]
		m.collapseAnchor = ""
		m.expandCategory(id)
		m.ensureSelection()
		return
	}
	tool, ok := m.selectedTool()
	if !ok {
		return
	}
	m.collapsed[tool.CategoryID] = true
	m.collapseStack = append(m.collapseStack, tool.CategoryID)
	m.selected = m.firstVisibleToolID()
	m.collapseAnchor = m.selected
	m.ensureSelection()
}

func (m *Model) queueAllUpdates() {
	queued := 0
	for _, tool := range m.catalog.Tools {
		state := m.states[tool.ID]
		if state.IsOutdated() && m.actions[tool.ID] != domain.ActionUpdate {
			m.actions[tool.ID] = domain.ActionUpdate
			queued++
		}
	}
	if queued == 0 {
		m.notice = "All outdated tools already queued"
	} else {
		m.notice = fmt.Sprintf("Queued %d updates", queued)
	}
	m.rebuildPlan()
	m.ensureSelection()
}

func (m *Model) queueFocusedUpdate() {
	tool, ok := m.selectedTool()
	if !ok {
		return
	}
	state := m.states[tool.ID]
	switch {
	case !state.IsInstalled():
		m.actions[tool.ID] = domain.ActionInstall
		m.notice = tool.Name + ": Install queued"
	case state.IsOutdated():
		m.actions[tool.ID] = domain.ActionUpdate
		m.notice = tool.Name + ": Update queued"
	default:
		m.notice = tool.Name + ": Already up to date"
	}
	m.rebuildPlan()
	m.ensureSelection()
}

func (m *Model) toggleFocusedCategoryActions() {
	tool, ok := m.selectedTool()
	if !ok {
		return
	}
	tools := m.catalog.ToolsByCategory(tool.CategoryID)
	anyUnqueued := false
	for _, item := range tools {
		state := m.states[item.ID]
		action := m.actions[item.ID]
		if (!state.IsInstalled() && action != domain.ActionInstall) ||
			(state.IsOutdated() && action != domain.ActionUpdate) {
			anyUnqueued = true
			break
		}
	}

	categoryTitle := tool.CategoryID
	for _, category := range m.catalog.Categories {
		if category.ID == tool.CategoryID {
			categoryTitle = category.Title
			break
		}
	}

	if anyUnqueued {
		for _, item := range tools {
			state := m.states[item.ID]
			switch {
			case !state.IsInstalled():
				m.actions[item.ID] = domain.ActionInstall
			case state.IsOutdated():
				m.actions[item.ID] = domain.ActionUpdate
			}
		}
		m.notice = "Queued category " + categoryTitle
	} else {
		for _, item := range tools {
			delete(m.actions, item.ID)
		}
		m.notice = "Cleared queued changes in " + categoryTitle
	}
	m.rebuildPlan()
	m.ensureSelection()
}

func (m *Model) resetPlan() {
	m.actions = make(map[string]domain.DesiredAction)
	m.rebuildPlan()
	m.notice = "Cleared all pending plan actions"
	m.ensureSelection()
}

func (m *Model) cycleFilter() {
	order := []filterMode{filterAll, filterOutdated, filterPending, filterInstalled, filterUninstalled}
	index := 0
	for i, mode := range order {
		if mode == m.filter {
			index = i
			break
		}
	}
	m.filter = order[(index+1)%len(order)]
	m.scrollOffset = 0
	m.ensureSelection()
}

type shadowEntry struct {
	tool  domain.Tool
	state domain.ToolState
}

// shadowEntries returns shadowed tools in catalog order for display and
// selective removal.
func (m *Model) shadowEntries() []shadowEntry {
	entries := make([]shadowEntry, 0)
	for _, tool := range m.catalog.Tools {
		if state := m.states[tool.ID]; state.ShadowedBy != "" {
			entries = append(entries, shadowEntry{tool: tool, state: state})
		}
	}
	return entries
}

func (m *Model) openShadows() {
	entries := m.shadowEntries()
	if len(entries) == 0 {
		m.notice = "No shadowed binaries"
		return
	}
	m.shadowSelected = make(map[string]bool, len(entries))
	for _, entry := range entries {
		m.shadowSelected[entry.tool.ID] = true
	}
	m.shadowCursor = 0
	m.view = "shadows"
	m.notice = ""
}

func (m *Model) shadowSelectedFor(id string) bool {
	if m.shadowSelected == nil {
		return true
	}
	return m.shadowSelected[id]
}

func (m *Model) selectedShadowEntries() []shadowEntry {
	selected := make([]shadowEntry, 0)
	for _, entry := range m.shadowEntries() {
		if m.shadowSelectedFor(entry.tool.ID) {
			selected = append(selected, entry)
		}
	}
	return selected
}

func (m *Model) selectedShadowPaths() []string {
	seen := make(map[string]bool)
	paths := make([]string, 0)
	for _, entry := range m.selectedShadowEntries() {
		if !seen[entry.state.ShadowedBy] {
			seen[entry.state.ShadowedBy] = true
			paths = append(paths, entry.state.ShadowedBy)
		}
	}
	return paths
}

// updateShadowsKey drives the Shadowed Binaries selection modal.
func (m *Model) updateShadowsKey(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	entries := m.shadowEntries()
	if len(entries) == 0 {
		m.view = "catalog"
		m.notice = "No shadowed binaries"
		return m, nil
	}
	if m.shadowCursor < 0 {
		m.shadowCursor = 0
	}
	if m.shadowCursor >= len(entries) {
		m.shadowCursor = len(entries) - 1
	}
	switch key.String() {
	case "esc", "q", "n":
		m.view = "catalog"
		m.notice = "Shadow sweep cancelled"
	case "j", "down":
		m.shadowCursor = (m.shadowCursor + 1) % len(entries)
	case "k", "up":
		m.shadowCursor = (m.shadowCursor - 1 + len(entries)) % len(entries)
	case "pgdown":
		m.shadowCursor = minInt(m.shadowCursor+5, len(entries)-1)
	case "pgup":
		m.shadowCursor = maxInt(m.shadowCursor-5, 0)
	case "a":
		selectAll := false
		for _, entry := range entries {
			if !m.shadowSelectedFor(entry.tool.ID) {
				selectAll = true
				break
			}
		}
		if m.shadowSelected == nil {
			m.shadowSelected = make(map[string]bool, len(entries))
		}
		for _, entry := range entries {
			m.shadowSelected[entry.tool.ID] = selectAll
		}
	case "enter", "y":
		if len(m.selectedShadowEntries()) == 0 {
			m.notice = "Select at least one shadowed binary to remove"
			return m, nil
		}
		m.view = "catalog"
		return m.beginShadowSweep()
	default:
		if key.String() == " " || key.String() == "space" || key.Key().Code == tea.KeySpace {
			id := entries[m.shadowCursor].tool.ID
			if m.shadowSelected == nil {
				m.shadowSelected = make(map[string]bool, len(entries))
				for _, entry := range entries {
					m.shadowSelected[entry.tool.ID] = true
				}
			}
			m.shadowSelected[id] = !m.shadowSelected[id]
		}
	}
	return m, nil
}

// shadowedPaths returns the shadow binary paths in catalog order.
func (m *Model) shadowedPaths() []string {
	var paths []string
	for _, tool := range m.catalog.Tools {
		if state := m.states[tool.ID]; state.ShadowedBy != "" {
			paths = append(paths, state.ShadowedBy)
		}
	}
	return paths
}

// beginShadowSweep runs the confirmed selective sweep, detouring through
// the sudo pre-flight when any selected shadow lives in a system directory.
func (m *Model) beginShadowSweep() (tea.Model, tea.Cmd) {
	for _, path := range m.selectedShadowPaths() {
		if domain.RemovalNeedsSudo(path) {
			m.pendingShadowSweep = true
			m.notice = "Verifying sudo credentials before removing shadowed binaries…"
			return m, tea.ExecProcess(adapter.VerifySudoCommand(), func(err error) tea.Msg {
				return shadowSudoVerifiedMsg{err: err}
			})
		}
	}
	m.runShadowSweep()
	return m, nil
}

func (m *Model) handleShadowSudoVerified(msg shadowSudoVerifiedMsg) (tea.Model, tea.Cmd) {
	if !m.pendingShadowSweep {
		return m, nil
	}
	m.pendingShadowSweep = false
	if msg.err != nil {
		m.notice = "sudo verification failed: " + msg.err.Error()
		return m, nil
	}
	m.runShadowSweep()
	return m, nil
}

// runShadowSweep deletes the selected shadowed binaries via the
// configured remover and clears markers only for paths that are gone;
// unchecked shadows remain reported.
func (m *Model) runShadowSweep() {
	paths := m.selectedShadowPaths()
	if len(paths) == 0 {
		m.notice = "Select at least one shadowed binary to remove"
		return
	}
	if m.onRemoveShadows == nil {
		m.notice = "Remove manually: $ " + domain.RemovalCommand(paths[0])
		return
	}
	removed, err := m.onRemoveShadows(paths)
	gone := make(map[string]bool, len(removed))
	for _, path := range removed {
		gone[path] = true
	}
	for id, state := range m.states {
		if gone[state.ShadowedBy] {
			state.ShadowedBy = ""
			state.ShadowVersion = ""
			m.states[id] = state
		}
	}
	if err != nil {
		m.notice = fmt.Sprintf("Removed %d of %d shadowed binaries; %v", len(removed), len(paths), err)
		return
	}
	noun := "binaries"
	if len(removed) == 1 {
		noun = "binary"
	}
	m.notice = fmt.Sprintf("Removed %d shadowed %s", len(removed), noun)
}

// applyShellFix runs the confirmed shell-integration fix and clears the
// persistent warning on success.
func (m *Model) applyShellFix() {
	if m.onFixShell == nil {
		m.notice = "Run: outfitter doctor --fix"
		return
	}
	msg, err := m.onFixShell()
	if err != nil {
		m.notice = "Shell fix failed: " + err.Error()
		return
	}
	m.shellWarning = ""
	m.notice = msg
}

func (m *Model) toggleBootstrap() {
	if !m.demo && m.miseInstalled {
		m.notice = "mise is installed; verified bootstrap is not required"
		return
	}
	m.miseInstalled = !m.miseInstalled
	if m.miseInstalled {
		m.notice = "mise marked installed"
	} else {
		m.notice = "! mise is not installed on this machine. Outfitter will auto-bootstrap ~/.local/bin/mise (SHA-256 verified) on Apply [a] or [b]."
	}
	m.rebuildPlan()
	m.ensureSelection()
}

func (m *Model) cycleTheme() {
	order := []theme.Mode{theme.ModeDark, theme.ModeLight, theme.ModeMono}
	next := theme.ModeDark
	for index, mode := range order {
		if mode == m.mode {
			next = order[(index+1)%len(order)]
			break
		}
	}
	m.mode = next
	m.styles = theme.NewStyles(next)
	m.notice = "Theme: " + string(next)
}

// catalogNotice describes the effective catalog for the `c` key.
func (m *Model) catalogNotice() string {
	meta := m.catalog.Meta
	counts := fmt.Sprintf("%d tools · %d categories · %d profiles",
		len(m.catalog.Tools), len(m.catalog.Categories), len(m.catalog.Profiles))
	switch {
	case m.demo || meta.Source == catalog.SourceDemo:
		return fmt.Sprintf("Catalog: demo snapshot (%s)", counts)
	case meta.Source == catalog.SourceUser && meta.Mode == catalog.ModeReplace:
		return fmt.Sprintf("Catalog: replace · %s · %s (%s)", meta.EffectiveName(), meta.Path, counts)
	case meta.Source == catalog.SourceUser:
		return fmt.Sprintf("Catalog: built-in extended by %s · %s (%s)", meta.EffectiveName(), meta.Path, counts)
	default:
		return fmt.Sprintf("Catalog: built-in (%s)", counts)
	}
}

// catalogLabel is the compact header label for user catalogs. It is empty
// for the built-in catalog so the default header is unchanged.
func (m *Model) catalogLabel() string {
	meta := m.catalog.Meta
	switch {
	case m.demo || meta.Source == catalog.SourceDemo:
		return "demo"
	case meta.Source != catalog.SourceUser:
		return ""
	case meta.Mode == catalog.ModeReplace:
		if meta.Name != "" {
			return "replace · " + meta.Name
		}
		return "replace"
	default:
		if meta.Name != "" {
			return "extend · " + meta.Name
		}
		return "extend"
	}
}

func (m *Model) cycleProfile() {
	if len(m.catalog.Profiles) == 0 {
		m.notice = "This catalog defines no profiles; all tools are shown."
		return
	}
	m.profileCursor = (m.activeProfileIndex() + 1) % len(m.catalog.Profiles)
	m.applySelectedProfile()
}

func (m *Model) activeProfileIndex() int {
	for index, profile := range m.catalog.Profiles {
		if profile.ID == m.activeProfileID {
			return index
		}
	}
	return 0
}

func (m *Model) applySelectedProfile() {
	if len(m.catalog.Profiles) == 0 {
		return
	}
	if m.profileCursor < 0 || m.profileCursor >= len(m.catalog.Profiles) {
		m.profileCursor = 0
	}
	profile := m.catalog.Profiles[m.profileCursor]
	members := make(map[string]struct{}, len(profile.Tools))
	for _, id := range profile.Tools {
		members[id] = struct{}{}
	}
	added := 0
	for _, tool := range m.catalog.Tools {
		if _, ok := members[tool.ID]; ok {
			state := m.states[tool.ID]
			switch {
			case !state.IsInstalled():
				m.actions[tool.ID] = domain.ActionInstall
				added++
			case state.IsOutdated():
				m.actions[tool.ID] = domain.ActionUpdate
			default:
				delete(m.actions, tool.ID)
			}
			continue
		}
		if !m.states[tool.ID].IsInstalled() {
			delete(m.actions, tool.ID)
		}
	}
	m.activeProfileID = profile.ID
	m.notice = fmt.Sprintf("Profile '%s' loaded (+%d to install)", profile.Name, added)
	m.rebuildPlan()
	m.ensureSelection()
	m.view = "catalog"
}

func (m *Model) rebuildPlan() {
	actions := m.actions
	if !m.miseInstalled {
		actions = cloneActions(m.actions)
		actions["mise"] = domain.ActionInstall
	}
	m.plan = domain.BuildActionPlan(m.catalog.Tools, m.states, actions, m.miseInstalled, m.miseVersion)
	m.plan.RemoveStrays = m.removeStrays
}

func (m *Model) startExecution() {
	steps := m.plan.Steps()
	m.execSteps = make([]executionStep, 0, len(steps))
	for index, step := range steps {
		status := StepQueued
		elapsed := "queued"
		if index == 0 {
			status = StepRunning
			elapsed = "running…"
		}
		m.execSteps = append(m.execSteps, executionStep{
			step:    step,
			status:  status,
			elapsed: elapsed,
			logs:    []string{"$ " + step.Command},
		})
	}
}

// beginExecution prepares the progress rows and either advances the offline
// demo or launches the live runner (after the sudo pre-flight when required).
func (m *Model) beginExecution() (tea.Model, tea.Cmd) {
	m.startExecution()
	m.view = "applying"
	if m.demo || len(m.execSteps) == 0 {
		return m, nil
	}
	if m.plan.NeedsSudo() {
		m.notice = "Verifying sudo credentials before running apt steps…"
		return m, tea.ExecProcess(adapter.VerifySudoCommand(), func(err error) tea.Msg {
			return sudoVerifiedMsg{err: err}
		})
	}
	return m, m.startLiveExecution()
}

func (m *Model) handleSudoVerified(msg sudoVerifiedMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.view = "plan"
		m.notice = "sudo verification failed: " + msg.err.Error()
		return m, nil
	}
	m.view = "applying"
	m.notice = "sudo verified"
	return m, m.startLiveExecution()
}

func (m *Model) startLiveExecution() tea.Cmd {
	if m.liveEvents != nil {
		return nil
	}
	ctx := m.baseCtx
	if ctx == nil {
		ctx = context.Background()
	}
	runCtx, cancel := context.WithCancel(ctx)
	m.execCancel = cancel

	events := make(chan engine.StepEvent, 256)
	m.liveEvents = events
	plan := m.plan
	execute := m.onExecute
	if execute == nil {
		execute = func(
			ctx context.Context,
			plan domain.ActionPlan,
			onEvent func(engine.StepEvent),
		) engine.PlanResult {
			return engine.NewRunner(engine.RunnerOptions{}).Run(ctx, plan, onEvent)
		}
	}

	runCmd := func() tea.Msg {
		result := execute(runCtx, plan, func(event engine.StepEvent) {
			select {
			case events <- event:
			case <-runCtx.Done():
			}
		})
		cancel()
		close(events)
		return livePlanDoneMsg{result: result}
	}
	return tea.Batch(runCmd, waitForLiveEvent(events))
}

func waitForLiveEvent(events <-chan engine.StepEvent) tea.Cmd {
	if events == nil {
		return nil
	}
	return func() tea.Msg {
		event, ok := <-events
		if !ok {
			return liveStreamClosedMsg{}
		}
		return liveStepMsg{event: event}
	}
}

func (m *Model) applyLiveEvent(event engine.StepEvent) {
	status := StepRunning
	switch event.Status {
	case string(engine.StepStatusDone):
		status = StepDone
	case string(engine.StepStatusFailed):
		status = StepFailed
	}

	logs := make([]string, 0, 2)
	if event.Line != "" {
		logs = append(logs, event.Line)
	}
	elapsed := ""
	if status == StepDone || status == StepFailed {
		elapsed = formatStepDuration(event.Duration)
	}
	if status == StepDone && event.Line == "" {
		logs = append(logs, "✓ "+event.Step.Tool.Name+" completed")
	}
	if status == StepFailed {
		message := "✗ " + event.Step.Tool.Name + " failed"
		if event.Err != nil {
			message += ": " + event.Err.Error()
		}
		logs = append(logs, message)
	}

	m.applyStepProgress(StepProgressMsg{
		Index:   event.StepIndex,
		Status:  status,
		Elapsed: elapsed,
		Log:     logs,
	})
}

func (m *Model) finishLiveExecution(result engine.PlanResult) {
	m.liveResult = &result
	m.lastLogPath = result.LogPath
	m.execCancel = nil
	m.removeStrays = false

	straySuffix := ""
	removedStrays, failedStrays := 0, 0
	for _, stray := range result.Strays {
		if stray.Removed {
			removedStrays++
			for id, state := range m.states {
				if state.ShadowedBy == stray.Path {
					state.ShadowedBy = ""
					state.ShadowVersion = ""
					m.states[id] = state
				}
			}
		} else {
			failedStrays++
		}
	}
	if removedStrays > 0 {
		noun := "strays"
		if removedStrays == 1 {
			noun = "stray"
		}
		straySuffix += fmt.Sprintf(" · removed %d %s", removedStrays, noun)
	}
	if failedStrays > 0 {
		straySuffix += fmt.Sprintf(" · %d stray removals failed", failedStrays)
	}

	if result.LogPath != "" {
		m.notice = fmt.Sprintf("Execution finished: %d succeeded, %d failed%s · log %s",
			result.Succeeded(), result.Failed(), straySuffix, result.LogPath)
		return
	}
	m.notice = fmt.Sprintf("Execution finished: %d succeeded, %d failed%s",
		result.Succeeded(), result.Failed(), straySuffix)
}

func (m *Model) cancelExecution() {
	if m.execCancel != nil {
		m.execCancel()
		m.execCancel = nil
	}
}

func spinnerTickCmd() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(time.Time) tea.Msg {
		return spinnerTickMsg{}
	})
}

func (m *Model) spinnerFrame() string {
	if len(spinnerFrames) == 0 {
		return "⠋"
	}
	return spinnerFrames[m.spinnerIdx%len(spinnerFrames)]
}

func (m *Model) beginRefreshState() {
	m.refreshing = true
	m.refreshPhase = "Scanning local tools…"
	m.refreshChecked = 0
	m.refreshTotal = len(m.catalog.Tools)
	for _, tool := range m.catalog.Tools {
		state := m.states[tool.ID]
		state.CheckStatus = domain.StatusChecking
		m.states[tool.ID] = state
	}
}

func (m *Model) startRefresh() tea.Cmd {
	if m.refreshEvents != nil {
		return nil
	}
	m.beginRefreshState()
	ctx := m.baseCtx
	if ctx == nil {
		ctx = context.Background()
	}

	if m.onRefresh != nil {
		refresh := m.onRefresh
		return func() tea.Msg {
			states, err := refresh(ctx)
			return refreshDoneMsg{states: states, err: err}
		}
	}

	total := len(m.catalog.Tools)
	events := make(chan refreshProgressMsg, total*2+8)
	m.refreshEvents = events
	cat := m.catalog
	selfCheck := m.onSelfCheck
	initialCached := cloneStates(m.states)

	runCmd := func() tea.Msg {
		defer close(events)
		if err := ctx.Err(); err != nil {
			return refreshDoneMsg{err: err}
		}
		inspector := engine.NewInspector("")
		localStates := inspector.InspectLocal(ctx, cat)
		for id, state := range localStates {
			if cached, ok := initialCached[id]; ok && state.LatestVersion == "" && cached.LatestVersion != "" {
				state.LatestVersion = cached.LatestVersion
			}
			state.CheckStatus = domain.StatusChecking
			localStates[id] = state
		}
		select {
		case events <- refreshProgressMsg{
			phase:  "Checking upstream versions…",
			states: cloneStates(localStates),
			total:  total,
		}:
		case <-ctx.Done():
			return refreshDoneMsg{err: ctx.Err()}
		}

		checked := 0
		finalStates := inspector.InspectUpstream(ctx, cat, localStates, func(toolID string, state domain.ToolState) {
			checked++
			select {
			case events <- refreshProgressMsg{
				phase:   "Checking upstream versions…",
				toolID:  toolID,
				state:   state,
				checked: checked,
				total:   total,
			}:
			case <-ctx.Done():
			}
		})
		if cachePath := engine.DefaultCachePath(""); cachePath != "" {
			_ = engine.SaveCache(cachePath, engine.CacheSnapshot{States: finalStates})
		}
		var selfState *engine.SelfState
		if selfCheck != nil {
			if state, err := selfCheck(ctx); err == nil {
				selfState = &state
			}
		}
		return refreshDoneMsg{states: finalStates, selfState: selfState}
	}

	return tea.Batch(runCmd, waitForRefreshEvent(events), spinnerTickCmd())
}

func waitForRefreshEvent(events <-chan refreshProgressMsg) tea.Cmd {
	if events == nil {
		return nil
	}
	return func() tea.Msg {
		msg, ok := <-events
		if !ok {
			return refreshStreamClosedMsg{}
		}
		return msg
	}
}

func (m *Model) applyRefreshProgress(msg refreshProgressMsg) {
	if msg.phase != "" {
		m.refreshPhase = msg.phase
	}
	if msg.total > 0 {
		m.refreshTotal = msg.total
	}
	if msg.states != nil {
		m.states = cloneStates(msg.states)
	}
	if msg.toolID != "" {
		m.states[msg.toolID] = msg.state
		m.refreshChecked = msg.checked
	}
	if _, found := engine.FindMise(nil, ""); found {
		m.miseInstalled = true
	}
	m.rebuildPlan()
	m.ensureSelection()
}

func (m *Model) applyRefresh(msg refreshDoneMsg) {
	m.refreshing = false
	m.refreshPhase = ""
	if msg.err != nil {
		m.notice = "Refresh failed: " + msg.err.Error()
		return
	}
	if msg.states != nil {
		m.states = cloneStates(msg.states)
	}
	if msg.selfState != nil {
		m.selfState = *msg.selfState
	}
	if _, found := engine.FindMise(nil, ""); found {
		m.miseInstalled = true
	}
	m.rebuildPlan()
	m.ensureSelection()
	m.notice = "Installed and upstream state refreshed"
}

func formatStepDuration(duration time.Duration) string {
	if duration <= 0 {
		return "0.0s"
	}
	return duration.Round(100 * time.Millisecond).String()
}

// AdvanceExecution advances the deterministic demo execution by one step.
// Phase 5 can use StepProgressMsg for the same transitions from a live runner.
func (m *Model) AdvanceExecution() {
	index := -1
	for i, step := range m.execSteps {
		if step.status == StepRunning {
			index = i
			break
		}
	}
	if index < 0 {
		for i, step := range m.execSteps {
			if step.status == StepQueued {
				index = i
				break
			}
		}
	}
	if index < 0 {
		return
	}
	m.applyStepProgress(StepProgressMsg{
		Index:   index,
		Status:  StepDone,
		Elapsed: fmt.Sprintf("%d.%ds", index+1, (index*3)%10),
		Log:     []string{"✓ " + m.execSteps[index].step.Tool.Name + " completed"},
	})
}

func (m *Model) applyStepProgress(msg StepProgressMsg) {
	if msg.Index < 0 || msg.Index >= len(m.execSteps) {
		return
	}
	step := &m.execSteps[msg.Index]
	if msg.Status != "" {
		// Terminal steps never regress: late line events (e.g. stray
		// removals after a step finished) only append logs.
		terminal := step.status == StepDone || step.status == StepFailed
		if !terminal || msg.Status != StepRunning {
			step.status = msg.Status
		}
	}
	if msg.Elapsed != "" {
		step.elapsed = msg.Elapsed
	}
	step.logs = append(step.logs, msg.Log...)
	if step.status == StepDone || step.status == StepFailed {
		for index := range m.execSteps {
			if m.execSteps[index].status == StepQueued {
				m.execSteps[index].status = StepRunning
				m.execSteps[index].elapsed = "running…"
				break
			}
		}
	}
}

func (m *Model) filterLabel() string {
	total, installed, outdated, _, _, _ := m.counts()
	switch m.filter {
	case filterOutdated:
		return fmt.Sprintf("[ Outdated (%d) ]", outdated)
	case filterPending:
		return fmt.Sprintf("[ Pending (%d) ]", m.pendingActionCount())
	case filterInstalled:
		return fmt.Sprintf("[ Installed (%d) ]", installed)
	case filterUninstalled:
		return fmt.Sprintf("[ Not installed (%d) ]", total-installed)
	default:
		return fmt.Sprintf("[ All (%d) ]", total)
	}
}

func (m *Model) pendingActionCount() int {
	pending := 0
	for _, tool := range m.catalog.Tools {
		action := m.actions[tool.ID]
		if action != "" && action != domain.ActionNone {
			pending++
		}
	}
	return pending
}

func (m *Model) counts() (total, installed, outdated, installs, updates, removals int) {
	total = len(m.catalog.Tools)
	for _, tool := range m.catalog.Tools {
		state := m.states[tool.ID]
		if state.IsInstalled() {
			installed++
		}
		if state.IsOutdated() {
			outdated++
		}
	}
	installs, updates, removals = m.plan.Counts()
	return total, installed, outdated, installs, updates, removals
}

func cloneStates(states map[string]domain.ToolState) map[string]domain.ToolState {
	if states == nil {
		return nil
	}
	cloned := make(map[string]domain.ToolState, len(states))
	for id, state := range states {
		cloned[id] = state
	}
	return cloned
}

func cloneActions(actions map[string]domain.DesiredAction) map[string]domain.DesiredAction {
	if actions == nil {
		return nil
	}
	cloned := make(map[string]domain.DesiredAction, len(actions))
	for id, action := range actions {
		cloned[id] = action
	}
	return cloned
}

func (m *Model) render() string {
	var lines []string
	switch m.view {
	case "quit":
		lines = m.renderQuit()
	case "selfupdate":
		lines = m.renderSelfUpdate()
	case "shadows":
		lines = m.renderShadows()
	case "profiles":
		lines = m.renderProfiles()
	case "plan":
		lines = m.renderPlanScreen()
	case "applying":
		lines = m.renderApplying()
	case "help":
		lines = m.renderHelp()
	default:
		if m.view == "detail" {
			lines = m.renderDetail()
		} else {
			lines = m.renderCatalog()
		}
	}
	return m.joinScreen(lines, m.width, m.height)
}
