package catalog_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/juanrubio/outfitter/internal/catalog"
	"github.com/juanrubio/outfitter/internal/domain"
)

const minimalReplace = `
[catalog]
schema_version = 1
mode = "replace"
name = "Workstation"

[[tools]]
id = "ttyd"
binary = "ttyd"
backend = "mise:ubi"
package = "tsl0922/ttyd"
`

func TestReplaceMinimalToolNoProfiles(t *testing.T) {
	cat, err := catalog.LoadFromBytes([]byte(minimalReplace))
	if err != nil {
		t.Fatal(err)
	}
	if len(cat.Tools) != 1 || len(cat.Profiles) != 0 {
		t.Fatalf("tools=%d profiles=%d", len(cat.Tools), len(cat.Profiles))
	}
	tool, ok := cat.ToolByID("ttyd")
	if !ok {
		t.Fatal("ttyd missing")
	}
	if tool.Name != "ttyd" || tool.CategoryID != "utilities" {
		t.Fatalf("defaults: %#v", tool)
	}
	if len(tool.VersionArgs) != 1 || tool.VersionArgs[0] != "--version" {
		t.Fatalf("version args = %v", tool.VersionArgs)
	}
	if tool.InstallCmd != "mise use --global ubi:tsl0922/ttyd@latest" {
		t.Fatalf("install cmd = %q", tool.InstallCmd)
	}
	if cat.Settings != domain.DefaultSettings() {
		t.Fatalf("settings = %#v", cat.Settings)
	}
	if len(cat.Categories) != 1 || cat.Categories[0].ID != "utilities" {
		t.Fatalf("categories = %#v", cat.Categories)
	}
	meta := cat.Meta
	if meta.Mode != catalog.ModeReplace || meta.SchemaVersion != 1 || meta.Name != "Workstation" ||
		meta.Source != catalog.SourceUser || meta.Path != "" || meta.UsesBuiltin {
		t.Fatalf("meta = %#v", meta)
	}
	if _, ok := cat.ToolByID("codex"); ok {
		t.Fatal("replace catalog must not inherit built-ins")
	}
}

func TestReplaceSettingsFieldDefaults(t *testing.T) {
	cat, err := catalog.LoadFromBytes([]byte(`
[catalog]
mode = "replace"
[settings]
auto_bootstrap_mise = false
[[tools]]
id = "x"
binary = "x"
backend = "apt"
package = "x"
`))
	if err != nil {
		t.Fatal(err)
	}
	if cat.Settings.InstallDir != domain.DefaultInstallDir || cat.Settings.AutoBootstrapMise ||
		!cat.Settings.GitHubTokenFromGH {
		t.Fatalf("settings = %#v", cat.Settings)
	}
	tool, _ := cat.ToolByID("x")
	if !tool.Sudo {
		t.Fatal("apt tool should default to sudo")
	}
}

func TestReplaceBothProfileSyntaxes(t *testing.T) {
	tools := `
[[tools]]
id = "ttyd"
binary = "ttyd"
backend = "mise:ubi"
package = "tsl0922/ttyd"
profiles = ["worker"]
[[tools]]
id = "jq"
binary = "jq"
backend = "mise:aqua"
package = "jqlang/jq"
`
	for name, profiles := range map[string]string{
		"table": "[profiles.worker]\nname = \"Worker\"\ntools = [\"ttyd\", \"jq\"]\n",
		"array": "[[profiles]]\nid = \"worker\"\nname = \"Worker\"\ntools = [\"ttyd\", \"jq\"]\n",
	} {
		t.Run(name, func(t *testing.T) {
			cat, err := catalog.LoadFromBytes([]byte("[catalog]\nmode = \"replace\"\n" + tools + profiles))
			if err != nil {
				t.Fatal(err)
			}
			profile, ok := cat.ProfileByID("worker")
			if !ok || len(profile.Tools) != 2 {
				t.Fatalf("profile = %#v ok=%v", profile, ok)
			}
			jq, _ := cat.ToolByID("jq")
			if !contains(jq.Profiles, "worker") {
				t.Fatalf("jq profiles = %v", jq.Profiles)
			}
		})
	}
}

func TestReplaceNoSameIDInheritance(t *testing.T) {
	cat, err := catalog.LoadFromBytes([]byte(`
[catalog]
mode = "replace"
[[tools]]
id = "codex"
binary = "codex"
backend = "mise:npm"
package = "@openai/codex"
`))
	if err != nil {
		t.Fatal(err)
	}
	tool, _ := cat.ToolByID("codex")
	// Built-in codex has name "OpenAI Codex CLI", category "harnesses", a
	// description, and a version regex; none may be inherited.
	if tool.Name != "codex" || tool.CategoryID != "utilities" || tool.Description != "" ||
		tool.VersionRegex != "" || len(tool.Profiles) != 0 {
		t.Fatalf("inherited built-in fields: %#v", tool)
	}
}

func TestHeaderDefaultsAndExtendMeta(t *testing.T) {
	// Headerless legacy overlay.
	cat, err := catalog.LoadFromBytes([]byte("[[tools]]\nid = \"btop\"\ndisabled = true\n"))
	if err != nil {
		t.Fatal(err)
	}
	if cat.Meta.Mode != catalog.ModeExtend || cat.Meta.SchemaVersion != 1 ||
		!cat.Meta.UsesBuiltin || cat.Meta.Source != catalog.SourceUser {
		t.Fatalf("legacy meta = %#v", cat.Meta)
	}

	// Explicit extend header with messy mode casing/spacing.
	cat, err = catalog.LoadFromBytes([]byte("[catalog]\nmode = \" Extend \"\nname = \" Tweaks \"\n"))
	if err != nil {
		t.Fatal(err)
	}
	if cat.Meta.Mode != catalog.ModeExtend || cat.Meta.Name != "Tweaks" || len(cat.Tools) != 49 {
		t.Fatalf("extend meta = %#v tools=%d", cat.Meta, len(cat.Tools))
	}
	if cat.Meta.EffectiveName() != "Tweaks" {
		t.Fatalf("effective name = %q", cat.Meta.EffectiveName())
	}
}

func TestLoadDefaultMeta(t *testing.T) {
	cat, err := catalog.LoadDefault()
	if err != nil {
		t.Fatal(err)
	}
	if cat.Meta.Source != catalog.SourceEmbedded || !cat.Meta.UsesBuiltin || cat.Meta.Path != "" ||
		cat.Meta.SchemaVersion != 1 {
		t.Fatalf("meta = %#v", cat.Meta)
	}
	if cat.Meta.EffectiveName() != "Built-in catalog" {
		t.Fatalf("effective name = %q", cat.Meta.EffectiveName())
	}
}

func TestHeaderErrors(t *testing.T) {
	tests := []struct{ name, toml, want string }{
		{"invalid mode", "[catalog]\nmode = \"overlay\"\n", "invalid catalog mode"},
		{"schema too new", "[catalog]\nschema_version = 2\n", "unsupported catalog schema_version 2"},
		{"schema zero", "[catalog]\nschema_version = 0\n", "must be positive"},
		{"schema negative", "[catalog]\nschema_version = -1\n", "must be positive"},
		{"schema string", "[catalog]\nschema_version = \"1\"\n", "must be an integer"},
		{"unknown catalog key", "[catalog]\nmode = \"extend\"\nextra = true\n", "unknown key \"extra\" in [catalog]"},
		{"unknown top key", "[catalog]\nmode = \"extend\"\n[wat]\nx = 1\n", "unknown key \"wat\""},
		{"unknown settings key", "[catalog]\nmode = \"extend\"\n[settings]\nnope = 1\n", "unknown key \"nope\" in [settings]"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := catalog.LoadFromBytes([]byte(test.toml))
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("err = %v, want substring %q", err, test.want)
			}
		})
	}
}

func TestReplaceValidationErrors(t *testing.T) {
	base := "[catalog]\nmode = \"replace\"\n"
	good := "[[tools]]\nid = \"ttyd\"\nbinary = \"ttyd\"\nbackend = \"mise:ubi\"\npackage = \"tsl0922/ttyd\"\n"
	tests := []struct{ name, toml, want string }{
		{"zero tools", base, "no active tools"},
		{"only disabled tool", base + "[[tools]]\nid = \"x\"\nbinary = \"x\"\nbackend = \"apt\"\npackage = \"x\"\ndisabled = true\n", "no active tools"},
		{"unknown tool key", base + "[[tools]]\nid = \"x\"\nbinary = \"x\"\nbackend = \"apt\"\npackage = \"x\"\nbogus = 1\n", "unknown key \"bogus\" in [[tools]]"},
		{"unknown category key", base + "[[categories]]\nid = \"c\"\nbogus = 1\n" + good, "unknown key \"bogus\" in [[categories]]"},
		{"unknown profile key", base + good + "[profiles.w]\ntools = [\"ttyd\"]\nbogus = 1\n", "unknown key \"bogus\" in [profiles.w]"},
		{"duplicate tools", base + good + good, "duplicate tool id"},
		{"duplicate categories", base + "[[categories]]\nid = \"c\"\n[[categories]]\nid = \"c\"\n" + good, "duplicate category id"},
		{"duplicate profiles", base + good + "[[profiles]]\nid = \"w\"\ntools = [\"ttyd\"]\n[[profiles]]\nid = \"w\"\ntools = [\"ttyd\"]\n", "duplicate profile id"},
		{"invalid tool id", base + "[[tools]]\nid = \"My Tool\"\nbinary = \"x\"\nbackend = \"apt\"\npackage = \"x\"\n", "invalid tool id"},
		{"invalid backend", base + "[[tools]]\nid = \"x\"\nbinary = \"x\"\nbackend = \"bogus\"\n", "invalid backend"},
		{"missing package mise", base + "[[tools]]\nid = \"x\"\nbinary = \"x\"\nbackend = \"mise:ubi\"\n", "package is required"},
		{"missing package apt", base + "[[tools]]\nid = \"x\"\nbinary = \"x\"\nbackend = \"apt\"\n", "package is required"},
		{"missing package uv", base + "[[tools]]\nid = \"x\"\nbinary = \"x\"\nbackend = \"uv\"\n", "package is required"},
		{"source without dir or build", base + "[[tools]]\nid = \"x\"\nbinary = \"x\"\nbackend = \"source\"\n", "source_dir or build_cmd"},
		{"vendor without commands", base + "[[tools]]\nid = \"x\"\nbinary = \"x\"\nbackend = \"vendor\"\npackage = \"x\"\n", "missing effective install command"},
		{"vendor partial commands", base + "[[tools]]\nid = \"x\"\nbinary = \"x\"\nbackend = \"vendor\"\ninstall_cmd = \"i\"\nupdate_cmd = \"u\"\n", "missing effective remove command"},
		{"bad version regex", base + "[[tools]]\nid = \"x\"\nbinary = \"x\"\nbackend = \"apt\"\npackage = \"x\"\nversion_regex = '(['\n", "invalid version_regex"},
		{"profile unknown tool", base + good + "[profiles.w]\ntools = [\"ghost\"]\n", "references unknown tool"},
		{"tool unknown profile", base + "[[tools]]\nid = \"x\"\nbinary = \"x\"\nbackend = \"apt\"\npackage = \"x\"\nprofiles = [\"ghost\"]\n", "references unknown profile"},
		{"profile disabled tool", base + good + "[[tools]]\nid = \"off\"\nbinary = \"off\"\nbackend = \"apt\"\npackage = \"off\"\ndisabled = true\n[profiles.w]\ntools = [\"off\"]\n", "references disabled tool"},
		{"empty install_dir", base + "[settings]\ninstall_dir = \"\"\n" + good, "install_dir must not be empty"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := catalog.LoadFromBytes([]byte(test.toml))
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("err = %v, want substring %q", err, test.want)
			}
		})
	}
}

func TestReplaceVendorAndSourceTools(t *testing.T) {
	cat, err := catalog.LoadFromBytes([]byte(`
[catalog]
mode = "replace"
[[tools]]
id = "v"
binary = "v"
backend = "vendor"
install_cmd = "install-v"
update_cmd = "update-v"
remove_cmd = "remove-v"
[[tools]]
id = "s"
binary = "s"
backend = "source"
source_dir = "~/repos/s"
`))
	if err != nil {
		t.Fatal(err)
	}
	source, _ := cat.ToolByID("s")
	if source.InstallCmd != "go -C ~/repos/s build -o ~/.local/bin/s ./cmd/s" {
		t.Fatalf("source install cmd = %q", source.InstallCmd)
	}
}

func TestLoadWithOptionsHomeAndIgnore(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, ".config", "outfitter")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "catalog.toml")
	if err := os.WriteFile(path, []byte(minimalReplace), 0o600); err != nil {
		t.Fatal(err)
	}

	cat, err := catalog.LoadWithOptions(catalog.LoadOptions{HomeDir: home})
	if err != nil {
		t.Fatal(err)
	}
	if len(cat.Tools) != 1 || cat.Meta.Path != path || cat.Meta.Mode != catalog.ModeReplace {
		t.Fatalf("cat meta=%#v tools=%d", cat.Meta, len(cat.Tools))
	}

	cat, err = catalog.LoadWithOptions(catalog.LoadOptions{HomeDir: home, IgnoreUserCatalog: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(cat.Tools) != 49 || cat.Meta.Source != catalog.SourceEmbedded {
		t.Fatalf("ignored cat meta=%#v tools=%d", cat.Meta, len(cat.Tools))
	}

	// A broken default catalog is skipped entirely when ignored.
	if err := os.WriteFile(path, []byte("[catalog\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := catalog.LoadWithOptions(catalog.LoadOptions{HomeDir: home}); err == nil {
		t.Fatal("broken default catalog should error")
	}
	cat, err = catalog.LoadWithOptions(catalog.LoadOptions{HomeDir: home, IgnoreUserCatalog: true})
	if err != nil || len(cat.Tools) != 49 {
		t.Fatalf("ignore broken: err=%v tools=%d", err, len(cat.Tools))
	}

	// Explicit path records the resolved path and errors when missing.
	explicit := filepath.Join(t.TempDir(), "mine.toml")
	if err := os.WriteFile(explicit, []byte(minimalReplace), 0o600); err != nil {
		t.Fatal(err)
	}
	cat, err = catalog.Load(explicit)
	if err != nil {
		t.Fatal(err)
	}
	if cat.Meta.Path != explicit {
		t.Fatalf("meta path = %q, want %q", cat.Meta.Path, explicit)
	}
	if _, err := catalog.Load(filepath.Join(t.TempDir(), "missing.toml")); err == nil {
		t.Fatal("missing explicit catalog should error")
	}
}

func TestTemplates(t *testing.T) {
	seeded, err := catalog.LoadFromBytes([]byte(catalog.SeededCatalogTemplate()))
	if err != nil {
		t.Fatal(err)
	}
	if len(seeded.Tools) != 49 || len(seeded.Categories) != 4 || len(seeded.Profiles) != 4 {
		t.Fatalf("seeded counts tools=%d cats=%d profiles=%d",
			len(seeded.Tools), len(seeded.Categories), len(seeded.Profiles))
	}
	if seeded.Meta.Mode != catalog.ModeReplace || seeded.Meta.UsesBuiltin {
		t.Fatalf("seeded meta = %#v", seeded.Meta)
	}
	defaults, err := catalog.LoadDefault()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range defaults.Tools {
		got, ok := seeded.ToolByID(want.ID)
		if !ok {
			t.Fatalf("seeded missing %q", want.ID)
		}
		if got.Name != want.Name || got.Binary != want.Binary || got.CategoryID != want.CategoryID ||
			got.Backend != want.Backend || got.Package != want.Package || got.Sudo != want.Sudo ||
			got.InstallCmd != want.InstallCmd || got.UpdateCmd != want.UpdateCmd ||
			got.RemoveCmd != want.RemoveCmd || got.VersionRegex != want.VersionRegex ||
			strings.Join(got.Profiles, ",") != strings.Join(want.Profiles, ",") ||
			strings.Join(got.VersionArgs, ",") != strings.Join(want.VersionArgs, ",") {
			t.Fatalf("seeded tool %q differs:\ngot  %#v\nwant %#v", want.ID, got, want)
		}
	}

	extend, err := catalog.LoadFromBytes([]byte(catalog.ExtendCatalogTemplate()))
	if err != nil {
		t.Fatal(err)
	}
	if len(extend.Tools) != 49 || extend.Meta.Mode != catalog.ModeExtend {
		t.Fatalf("extend template tools=%d meta=%#v", len(extend.Tools), extend.Meta)
	}

	if _, err := catalog.LoadFromBytes([]byte(catalog.BlankCatalogTemplate())); err == nil ||
		!strings.Contains(err.Error(), "no active tools") {
		t.Fatalf("blank template err = %v, want no active tools", err)
	}
}

func TestEmbeddedExportRegression(t *testing.T) {
	parsed, err := catalog.ParseCatalog([]byte(catalog.ExportDefault()))
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.Tools) != 49 || len(parsed.Categories) != 4 || len(parsed.Profiles) != 4 {
		t.Fatalf("parsed counts tools=%d cats=%d profiles=%d",
			len(parsed.Tools), len(parsed.Categories), len(parsed.Profiles))
	}
	codexbar, ok := parsed.ToolByID("codexbar")
	if !ok || codexbar.InstallCmd == "" || codexbar.UpdateCmd == "" || codexbar.RemoveCmd == "" {
		t.Fatalf("codexbar = %#v", codexbar)
	}
}
