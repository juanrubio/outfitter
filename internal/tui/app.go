// Package tui implements Outfitter's terminal interface.
package tui

import (
	"context"
	"os"

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

	width  int
	height int
	mode   theme.Mode
	styles theme.Styles

	selected     string
	collapsed    map[string]bool
	view         string
	scrollOffset int
	plan         domain.ActionPlan
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
		width:         width,
		height:        height,
		mode:          mode,
		styles:        theme.NewStyles(mode),
		states:        cloneStates(opts.States),
		actions:       cloneActions(opts.Actions),
		collapsed:     make(map[string]bool),
		view:          "catalog",
		miseInstalled: opts.MiseInstalled,
		miseVersion:   opts.MiseVersion,
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

	m.selected = m.firstVisibleToolID()
	m.rebuildPlan()
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
	case tea.KeyPressMsg:
		return m.updateKey(msg)
	default:
		return m, nil
	}
}

func (m *Model) updateKey(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "j", "down":
		m.moveSelection(1)
	case "k", "up":
		m.moveSelection(-1)
	case "esc":
		if m.view == "detail" {
			m.view = "catalog"
		}
	case "enter":
		if m.width < 100 && m.view == "catalog" {
			m.view = "detail"
		}
	case " ", "space":
		m.toggleSelected()
	case "tab":
		m.toggleFocusedCategory()
	case "u":
		m.queueAllUpdates()
	case "x":
		m.resetPlan()
	}
	return m, nil
}

// View implements tea.Model.
func (m *Model) View() tea.View {
	content := m.render()
	view := tea.NewView(content)
	view.AltScreen = true
	return view
}

func (m *Model) firstVisibleToolID() string {
	for _, category := range m.catalog.Categories {
		if m.collapsed[category.ID] {
			continue
		}
		tools := m.catalog.ToolsByCategory(category.ID)
		if len(tools) > 0 {
			return tools[0].ID
		}
	}
	return ""
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
			ids = append(ids, tool.ID)
		}
	}
	return ids
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
	m.rebuildPlan()
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
}

func (m *Model) queueAllUpdates() {
	for _, tool := range m.catalog.Tools {
		state := m.states[tool.ID]
		if state.IsOutdated() && m.actions[tool.ID] != domain.ActionUpdate {
			m.actions[tool.ID] = domain.ActionUpdate
		}
	}
	m.rebuildPlan()
}

func (m *Model) resetPlan() {
	m.actions = make(map[string]domain.DesiredAction)
	m.rebuildPlan()
}

func (m *Model) rebuildPlan() {
	m.plan = domain.BuildActionPlan(m.catalog.Tools, m.states, m.actions, m.miseInstalled, m.miseVersion)
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
	if m.view == "detail" && m.width < 100 {
		lines = m.renderDetail()
	} else {
		lines = m.renderCatalog()
	}
	return joinScreen(lines, m.width, m.height)
}
