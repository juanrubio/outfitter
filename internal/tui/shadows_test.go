package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func shadowModel(t *testing.T, onRemove func([]string) ([]string, error)) *Model {
	return shadowModelSized(t, onRemove, 120, 40)
}

func shadowModelSized(t *testing.T, onRemove func([]string) ([]string, error), width, height int) *Model {
	t.Helper()
	m, err := NewModel(Options{
		Demo: true, Width: width, Height: height,
		OnRemoveShadows: onRemove,
	})
	if err != nil {
		t.Fatal(err)
	}
	for id, shadow := range map[string]string{
		"tmux": "/home/test/old/tmux",
		"dust": "/home/test/old/dust",
	} {
		state := m.states[id]
		state.ShadowedBy = shadow
		state.ShadowVersion = "0.0.1"
		m.states[id] = state
	}
	return m
}

func TestShadowModalListsBinariesAndSelectiveRemoval(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	var swept []string
	m := shadowModel(t, func(paths []string) ([]string, error) {
		swept = append([]string(nil), paths...)
		return paths, nil
	})

	pressKey(m, 'S', "S")
	if m.view != "shadows" {
		t.Fatalf("view after S = %q, want shadows", m.view)
	}
	screen := m.View().Content
	assertScreen(t, screen, 120, 40)
	for _, want := range []string{"SHADOWED BINARIES", "tmux", "/home/test/old/tmux", "dust", "/home/test/old/dust", "[x]", "Space", "Enter"} {
		if !strings.Contains(screen, want) {
			t.Fatalf("shadow modal missing %q:\n%s", want, screen)
		}
	}

	// Catalog order puts tmux first; move to dust and uncheck it.
	pressKey(m, 'j', "j")
	pressKey(m, ' ', " ")
	if !strings.Contains(m.View().Content, "[ ]") {
		t.Fatalf("unchecked row not rendered:\n%s", m.View().Content)
	}

	_, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if len(swept) != 1 || swept[0] != "/home/test/old/tmux" {
		t.Fatalf("swept = %v, want only tmux", swept)
	}
	if m.states["tmux"].ShadowedBy != "" {
		t.Fatal("removed tmux shadow should clear")
	}
	if m.states["dust"].ShadowedBy != "/home/test/old/dust" {
		t.Fatalf("unchecked dust shadow changed: %q", m.states["dust"].ShadowedBy)
	}
	if m.view != "catalog" || !strings.Contains(m.View().Content, "⚠ 1 shadowed") {
		t.Fatalf("after selective sweep view=%q screen:\n%s", m.view, m.View().Content)
	}
}

func TestShadowModalCompactLayout(t *testing.T) {
	m := shadowModelSized(t, nil, 80, 24)
	pressKey(m, 'S', "S")
	screen := m.View().Content
	assertScreen(t, screen, 80, 24)
	if !strings.Contains(screen, "/home/test/old/tmux") || !strings.Contains(screen, "Enter Remove") {
		t.Fatalf("compact shadow modal missing path or action:\n%s", screen)
	}
}

func TestShadowModalCancelAndNoneSelected(t *testing.T) {
	called := false
	m := shadowModel(t, func(paths []string) ([]string, error) {
		called = true
		return paths, nil
	})

	pressKey(m, 'S', "S")
	_, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.view != "catalog" || called {
		t.Fatalf("after Esc view=%q called=%v", m.view, called)
	}

	// Reopening starts from all checked; unchecking both makes Enter a no-op.
	pressKey(m, 'S', "S")
	pressKey(m, ' ', " ")
	pressKey(m, 'j', "j")
	pressKey(m, ' ', " ")
	_, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if called {
		t.Fatal("sweep ran with nothing selected")
	}
	if m.view != "shadows" || !strings.Contains(m.notice, "Select at least one") {
		t.Fatalf("none-selected view=%q notice=%q", m.view, m.notice)
	}
}
