package tui

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/juanrubio/outfitter/internal/domain"
	"github.com/juanrubio/outfitter/internal/engine"
)

func selfOutdated() engine.SelfState {
	return engine.SelfState{
		CurrentVersion: "0.5.0",
		LatestVersion:  "0.6.0",
		Outdated:       true,
		ResolvedPath:   "/home/test/.local/bin/outfitter",
		InstallKind:    engine.SelfInstallStandalone,
		CanSelfUpdate:  true,
		UpdateCommand:  "download, verify, and replace /home/test/.local/bin/outfitter",
	}
}

func selfModel(t *testing.T, state engine.SelfState, onUpdate func(context.Context) (engine.SelfUpdateResult, error)) *Model {
	t.Helper()
	m, err := NewModel(Options{
		Catalog:      loadCatalogBytes(t, replaceNoProfiles),
		States:       map[string]domain.ToolState{},
		Width:        160,
		Height:       40,
		AppVersion:   "0.5.0",
		SelfState:    state,
		OnSelfUpdate: onUpdate,
	})
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestSelfUpdateWarningHeader(t *testing.T) {
	m := selfModel(t, selfOutdated(), nil)
	screen := m.View().Content
	assertScreen(t, screen, 160, 40)
	if !strings.Contains(screen, "Outfitter 0.6.0 available") || !strings.Contains(screen, "you have 0.5.0") ||
		!strings.Contains(screen, "[O]") {
		t.Fatalf("header missing self-update warning:\n%s", screen)
	}
}

func TestSelfUpdateModalConfirmAndSuccess(t *testing.T) {
	called := false
	m := selfModel(t, selfOutdated(), func(context.Context) (engine.SelfUpdateResult, error) {
		called = true
		return engine.SelfUpdateResult{
			State:           selfOutdated(),
			Updated:         true,
			RequiresRestart: true,
			Message:         "Outfitter updated to 0.6.0. Restart Outfitter to run the new version.",
		}, nil
	})

	pressKey(m, 'O', "O")
	if m.view != "selfupdate" {
		t.Fatalf("view after O = %q, want selfupdate", m.view)
	}
	screen := m.View().Content
	assertScreen(t, screen, 160, 40)
	if !strings.Contains(screen, "Update Outfitter?") || !strings.Contains(screen, "0.5.0") ||
		!strings.Contains(screen, "0.6.0") || !strings.Contains(screen, "Cancel") || !strings.Contains(screen, "Update") {
		t.Fatalf("self-update modal incomplete:\n%s", screen)
	}

	// Cancel is the default choice; Esc returns without updating.
	_, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.view != "catalog" || called {
		t.Fatalf("after cancel view=%q called=%v", m.view, called)
	}

	pressKey(m, 'O', "O")
	cmd := pressKey(m, 'y', "y")
	if cmd == nil {
		t.Fatal("confirming self-update returned no command")
	}
	msg := cmd()
	if _, cmd := m.Update(msg); cmd != nil {
		_ = cmd
	}
	if !called {
		t.Fatal("self-update callback was not called")
	}
	if !strings.Contains(m.notice, "updated to 0.6.0") {
		t.Fatalf("notice after update = %q", m.notice)
	}
	if strings.Contains(m.View().Content, "Outfitter 0.6.0 available") {
		t.Fatalf("warning should clear after successful update:\n%s", m.View().Content)
	}
}

func TestSelfUpdateManualInstallExplainsInsteadOfUpdating(t *testing.T) {
	state := selfOutdated()
	state.InstallKind = engine.SelfInstallManual
	state.CanSelfUpdate = false
	state.ManualHint = "download the latest release manually"
	called := false
	m := selfModel(t, state, func(context.Context) (engine.SelfUpdateResult, error) {
		called = true
		return engine.SelfUpdateResult{}, nil
	})
	pressKey(m, 'O', "O")
	if m.view != "catalog" || called {
		t.Fatalf("manual install opened updater: view=%q called=%v", m.view, called)
	}
	if !strings.Contains(m.notice, "download the latest release manually") {
		t.Fatalf("manual notice = %q", m.notice)
	}
}
