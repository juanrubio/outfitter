package demo_test

import (
	"go/parser"
	"go/token"
	"strconv"
	"testing"

	"github.com/juanrubio/outfitter/internal/demo"
	"github.com/juanrubio/outfitter/internal/domain"
)

func TestLoadSnapshot(t *testing.T) {
	snapshot, err := demo.Load()
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Catalog == nil {
		t.Fatal("catalog is nil")
	}
	if len(snapshot.Catalog.Tools) != 45 {
		t.Fatalf("catalog tools = %d, want 45", len(snapshot.Catalog.Tools))
	}
	if len(snapshot.States) != 45 {
		t.Fatalf("states = %d, want 45", len(snapshot.States))
	}
	if !snapshot.MiseInstalled {
		t.Fatal("demo mise should be installed")
	}
	if snapshot.MiseVersion != "2026.10.1" {
		t.Fatalf("mise version = %q", snapshot.MiseVersion)
	}
	if snapshot.PlatformNotice != "Ubuntu 24.04 · amd64" {
		t.Fatalf("platform notice = %q", snapshot.PlatformNotice)
	}
}

func TestSnapshotCounts(t *testing.T) {
	snapshot, err := demo.Load()
	if err != nil {
		t.Fatal(err)
	}

	installed := 0
	outdated := []string{}
	for id, state := range snapshot.States {
		if state.IsInstalled() {
			installed++
		}
		if state.IsOutdated() {
			outdated = append(outdated, id)
		}
	}
	if installed != 39 {
		t.Fatalf("installed = %d, want 39", installed)
	}
	wantOutdated := map[string]bool{
		"codex": true, "muse": true, "trailmark": true, "lazygit": true, "browser-use": true,
	}
	if len(outdated) != len(wantOutdated) {
		t.Fatalf("outdated = %v, want %v", outdated, wantOutdated)
	}
	for _, id := range outdated {
		if !wantOutdated[id] {
			t.Errorf("unexpected outdated tool %q", id)
		}
	}
}

func TestSnapshotActions(t *testing.T) {
	snapshot, err := demo.Load()
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]domain.DesiredAction{
		"gemini-cli": domain.ActionInstall,
		"quotary":    domain.ActionInstall,
	}
	if len(snapshot.Actions) != len(want) {
		t.Fatalf("actions = %#v, want %#v", snapshot.Actions, want)
	}
	for id, action := range want {
		if snapshot.Actions[id] != action {
			t.Errorf("action %q = %q, want %q", id, snapshot.Actions[id], action)
		}
	}
}

func TestQuotarySourceOverride(t *testing.T) {
	snapshot, err := demo.Load()
	if err != nil {
		t.Fatal(err)
	}
	tool, ok := snapshot.Catalog.ToolByID("quotary")
	if !ok {
		t.Fatal("quotary missing")
	}
	if tool.Backend != domain.BackendSource {
		t.Fatalf("quotary backend = %q, want source", tool.Backend)
	}
	if tool.SourceDir != "~/repos/quotary" {
		t.Fatalf("quotary source dir = %q", tool.SourceDir)
	}
	state := snapshot.States["quotary"]
	if state.InstalledVersion != "" {
		t.Fatalf("quotary should not be installed: %q", state.InstalledVersion)
	}
	if state.LatestVersion != "0.1.0-dev" {
		t.Fatalf("quotary latest = %q", state.LatestVersion)
	}
}

func TestSpecificFixtureVersions(t *testing.T) {
	snapshot, err := demo.Load()
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string][2]string{
		"claude-code": {"2.1.289", "2.1.289"},
		"codex":       {"0.163.0", "0.165.0"},
		"muse":        {"1.4.10-R6427.1", "1.4.12-R6501.0"},
		"trailmark":   {"0.3.0", "0.3.1"},
		"lazygit":     {"0.44.0", "0.45.1"},
		"browser-use": {"0.12.9", "0.13.1"},
		"gemini-cli":  {"", "0.43.0"},
	}
	for id, versions := range cases {
		state := snapshot.States[id]
		if state.InstalledVersion != versions[0] || state.LatestVersion != versions[1] {
			t.Errorf("%s = %q → %q, want %q → %q", id,
				state.InstalledVersion, state.LatestVersion, versions[0], versions[1])
		}
	}
}

func TestLoadIsDeterministic(t *testing.T) {
	first, err := demo.Load()
	if err != nil {
		t.Fatal(err)
	}
	second, err := demo.Load()
	if err != nil {
		t.Fatal(err)
	}
	for id, state := range first.States {
		if second.States[id] != state {
			t.Errorf("state %q changed between loads: %#v vs %#v", id, state, second.States[id])
		}
	}
	for id, action := range first.Actions {
		if second.Actions[id] != action {
			t.Errorf("action %q changed between loads", id)
		}
	}
	if len(first.Catalog.Tools) != len(second.Catalog.Tools) {
		t.Fatal("catalog tool count changed between loads")
	}
}

func TestDemoHasNoExternalOperations(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "demo.go", nil, parser.ImportsOnly)
	if err != nil {
		t.Fatal(err)
	}
	forbidden := map[string]bool{
		"os": true, "os/exec": true, "net": true, "net/http": true, "net/url": true,
	}
	for _, imported := range file.Imports {
		path, err := strconv.Unquote(imported.Path.Value)
		if err != nil {
			t.Fatal(err)
		}
		if forbidden[path] {
			t.Errorf("demo package must stay offline but imports %q", path)
		}
	}
}
