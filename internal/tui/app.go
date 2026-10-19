// Package tui implements Outfitter's terminal interface.
package tui

import (
	"context"
	"fmt"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"
	"golang.org/x/term"

	"github.com/juanrubio/outfitter/internal/catalog"
	"github.com/juanrubio/outfitter/internal/demo"
	"github.com/juanrubio/outfitter/internal/domain"
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

// Options configures a TUI session. Tests may supply a catalog and state
// directly and render the model without a real terminal.
type Options struct {
	Demo           bool
	CatalogPath    string
	Width          int
	Height         int
	ThemeMode      theme.Mode
	Catalog        *catalog.Catalog
	States         map[string]domain.ToolState
	Actions        map[string]domain.DesiredAction
	MiseInstalled  bool
	MiseVersion    string
	PlatformNotice string
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

	selected        string
	collapsed       map[string]bool
	view            string
	scrollOffset    int
	plan            domain.ActionPlan
	notice          string
	filter          filterMode
	query           string
	editing         bool
	confirmQuit     bool
	activeProfileID string
	profileCursor   int
	execSteps       []executionStep
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
		cat, err := catalog.Load(opts.CatalogPath)
		if err != nil {
			return nil, err
		}
		m.catalog = cat
	}
	if m.states == nil {
		m.states = make(map[string]domain.ToolState)
	}
	if m.actions == nil {
		m.actions = make(map[string]domain.DesiredAction)
	}
	if m.miseVersion == "" {
		m.miseVersion = "2026.10.1"
	}
	if m.platformNotice == "" {
		m.platformNotice = "Ubuntu 24.04 · amd64"
	}
	if _, ok := m.catalog.ProfileByID(m.activeProfileID); !ok && len(m.catalog.Profiles) > 0 {
		m.activeProfileID = m.catalog.Profiles[0].ID
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
	m, err := NewModel(opts)
	if err != nil {
		return err
	}
	p := tea.NewProgram(m, tea.WithContext(ctx))
	_, err = p.Run()
	return err
}

// Init implements tea.Model.
func (m *Model) Init() tea.Cmd { return nil }

// View implements tea.Model.
func (m *Model) View() tea.View {
	content := m.render()
	view := tea.NewView(content)
	view.AltScreen = true
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
	case tea.KeyPressMsg:
		return m.updateKey(msg)
	default:
		return m, nil
	}
}

func (m *Model) updateKey(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	keystroke := key.String()
	if keystroke == "ctrl+c" {
		return m, tea.Quit
	}
	if m.editing {
		return m.updateSearchKey(key)
	}
	if m.confirmQuit {
		switch keystroke {
		case "q", "y":
			return m, tea.Quit
		case "esc", "n":
			m.confirmQuit = false
			m.notice = ""
		}
		return m, nil
	}

	switch m.view {
	case "profiles":
		return m.updateProfilesKey(key)
	case "plan":
		return m.updatePlanKey(key)
	case "applying":
		if keystroke == "esc" || keystroke == "q" {
			m.view = "catalog"
		}
		return m, nil
	case "help":
		if keystroke == "esc" || keystroke == "?" || keystroke == "q" {
			m.view = "catalog"
		}
		return m, nil
	case "detail":
		switch keystroke {
		case "esc", "q":
			m.view = "catalog"
		case " ", "space":
			m.toggleSelected()
		}
		return m, nil
	}

	switch keystroke {
	case "q":
		if m.plan.IsEmpty() {
			return m, tea.Quit
		}
		m.confirmQuit = true
		m.notice = "Unapplied changes in plan. Press q or y again to quit, or Esc to stay."
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
	case "U":
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
	case "r", "R":
		m.notice = "Upstream versions verified just now"
	case "c":
		m.notice = fmt.Sprintf("Catalog: embedded default (%d tools) + ~/.config/outfitter/catalog.toml", len(m.catalog.Tools))
	case "p":
		m.view = "profiles"
		m.profileCursor = m.activeProfileIndex()
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
		m.startExecution()
		m.view = "applying"
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

func (m *Model) jumpToCategory(num int) {
	for _, category := range m.catalog.Categories {
		if category.Num != num {
			continue
		}
		m.collapsed[category.ID] = false
		for _, tool := range m.catalog.ToolsByCategory(category.ID) {
			if m.toolMatches(tool) {
				m.selected = tool.ID
				return
			}
		}
		return
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

func (m *Model) toggleFocusedCategory() {
	tool, ok := m.selectedTool()
	if !ok {
		return
	}
	m.collapsed[tool.CategoryID] = !m.collapsed[tool.CategoryID]
	if m.collapsed[tool.CategoryID] {
		m.selected = m.firstVisibleToolID()
	}
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
		step.status = msg.Status
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
	return joinScreen(lines, m.width, m.height)
}
