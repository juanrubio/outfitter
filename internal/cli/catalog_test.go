package cli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/juanrubio/outfitter/internal/catalog"
	"github.com/juanrubio/outfitter/internal/tui"
)

const cliMinimalReplace = `
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

func writeUserCatalog(t *testing.T, deps Deps, content string) string {
	t.Helper()
	path := filepath.Join(deps.HomeDir, ".config", "outfitter", "catalog.toml")
	writeFile(t, path, content)
	return path
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func runShowJSON(t *testing.T, deps Deps, args ...string) catalogReport {
	t.Helper()
	out, err := runCLI(t, deps, args...)
	if err != nil {
		t.Fatalf("show %v: %v\n%s", args, err, out)
	}
	var report catalogReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("show output is not JSON: %v\n%s", err, out)
	}
	return report
}

func TestCatalogInitSeeded(t *testing.T) {
	deps, _ := testDeps(t)
	out, err := runCLI(t, deps, "catalog", "init")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(deps.HomeDir, ".config", "outfitter", "catalog.toml")
	for _, want := range []string{"Created", "replace", path, "catalog validate"} {
		if !strings.Contains(out, want) {
			t.Fatalf("init output missing %q:\n%s", want, out)
		}
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "mode = \"replace\"") {
		t.Fatalf("seeded catalog lacks replace header:\n%s", content[:200])
	}
	report := runShowJSON(t, deps, "catalog", "show", "--json")
	if report.Mode != "replace" || report.ToolCount != 48 || report.CategoryCount != 4 ||
		report.ProfileCount != 4 || report.UsesBuiltin || report.Path != path {
		t.Fatalf("show report = %+v", report)
	}
	out, err = runCLI(t, deps, "catalog", "validate")
	if err != nil {
		t.Fatalf("validate seeded: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Catalog is valid.") {
		t.Fatalf("validate output:\n%s", out)
	}
}

func TestCatalogInitExtendAndBlank(t *testing.T) {
	deps, _ := testDeps(t)
	extendPath := filepath.Join(deps.HomeDir, "nested", "extend.toml")
	out, err := runCLI(t, deps, "catalog", "init", "--extend", "--output", extendPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "extend") {
		t.Fatalf("init --extend output:\n%s", out)
	}
	out, err = runCLI(t, deps, "catalog", "validate", extendPath, "--json")
	if err != nil {
		t.Fatalf("validate extend template: %v\n%s", err, out)
	}
	var validateReport catalogValidateReport
	if err := json.Unmarshal([]byte(out), &validateReport); err != nil {
		t.Fatal(err)
	}
	if !validateReport.Valid || validateReport.Mode != "extend" || validateReport.ToolCount != 48 {
		t.Fatalf("validate report = %+v", validateReport)
	}

	blankPath := filepath.Join(deps.HomeDir, "blank.toml")
	out, err = runCLI(t, deps, "catalog", "init", "--blank", "--output", blankPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "not usable") || !strings.Contains(out, "at least one") {
		t.Fatalf("init --blank must warn it is unusable:\n%s", out)
	}
	_, err = runCLI(t, deps, "catalog", "validate", blankPath)
	requireExit(t, err, 1)

	_, err = runCLI(t, deps, "catalog", "init", "--blank", "--extend")
	requireExit(t, err, 2)
}

func TestCatalogInitRefusalAndForceBackup(t *testing.T) {
	deps, _ := testDeps(t)
	path := writeUserCatalog(t, deps, "# my old catalog\n")

	_, err := runCLI(t, deps, "catalog", "init")
	if err == nil || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("init over existing file err = %v", err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "# my old catalog\n" {
		t.Fatal("refused init must not touch the existing file")
	}

	out, err := runCLI(t, deps, "catalog", "init", "--force")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "backed up") {
		t.Fatalf("init --force output:\n%s", out)
	}
	backups, err := filepath.Glob(path + ".*.bak")
	if err != nil || len(backups) != 1 {
		t.Fatalf("backups = %v (err %v)", backups, err)
	}
	backupContent, err := os.ReadFile(backups[0])
	if err != nil {
		t.Fatal(err)
	}
	if string(backupContent) != "# my old catalog\n" {
		t.Fatalf("backup content = %q", backupContent)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o644 {
		t.Fatalf("catalog mode = %o, want 644", info.Mode().Perm())
	}
}

func TestCatalogShowBuiltinExtendReplace(t *testing.T) {
	deps, _ := testDeps(t)

	report := runShowJSON(t, deps, "catalog", "show", "--json")
	if report.Origin != "embedded" || !report.UsesBuiltin || report.ToolCount != 48 ||
		report.SchemaVersion != 1 || report.Path != "" {
		t.Fatalf("built-in report = %+v", report)
	}
	out, err := runCLI(t, deps, "catalog", "show")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Name: Built-in catalog", "Mode: built-in", "Tools: 48"} {
		if !strings.Contains(out, want) {
			t.Fatalf("show output missing %q:\n%s", want, out)
		}
	}

	path := writeUserCatalog(t, deps, "[[tools]]\nid = \"btop\"\ndisabled = true\n")
	report = runShowJSON(t, deps, "catalog", "show", "--json")
	if report.Mode != "extend" || report.Origin != "user" || !report.UsesBuiltin ||
		report.ToolCount != 47 || report.Path != path {
		t.Fatalf("extend report = %+v", report)
	}

	writeUserCatalog(t, deps, cliMinimalReplace)
	report = runShowJSON(t, deps, "catalog", "show", "--json")
	if report.Mode != "replace" || report.Name != "Workstation" || report.UsesBuiltin ||
		report.ToolCount != 1 || report.ProfileCount != 0 {
		t.Fatalf("replace report = %+v", report)
	}
}

func TestCatalogShowIgnoresDemo(t *testing.T) {
	deps, _ := testDeps(t)
	writeUserCatalog(t, deps, cliMinimalReplace)
	report := runShowJSON(t, deps, "--demo", "catalog", "show", "--json")
	if report.ToolCount != 1 || report.Mode != "replace" {
		t.Fatalf("demo must not replace catalog admin results: %+v", report)
	}
}

func TestCatalogValidatePrecedenceAndFailures(t *testing.T) {
	deps, _ := testDeps(t)

	// No user file: validates the built-in effective catalog.
	out, err := runCLI(t, deps, "catalog", "validate", "--json")
	if err != nil {
		t.Fatalf("validate built-in: %v\n%s", err, out)
	}
	var report catalogValidateReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatal(err)
	}
	if !report.Valid || report.Origin != "embedded" || report.ToolCount != 48 {
		t.Fatalf("built-in validate report = %+v", report)
	}

	good := filepath.Join(deps.HomeDir, "good.toml")
	writeFile(t, good, cliMinimalReplace)
	broken := filepath.Join(deps.HomeDir, "broken.toml")
	writeFile(t, broken, "[catalog]\nmode = \"replace\"\n")
	writeUserCatalog(t, deps, "[catalog\n")

	// Positional path wins over --catalog and the broken default file.
	out, err = runCLI(t, deps, "--catalog", broken, "catalog", "validate", good)
	if err != nil {
		t.Fatalf("validate with positional precedence: %v\n%s", err, out)
	}
	// --catalog wins over the broken default file.
	out, err = runCLI(t, deps, "--catalog", good, "catalog", "validate")
	if err != nil {
		t.Fatalf("validate with --catalog: %v\n%s", err, out)
	}
	// The broken default file fails with exit 1 and JSON on failure.
	out, err = runCLI(t, deps, "catalog", "validate", "--json")
	requireExit(t, err, 1)
	var failure catalogValidateReport
	if err := json.Unmarshal([]byte(out), &failure); err != nil {
		t.Fatalf("validate failure output is not JSON: %v\n%s", err, out)
	}
	if failure.Valid || failure.Error == "" {
		t.Fatalf("failure report = %+v", failure)
	}
	// A missing explicit target is an error.
	_, err = runCLI(t, deps, "catalog", "validate", filepath.Join(deps.HomeDir, "missing.toml"))
	requireExit(t, err, 1)
	// Two paths are a usage error.
	_, err = runCLI(t, deps, "catalog", "validate", good, good)
	requireExit(t, err, 2)
}

func TestIgnoreUserCatalogAndConflicts(t *testing.T) {
	deps, _ := testDeps(t)
	writeUserCatalog(t, deps, "[catalog\n")

	_, err := runCLI(t, deps, "status")
	if err == nil {
		t.Fatal("status with a broken user catalog should fail")
	}
	out, err := runCLI(t, deps, "--ignore-user-catalog", "status", "--json")
	if err != nil {
		t.Fatalf("status ignoring user catalog: %v", err)
	}
	var entries []statusEntry
	if err := json.Unmarshal([]byte(out), &entries); err != nil {
		t.Fatal(err)
	}
	if len(entries) != 48 {
		t.Fatalf("entries = %d, want 48 built-ins", len(entries))
	}

	writeUserCatalog(t, deps, cliMinimalReplace)
	out, err = runCLI(t, deps, "status", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(out), &entries); err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].ID != "ttyd" {
		t.Fatalf("replace status entries = %+v", entries)
	}
	out, err = runCLI(t, deps, "--ignore-user-catalog", "status", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(out), &entries); err != nil {
		t.Fatal(err)
	}
	if len(entries) != 48 {
		t.Fatalf("ignored replace entries = %d", len(entries))
	}

	// --catalog + --ignore-user-catalog is a usage error everywhere.
	somePath := filepath.Join(deps.HomeDir, "x.toml")
	for _, args := range [][]string{
		{"--catalog", somePath, "--ignore-user-catalog", "status"},
		{"--catalog", somePath, "--ignore-user-catalog", "catalog", "validate"},
		{"--catalog", somePath, "--ignore-user-catalog"},
	} {
		_, err := runCLI(t, deps, args...)
		requireExit(t, err, 2)
	}
}

func TestCatalogAdminUsableWithBrokenDefault(t *testing.T) {
	deps, _ := testDeps(t)
	writeUserCatalog(t, deps, "[catalog\n")

	out, err := runCLI(t, deps, "catalog", "export")
	if err != nil {
		t.Fatalf("export with broken default: %v", err)
	}
	if out != catalog.ExportDefault() {
		t.Fatal("export must be exactly the embedded catalog")
	}

	target := filepath.Join(deps.HomeDir, "fresh.toml")
	if _, err := runCLI(t, deps, "catalog", "init", "--output", target); err != nil {
		t.Fatalf("init with broken default: %v", err)
	}
	if _, err := runCLI(t, deps, "catalog", "validate", target); err != nil {
		t.Fatalf("validate fresh catalog: %v", err)
	}

	_, err = runCLI(t, deps, "catalog", "show")
	if err == nil || !strings.Contains(err.Error(), "catalog validate") {
		t.Fatalf("show with broken default should hint at validate, err = %v", err)
	}
}

func TestTUIFlagWiringAndHelp(t *testing.T) {
	deps, _ := testDeps(t)
	var got tui.Options
	deps.RunTUI = func(_ context.Context, opts tui.Options) error {
		got = opts
		return nil
	}
	if _, err := runCLI(t, deps, "--ignore-user-catalog"); err != nil {
		t.Fatal(err)
	}
	if !got.IgnoreUserCatalog || got.HomeDir != deps.HomeDir {
		t.Fatalf("TUI options = %+v", got)
	}

	out, err := runCLI(t, deps, "--help")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "--ignore-user-catalog") {
		t.Fatalf("root help missing --ignore-user-catalog:\n%s", out)
	}
	out, err = runCLI(t, deps, "catalog", "--help")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"init", "show", "validate", "export"} {
		if !strings.Contains(out, want) {
			t.Fatalf("catalog help missing %q:\n%s", want, out)
		}
	}
	out, err = runCLI(t, deps, "catalog", "init", "--help")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"--blank", "--extend", "--force", "--output"} {
		if !strings.Contains(out, want) {
			t.Fatalf("init help missing %q:\n%s", want, out)
		}
	}
}
