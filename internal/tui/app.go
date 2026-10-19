// Package tui implements Outfitter's terminal interface.
package tui

import (
	"context"
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"golang.org/x/term"
)

// Options configures a TUI session.
type Options struct {
	Demo        bool
	CatalogPath string
}

type model struct {
	demo        bool
	catalogPath string
}

// Run starts the interactive application. In Phase 0 the TUI is a stub that
// renders a placeholder and exits on q or Ctrl+C.
func Run(ctx context.Context, opts Options) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !term.IsTerminal(int(os.Stdin.Fd())) || !term.IsTerminal(int(os.Stdout.Fd())) {
		return nil
	}
	m := &model{demo: opts.Demo, catalogPath: opts.CatalogPath}
	p := tea.NewProgram(m, tea.WithContext(ctx))
	_, err := p.Run()
	return err
}

func (m *model) Init() tea.Cmd { return nil }

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyPressMsg); ok {
		switch key.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m *model) View() tea.View {
	title := lipgloss.NewStyle().Bold(true).Render("OUTFITTER")
	label := "starting"
	if m.demo {
		label = "offline demo · fictional data"
	}
	catalog := "embedded catalog"
	if m.catalogPath != "" {
		catalog = m.catalogPath
	}
	view := tea.NewView(fmt.Sprintf("\n  %s\n\n  %s\n  Catalog: %s\n\n  q Quit\n", title, label, catalog))
	view.AltScreen = true
	return view
}
