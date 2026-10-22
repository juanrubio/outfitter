package cli

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/juanrubio/outfitter/internal/catalog"
	"github.com/juanrubio/outfitter/internal/engine"
)

func outdatedSelfState() engine.SelfState {
	return engine.SelfState{
		CurrentVersion: "0.5.0",
		LatestVersion:  "0.6.0",
		Outdated:       true,
		ExecutablePath: "/home/test/.local/bin/outfitter",
		ResolvedPath:   "/home/test/.local/bin/outfitter",
		InstallKind:    engine.SelfInstallStandalone,
		CanSelfUpdate:  true,
		ReleaseURL:     "https://github.com/juanrubio/outfitter/releases/latest",
		UpdateCommand:  "download, verify, and replace /home/test/.local/bin/outfitter",
	}
}

func TestCheckReportsSelfUpdate(t *testing.T) {
	deps, _ := testDeps(t)
	deps.SelfCheck = func(_ context.Context, _ *catalog.Catalog, force bool) (engine.SelfState, error) {
		if !force {
			t.Error("check should force a fresh self-check")
		}
		return outdatedSelfState(), nil
	}
	out, err := runCLI(t, deps, "check")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Outfitter update available: 0.5.0 → 0.6.0") ||
		!strings.Contains(out, "outfitter self-update") {
		t.Fatalf("check output missing self update:\n%s", out)
	}

	out, err = runCLI(t, deps, "check", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var payload struct {
		Self engine.SelfState `json:"self"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatal(err)
	}
	if !payload.Self.Outdated || payload.Self.LatestVersion != "0.6.0" {
		t.Fatalf("json self = %#v", payload.Self)
	}
}

func TestStatusReportsCachedSelfUpdateWithoutChangingJSON(t *testing.T) {
	deps, _ := testDeps(t)
	deps.CachedSelf = func(_ *catalog.Catalog) engine.SelfState { return outdatedSelfState() }
	out, err := runCLI(t, deps, "status")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Outfitter update available: 0.5.0 → 0.6.0") {
		t.Fatalf("status output missing cached self update:\n%s", out)
	}

	out, err = runCLI(t, deps, "status", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var entries []statusEntry
	if err := json.Unmarshal([]byte(out), &entries); err != nil {
		t.Fatalf("status JSON should remain an entries array: %v", err)
	}
	if len(entries) != 48 {
		t.Fatalf("status entries = %d, want 48", len(entries))
	}
}

func TestSelfUpdateCheckAndDryRun(t *testing.T) {
	deps, _ := testDeps(t)
	updateCalled := false
	deps.SelfCheck = func(_ context.Context, _ *catalog.Catalog, _ bool) (engine.SelfState, error) {
		return outdatedSelfState(), nil
	}
	deps.SelfUpdate = func(_ context.Context, _ *catalog.Catalog, opts engine.SelfUpdateOptions) (engine.SelfUpdateResult, error) {
		updateCalled = true
		return engine.SelfUpdateResult{
			State:   outdatedSelfState(),
			DryRun:  opts.DryRun,
			Command: outdatedSelfState().UpdateCommand,
			Message: "Would update Outfitter 0.5.0 to 0.6.0",
		}, nil
	}

	out, err := runCLI(t, deps, "self-update", "--check")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "0.5.0 → 0.6.0") || updateCalled {
		t.Fatalf("check output=%q updateCalled=%v", out, updateCalled)
	}

	out, err = runCLI(t, deps, "self-update", "--dry-run", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var report selfUpdateReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatal(err)
	}
	if !report.DryRun || report.Updated || !updateCalled {
		t.Fatalf("dry-run report = %#v called=%v", report, updateCalled)
	}
}

func TestSelfUpdateRequiresYesAndExecutesWithYes(t *testing.T) {
	deps, _ := testDeps(t)
	updateCalled := false
	deps.SelfCheck = func(_ context.Context, _ *catalog.Catalog, _ bool) (engine.SelfState, error) {
		return outdatedSelfState(), nil
	}
	deps.SelfUpdate = func(_ context.Context, _ *catalog.Catalog, _ engine.SelfUpdateOptions) (engine.SelfUpdateResult, error) {
		updateCalled = true
		return engine.SelfUpdateResult{
			State:           outdatedSelfState(),
			Updated:         true,
			Command:         outdatedSelfState().UpdateCommand,
			RequiresRestart: true,
			Message:         "Outfitter updated to 0.6.0. Restart Outfitter to run the new version.",
		}, nil
	}

	_, err := runCLI(t, deps, "self-update")
	requireExit(t, err, 2)
	if updateCalled {
		t.Fatal("update ran without --yes in a non-interactive environment")
	}

	out, err := runCLI(t, deps, "self-update", "--yes", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var report selfUpdateReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatal(err)
	}
	if !report.Updated || !report.RequiresRestart || !updateCalled {
		t.Fatalf("update report = %#v called=%v", report, updateCalled)
	}
}

func TestSelfUpdateUnsupportedAndDemo(t *testing.T) {
	deps, _ := testDeps(t)
	manual := outdatedSelfState()
	manual.InstallKind = engine.SelfInstallManual
	manual.CanSelfUpdate = false
	manual.ManualHint = "download the latest release manually"
	deps.SelfCheck = func(_ context.Context, _ *catalog.Catalog, _ bool) (engine.SelfState, error) {
		return manual, nil
	}
	deps.SelfUpdate = func(_ context.Context, _ *catalog.Catalog, _ engine.SelfUpdateOptions) (engine.SelfUpdateResult, error) {
		return engine.SelfUpdateResult{State: manual, Message: manual.ManualHint},
			engine.ErrSelfUpdateUnsupported
	}
	_, err := runCLI(t, deps, "self-update", "--yes")
	requireExit(t, err, 1)
	if !errors.Is(err, engine.ErrSelfUpdateUnsupported) {
		t.Fatalf("unsupported error = %v", err)
	}

	_, err = runCLI(t, deps, "--demo", "self-update", "--check")
	requireExit(t, err, 2)
}
