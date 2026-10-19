package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/juanrubio/outfitter/internal/domain"
	"github.com/juanrubio/outfitter/internal/theme"
)

func TestRenderDimensions(t *testing.T) {
	sizes := []struct {
		width  int
		height int
	}{
		{160, 48},
		{120, 40},
		{100, 32},
		{80, 24},
	}
	for _, size := range sizes {
		t.Run(displaySize(size.width, size.height), func(t *testing.T) {
			m, err := NewModel(Options{Demo: true, Width: size.width, Height: size.height})
			if err != nil {
				t.Fatal(err)
			}
			assertScreen(t, m.View().Content, size.width, size.height)
			if size.width >= 100 {
				if !strings.Contains(m.View().Content, "INSPECTOR") {
					t.Error("wide layout is missing the inspector")
				}
				if !strings.Contains(m.View().Content, "PENDING ACTION PLAN") {
					t.Error("wide layout is missing the pending action plan")
				}
			}
		})
	}
}

func TestRenderThemesPreserveMarkers(t *testing.T) {
	actions := map[string]domain.DesiredAction{
		"gemini-cli": domain.ActionInstall,
		"quotary":    domain.ActionInstall,
		"codex":      domain.ActionUpdate,
	}
	cases := []struct {
		name string
		mode theme.Mode
	}{
		{"midnight", theme.ModeDark},
		{"daylight", theme.ModeLight},
		{"monochrome", theme.ModeMono},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if test.mode == theme.ModeMono {
				t.Setenv("NO_COLOR", "1")
			} else {
				t.Setenv("NO_COLOR", "")
			}
			m, err := NewModel(Options{
				Demo:      true,
				Width:     120,
				Height:    40,
				ThemeMode: test.mode,
				Actions:   actions,
			})
			if err != nil {
				t.Fatal(err)
			}
			screen := m.View().Content
			assertScreen(t, screen, 120, 40)
			for _, marker := range []string{"[✓]", "[!]", "[↑]", "[+]", "[ ]", ">"} {
				if !strings.Contains(screen, marker) {
					t.Errorf("screen is missing marker %q", marker)
				}
			}
		})
	}
}

func TestCompactEnterOpensInspectorAndEscReturns(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	m, err := NewModel(Options{Demo: true, Width: 80, Height: 24})
	if err != nil {
		t.Fatal(err)
	}
	catalogScreen := m.View().Content
	if strings.Contains(catalogScreen, "PENDING ACTION PLAN") {
		t.Fatal("compact catalog should not show the inspector block")
	}

	_, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.view != "detail" {
		t.Fatalf("view after Enter = %q, want detail", m.view)
	}
	detailScreen := m.View().Content
	assertScreen(t, detailScreen, 80, 24)
	if !strings.Contains(detailScreen, "INSPECTOR") {
		t.Error("detail view is missing the inspector")
	}
	if !strings.Contains(detailScreen, "PENDING ACTION PLAN") {
		t.Error("detail view is missing the pending action plan")
	}

	_, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.view != "catalog" {
		t.Fatalf("view after Esc = %q, want catalog", m.view)
	}
	backScreen := m.View().Content
	assertScreen(t, backScreen, 80, 24)
	if strings.Contains(backScreen, "PENDING ACTION PLAN") {
		t.Error("catalog after Esc still shows the inspector block")
	}
}

func TestNavigationAndToggle(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	m, err := NewModel(Options{Demo: true, Width: 120, Height: 40})
	if err != nil {
		t.Fatal(err)
	}
	m.selected = "codex"
	_, _ = m.Update(tea.KeyPressMsg{Code: ' ', Text: " "})
	if m.actions["codex"] != domain.ActionUpdate {
		t.Fatalf("space on outdated codex queued %q, want update", m.actions["codex"])
	}
	_, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	if m.selected != "muse" {
		t.Fatalf("selection after j = %q, want muse", m.selected)
	}
	_, _ = m.Update(tea.KeyPressMsg{Code: 'k', Text: "k"})
	if m.selected != "codex" {
		t.Fatalf("selection after k = %q, want codex", m.selected)
	}
}

func TestWindowResize(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	m, err := NewModel(Options{Demo: true})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	assertScreen(t, m.View().Content, 80, 24)
}

func TestNoRawHexColorsInTUI(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	pattern := regexp.MustCompile(`#[0-9a-fA-F]{3,6}`)
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if match := pattern.FindString(string(data)); match != "" {
			t.Errorf("%s contains raw color literal %q", file, match)
		}
	}
}

func assertScreen(t *testing.T, content string, width, height int) {
	t.Helper()
	lines := strings.Split(content, "\n")
	if len(lines) != height {
		t.Fatalf("screen height = %d, want %d", len(lines), height)
	}
	for index, line := range lines {
		if got := lipgloss.Width(line); got != width {
			t.Errorf("line %d width = %d, want %d", index, got, width)
		}
	}
}

func displaySize(width, height int) string {
	return fmt.Sprintf("%dx%d", width, height)
}
