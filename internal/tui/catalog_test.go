package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/juanrubio/outfitter/internal/catalog"
	"github.com/juanrubio/outfitter/internal/domain"
)

func modelForCatalog(t *testing.T, cat *catalog.Catalog) *Model {
	t.Helper()
	m, err := NewModel(Options{
		Catalog: cat,
		States:  map[string]domain.ToolState{},
		Width:   160,
		Height:  40,
	})
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func loadCatalogBytes(t *testing.T, content string) *catalog.Catalog {
	t.Helper()
	cat, err := catalog.LoadFromBytes([]byte(content))
	if err != nil {
		t.Fatal(err)
	}
	return cat
}

const replaceNoProfiles = `
[catalog]
mode = "replace"
name = "Workstation"
[[tools]]
id = "ttyd"
binary = "ttyd"
backend = "mise:ubi"
package = "tsl0922/ttyd"
`

func TestReplaceCatalogWithoutProfiles(t *testing.T) {
	m := modelForCatalog(t, loadCatalogBytes(t, replaceNoProfiles))
	if m.activeProfileID != "" {
		t.Fatalf("active profile = %q, want empty", m.activeProfileID)
	}
	screen := m.View().Content
	assertScreen(t, screen, 160, 40)
	if !strings.Contains(screen, "All tools") {
		t.Fatalf("header should label the profile as all tools:\n%s", screen)
	}
	if strings.Contains(screen, "Full Desktop") {
		t.Fatalf("header must not invent a Full Desktop profile:\n%s", screen)
	}
	if !strings.Contains(screen, "[replace · Workstation]") {
		t.Fatalf("wide header should carry the catalog label:\n%s", screen)
	}

	pressKey(m, 'p', "p")
	if m.view != "catalog" {
		t.Fatalf("view after p with no profiles = %q, want catalog", m.view)
	}
	if !strings.Contains(m.notice, "no profiles") {
		t.Fatalf("notice after p = %q", m.notice)
	}
}

func TestFirstProfileSelectedWhenDesktopMissing(t *testing.T) {
	cat := loadCatalogBytes(t, `
[catalog]
mode = "replace"
[[tools]]
id = "ttyd"
binary = "ttyd"
backend = "mise:ubi"
package = "tsl0922/ttyd"
[profiles.worker]
name = "Worker"
tools = ["ttyd"]
`)
	m := modelForCatalog(t, cat)
	if m.activeProfileID != "worker" {
		t.Fatalf("active profile = %q, want worker", m.activeProfileID)
	}
	pressKey(m, 'p', "p")
	if m.view != "profiles" {
		t.Fatalf("view after p = %q, want profiles", m.view)
	}
}

func TestCatalogNoticesAndLabels(t *testing.T) {
	// Built-in catalog.
	defaults, err := catalog.LoadDefault()
	if err != nil {
		t.Fatal(err)
	}
	m := modelForCatalog(t, defaults)
	if got := m.catalogLabel(); got != "" {
		t.Fatalf("built-in label = %q, want empty", got)
	}
	pressKey(m, 'c', "c")
	if !strings.Contains(m.notice, "built-in") || !strings.Contains(m.notice, "49 tools") {
		t.Fatalf("built-in notice = %q", m.notice)
	}

	// Replace catalog loaded from a path.
	dir := t.TempDir()
	path := filepath.Join(dir, "catalog.toml")
	if err := os.WriteFile(path, []byte(replaceNoProfiles), 0o600); err != nil {
		t.Fatal(err)
	}
	replaceCat, err := catalog.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	m = modelForCatalog(t, replaceCat)
	pressKey(m, 'c', "c")
	for _, want := range []string{"replace", "Workstation", path, "1 tools"} {
		if !strings.Contains(m.notice, want) {
			t.Fatalf("replace notice %q missing %q", m.notice, want)
		}
	}
	if got := m.catalogLabel(); got != "replace · Workstation" {
		t.Fatalf("replace label = %q", got)
	}

	// Extend overlay.
	extendCat := loadCatalogBytes(t, "[catalog]\nmode = \"extend\"\nname = \"Tweaks\"\n")
	m = modelForCatalog(t, extendCat)
	pressKey(m, 'c', "c")
	if !strings.Contains(m.notice, "built-in extended by Tweaks") {
		t.Fatalf("extend notice = %q", m.notice)
	}
	if got := m.catalogLabel(); got != "extend · Tweaks" {
		t.Fatalf("extend label = %q", got)
	}

	// Demo snapshot.
	demoModel, err := NewModel(Options{Demo: true, Width: 160, Height: 40})
	if err != nil {
		t.Fatal(err)
	}
	pressKey(demoModel, 'c', "c")
	if !strings.Contains(demoModel.notice, "demo") {
		t.Fatalf("demo notice = %q", demoModel.notice)
	}
	if demoModel.catalog.Meta.Source != catalog.SourceDemo {
		t.Fatalf("demo catalog source = %q", demoModel.catalog.Meta.Source)
	}
}

func TestIgnoreUserCatalogOption(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, ".config", "outfitter")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "catalog.toml"), []byte("[catalog\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	m, err := NewModel(Options{
		HomeDir:           home,
		IgnoreUserCatalog: true,
		States:            map[string]domain.ToolState{},
		Width:             120,
		Height:            40,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(m.catalog.Tools) != 49 || m.catalog.Meta.Source != catalog.SourceEmbedded {
		t.Fatalf("ignored catalog tools=%d source=%q", len(m.catalog.Tools), m.catalog.Meta.Source)
	}
	if _, err := NewModel(Options{
		HomeDir: home,
		States:  map[string]domain.ToolState{},
		Width:   120,
		Height:  40,
	}); err == nil {
		t.Fatal("broken user catalog should error without IgnoreUserCatalog")
	}
}
