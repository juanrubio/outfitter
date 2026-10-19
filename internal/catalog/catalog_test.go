package catalog_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/juanrubio/outfitter/internal/catalog"
	"github.com/juanrubio/outfitter/internal/domain"
)

func TestLoadDefault(t *testing.T) {
	cat, err := catalog.LoadDefault()
	if err != nil {
		t.Fatal(err)
	}

	if cat.Settings != domain.DefaultSettings() {
		t.Fatalf("settings = %#v, want %#v", cat.Settings, domain.DefaultSettings())
	}

	if len(cat.Tools) != 31 {
		t.Fatalf("tools = %d, want 31", len(cat.Tools))
	}
	wantIDs := []string{
		"claude-code", "codex", "muse", "antigravity-cli", "opencode", "pi", "gemini-cli", "copilot-cli",
		"boardwalk", "trailmark", "quotary", "codexbar", "openclaw", "mercadona-cli",
		"tmux", "hive", "gh", "lazygit", "fzf", "delta", "glow", "ripgrep", "fd", "jq", "btop",
		"browser-use", "mcporter", "oracle", "summarize", "wacli", "hcloud",
	}
	for index, want := range wantIDs {
		if cat.Tools[index].ID != want {
			t.Errorf("tool %d = %q, want %q", index, cat.Tools[index].ID, want)
		}
	}
	if len(cat.Categories) != 4 {
		t.Fatalf("categories = %d, want 4", len(cat.Categories))
	}
	if len(cat.Profiles) != 4 {
		t.Fatalf("profiles = %d, want 4", len(cat.Profiles))
	}

	categories := []struct {
		id    string
		num   int
		title string
		count int
	}{
		{"harnesses", 1, "CODING HARNESSES", 8},
		{"ai-tuis", 2, "AI TUIS & PERSONAL TOOLS", 6},
		{"terminal", 3, "TERMINAL & SESSION MANAGEMENT", 11},
		{"utilities", 4, "AGENT & CLI UTILITIES", 6},
	}
	for index, want := range categories {
		got := cat.Categories[index]
		if got.ID != want.id || got.Num != want.num || got.Title != want.title {
			t.Errorf("category %d = %#v, want %#v", index, got, want)
		}
		if tools := cat.ToolsByCategory(want.id); len(tools) != want.count {
			t.Errorf("%s tools = %d, want %d", want.id, len(tools), want.count)
		}
	}

	profiles := map[string]int{
		"desktop":   31,
		"server":    21,
		"harnesses": 8,
		"minimal":   9,
	}
	for id, want := range profiles {
		profile, ok := cat.ProfileByID(id)
		if !ok {
			t.Errorf("profile %q missing", id)
			continue
		}
		if len(profile.Tools) != want {
			t.Errorf("profile %q tools = %d, want %d", id, len(profile.Tools), want)
		}
	}

	if got := len(cat.Profiles[0].Tools); got != 31 {
		t.Fatalf("desktop profile has %d tools, want 31", got)
	}
	if got := len(cat.Profiles[1].Tools); got != 21 {
		t.Fatalf("server profile has %d tools, want 21", got)
	}
	if got := len(cat.Profiles[2].Tools); got != 8 {
		t.Fatalf("harnesses profile has %d tools, want 8", got)
	}
	if got := len(cat.Profiles[3].Tools); got != 9 {
		t.Fatalf("minimal profile has %d tools, want 9", got)
	}

	for _, tool := range cat.Tools {
		if tool.ID == "" || tool.Name == "" || tool.Binary == "" || tool.CategoryID == "" {
			t.Errorf("tool has empty required field: %#v", tool)
		}
		if !tool.Backend.IsValid() {
			t.Errorf("tool %q has invalid backend %q", tool.ID, tool.Backend)
		}
		if tool.Package == "" {
			t.Errorf("tool %q has no package", tool.ID)
		}
		if tool.InstallCmd == "" || tool.UpdateCmd == "" || tool.RemoveCmd == "" {
			t.Errorf("tool %q has an empty command", tool.ID)
		}
		if len(tool.Profiles) == 0 {
			t.Errorf("tool %q has no profiles", tool.ID)
		}
		if tool.VersionRegex != "" {
			if _, err := regexp.Compile(tool.VersionRegex); err != nil {
				t.Errorf("tool %q has invalid version regex %q: %v", tool.ID, tool.VersionRegex, err)
			}
		}
		for _, profileID := range tool.Profiles {
			profile, ok := cat.ProfileByID(profileID)
			if !ok {
				t.Errorf("tool %q references missing profile %q", tool.ID, profileID)
				continue
			}
			if !contains(profile.Tools, tool.ID) {
				t.Errorf("profile %q does not contain tool %q", profileID, tool.ID)
			}
		}
	}

	for _, profile := range cat.Profiles {
		for _, toolID := range profile.Tools {
			tool, ok := cat.ToolByID(toolID)
			if !ok {
				t.Errorf("profile %q references missing tool %q", profile.ID, toolID)
				continue
			}
			if !contains(tool.Profiles, profile.ID) {
				t.Errorf("tool %q does not reference profile %q", toolID, profile.ID)
			}
		}
	}

	assertTool(t, cat, "codex", "OpenAI Codex CLI", "codex", "harnesses", domain.BackendMiseNPM, "@openai/codex")
	assertTool(t, cat, "quotary", "Quotary", "quotary", "ai-tuis", domain.BackendMiseUbi, "juanrubio/quotary")
	assertTool(t, cat, "tmux", "tmux", "tmux", "terminal", domain.BackendAPT, "tmux")
	assertTool(t, cat, "browser-use", "browser-use", "browser-use", "utilities", domain.BackendUV, "browser-use")

	quotary, _ := cat.ToolByID("quotary")
	if quotary.InstallCmd != "mise use --global ubi:juanrubio/quotary[exe=quotary]@latest" {
		t.Fatalf("quotary install command = %q", quotary.InstallCmd)
	}
	if !contains(quotary.Profiles, "desktop") {
		t.Fatalf("quotary profiles = %v", quotary.Profiles)
	}

	tmux, _ := cat.ToolByID("tmux")
	if !tmux.Sudo {
		t.Fatal("tmux should require sudo")
	}
	if tmux.InstallCmd != "sudo apt-get install -y tmux" {
		t.Fatalf("tmux install command = %q", tmux.InstallCmd)
	}

	exported := catalog.ExportDefault()
	for _, id := range []string{"claude-code", "quotary", "tmux", "browser-use", "hcloud"} {
		if !strings.Contains(exported, `id = "`+id+`"`) {
			t.Errorf("exported default catalog missing %q", id)
		}
	}

	again, err := catalog.LoadDefault()
	if err != nil {
		t.Fatal(err)
	}
	if len(again.Tools) != len(cat.Tools) {
		t.Fatal("default catalog is not deterministic")
	}
	for index := range cat.Tools {
		if again.Tools[index].ID != cat.Tools[index].ID {
			t.Fatalf("tool order changed at %d: %q != %q", index, again.Tools[index].ID, cat.Tools[index].ID)
		}
	}
}

func TestLoadFromBytesProductExample(t *testing.T) {
	user := `
[settings]
install_dir = "~/.local/bin"
auto_bootstrap_mise = true
github_token_from_gh = true

[[tools]]
id = "quotary"
backend = "source"
source_dir = "~/repos/quotary"
build_cmd = "go build -o ~/.local/bin/quotary ./cmd/quotary"

[[tools]]
id = "ttyd"
name = "ttyd"
category = "terminal"
binary = "ttyd"
backend = "mise:ubi"
package = "tsl0922/ttyd"
version_args = ["--version"]
version_regex = 'ttyd version ([0-9.]+)'
profiles = ["desktop", "server"]
description = "Share terminal sessions over the web"

[profiles.vps-worker]
name = "VPS Worker"
description = "Headless agent worker for Hetzner VPS"
tools = ["tmux", "gh", "claude-code", "codex", "openclaw", "boardwalk", "ripgrep", "jq"]
`
	cat, err := catalog.LoadFromBytes([]byte(user))
	if err != nil {
		t.Fatal(err)
	}

	if len(cat.Tools) != 32 {
		t.Fatalf("tools = %d, want 32", len(cat.Tools))
	}
	if len(cat.Categories) != 4 {
		t.Fatalf("categories = %d, want 4", len(cat.Categories))
	}
	if len(cat.Profiles) != 5 {
		t.Fatalf("profiles = %d, want 5", len(cat.Profiles))
	}

	quotary, ok := cat.ToolByID("quotary")
	if !ok {
		t.Fatal("quotary missing")
	}
	if quotary.Backend != domain.BackendSource {
		t.Fatalf("quotary backend = %q", quotary.Backend)
	}
	if quotary.SourceDir != "~/repos/quotary" {
		t.Fatalf("quotary source dir = %q", quotary.SourceDir)
	}
	if quotary.BuildCmd != "go build -o ~/.local/bin/quotary ./cmd/quotary" {
		t.Fatalf("quotary build command = %q", quotary.BuildCmd)
	}
	if quotary.InstallCmd != quotary.BuildCmd {
		t.Fatalf("quotary install command = %q", quotary.InstallCmd)
	}
	if quotary.UpdateCmd != quotary.BuildCmd {
		t.Fatalf("quotary update command = %q", quotary.UpdateCmd)
	}
	if quotary.RemoveCmd != "rm -f ~/.local/bin/quotary" {
		t.Fatalf("quotary remove command = %q", quotary.RemoveCmd)
	}

	ttyd, ok := cat.ToolByID("ttyd")
	if !ok {
		t.Fatal("custom ttyd tool missing")
	}
	if ttyd.Name != "ttyd" || ttyd.Binary != "ttyd" || ttyd.CategoryID != "terminal" {
		t.Fatalf("ttyd fields = %#v", ttyd)
	}
	if ttyd.Backend != domain.BackendMiseUbi || ttyd.Package != "tsl0922/ttyd" {
		t.Fatalf("ttyd backend/package = %q/%q", ttyd.Backend, ttyd.Package)
	}
	if ttyd.InstallCmd != "mise use --global ubi:tsl0922/ttyd@latest" {
		t.Fatalf("ttyd install command = %q", ttyd.InstallCmd)
	}
	if ttyd.UpdateCmd != "mise upgrade ubi:tsl0922/ttyd" {
		t.Fatalf("ttyd update command = %q", ttyd.UpdateCmd)
	}
	if ttyd.RemoveCmd != "mise unuse --global ubi:tsl0922/ttyd" {
		t.Fatalf("ttyd remove command = %q", ttyd.RemoveCmd)
	}
	if len(ttyd.VersionArgs) != 1 || ttyd.VersionArgs[0] != "--version" {
		t.Fatalf("ttyd version args = %v", ttyd.VersionArgs)
	}
	if ttyd.VersionRegex != "ttyd version ([0-9.]+)" {
		t.Fatalf("ttyd version regex = %q", ttyd.VersionRegex)
	}
	if strings.Join(ttyd.Profiles, ",") != "desktop,server" {
		t.Fatalf("ttyd profiles = %v", ttyd.Profiles)
	}

	profile, ok := cat.ProfileByID("vps-worker")
	if !ok {
		t.Fatal("vps-worker profile missing")
	}
	if profile.Name != "VPS Worker" || profile.Short != "VPS Worker" {
		t.Fatalf("vps-worker name/short = %q/%q", profile.Name, profile.Short)
	}
	if profile.Description != "Headless agent worker for Hetzner VPS" {
		t.Fatalf("vps-worker description = %q", profile.Description)
	}
	if len(profile.Tools) != 8 {
		t.Fatalf("vps-worker tools = %v", profile.Tools)
	}
	for _, toolID := range profile.Tools {
		tool, ok := cat.ToolByID(toolID)
		if !ok {
			t.Fatalf("vps-worker references missing tool %q", toolID)
		}
		if !contains(tool.Profiles, "vps-worker") {
			t.Fatalf("tool %q does not include vps-worker: %v", toolID, tool.Profiles)
		}
	}

	desktop, _ := cat.ProfileByID("desktop")
	if !contains(desktop.Tools, "ttyd") {
		t.Fatal("ttyd should be added to the desktop profile")
	}
	server, _ := cat.ProfileByID("server")
	if !contains(server.Tools, "ttyd") {
		t.Fatal("ttyd should be added to the server profile")
	}
}

func TestDisabledBuiltInTool(t *testing.T) {
	cat, err := catalog.LoadFromBytes([]byte(`
[[tools]]
id = "btop"
disabled = true
`))
	if err != nil {
		t.Fatal(err)
	}

	if _, ok := cat.ToolByID("btop"); ok {
		t.Fatal("disabled btop should not be active")
	}
	if len(cat.Tools) != 30 {
		t.Fatalf("tools = %d, want 30", len(cat.Tools))
	}

	for _, profileID := range []string{"desktop", "server"} {
		profile, _ := cat.ProfileByID(profileID)
		if contains(profile.Tools, "btop") {
			t.Fatalf("profile %q still contains disabled btop", profileID)
		}
	}
}

func TestCustomToolDefaultsAndCategory(t *testing.T) {
	cat, err := catalog.LoadFromBytes([]byte(`
[[tools]]
id = "mytool"
binary = "mytool"
backend = "apt"
package = "mytool"
category = "security"
`))
	if err != nil {
		t.Fatal(err)
	}

	tool, ok := cat.ToolByID("mytool")
	if !ok {
		t.Fatal("custom tool missing")
	}
	if tool.Name != "mytool" {
		t.Fatalf("custom tool name = %q", tool.Name)
	}
	if tool.CategoryID != "security" {
		t.Fatalf("custom tool category = %q", tool.CategoryID)
	}
	if !tool.Sudo {
		t.Fatal("apt custom tool should default to sudo")
	}
	if tool.InstallCmd != "sudo apt-get install -y mytool" {
		t.Fatalf("custom install command = %q", tool.InstallCmd)
	}
	if tool.UpdateCmd != "sudo apt-get install --only-upgrade -y mytool" {
		t.Fatalf("custom update command = %q", tool.UpdateCmd)
	}
	if tool.RemoveCmd != "sudo apt-get remove -y mytool" {
		t.Fatalf("custom remove command = %q", tool.RemoveCmd)
	}

	if len(cat.Categories) != 5 {
		t.Fatalf("categories = %d, want 5", len(cat.Categories))
	}
	category := cat.Categories[4]
	if category.ID != "security" || category.Num != 5 || category.Title != "SECURITY" {
		t.Fatalf("custom category = %#v", category)
	}

	plain, err := catalog.LoadFromBytes([]byte(`
[[tools]]
id = "plain"
binary = "plain"
backend = "mise:ubi"
package = "owner/plain"
`))
	if err != nil {
		t.Fatal(err)
	}
	plainTool, _ := plain.ToolByID("plain")
	if plainTool.CategoryID != "utilities" {
		t.Fatalf("plain tool category = %q", plainTool.CategoryID)
	}
}

func TestSettingsOnlyOverrideSpecifiedFields(t *testing.T) {
	cat, err := catalog.LoadFromBytes([]byte(`
[settings]
auto_bootstrap_mise = false
`))
	if err != nil {
		t.Fatal(err)
	}
	if cat.Settings.InstallDir != domain.DefaultInstallDir {
		t.Fatalf("install dir = %q", cat.Settings.InstallDir)
	}
	if cat.Settings.AutoBootstrapMise {
		t.Fatal("auto_bootstrap_mise should be false")
	}
	if !cat.Settings.GitHubTokenFromGH {
		t.Fatal("github_token_from_gh should retain its default")
	}

	cat, err = catalog.LoadFromBytes([]byte(`
[settings]
install_dir = "~/bin"
github_token_from_gh = false
`))
	if err != nil {
		t.Fatal(err)
	}
	if cat.Settings.InstallDir != "~/bin" {
		t.Fatalf("install dir = %q", cat.Settings.InstallDir)
	}
	if !cat.Settings.AutoBootstrapMise {
		t.Fatal("auto_bootstrap_mise should retain its default")
	}
	if cat.Settings.GitHubTokenFromGH {
		t.Fatal("github_token_from_gh should be false")
	}
}

func TestProfilesArrayAndMembershipSync(t *testing.T) {
	cat, err := catalog.LoadFromBytes([]byte(`
[[profiles]]
id = "vps"
name = "VPS Worker"
short = "VPS"
description = "Remote worker"
tools = ["tmux", "gh"]
`))
	if err != nil {
		t.Fatal(err)
	}

	profile, ok := cat.ProfileByID("vps")
	if !ok {
		t.Fatal("vps profile missing")
	}
	if len(profile.Tools) != 2 {
		t.Fatalf("vps tools = %v", profile.Tools)
	}
	for _, toolID := range []string{"tmux", "gh"} {
		tool, _ := cat.ToolByID(toolID)
		if !contains(tool.Profiles, "vps") {
			t.Fatalf("tool %q missing vps membership: %v", toolID, tool.Profiles)
		}
	}
}

func TestExistingProfileOverrideReplacesTools(t *testing.T) {
	cat, err := catalog.LoadFromBytes([]byte(`
[profiles.desktop]
name = "Desktop Only"
tools = ["tmux", "gh"]
`))
	if err != nil {
		t.Fatal(err)
	}

	desktop, _ := cat.ProfileByID("desktop")
	if strings.Join(desktop.Tools, ",") != "tmux,gh" {
		t.Fatalf("desktop tools = %v", desktop.Tools)
	}
	if desktop.Name != "Desktop Only" || desktop.Short != "Desktop Only" {
		t.Fatalf("desktop name/short = %q/%q", desktop.Name, desktop.Short)
	}

	codex, _ := cat.ToolByID("codex")
	if contains(codex.Profiles, "desktop") {
		t.Fatalf("codex should no longer be in desktop: %v", codex.Profiles)
	}
	tmux, _ := cat.ToolByID("tmux")
	if !contains(tmux.Profiles, "desktop") {
		t.Fatalf("tmux should be in desktop: %v", tmux.Profiles)
	}
}

func TestToolProfilesOverrideUpdatesProfileMembership(t *testing.T) {
	cat, err := catalog.LoadFromBytes([]byte(`
[[tools]]
id = "glow"
profiles = ["server"]
`))
	if err != nil {
		t.Fatal(err)
	}

	glow, _ := cat.ToolByID("glow")
	if strings.Join(glow.Profiles, ",") != "server" {
		t.Fatalf("glow profiles = %v", glow.Profiles)
	}
	desktop, _ := cat.ProfileByID("desktop")
	if contains(desktop.Tools, "glow") {
		t.Fatal("glow should be removed from desktop")
	}
	server, _ := cat.ProfileByID("server")
	if !contains(server.Tools, "glow") {
		t.Fatal("glow should be added to server")
	}
}

func TestValidationErrors(t *testing.T) {
	tests := []struct {
		name string
		toml string
		want string
	}{
		{
			name: "unknown backend",
			toml: "[[tools]]\nid = \"x\"\nbinary = \"x\"\nbackend = \"bogus\"\n",
			want: "invalid backend",
		},
		{
			name: "missing binary",
			toml: "[[tools]]\nid = \"x\"\nbackend = \"mise:ubi\"\n",
			want: "binary",
		},
		{
			name: "missing id",
			toml: "[[tools]]\nbinary = \"x\"\nbackend = \"mise:ubi\"\n",
			want: "id",
		},
		{
			name: "duplicate tool ids",
			toml: "[[tools]]\nid = \"x\"\nbinary = \"x\"\nbackend = \"mise:ubi\"\n\n[[tools]]\nid = \"x\"\nbinary = \"y\"\nbackend = \"mise:ubi\"\n",
			want: "duplicate tool id",
		},
		{
			name: "invalid built-in backend override",
			toml: "[[tools]]\nid = \"codex\"\nbackend = \"bogus\"\n",
			want: "invalid backend",
		},
		{
			name: "empty binary override",
			toml: "[[tools]]\nid = \"codex\"\nbinary = \"\"\n",
			want: "binary",
		},
		{
			name: "malformed toml",
			toml: "[settings\n",
			want: "parse user catalog",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := catalog.LoadFromBytes([]byte(test.toml))
			if err == nil {
				t.Fatal("expected an error")
			}
			if !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %q, want substring %q", err, test.want)
			}
		})
	}
}

func TestLoadFromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "catalog.toml")
	content := "[[tools]]\nid = \"local\"\nbinary = \"local\"\nbackend = \"source\"\nsource_dir = \"~/repos/local\"\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	cat, err := catalog.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	tool, ok := cat.ToolByID("local")
	if !ok {
		t.Fatal("local tool missing")
	}
	if tool.InstallCmd != "go -C ~/repos/local build -o ~/.local/bin/local ./cmd/local" {
		t.Fatalf("local install command = %q", tool.InstallCmd)
	}

	if _, err := catalog.Load(filepath.Join(dir, "missing.toml")); err == nil {
		t.Fatal("expected an error for a missing catalog file")
	}
}

func TestLoadFromEmptyBytes(t *testing.T) {
	cat, err := catalog.LoadFromBytes(nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(cat.Tools) != 31 {
		t.Fatalf("tools = %d, want 31", len(cat.Tools))
	}
}

func TestLoadUsesDefaultUserCatalogWhenPresent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cat, err := catalog.Load("")
	if err != nil {
		t.Fatal(err)
	}
	if len(cat.Tools) != 31 {
		t.Fatalf("tools without user catalog = %d, want 31", len(cat.Tools))
	}

	dir := filepath.Join(home, ".config", "outfitter")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	content := "[[tools]]\nid = \"home-tool\"\nbinary = \"home-tool\"\nbackend = \"mise:ubi\"\npackage = \"owner/home-tool\"\n"
	if err := os.WriteFile(filepath.Join(dir, "catalog.toml"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	cat, err = catalog.Load("")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := cat.ToolByID("home-tool"); !ok {
		t.Fatal("default user catalog was not loaded")
	}
}

func assertTool(
	t *testing.T,
	cat *catalog.Catalog,
	id, name, binary, category string,
	backend domain.BackendType,
	pkg string,
) {
	t.Helper()
	tool, ok := cat.ToolByID(id)
	if !ok {
		t.Fatalf("tool %q missing", id)
	}
	if tool.Name != name || tool.Binary != binary || tool.CategoryID != category {
		t.Errorf("tool %q = name %q binary %q category %q", id, tool.Name, tool.Binary, tool.CategoryID)
	}
	if tool.Backend != backend || tool.Package != pkg {
		t.Errorf("tool %q backend/package = %q/%q, want %q/%q", id, tool.Backend, tool.Package, backend, pkg)
	}
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
