package domain_test

import (
	"reflect"
	"testing"

	"github.com/juanrubio/outfitter/internal/domain"
)

func TestBackendTypeValidation(t *testing.T) {
	valid := []domain.BackendType{
		domain.BackendMiseAqua,
		domain.BackendMiseUbi,
		domain.BackendMiseGitHub,
		domain.BackendMiseNPM,
		domain.BackendUV,
		domain.BackendAPT,
		domain.BackendVendor,
		domain.BackendSource,
	}
	for _, backend := range valid {
		if !backend.IsValid() {
			t.Errorf("%q should be valid", backend)
		}
	}
	for _, backend := range []domain.BackendType{"", "mise", "pipx", "npm"} {
		if backend.IsValid() {
			t.Errorf("%q should be invalid", backend)
		}
	}

	requiresMise := map[domain.BackendType]bool{
		domain.BackendMiseAqua:   true,
		domain.BackendMiseUbi:    true,
		domain.BackendMiseGitHub: true,
		domain.BackendMiseNPM:    true,
		domain.BackendUV:         true,
		domain.BackendAPT:        false,
		domain.BackendVendor:     false,
		domain.BackendSource:     false,
	}
	for backend, want := range requiresMise {
		if got := backend.RequiresMise(); got != want {
			t.Errorf("%q RequiresMise() = %v, want %v", backend, got, want)
		}
	}
}

func TestNextAction(t *testing.T) {
	tests := []struct {
		name      string
		current   domain.DesiredAction
		installed bool
		outdated  bool
		want      domain.DesiredAction
	}{
		{"not installed empty", "", false, false, domain.ActionInstall},
		{"not installed none", domain.ActionNone, false, false, domain.ActionInstall},
		{"not installed install", domain.ActionInstall, false, false, domain.ActionNone},
		{"not installed remove", domain.ActionRemove, false, false, domain.ActionInstall},
		{"not installed update", domain.ActionUpdate, false, false, domain.ActionNone},

		{"outdated empty", "", true, true, domain.ActionUpdate},
		{"outdated none", domain.ActionNone, true, true, domain.ActionUpdate},
		{"outdated update", domain.ActionUpdate, true, true, domain.ActionRemove},
		{"outdated remove", domain.ActionRemove, true, true, domain.ActionNone},
		{"outdated install", domain.ActionInstall, true, true, domain.ActionUpdate},

		{"current empty", "", true, false, domain.ActionRemove},
		{"current none", domain.ActionNone, true, false, domain.ActionRemove},
		{"current install", domain.ActionInstall, true, false, domain.ActionRemove},
		{"current update", domain.ActionUpdate, true, false, domain.ActionRemove},
		{"current remove", domain.ActionRemove, true, false, domain.ActionNone},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := domain.NextAction(test.current, test.installed, test.outdated)
			if got != test.want {
				t.Fatalf("NextAction(%q, %v, %v) = %q, want %q", test.current, test.installed, test.outdated, got, test.want)
			}
		})
	}
}

func TestMarker(t *testing.T) {
	tests := []struct {
		name      string
		action    domain.DesiredAction
		installed bool
		outdated  bool
		mark      string
		token     string
		label     string
	}{
		{"install queued", domain.ActionInstall, false, false, "[+]", "accent", "Install queued"},
		{"update queued", domain.ActionUpdate, true, true, "[↑]", "accent", "Update queued"},
		{"remove queued", domain.ActionRemove, true, false, "[-]", "error", "Remove queued"},
		{"outdated", domain.ActionNone, true, true, "[!]", "low", "Update available"},
		{"current", domain.ActionNone, true, false, "[✓]", "good", "Installed (current)"},
		{"missing", domain.ActionNone, false, false, "[ ]", "muted", "Not installed"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mark, token, label := domain.Marker(test.action, test.installed, test.outdated)
			if mark != test.mark || token != test.token || label != test.label {
				t.Fatalf("Marker() = (%q, %q, %q), want (%q, %q, %q)",
					mark, token, label, test.mark, test.token, test.label)
			}
		})
	}
}

func TestToolState(t *testing.T) {
	missing := domain.ToolState{LatestVersion: "1.0.0"}
	if missing.IsInstalled() {
		t.Fatal("missing tool should not be installed")
	}
	if missing.IsOutdated() {
		t.Fatal("missing tool should not be outdated")
	}

	current := domain.ToolState{InstalledVersion: "1.0.0", LatestVersion: "1.0.0"}
	if !current.IsInstalled() {
		t.Fatal("current tool should be installed")
	}
	if current.IsOutdated() {
		t.Fatal("current tool should not be outdated")
	}

	outdated := domain.ToolState{InstalledVersion: "1.0.0", LatestVersion: "1.1.0"}
	if !outdated.IsOutdated() {
		t.Fatal("older tool should be outdated")
	}

	unknownLatest := domain.ToolState{InstalledVersion: "1.0.0"}
	if unknownLatest.IsOutdated() {
		t.Fatal("tool with unknown latest version should not be outdated")
	}

	newerInstalled := domain.ToolState{InstalledVersion: "2.0.23", LatestVersion: "1.19.3"}
	if newerInstalled.IsOutdated() {
		t.Fatal("newer installed version (e.g. OpenCode v2 vs v1 release tag) should not be marked outdated")
	}

	nonNumericLatest := domain.ToolState{InstalledVersion: "1.8.1", LatestVersion: "jq"}
	if nonNumericLatest.IsOutdated() {
		t.Fatal("non-numeric latest version should not be marked outdated")
	}

	if got := domain.CompareVersions("2.0.23", "1.19.3"); got <= 0 {
		t.Fatalf("CompareVersions(2.0.23, 1.19.3) = %d, want > 0", got)
	}
	if got := domain.CompareVersions("1.4.12-R6501.0", "1.4.10-R6427.1"); got <= 0 {
		t.Fatalf("CompareVersions(1.4.12-R6501.0, 1.4.10-R6427.1) = %d, want > 0", got)
	}
}

func TestDefaultCommands(t *testing.T) {
	tests := []struct {
		name    string
		tool    domain.Tool
		install string
		update  string
		remove  string
	}{
		{
			name:    "aqua",
			tool:    domain.Tool{Backend: domain.BackendMiseAqua, Package: "jesseduffield/lazygit", Binary: "lazygit"},
			install: "mise use --global aqua:jesseduffield/lazygit@latest",
			update:  "mise upgrade aqua:jesseduffield/lazygit",
			remove:  "mise unuse --global aqua:jesseduffield/lazygit",
		},
		{
			name:    "ubi already prefixed",
			tool:    domain.Tool{Backend: domain.BackendMiseUbi, Package: "ubi:juanrubio/boardwalk", Binary: "boardwalk"},
			install: "mise use --global ubi:juanrubio/boardwalk@latest",
			update:  "mise upgrade ubi:juanrubio/boardwalk",
			remove:  "mise unuse --global ubi:juanrubio/boardwalk",
		},
		{
			name:    "npm",
			tool:    domain.Tool{Backend: domain.BackendMiseNPM, Package: "@openai/codex", Binary: "codex"},
			install: "mise use --global npm:@openai/codex@latest",
			update:  "mise upgrade npm:@openai/codex",
			remove:  "mise unuse --global npm:@openai/codex",
		},
		{
			name:    "uv",
			tool:    domain.Tool{Backend: domain.BackendUV, Package: "browser-use", Binary: "browser-use"},
			install: "mise x uv -- uv tool install browser-use",
			update:  "mise x uv -- uv tool upgrade browser-use",
			remove:  "mise x uv -- uv tool uninstall browser-use",
		},
		{
			name:    "apt",
			tool:    domain.Tool{Backend: domain.BackendAPT, Package: "tmux", Binary: "tmux"},
			install: "sudo apt-get install -y tmux",
			update:  "sudo apt-get install --only-upgrade -y tmux",
			remove:  "sudo apt-get remove -y tmux",
		},
		{
			name:    "source",
			tool:    domain.Tool{Backend: domain.BackendSource, Binary: "quotary", SourceDir: "~/repos/quotary"},
			install: "go -C ~/repos/quotary build -o ~/.local/bin/quotary ./cmd/quotary",
			update:  "go -C ~/repos/quotary build -o ~/.local/bin/quotary ./cmd/quotary",
			remove:  "rm -f ~/.local/bin/quotary",
		},
		{
			name:    "source build override",
			tool:    domain.Tool{Backend: domain.BackendSource, Binary: "quotary", BuildCmd: "go build -o out/quotary ./cmd/quotary"},
			install: "go build -o out/quotary ./cmd/quotary",
			update:  "go build -o out/quotary ./cmd/quotary",
			remove:  "rm -f ~/.local/bin/quotary",
		},
		{
			name:    "vendor without explicit commands",
			tool:    domain.Tool{Backend: domain.BackendVendor, Package: "vendor/tool", Binary: "tool"},
			install: "",
			update:  "",
			remove:  "",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.tool.InstallCommand(); got != test.install {
				t.Errorf("InstallCommand() = %q, want %q", got, test.install)
			}
			if got := test.tool.UpdateCommand(); got != test.update {
				t.Errorf("UpdateCommand() = %q, want %q", got, test.update)
			}
			if got := test.tool.RemoveCommand(); got != test.remove {
				t.Errorf("RemoveCommand() = %q, want %q", got, test.remove)
			}
		})
	}
}

func TestExplicitCommandsWin(t *testing.T) {
	tool := domain.Tool{
		Backend:    domain.BackendAPT,
		Package:    "tmux",
		Binary:     "tmux",
		InstallCmd: "custom install",
		UpdateCmd:  "custom update",
		RemoveCmd:  "custom remove",
	}
	if got := tool.CommandFor(domain.ActionInstall); got != "custom install" {
		t.Fatalf("install command = %q", got)
	}
	if got := tool.CommandFor(domain.ActionUpdate); got != "custom update" {
		t.Fatalf("update command = %q", got)
	}
	if got := tool.CommandFor(domain.ActionRemove); got != "custom remove" {
		t.Fatalf("remove command = %q", got)
	}
	if got := tool.CommandFor(domain.ActionNone); got != "" {
		t.Fatalf("none command = %q", got)
	}
}

func TestBuildActionPlan(t *testing.T) {
	tools := []domain.Tool{
		{ID: "tmux", Name: "tmux", Binary: "tmux", Backend: domain.BackendAPT, Package: "tmux", Sudo: true},
		{ID: "codex", Name: "Codex", Binary: "codex", Backend: domain.BackendMiseNPM, Package: "@openai/codex"},
		{
			ID: "claude-code", Name: "Claude Code", Binary: "claude", Backend: domain.BackendVendor,
			InstallCmd: "install claude", UpdateCmd: "update claude", RemoveCmd: "remove claude",
		},
		{ID: "quotary", Name: "Quotary", Binary: "quotary", Backend: domain.BackendSource, SourceDir: "~/repos/quotary"},
	}
	states := map[string]domain.ToolState{
		"tmux":        {LatestVersion: "3.4"},
		"codex":       {InstalledVersion: "0.163.0", LatestVersion: "0.165.0"},
		"claude-code": {InstalledVersion: "2.1.289", LatestVersion: "2.1.289"},
		"quotary":     {LatestVersion: "0.1.0-dev"},
	}
	actions := map[string]domain.DesiredAction{
		"tmux":        domain.ActionInstall,
		"codex":       domain.ActionUpdate,
		"claude-code": domain.ActionRemove,
		"quotary":     domain.ActionInstall,
	}

	plan := domain.BuildActionPlan(tools, states, actions, false, "2026.10.1")

	if plan.BootstrapStep == nil {
		t.Fatal("expected a mise bootstrap step")
	}
	if plan.BootstrapStep.ToVersion != "2026.10.1" {
		t.Fatalf("bootstrap version = %q", plan.BootstrapStep.ToVersion)
	}
	if plan.BootstrapStep.Command != domain.BootstrapMiseCommand {
		t.Fatalf("bootstrap command = %q", plan.BootstrapStep.Command)
	}

	installs, updates, removals := plan.Counts()
	if installs != 2 || updates != 1 || removals != 1 {
		t.Fatalf("counts = (%d, %d, %d), want (2, 1, 1)", installs, updates, removals)
	}
	if plan.TotalSteps() != 5 {
		t.Fatalf("TotalSteps() = %d, want 5", plan.TotalSteps())
	}
	if !plan.NeedsSudo() {
		t.Fatal("plan with an apt install should need sudo")
	}
	if plan.IsEmpty() {
		t.Fatal("plan should not be empty")
	}

	if len(plan.Installations) != 2 {
		t.Fatalf("installations = %d", len(plan.Installations))
	}
	if plan.Installations[0].Tool.ID != "tmux" || plan.Installations[1].Tool.ID != "quotary" {
		t.Fatalf("installation order = %q, %q", plan.Installations[0].Tool.ID, plan.Installations[1].Tool.ID)
	}
	if plan.Installations[0].Command != "sudo apt-get install -y tmux" {
		t.Fatalf("tmux command = %q", plan.Installations[0].Command)
	}
	if plan.Installations[1].Command != "go -C ~/repos/quotary build -o ~/.local/bin/quotary ./cmd/quotary" {
		t.Fatalf("quotary command = %q", plan.Installations[1].Command)
	}

	if len(plan.Updates) != 1 {
		t.Fatalf("updates = %d", len(plan.Updates))
	}
	update := plan.Updates[0]
	if update.FromVersion != "0.163.0" || update.ToVersion != "0.165.0" {
		t.Fatalf("update versions = %q -> %q", update.FromVersion, update.ToVersion)
	}
	if update.Command != "mise upgrade npm:@openai/codex" {
		t.Fatalf("update command = %q", update.Command)
	}

	if len(plan.Removals) != 1 || plan.Removals[0].Command != "remove claude" {
		t.Fatalf("removals = %#v", plan.Removals)
	}

	steps := plan.Steps()
	if len(steps) != 5 {
		t.Fatalf("steps = %d", len(steps))
	}
	if steps[0].Action != domain.ActionInstall || steps[0].Tool.ID != "mise" {
		t.Fatalf("first step = %#v", steps[0])
	}
	if steps[1].Tool.ID != "tmux" || steps[2].Tool.ID != "quotary" {
		t.Fatalf("install step order = %q, %q", steps[1].Tool.ID, steps[2].Tool.ID)
	}
}

// TestBuildActionPlanMigratesUnmanagedUpdates verifies that an update
// queued against an installation the catalog backend does not own (an
// external PATH binary) becomes a migration: the step keeps the update
// action and version range but carries the install command, because
// upgrade-only commands (mise upgrade, apt --only-upgrade) are no-ops for
// tools their backend never installed.
func TestBuildActionPlanMigratesUnmanagedUpdates(t *testing.T) {
	tools := []domain.Tool{
		{ID: "dust", Name: "dust", Binary: "dust", Backend: domain.BackendMiseAqua, Package: "bootandy/dust"},
		{ID: "tig", Name: "tig", Binary: "tig", Backend: domain.BackendAPT, Package: "tig", Sudo: true},
		{ID: "codex", Name: "Codex", Binary: "codex", Backend: domain.BackendMiseNPM, Package: "@openai/codex"},
	}
	states := map[string]domain.ToolState{
		"dust":  {InstalledVersion: "1.2.4", LatestVersion: "1.2.6", Provenance: domain.ProvenanceExternal},
		"tig":   {InstalledVersion: "2.5.2", LatestVersion: "2.6.0", Provenance: domain.ProvenanceExternal},
		"codex": {InstalledVersion: "0.163.0", LatestVersion: "0.165.0", Provenance: domain.ProvenanceMise},
	}
	actions := map[string]domain.DesiredAction{
		"dust":  domain.ActionUpdate,
		"tig":   domain.ActionUpdate,
		"codex": domain.ActionUpdate,
	}

	plan := domain.BuildActionPlan(tools, states, actions, true, "")

	if len(plan.Updates) != 3 {
		t.Fatalf("updates = %d, want 3", len(plan.Updates))
	}
	byID := make(map[string]domain.PlanStep, len(plan.Updates))
	for _, step := range plan.Updates {
		byID[step.Tool.ID] = step
	}

	dust := byID["dust"]
	if !dust.Migrate {
		t.Errorf("dust: Migrate = false, want true for external provenance")
	}
	if dust.Command != "mise use --global aqua:bootandy/dust@latest" {
		t.Errorf("dust: command = %q, want install command", dust.Command)
	}
	if dust.FromVersion != "1.2.4" || dust.ToVersion != "1.2.6" {
		t.Errorf("dust: versions = %q -> %q, want 1.2.4 -> 1.2.6", dust.FromVersion, dust.ToVersion)
	}

	tig := byID["tig"]
	if !tig.Migrate {
		t.Errorf("tig: Migrate = false, want true for external provenance")
	}
	if tig.Command != "sudo apt-get install -y tig" {
		t.Errorf("tig: command = %q, want install command", tig.Command)
	}

	codex := byID["codex"]
	if codex.Migrate {
		t.Errorf("codex: Migrate = true, want false for mise provenance")
	}
	if codex.Command != "mise upgrade npm:@openai/codex" {
		t.Errorf("codex: command = %q, want update command", codex.Command)
	}
}
func TestBuildActionPlanBootstrapRules(t *testing.T) {
	miseTool := domain.Tool{ID: "codex", Binary: "codex", Backend: domain.BackendMiseNPM, Package: "@openai/codex"}
	uvTool := domain.Tool{ID: "browser-use", Binary: "browser-use", Backend: domain.BackendUV, Package: "browser-use"}
	aptTool := domain.Tool{ID: "tmux", Binary: "tmux", Backend: domain.BackendAPT, Package: "tmux"}

	t.Run("installed mise skips bootstrap", func(t *testing.T) {
		plan := domain.BuildActionPlan(
			[]domain.Tool{miseTool},
			map[string]domain.ToolState{"codex": {LatestVersion: "1.0.0"}},
			map[string]domain.DesiredAction{"codex": domain.ActionInstall},
			true,
			"2026.10.1",
		)
		if plan.BootstrapStep != nil {
			t.Fatal("bootstrap should be skipped when mise is installed")
		}
	})

	t.Run("apt only does not bootstrap", func(t *testing.T) {
		plan := domain.BuildActionPlan(
			[]domain.Tool{aptTool},
			map[string]domain.ToolState{"tmux": {LatestVersion: "3.4"}},
			map[string]domain.DesiredAction{"tmux": domain.ActionInstall},
			false,
			"2026.10.1",
		)
		if plan.BootstrapStep != nil {
			t.Fatal("apt-only plan should not bootstrap mise")
		}
	})

	t.Run("uv requires mise", func(t *testing.T) {
		plan := domain.BuildActionPlan(
			[]domain.Tool{uvTool},
			map[string]domain.ToolState{"browser-use": {LatestVersion: "0.13.1"}},
			map[string]domain.DesiredAction{"browser-use": domain.ActionInstall},
			false,
			"2026.10.1",
		)
		if plan.BootstrapStep == nil {
			t.Fatal("uv plan should bootstrap mise")
		}
	})

	t.Run("explicit bootstrap request", func(t *testing.T) {
		plan := domain.BuildActionPlan(
			nil,
			nil,
			map[string]domain.DesiredAction{"mise": domain.ActionInstall},
			false,
			"2026.10.1",
		)
		if plan.BootstrapStep == nil {
			t.Fatal("explicit bootstrap request should add a bootstrap step")
		}
		if plan.TotalSteps() != 1 {
			t.Fatalf("TotalSteps() = %d", plan.TotalSteps())
		}
		if plan.IsEmpty() {
			t.Fatal("bootstrap-only plan should not be empty")
		}
	})

	t.Run("none actions are ignored", func(t *testing.T) {
		plan := domain.BuildActionPlan(
			[]domain.Tool{miseTool},
			map[string]domain.ToolState{"codex": {InstalledVersion: "1.0.0", LatestVersion: "1.0.0"}},
			map[string]domain.DesiredAction{"codex": domain.ActionNone},
			false,
			"2026.10.1",
		)
		if !plan.IsEmpty() {
			t.Fatalf("plan should be empty: %#v", plan)
		}
	})
}

func TestAptStepAlwaysNeedsSudo(t *testing.T) {
	tool := domain.Tool{ID: "tmux", Binary: "tmux", Backend: domain.BackendAPT, Package: "tmux", Sudo: false}
	plan := domain.BuildActionPlan(
		[]domain.Tool{tool},
		map[string]domain.ToolState{"tmux": {InstalledVersion: "3.4", LatestVersion: "3.4"}},
		map[string]domain.DesiredAction{"tmux": domain.ActionRemove},
		true,
		"",
	)
	if len(plan.Removals) != 1 {
		t.Fatalf("removals = %d", len(plan.Removals))
	}
	if !plan.Removals[0].Sudo {
		t.Fatal("apt removal should be marked sudo")
	}
}

func TestPlanStepOrderingIsStableWithinBackend(t *testing.T) {
	tools := []domain.Tool{
		{ID: "a", Binary: "a", Backend: domain.BackendMiseAqua, Package: "owner/a"},
		{ID: "b", Binary: "b", Backend: domain.BackendMiseNPM, Package: "b"},
		{ID: "c", Binary: "c", Backend: domain.BackendMiseAqua, Package: "owner/c"},
	}
	actions := map[string]domain.DesiredAction{
		"a": domain.ActionInstall,
		"b": domain.ActionInstall,
		"c": domain.ActionInstall,
	}
	plan := domain.BuildActionPlan(tools, nil, actions, true, "")
	got := []string{plan.Installations[0].Tool.ID, plan.Installations[1].Tool.ID, plan.Installations[2].Tool.ID}
	want := []string{"a", "b", "c"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("install order = %v, want %v", got, want)
	}
}
