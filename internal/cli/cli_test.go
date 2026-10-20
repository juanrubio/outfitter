package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/juanrubio/outfitter/internal/catalog"
	"github.com/juanrubio/outfitter/internal/domain"
	"github.com/juanrubio/outfitter/internal/engine"
	"github.com/juanrubio/outfitter/internal/tui"
)

func testDeps(t *testing.T) (Deps, *catalog.Catalog) {
	t.Helper()
	cat, err := catalog.LoadDefault()
	if err != nil {
		t.Fatal(err)
	}
	states := make(map[string]domain.ToolState, len(cat.Tools))
	for _, tool := range cat.Tools {
		states[tool.ID] = domain.ToolState{
			InstalledVersion: "1.0.0",
			LatestVersion:    "1.0.0",
			BinaryPath:       "/usr/bin/" + tool.Binary,
			Provenance:       domain.ProvenanceExternal,
			CheckStatus:      domain.StatusReady,
		}
	}
	states["codex"] = domain.ToolState{
		InstalledVersion: "0.163.0",
		LatestVersion:    "0.165.0",
		BinaryPath:       "/usr/bin/codex",
		Provenance:       domain.ProvenanceMise,
		CheckStatus:      domain.StatusReady,
	}
	states["gemini-cli"] = domain.ToolState{
		LatestVersion: "0.43.0",
		Provenance:    domain.ProvenanceNone,
		CheckStatus:   domain.StatusReady,
	}
	states["codexbar"] = domain.ToolState{
		LatestVersion: "0.77.0",
		Provenance:    domain.ProvenanceNone,
		CheckStatus:   domain.StatusReady,
	}

	home := t.TempDir()
	deps := Deps{
		HomeDir: home,
		IsTTY:   func(_ io.Reader, _ io.Writer) bool { return false },
		InspectLocal: func(_ context.Context, _ *catalog.Catalog) map[string]domain.ToolState {
			return cloneStateMap(states)
		},
		InspectUpstream: func(
			_ context.Context,
			_ *catalog.Catalog,
			current map[string]domain.ToolState,
			_ func(string, domain.ToolState),
		) map[string]domain.ToolState {
			return cloneStateMap(current)
		},
		RunPlan: func(_ context.Context, plan domain.ActionPlan, _ func(engine.StepEvent)) engine.PlanResult {
			return successfulRun(plan)
		},
		RunTUI: func(context.Context, tui.Options) error {
			return nil
		},
		MiseInstalled: func() (string, bool) {
			return "/home/test/.local/bin/mise", true
		},
		ShellSetup: func() *engine.ShellSetup {
			return &engine.ShellSetup{
				HomeDir: home,
				Getenv: func(key string) string {
					switch key {
					case "SHELL":
						return "/bin/bash"
					case "MISE_SHELL":
						return "bash"
					default:
						return ""
					}
				},
			}
		},
	}
	return deps, cat
}

func successfulRun(plan domain.ActionPlan) engine.PlanResult {
	result := engine.PlanResult{LogPath: "/tmp/outfitter-run.log"}
	for _, step := range plan.Steps() {
		result.Steps = append(result.Steps, engine.StepResult{Step: step, Success: true})
	}
	return result
}

func runCLI(t *testing.T, deps Deps, args ...string) (string, error) {
	t.Helper()
	cmd := NewRootCmdWithDeps(deps)
	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetIn(strings.NewReader(""))
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}

func requireExit(t *testing.T, err error, want int) {
	t.Helper()
	if got := ExitCode(err); got != want {
		t.Fatalf("exit code = %d, want %d (err=%v)", got, want, err)
	}
}

func TestStatusHumanAndJSON(t *testing.T) {
	deps, _ := testDeps(t)
	out, err := runCLI(t, deps, "status")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"codex", "0.163.0", "0.165.0", "update available", "gemini-cli", "not installed"} {
		if !strings.Contains(out, want) {
			t.Fatalf("status output missing %q:\n%s", want, out)
		}
	}

	out, err = runCLI(t, deps, "status", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var entries []statusEntry
	if err := json.Unmarshal([]byte(out), &entries); err != nil {
		t.Fatalf("status --json is not valid JSON: %v\n%s", err, out)
	}
	if len(entries) != 45 {
		t.Fatalf("status entries = %d, want 45", len(entries))
	}
	found := false
	for _, entry := range entries {
		if entry.ID == "codex" {
			found = true
			if !entry.Installed || !entry.Outdated || entry.LatestVersion != "0.165.0" {
				t.Fatalf("unexpected codex entry: %+v", entry)
			}
		}
	}
	if !found {
		t.Fatal("status JSON does not contain codex")
	}
}

func TestStatusMergesCachedUpstreamVersions(t *testing.T) {
	deps, cat := testDeps(t)
	deps.InspectLocal = func(_ context.Context, _ *catalog.Catalog) map[string]domain.ToolState {
		return map[string]domain.ToolState{
			"codex": {InstalledVersion: "0.163.0", Provenance: domain.ProvenanceMise},
		}
	}
	path := engine.DefaultCachePath(deps.HomeDir)
	if err := engine.SaveCache(path, engine.CacheSnapshot{States: map[string]domain.ToolState{
		"codex": {LatestVersion: "0.165.0", CheckStatus: domain.StatusReady},
	}}); err != nil {
		t.Fatal(err)
	}
	if len(cat.Tools) == 0 {
		t.Fatal("catalog is empty")
	}
	out, err := runCLI(t, deps, "status", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var entries []statusEntry
	if err := json.Unmarshal([]byte(out), &entries); err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.ID == "codex" {
			if entry.LatestVersion != "0.165.0" || !entry.Outdated {
				t.Fatalf("cached latest version was not merged: %+v", entry)
			}
			return
		}
	}
	t.Fatal("codex not found")
}

func TestCheckHumanJSONAndCache(t *testing.T) {
	deps, _ := testDeps(t)
	out, err := runCLI(t, deps, "check")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Updates available:", "codex", "0.163.0 → 0.165.0", "1 outdated"} {
		if !strings.Contains(out, want) {
			t.Fatalf("check output missing %q:\n%s", want, out)
		}
	}
	if _, err := os.Stat(engine.DefaultCachePath(deps.HomeDir)); err != nil {
		t.Fatalf("check did not save the state cache: %v", err)
	}

	out, err = runCLI(t, deps, "check", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var payload struct {
		Tools         []statusEntry `json:"tools"`
		OutdatedCount int           `json:"outdated_count"`
		Updates       []string      `json:"updates"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("check --json is not valid JSON: %v\n%s", err, out)
	}
	if payload.OutdatedCount != 1 || len(payload.Updates) != 1 || payload.Updates[0] != "codex" {
		t.Fatalf("unexpected check payload: %+v", payload)
	}
	if len(payload.Tools) != 45 {
		t.Fatalf("check tools = %d, want 45", len(payload.Tools))
	}
}

func TestDemoStatusAndCheckStayOffline(t *testing.T) {
	deps, _ := testDeps(t)
	deps.InspectLocal = func(context.Context, *catalog.Catalog) map[string]domain.ToolState {
		t.Fatal("demo status must not inspect the host")
		return nil
	}
	deps.InspectUpstream = func(
		context.Context,
		*catalog.Catalog,
		map[string]domain.ToolState,
		func(string, domain.ToolState),
	) map[string]domain.ToolState {
		t.Fatal("demo check must not inspect upstream")
		return nil
	}
	out, err := runCLI(t, deps, "--demo", "status", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `"codex"`) {
		t.Fatalf("demo status missing codex:\n%s", out)
	}
	if _, err := runCLI(t, deps, "--demo", "check"); err != nil {
		t.Fatal(err)
	}
}

func TestProfileListHumanAndJSON(t *testing.T) {
	deps, _ := testDeps(t)
	out, err := runCLI(t, deps, "profile", "list")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"desktop", "Full Desktop", "server", "Headless Server / VPS"} {
		if !strings.Contains(out, want) {
			t.Fatalf("profile list missing %q:\n%s", want, out)
		}
	}

	out, err = runCLI(t, deps, "profile", "list", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var profiles []profileEntry
	if err := json.Unmarshal([]byte(out), &profiles); err != nil {
		t.Fatalf("profile list --json is not valid JSON: %v\n%s", err, out)
	}
	if len(profiles) != 4 {
		t.Fatalf("profiles = %d, want 4", len(profiles))
	}
	for _, profile := range profiles {
		if profile.ToolCount != len(profile.Tools) || profile.ToolCount == 0 {
			t.Fatalf("invalid profile entry: %+v", profile)
		}
	}
}

func TestProfileApplyDryRun(t *testing.T) {
	deps, _ := testDeps(t)
	out, err := runCLI(t, deps, "profile", "apply", "server", "--dry-run")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Plan:") || !strings.Contains(out, "$ mise upgrade") {
		t.Fatalf("unexpected dry-run output:\n%s", out)
	}

	out, err = runCLI(t, deps, "profile", "apply", "server", "--dry-run", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var plan domain.ActionPlan
	if err := json.Unmarshal([]byte(out), &plan); err != nil {
		t.Fatalf("dry-run JSON is not a plan: %v\n%s", err, out)
	}
	if plan.IsEmpty() || len(plan.Updates) == 0 {
		t.Fatalf("expected updates in plan: %+v", plan)
	}
}

func TestProfileApplyNonTTYRequiresYes(t *testing.T) {
	deps, _ := testDeps(t)
	out, err := runCLI(t, deps, "profile", "apply", "server")
	requireExit(t, err, 2)
	if !strings.Contains(out, "Plan:") {
		t.Fatalf("non-TTY refusal must print the dry-run plan:\n%s", out)
	}
	if !strings.Contains(err.Error(), "non-interactive") {
		t.Fatalf("unexpected refusal: %v", err)
	}
}

func TestProfileApplyConfirmedSuccess(t *testing.T) {
	deps, _ := testDeps(t)
	var seen domain.ActionPlan
	deps.RunPlan = func(_ context.Context, plan domain.ActionPlan, _ func(engine.StepEvent)) engine.PlanResult {
		seen = plan
		return successfulRun(plan)
	}
	out, err := runCLI(t, deps, "profile", "apply", "server", "--yes")
	if err != nil {
		t.Fatal(err)
	}
	if seen.IsEmpty() {
		t.Fatal("runner did not receive a plan")
	}
	if !strings.Contains(out, "Summary:") || !strings.Contains(out, "succeeded") {
		t.Fatalf("missing execution summary:\n%s", out)
	}
}

func TestProfileApplyJSONResult(t *testing.T) {
	deps, _ := testDeps(t)
	out, err := runCLI(t, deps, "profile", "apply", "server", "--yes", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var payload jsonRunResult
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("mutation JSON is invalid: %v\n%s", err, out)
	}
	if payload.Succeeded == 0 || payload.Failed != 0 || len(payload.Steps) != payload.Succeeded {
		t.Fatalf("unexpected JSON result: %+v", payload)
	}
}

func TestProfileApplyFailureExitCode(t *testing.T) {
	deps, _ := testDeps(t)
	deps.RunPlan = func(_ context.Context, plan domain.ActionPlan, _ func(engine.StepEvent)) engine.PlanResult {
		result := successfulRun(plan)
		if len(result.Steps) > 0 {
			result.Steps[0].Success = false
			result.Steps[0].Err = errors.New("simulated failure")
		}
		return result
	}
	out, err := runCLI(t, deps, "profile", "apply", "server", "--yes")
	requireExit(t, err, 1)
	if !strings.Contains(out, "✗") || !strings.Contains(out, "simulated failure") {
		t.Fatalf("failure summary missing details:\n%s", out)
	}
}

func TestProfileApplyUnknownExitCode(t *testing.T) {
	deps, _ := testDeps(t)
	_, err := runCLI(t, deps, "profile", "apply", "does-not-exist", "--dry-run")
	requireExit(t, err, 2)
	if !strings.Contains(err.Error(), "unknown profile") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestProfileApplyInteractiveAnswers(t *testing.T) {
	deps, _ := testDeps(t)
	deps.IsTTY = func(_ io.Reader, _ io.Writer) bool { return true }

	cmd := NewRootCmdWithDeps(deps)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetIn(strings.NewReader("n\n"))
	cmd.SetArgs([]string{"profile", "apply", "server"})
	err := cmd.Execute()
	requireExit(t, err, 2)
	if !strings.Contains(err.Error(), "confirmation refused") {
		t.Fatalf("unexpected error: %v", err)
	}

	cmd = NewRootCmdWithDeps(deps)
	out.Reset()
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetIn(strings.NewReader("y\n"))
	cmd.SetArgs([]string{"profile", "apply", "server"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Summary:") {
		t.Fatalf("interactive confirmation did not execute:\n%s", out.String())
	}
}

func TestInstallDryRunAndUnknown(t *testing.T) {
	deps, _ := testDeps(t)
	out, err := runCLI(t, deps, "install", "gemini-cli", "--dry-run")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "install gemini-cli") || !strings.Contains(out, "$ mise use --global") {
		t.Fatalf("unexpected install dry-run:\n%s", out)
	}

	out, err = runCLI(t, deps, "install", "gemini-cli", "--dry-run", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var plan domain.ActionPlan
	if err := json.Unmarshal([]byte(out), &plan); err != nil {
		t.Fatal(err)
	}
	if len(plan.Installations) != 1 {
		t.Fatalf("installations = %d, want 1", len(plan.Installations))
	}

	_, err = runCLI(t, deps, "install", "not-a-tool", "--dry-run")
	requireExit(t, err, 2)

	_, err = runCLI(t, deps, "install")
	requireExit(t, err, 2)
}

func TestInstallNonTTYAndConfirmedExitCodes(t *testing.T) {
	deps, _ := testDeps(t)
	_, err := runCLI(t, deps, "install", "gemini-cli")
	requireExit(t, err, 2)

	deps.RunPlan = func(_ context.Context, plan domain.ActionPlan, _ func(engine.StepEvent)) engine.PlanResult {
		result := successfulRun(plan)
		if len(result.Steps) > 0 {
			result.Steps[0].Success = false
			result.Steps[0].Err = errors.New("install failed")
		}
		return result
	}
	out, err := runCLI(t, deps, "install", "gemini-cli", "--yes", "--json")
	requireExit(t, err, 1)
	var payload jsonRunResult
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Failed != 1 || payload.Steps[0].Error != "install failed" {
		t.Fatalf("unexpected failure JSON: %+v", payload)
	}
}

func TestUpdateAllDryRunAndSpecific(t *testing.T) {
	deps, _ := testDeps(t)
	out, err := runCLI(t, deps, "update", "--all", "--dry-run")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "update codex") || strings.Contains(out, "update gemini-cli") {
		t.Fatalf("unexpected update --all plan:\n%s", out)
	}

	out, err = runCLI(t, deps, "update", "codex", "--dry-run", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var plan domain.ActionPlan
	if err := json.Unmarshal([]byte(out), &plan); err != nil {
		t.Fatal(err)
	}
	if len(plan.Updates) != 1 || plan.Updates[0].Tool.ID != "codex" {
		t.Fatalf("unexpected update plan: %+v", plan)
	}

	_, err = runCLI(t, deps, "update", "not-a-tool", "--dry-run")
	requireExit(t, err, 2)
}

// TestUpdateWarnsAboutStrayBinaries verifies that updating an unmanaged
// (external) install warns about the stray binary that will remain on
// disk and offers the exact command to remove it, both in the dry-run
// plan and in the post-run summary.
func TestUpdateWarnsAboutStrayBinaries(t *testing.T) {
	deps, _ := testDeps(t)
	baseInspect := deps.InspectLocal
	deps.InspectLocal = func(ctx context.Context, cat *catalog.Catalog) map[string]domain.ToolState {
		states := baseInspect(ctx, cat)
		states["dust"] = domain.ToolState{
			InstalledVersion: "1.2.4",
			LatestVersion:    "1.2.6",
			BinaryPath:       "/usr/bin/dust",
			Provenance:       domain.ProvenanceExternal,
			CheckStatus:      domain.StatusReady,
		}
		return states
	}

	out, err := runCLI(t, deps, "update", "dust", "--dry-run")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"mise use --global aqua:bootandy/dust@latest",
		"/usr/bin/dust",
		"sudo rm /usr/bin/dust",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("dry-run output missing %q:\n%s", want, out)
		}
	}

	out, err = runCLI(t, deps, "update", "dust", "--yes")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "✓ update dust") {
		t.Fatalf("unexpected update output:\n%s", out)
	}
	if !strings.Contains(out, "sudo rm /usr/bin/dust") {
		t.Errorf("post-run summary missing stray removal command:\n%s", out)
	}
}

// unintegratedShellDeps returns deps whose shell (zsh) has no mise
// integration, with rc files under the test home directory.
func unintegratedShellDeps(t *testing.T) Deps {
	t.Helper()
	deps, _ := testDeps(t)
	home := deps.HomeDir
	deps.ShellSetup = func() *engine.ShellSetup {
		return &engine.ShellSetup{
			HomeDir: home,
			Getenv: func(key string) string {
				switch key {
				case "SHELL":
					return "/usr/bin/zsh"
				case "PATH":
					return "/usr/local/bin:/usr/bin:/bin"
				default:
					return ""
				}
			},
		}
	}
	return deps
}

// TestRemoveUnmanagedDryRun verifies that removing an unmanaged install
// never presents the backend's no-op remove command: an external binary
// is deleted directly, and an install owned by another manager is
// blocked with an explanation.
func TestRemoveUnmanagedDryRun(t *testing.T) {
	deps, _ := testDeps(t)
	baseInspect := deps.InspectLocal
	deps.InspectLocal = func(ctx context.Context, cat *catalog.Catalog) map[string]domain.ToolState {
		states := baseInspect(ctx, cat)
		states["gemini-cli"] = domain.ToolState{
			InstalledVersion: "0.43.0", LatestVersion: "0.64.0",
			BinaryPath:  "/home/u/.nvm/versions/node/v24/bin/gemini",
			Provenance:  domain.ProvenanceExternal,
			CheckStatus: domain.StatusReady,
		}
		states["dust"] = domain.ToolState{
			InstalledVersion: "1.2.4", LatestVersion: "1.2.6",
			BinaryPath:       "/usr/bin/dust",
			Provenance:       domain.ProvenanceAPT,
			ProvenanceDetail: "APT package (du-dust)",
			CheckStatus:      domain.StatusReady,
		}
		return states
	}

	out, err := runCLI(t, deps, "remove", "gemini-cli", "--dry-run")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "$ rm /home/u/.nvm/versions/node/v24/bin/gemini") {
		t.Errorf("external remove should delete the binary directly:\n%s", out)
	}
	if strings.Contains(out, "mise unuse") {
		t.Errorf("external remove must not present the backend no-op:\n%s", out)
	}

	out, err = runCLI(t, deps, "remove", "dust", "--dry-run")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "cannot remove") || !strings.Contains(out, "installed via apt") {
		t.Errorf("blocked remove should explain itself:\n%s", out)
	}
	if strings.Contains(out, "$ mise") {
		t.Errorf("blocked remove must not present a runnable command:\n%s", out)
	}
}

func TestDoctorDiagnoseDryRunAndFix(t *testing.T) {
	deps := unintegratedShellDeps(t)
	rc := filepath.Join(deps.HomeDir, ".zshrc")

	out, err := runCLI(t, deps, "doctor")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Shell: zsh", "Integration: missing", "outfitter doctor --fix", "mise activate zsh"} {
		if !strings.Contains(out, want) {
			t.Errorf("doctor output missing %q:\n%s", want, out)
		}
	}

	out, err = runCLI(t, deps, "doctor", "--dry-run")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Would append to") || !strings.Contains(out, "# >>> outfitter mise >>>") {
		t.Errorf("dry-run output missing block preview:\n%s", out)
	}
	if _, err := os.Stat(rc); !os.IsNotExist(err) {
		t.Fatalf("dry-run must not create the rc file (stat err = %v)", err)
	}

	// Non-TTY without --yes refuses to write.
	_, err = runCLI(t, deps, "doctor", "--fix")
	requireExit(t, err, 2)
	if _, err := os.Stat(rc); !os.IsNotExist(err) {
		t.Fatalf("refused fix must not create the rc file (stat err = %v)", err)
	}

	out, err = runCLI(t, deps, "doctor", "--fix", "--yes")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Added mise integration to") {
		t.Errorf("fix output missing confirmation:\n%s", out)
	}
	data, err := os.ReadFile(rc)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `eval "$(/home/test/.local/bin/mise activate zsh)"`) {
		t.Errorf("rc missing activate line:\n%s", data)
	}

	out, err = runCLI(t, deps, "doctor")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Shell integration OK") {
		t.Errorf("post-fix doctor should report OK:\n%s", out)
	}
}

func TestDoctorShimsVariantAndJSON(t *testing.T) {
	deps := unintegratedShellDeps(t)
	out, err := runCLI(t, deps, "doctor", "--fix", "--shims", "--yes", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var report struct {
		MiseInstalled bool `json:"mise_installed"`
		Changed       bool `json:"changed"`
		Integration   struct {
			Shell      string `json:"shell"`
			Integrated bool   `json:"integrated"`
		} `json:"integration"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("doctor JSON invalid: %v\n%s", err, out)
	}
	if !report.MiseInstalled || !report.Changed || !report.Integration.Integrated || report.Integration.Shell != "zsh" {
		t.Fatalf("unexpected doctor report: %+v", report)
	}
	data, err := os.ReadFile(filepath.Join(deps.HomeDir, ".zshrc"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `export PATH="$HOME/.local/share/mise/shims:$PATH"`) {
		t.Errorf("rc missing shims line:\n%s", data)
	}
}

func TestStatusWarnsAboutShellIntegration(t *testing.T) {
	deps := unintegratedShellDeps(t)
	out, err := runCLI(t, deps, "status")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Shell integration:", "outfitter doctor --fix", "mise activate zsh"} {
		if !strings.Contains(out, want) {
			t.Errorf("status output missing %q:\n%s", want, out)
		}
	}

	integrated, _ := testDeps(t)
	out, err = runCLI(t, integrated, "status")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "Shell integration:") {
		t.Errorf("integrated shell must not warn:\n%s", out)
	}
}

func TestUpdateAllConfirmedAndFailure(t *testing.T) {
	deps, _ := testDeps(t)
	out, err := runCLI(t, deps, "update", "--all", "--yes")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "✓ update codex") {
		t.Fatalf("unexpected update output:\n%s", out)
	}

	deps.RunPlan = func(_ context.Context, plan domain.ActionPlan, _ func(engine.StepEvent)) engine.PlanResult {
		result := successfulRun(plan)
		result.Steps[0].Success = false
		result.Steps[0].Err = errors.New("update failed")
		return result
	}
	_, err = runCLI(t, deps, "update", "--all", "--yes")
	requireExit(t, err, 1)
}

func TestRemoveDryRunConfirmedAndUnknown(t *testing.T) {
	deps, _ := testDeps(t)
	out, err := runCLI(t, deps, "remove", "codex", "--dry-run")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "remove codex") || !strings.Contains(out, "$ mise unuse --global") {
		t.Fatalf("unexpected remove dry-run:\n%s", out)
	}

	out, err = runCLI(t, deps, "remove", "codex", "--yes", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var payload jsonRunResult
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Succeeded != 1 || len(payload.Plan.Removals) != 1 {
		t.Fatalf("unexpected remove result: %+v", payload)
	}

	_, err = runCLI(t, deps, "remove", "not-a-tool", "--dry-run")
	requireExit(t, err, 2)
	_, err = runCLI(t, deps, "remove")
	requireExit(t, err, 2)
}

func TestRemoveFailureExitCode(t *testing.T) {
	deps, _ := testDeps(t)
	deps.RunPlan = func(_ context.Context, plan domain.ActionPlan, _ func(engine.StepEvent)) engine.PlanResult {
		result := successfulRun(plan)
		result.Steps[0].Success = false
		result.Steps[0].Err = errors.New("remove failed")
		return result
	}
	_, err := runCLI(t, deps, "remove", "codex", "--yes", "--json")
	requireExit(t, err, 1)
}

func TestCatalogExportParses(t *testing.T) {
	deps, _ := testDeps(t)
	out, err := runCLI(t, deps, "catalog", "export")
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := catalog.ParseCatalog([]byte(out))
	if err != nil {
		t.Fatalf("exported catalog does not parse: %v\n%s", err, out)
	}
	if len(parsed.Tools) != 45 || len(parsed.Profiles) != 4 {
		t.Fatalf("exported catalog has %d tools and %d profiles", len(parsed.Tools), len(parsed.Profiles))
	}
}

func TestUnknownCommandIsUsageError(t *testing.T) {
	err := normalizeExecuteError(errors.New(`unknown command "frobnicate" for "outfitter"`))
	requireExit(t, err, 2)
	if err := normalizeExecuteError(errors.New("network down")); ExitCode(err) != 1 {
		t.Fatalf("runtime error exit code = %d, want 1", ExitCode(err))
	}
}

func TestExitCodeHelper(t *testing.T) {
	if got := ExitCode(nil); got != 0 {
		t.Fatalf("nil exit code = %d", got)
	}
	if got := ExitCode(errors.New("boom")); got != 1 {
		t.Fatalf("plain error exit code = %d", got)
	}
	if got := ExitCode(&ExitError{Code: 2, Err: errors.New("usage")}); got != 2 {
		t.Fatalf("usage exit code = %d", got)
	}
}

func TestStatusWarnsAboutShadowedInstalls(t *testing.T) {
	deps, _ := testDeps(t)
	orig := deps.InspectLocal
	deps.InspectLocal = func(ctx context.Context, cat *catalog.Catalog) map[string]domain.ToolState {
		states := orig(ctx, cat)
		states["dust"] = domain.ToolState{
			InstalledVersion: "1.2.6",
			LatestVersion:    "1.2.6",
			BinaryPath:       "/usr/bin/dust",
			Provenance:       domain.ProvenanceMise,
			ShadowedBy:       "/usr/bin/dust",
			ShadowVersion:    "1.2.4",
			CheckStatus:      domain.StatusReady,
		}
		return states
	}
	out, err := runCLI(t, deps, "status")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Shadowed installs", "/usr/bin/dust (1.2.4) shadows managed 1.2.6", "sudo rm /usr/bin/dust"} {
		if !strings.Contains(out, want) {
			t.Fatalf("status output missing %q:\n%s", want, out)
		}
	}
}
