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

func pressKey(m *Model, code rune, text string) tea.Cmd {
	_, cmd := m.Update(tea.KeyPressMsg{Code: code, Text: text})
	return cmd
}

func typeText(m *Model, text string) {
	for _, runeValue := range text {
		m.Update(tea.KeyPressMsg{Code: runeValue, Text: string(runeValue)})
	}
}

func TestCategoryJumpsAndTabCollapse(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	m, err := NewModel(Options{Demo: true, Width: 120, Height: 40})
	if err != nil {
		t.Fatal(err)
	}

	pressKey(m, '2', "2")
	if m.selected != "boardwalk" {
		t.Fatalf("2 selected %q, want boardwalk", m.selected)
	}
	if m.collapsed["ai-tuis"] {
		t.Fatal("category 2 was not expanded")
	}
	m.collapsed["ai-tuis"] = true
	pressKey(m, '2', "2")
	if m.collapsed["ai-tuis"] {
		t.Fatal("pressing 2 should expand a collapsed category")
	}

	pressKey(m, '3', "3")
	if m.selected != "tmux" {
		t.Fatalf("3 selected %q, want tmux", m.selected)
	}
	pressKey(m, '4', "4")
	if m.selected != "browser-use" {
		t.Fatalf("4 selected %q, want browser-use", m.selected)
	}
	pressKey(m, '1', "1")
	if m.selected != "claude-code" {
		t.Fatalf("1 selected %q, want claude-code", m.selected)
	}

	pressKey(m, tea.KeyTab, "")
	if !m.collapsed["harnesses"] {
		t.Fatal("Tab did not collapse the focused category")
	}
	if m.selected == "claude-code" {
		t.Fatal("selection remained inside a collapsed category")
	}
}

func TestAcceptanceNavigationAndPlanMutations(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	m, err := NewModel(Options{Demo: true, Width: 120, Height: 40})
	if err != nil {
		t.Fatal(err)
	}

	pressKey(m, tea.KeyPgDown, "")
	if m.selected != "quotary" {
		t.Fatalf("pgdown selected %q, want quotary", m.selected)
	}
	pressKey(m, tea.KeyPgUp, "")
	if m.selected != "claude-code" {
		t.Fatalf("pgup selected %q, want claude-code", m.selected)
	}

	m.selected = "codex"
	pressKey(m, ' ', " ")
	if m.actions["codex"] != domain.ActionUpdate {
		t.Fatalf("Space queued %q, want update", m.actions["codex"])
	}
	if !strings.Contains(m.notice, "Update queued") {
		t.Fatalf("Space notice = %q", m.notice)
	}

	m.actions = make(map[string]domain.DesiredAction)
	m.rebuildPlan()
	pressKey(m, 'u', "u")
	for _, id := range []string{"codex", "muse", "trailmark", "lazygit", "browser-use"} {
		if m.actions[id] != domain.ActionUpdate {
			t.Errorf("u did not queue update for %s: %q", id, m.actions[id])
		}
	}
	if m.notice != "Queued 5 updates" {
		t.Fatalf("u notice = %q", m.notice)
	}
	pressKey(m, 'u', "u")
	if m.notice != "All outdated tools already queued" {
		t.Fatalf("second u notice = %q", m.notice)
	}

	m.actions = make(map[string]domain.DesiredAction)
	m.rebuildPlan()
	m.selected = "muse"
	pressKey(m, 'U', "U")
	if m.actions["muse"] != domain.ActionUpdate {
		t.Fatalf("U queued %q, want update", m.actions["muse"])
	}
	m.selected = "gemini-cli"
	pressKey(m, 'U', "U")
	if m.actions["gemini-cli"] != domain.ActionInstall {
		t.Fatalf("U on uninstalled queued %q, want install", m.actions["gemini-cli"])
	}

	m.actions = make(map[string]domain.DesiredAction)
	m.rebuildPlan()
	m.selected = "codex"
	pressKey(m, 'g', "g")
	if m.actions["gemini-cli"] != domain.ActionInstall || m.actions["copilot-cli"] != domain.ActionInstall {
		t.Fatal("g did not queue installs for uninstalled category tools")
	}
	if m.actions["codex"] != domain.ActionUpdate || m.actions["muse"] != domain.ActionUpdate {
		t.Fatal("g did not queue updates for outdated category tools")
	}
	pressKey(m, 'g', "g")
	if len(m.actions) != 0 {
		t.Fatalf("second g left actions: %v", m.actions)
	}

	pressKey(m, 'x', "x")
	if len(m.actions) != 0 || m.notice != "Cleared all pending plan actions" {
		t.Fatalf("x did not reset plan: actions=%v notice=%q", m.actions, m.notice)
	}
}

func TestFilterCycling(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	m, err := NewModel(Options{Demo: true, Width: 120, Height: 40})
	if err != nil {
		t.Fatal(err)
	}
	if m.filter != filterAll || !strings.Contains(m.View().Content, "[ All (45) ]") {
		t.Fatalf("initial filter = %q", m.filter)
	}

	pressKey(m, 'f', "f")
	if m.filter != filterOutdated {
		t.Fatalf("filter = %q, want outdated", m.filter)
	}
	if !strings.Contains(m.View().Content, "[ Outdated (5) ]") {
		t.Fatal("outdated filter badge missing")
	}
	for _, id := range m.visibleToolIDs() {
		if !m.states[id].IsOutdated() {
			t.Errorf("outdated filter includes %s", id)
		}
	}

	pressKey(m, 'f', "f")
	if m.filter != filterPending {
		t.Fatalf("filter = %q, want pending", m.filter)
	}
	ids := m.visibleToolIDs()
	if len(ids) != 2 || ids[0] != "gemini-cli" || ids[1] != "quotary" {
		t.Fatalf("pending visible = %v", ids)
	}

	pressKey(m, 'f', "f")
	if m.filter != filterInstalled || !strings.Contains(m.View().Content, "[ Installed (39) ]") {
		t.Fatalf("filter = %q", m.filter)
	}
	pressKey(m, 'f', "f")
	if m.filter != filterUninstalled || !strings.Contains(m.View().Content, "[ Not installed (6) ]") {
		t.Fatalf("filter = %q", m.filter)
	}
	pressKey(m, 'f', "f")
	if m.filter != filterAll {
		t.Fatalf("filter wrapped to %q", m.filter)
	}
}

func TestIncrementalSearchCapturesShortcuts(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	m, err := NewModel(Options{Demo: true, Width: 120, Height: 40})
	if err != nil {
		t.Fatal(err)
	}
	initialActions := len(m.actions)

	pressKey(m, '/', "/")
	if !m.editing {
		t.Fatal("/ did not enter search editing mode")
	}
	typeText(m, "codexuq")
	if m.query != "codexuq" {
		t.Fatalf("query = %q, want codexuq", m.query)
	}
	if m.view != "catalog" || len(m.actions) != initialActions {
		t.Fatal("single-key shortcuts fired while editing search")
	}
	if !strings.Contains(m.View().Content, "/ ") || !strings.Contains(m.View().Content, "codexuq") || !strings.Contains(m.View().Content, "(Esc to clear)") {
		t.Fatal("search bar is not visible while editing")
	}

	pressKey(m, tea.KeyBackspace, "")
	if m.query != "codexu" {
		t.Fatalf("backspace query = %q", m.query)
	}
	pressKey(m, tea.KeyEnter, "")
	if m.editing || m.query != "codexu" {
		t.Fatalf("Enter did not commit search: editing=%v query=%q", m.editing, m.query)
	}
	pressKey(m, tea.KeyEscape, "")
	if m.query != "" {
		t.Fatalf("Esc did not clear committed query: %q", m.query)
	}

	pressKey(m, '/', "/")
	typeText(m, "codex")
	pressKey(m, tea.KeyEscape, "")
	if m.editing || m.query != "" {
		t.Fatalf("Esc in editing did not clear: editing=%v query=%q", m.editing, m.query)
	}
}

func TestProfileSelectorAppliesServerProfile(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	m, err := NewModel(Options{Demo: true, Width: 120, Height: 40})
	if err != nil {
		t.Fatal(err)
	}
	pressKey(m, 'p', "p")
	if m.view != "profiles" {
		t.Fatalf("view = %q, want profiles", m.view)
	}
	screen := m.View().Content
	for _, marker := range []string{"SELECT OUTFITTER PROFILE (p)", "Full Desktop", "Headless Server / VPS", "Coding Harnesses Only", "Minimal Core", "[active]"} {
		if !strings.Contains(screen, marker) {
			t.Errorf("profile modal missing %q", marker)
		}
	}

	pressKey(m, 'j', "j")
	if m.profileCursor != 1 {
		t.Fatalf("profileCursor = %d, want 1", m.profileCursor)
	}
	pressKey(m, tea.KeyEnter, "")
	if m.view != "catalog" || m.activeProfileID != "server" {
		t.Fatalf("applied view=%q profile=%q", m.view, m.activeProfileID)
	}
	if m.actions["btop"] != domain.ActionInstall {
		t.Fatalf("server profile did not queue btop install: %q", m.actions["btop"])
	}
	if _, ok := m.actions["gemini-cli"]; ok {
		t.Fatal("server profile kept a non-member uninstalled install")
	}
	if !strings.Contains(m.notice, "Headless Server / VPS") {
		t.Fatalf("profile notice = %q", m.notice)
	}

	pressKey(m, 'p', "p")
	pressKey(m, '4', "4")
	if m.activeProfileID != "minimal" {
		t.Fatalf("digit 4 applied profile %q", m.activeProfileID)
	}
}

func TestPlanReviewApplyingAndBootstrap(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	m, err := NewModel(Options{Demo: true, Width: 120, Height: 40})
	if err != nil {
		t.Fatal(err)
	}
	m.actions = map[string]domain.DesiredAction{
		"btop": domain.ActionInstall,
		"tmux": domain.ActionUpdate,
	}
	m.rebuildPlan()

	pressKey(m, 'a', "a")
	if m.view != "plan" {
		t.Fatalf("view = %q, want plan", m.view)
	}
	screen := m.View().Content
	for _, marker := range []string{
		"DRY-RUN EXECUTION PLAN (Nothing has been modified on disk yet)",
		"Summary: +1 install · ↑1 update · -0 remove",
		"[sudo required for apt]",
		"INSTALLATIONS (+1)",
		"UPDATES (↑1)",
		"[apt·sudo]",
	} {
		if !strings.Contains(screen, marker) {
			t.Errorf("plan screen missing %q", marker)
		}
	}

	pressKey(m, tea.KeyEnter, "")
	if m.view != "applying" {
		t.Fatalf("view = %q, want applying", m.view)
	}
	if len(m.execSteps) != 2 {
		t.Fatalf("execSteps = %d, want 2", len(m.execSteps))
	}
	screen = m.View().Content
	for _, marker := range []string{"LIVE EXECUTION PROGRESS", "Overall progress", "LIVE OUTPUT", "0 of 2 completed"} {
		if !strings.Contains(screen, marker) {
			t.Errorf("applying screen missing %q", marker)
		}
	}
	m.AdvanceExecution()
	done, _, _ := m.executionCounts()
	if done != 1 {
		t.Fatalf("AdvanceExecution done = %d, want 1", done)
	}
	if !strings.Contains(m.View().Content, "1 of 2 completed") {
		t.Fatal("progress header did not advance")
	}
	pressKey(m, tea.KeyEscape, "")
	if m.view != "catalog" {
		t.Fatalf("Esc from applying view = %q", m.view)
	}

	m.miseInstalled = false
	m.rebuildPlan()
	pressKey(m, 'a', "a")
	screen = m.View().Content
	if !strings.Contains(screen, "0. BOOTSTRAP PREREQUISITE (mise)") {
		t.Fatal("plan screen is missing the mise bootstrap prerequisite")
	}
}

// TestPlanReviewWarnsAboutStrayBinaries verifies that the Plan & Apply
// review warns when a migrating update leaves an unmanaged binary on
// disk and shows the exact command to remove it.
func TestPlanReviewWarnsAboutStrayBinaries(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	m, err := NewModel(Options{Demo: true, Width: 120, Height: 40})
	if err != nil {
		t.Fatal(err)
	}
	state := m.states["tmux"]
	state.BinaryPath = "/usr/local/bin/tmux"
	state.Provenance = domain.ProvenanceExternal
	m.states["tmux"] = state
	m.actions = map[string]domain.DesiredAction{"tmux": domain.ActionUpdate}
	m.rebuildPlan()

	if len(m.plan.Updates) != 1 || !m.plan.Updates[0].Migrate {
		t.Fatalf("expected one migrating update, got %+v", m.plan.Updates)
	}

	pressKey(m, 'a', "a")
	if m.view != "plan" {
		t.Fatalf("view = %q, want plan", m.view)
	}
	screen := m.View().Content
	for _, marker := range []string{
		"STRAY BINARIES",
		"/usr/local/bin/tmux",
		"sudo rm /usr/local/bin/tmux",
	} {
		if !strings.Contains(screen, marker) {
			t.Errorf("plan screen missing %q", marker)
		}
	}
}

// TestShellWarningHeaderAndFix verifies the persistent shell-integration
// warning line and the confirmed [B] fix action.
func TestShellWarningHeaderAndFix(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	fixed := false
	m, err := NewModel(Options{
		Demo: true, Width: 120, Height: 40,
		ShellWarning: "⚠ mise is not active in your zsh shell — mise-managed tools won't resolve in your terminal · [B] fix shell integration",
		OnFixShell: func() (string, error) {
			fixed = true
			return "Added mise activation to ~/.zshrc — open a new terminal to use it", nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(m.View().Content, "mise is not active in your zsh shell") {
		t.Fatalf("header missing shell warning:\n%s", m.View().Content)
	}

	// Cancelling the confirmation leaves everything untouched.
	pressKey(m, 'B', "B")
	if !m.confirmShellFix {
		t.Fatal("B should ask for confirmation")
	}
	pressKey(m, 'n', "n")
	if fixed {
		t.Fatal("cancelled fix must not run")
	}
	if m.shellWarning == "" {
		t.Fatal("cancelled fix must keep the warning")
	}

	// Confirming runs the fix and clears the warning.
	pressKey(m, 'B', "B")
	pressKey(m, 'y', "y")
	if !fixed {
		t.Fatal("confirmed fix did not run")
	}
	if m.shellWarning != "" {
		t.Fatalf("shellWarning = %q, want cleared after fix", m.shellWarning)
	}
	if !strings.Contains(m.notice, "Added mise activation") {
		t.Fatalf("notice = %q, want fix confirmation", m.notice)
	}
	if strings.Contains(m.View().Content, "mise is not active") {
		t.Fatal("warning still rendered after fix")
	}
}

// TestShadowSweepHeaderAndKey verifies the shadowed count in the status
// bar and the confirmed [S] sweep, including the sudo pre-flight detour.
func TestShadowSweepHeaderAndKey(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	var swept []string
	m, err := NewModel(Options{
		Demo: true, Width: 120, Height: 40,
		OnRemoveShadows: func(paths []string) ([]string, error) {
			swept = append([]string(nil), paths...)
			return paths, nil
		},
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

	if !strings.Contains(m.View().Content, "⚠ 2 shadowed") {
		t.Fatalf("status bar missing shadowed count:\n%s", m.View().Content)
	}

	// Cancel keeps everything.
	pressKey(m, 'S', "S")
	if !m.confirmShadowSweep {
		t.Fatal("S should ask for confirmation")
	}
	pressKey(m, 'n', "n")
	if len(swept) != 0 {
		t.Fatal("cancelled sweep must not run")
	}

	// Confirm sweeps (home paths need no sudo) and clears the count.
	pressKey(m, 'S', "S")
	pressKey(m, 'y', "y")
	if len(swept) != 2 {
		t.Fatalf("swept = %v, want 2 paths", swept)
	}
	if !strings.Contains(m.notice, "Removed 2 shadowed") {
		t.Fatalf("notice = %q, want removal summary", m.notice)
	}
	if strings.Contains(m.View().Content, "⚠ 2 shadowed") {
		t.Fatal("shadowed count still rendered after sweep")
	}
	for id := range map[string]string{"tmux": "", "dust": ""} {
		if m.states[id].ShadowedBy != "" {
			t.Errorf("%s shadow not cleared: %q", id, m.states[id].ShadowedBy)
		}
	}

	// A system path detours through the sudo pre-flight first.
	state := m.states["tig"]
	state.ShadowedBy = "/usr/local/bin/tig"
	state.ShadowVersion = "2.5.2"
	m.states["tig"] = state
	swept = nil
	pressKey(m, 'S', "S")
	cmd := pressKey(m, 'y', "y")
	if cmd == nil {
		t.Fatal("sudo-needed sweep should return the pre-flight command")
	}
	if len(swept) != 0 {
		t.Fatal("sweep ran before sudo verification")
	}
	if !m.pendingShadowSweep {
		t.Fatal("pendingShadowSweep should be set while sudo verifies")
	}
	m.Update(shadowSudoVerifiedMsg{})
	if len(swept) != 1 || swept[0] != "/usr/local/bin/tig" {
		t.Fatalf("swept after sudo = %v, want [/usr/local/bin/tig]", swept)
	}
}

// TestPlanReviewUnmanagedRemovals verifies the Plan & Apply review shows
// a direct rm for an external binary and a blocked warning for an
// install owned by a different manager.
func TestPlanReviewUnmanagedRemovals(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	m, err := NewModel(Options{Demo: true, Width: 120, Height: 40})
	if err != nil {
		t.Fatal(err)
	}
	tmux := m.states["tmux"]
	tmux.BinaryPath = "/usr/local/bin/tmux"
	tmux.Provenance = domain.ProvenanceExternal
	m.states["tmux"] = tmux
	dust := m.states["dust"]
	dust.BinaryPath = "/usr/bin/dust"
	dust.Provenance = domain.ProvenanceAPT
	dust.ProvenanceDetail = "APT package (du-dust)"
	m.states["dust"] = dust
	m.actions = map[string]domain.DesiredAction{
		"tmux": domain.ActionRemove,
		"dust": domain.ActionRemove,
	}
	m.rebuildPlan()

	pressKey(m, 'a', "a")
	screen := m.View().Content
	for _, marker := range []string{
		"sudo rm /usr/local/bin/tmux",
		"cannot remove",
		"installed via apt",
	} {
		if !strings.Contains(screen, marker) {
			t.Errorf("plan screen missing %q", marker)
		}
	}
}

func TestPlanEmptyStateAndHelp(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	m, err := NewModel(Options{Demo: true, Width: 80, Height: 24})
	if err != nil {
		t.Fatal(err)
	}
	pressKey(m, 'x', "x")
	pressKey(m, 'a', "a")
	if !strings.Contains(m.View().Content, "No pending changes in your plan.") {
		t.Fatal("empty plan state is missing")
	}
	pressKey(m, tea.KeyEscape, "")
	pressKey(m, '?', "?")
	if m.view != "help" {
		t.Fatalf("view = %q, want help", m.view)
	}
	screen := m.View().Content
	for _, marker := range []string{"OUTFITTER KEYBOARD SHORTCUTS (?)", "↑ / ↓ or j / k", "Space", "u", "U", "g", "p", "a", "x", "f", "/", "b", "r", "Esc / q"} {
		if !strings.Contains(screen, marker) {
			t.Errorf("help screen missing %q", marker)
		}
	}
	pressKey(m, '?', "?")
	if m.view != "catalog" {
		t.Fatalf("? did not close help: %q", m.view)
	}
}

func TestQuitConfirmation(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	m, err := NewModel(Options{Demo: true, Width: 120, Height: 40})
	if err != nil {
		t.Fatal(err)
	}
	cmd := pressKey(m, 'q', "q")
	if cmd != nil || !m.confirmQuit {
		t.Fatal("q with pending changes should prompt, not quit")
	}
	if !strings.Contains(m.notice, "Unapplied changes in plan") {
		t.Fatalf("quit notice = %q", m.notice)
	}
	pressKey(m, tea.KeyEscape, "")
	if m.confirmQuit {
		t.Fatal("Esc did not cancel quit confirmation")
	}

	pressKey(m, 'x', "x")
	cmd = pressKey(m, 'q', "q")
	if cmd == nil {
		t.Fatal("q with a clean plan should quit immediately")
	}

	m.actions["codex"] = domain.ActionUpdate
	m.rebuildPlan()
	pressKey(m, 'q', "q")
	cmd = pressKey(m, 'y', "y")
	if cmd == nil {
		t.Fatal("y during quit confirmation should quit")
	}
}

func TestAllViewsScreenGeometry(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	sizes := []struct {
		width  int
		height int
	}{
		{120, 40},
		{80, 24},
	}
	views := []struct {
		name  string
		setup func(*Model)
	}{
		{"catalog", func(*Model) {}},
		{"profiles", func(m *Model) { pressKey(m, 'p', "p") }},
		{"plan", func(m *Model) { m.actions["btop"] = domain.ActionInstall; m.rebuildPlan(); pressKey(m, 'a', "a") }},
		{"applying", func(m *Model) {
			m.actions["btop"] = domain.ActionInstall
			m.rebuildPlan()
			m.startExecution()
			m.view = "applying"
		}},
		{"help", func(m *Model) { pressKey(m, '?', "?") }},
		{"detail", func(m *Model) { m.view = "detail" }},
	}
	for _, size := range sizes {
		for _, view := range views {
			t.Run(fmt.Sprintf("%s/%s", displaySize(size.width, size.height), view.name), func(t *testing.T) {
				m, err := NewModel(Options{Demo: true, Width: size.width, Height: size.height})
				if err != nil {
					t.Fatal(err)
				}
				view.setup(m)
				assertScreen(t, m.View().Content, size.width, size.height)
			})
		}
	}
}

func TestProgramEnvDowngradesKittyTerm(t *testing.T) {
	original := []string{"HOME=/tmp", "TERM=xterm-kitty", "LANG=en_US.UTF-8"}
	out, kitty := programEnv(original)
	if !kitty {
		t.Fatal("expected kitty detection")
	}
	if out[1] != "TERM=xterm-256color" {
		t.Fatalf("got %q, want TERM=xterm-256color", out[1])
	}
	if original[1] != "TERM=xterm-kitty" {
		t.Fatal("programEnv mutated the input slice")
	}

	other, kitty := programEnv([]string{"TERM=tmux-256color"})
	if kitty || other[0] != "TERM=tmux-256color" {
		t.Fatalf("unexpected rewrite for non-kitty terminal: %v (%v)", other, kitty)
	}
}

func TestInspectorWarnsAboutShadowedInstall(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	m, err := NewModel(Options{Demo: true, Width: 120, Height: 40})
	if err != nil {
		t.Fatal(err)
	}
	tool, ok := m.selectedTool()
	if !ok {
		t.Fatal("no selected tool")
	}
	state := m.states[tool.ID]
	state.ShadowedBy = "/usr/bin/" + tool.Binary
	state.ShadowVersion = "0.0.1"
	m.states[tool.ID] = state
	screen := m.View().Content
	for _, want := range []string{"Shadowed by /usr/bin/", "sudo rm /usr/bin/"} {
		if !strings.Contains(screen, want) {
			t.Fatalf("inspector missing %q:\n%s", want, screen)
		}
	}
}
