package catalog

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

// Mode describes how a user catalog is composed with the built-in catalog.
type Mode string

// Catalog composition modes. ModeBuiltin is an effective mode only; it is
// never accepted in a [catalog] header.
const (
	ModeExtend  Mode = "extend"
	ModeReplace Mode = "replace"
	ModeBuiltin Mode = "built-in"
)

// Source identifies where an effective catalog came from.
type Source string

// Catalog sources.
const (
	SourceEmbedded Source = "embedded"
	SourceUser     Source = "user"
	SourceDemo     Source = "demo"
)

// CurrentSchemaVersion is the newest catalog schema this build understands.
const CurrentSchemaVersion = 1

// Meta carries the effective catalog's identity for the CLI and TUI.
type Meta struct {
	SchemaVersion int    `json:"schema_version"`
	Mode          Mode   `json:"mode"`
	Name          string `json:"name,omitempty"`
	Source        Source `json:"source"`
	Path          string `json:"path,omitempty"`
	UsesBuiltin   bool   `json:"uses_builtin"`
}

// EffectiveName returns the catalog name, or a sensible fallback.
func (m Meta) EffectiveName() string {
	if name := strings.TrimSpace(m.Name); name != "" {
		return name
	}
	switch m.Source {
	case SourceDemo:
		return "Demo catalog"
	case SourceEmbedded:
		return "Built-in catalog"
	}
	if m.Mode == ModeReplace {
		return "Replacement catalog"
	}
	return "Built-in catalog with extensions"
}

// IsUserCatalog reports whether a user file contributed to the catalog.
func (m Meta) IsUserCatalog() bool {
	return m.Source == SourceUser
}

type catalogHeader struct {
	SchemaVersion int
	Mode          Mode
	Name          string
	Present       bool
}

func defaultHeader() catalogHeader {
	return catalogHeader{SchemaVersion: CurrentSchemaVersion, Mode: ModeExtend}
}

func parseHeader(rawDoc map[string]any) (catalogHeader, error) {
	header := defaultHeader()
	raw, present := rawDoc["catalog"]
	if !present {
		return header, nil
	}
	header.Present = true
	table, ok := raw.(map[string]any)
	if !ok {
		return header, errors.New("[catalog] must be a table")
	}
	if value, ok := table["schema_version"]; ok {
		version, ok := value.(int64)
		if !ok {
			return header, fmt.Errorf("catalog schema_version must be an integer, got %T", value)
		}
		if version <= 0 {
			return header, fmt.Errorf("catalog schema_version must be positive, got %d", version)
		}
		if version > CurrentSchemaVersion {
			return header, fmt.Errorf(
				"unsupported catalog schema_version %d (this outfitter supports up to %d); upgrade outfitter",
				version, CurrentSchemaVersion)
		}
		header.SchemaVersion = int(version)
	}
	if value, ok := table["mode"]; ok {
		modeText, ok := value.(string)
		if !ok {
			return header, fmt.Errorf("catalog mode must be a string, got %T", value)
		}
		switch Mode(strings.ToLower(strings.TrimSpace(modeText))) {
		case ModeExtend:
			header.Mode = ModeExtend
		case ModeReplace:
			header.Mode = ModeReplace
		default:
			return header, fmt.Errorf("invalid catalog mode %q (want \"extend\" or \"replace\")", modeText)
		}
	}
	if value, ok := table["name"]; ok {
		name, ok := value.(string)
		if !ok {
			return header, fmt.Errorf("catalog name must be a string, got %T", value)
		}
		header.Name = strings.TrimSpace(name)
	}
	return header, nil
}

var (
	topLevelKeys = keySet("catalog", "settings", "categories", "tools", "profiles")
	catalogKeys  = keySet("schema_version", "mode", "name")
	settingsKeys = keySet("install_dir", "auto_bootstrap_mise", "github_token_from_gh")
	categoryKeys = keySet("id", "num", "title")
	toolKeys     = keySet("id", "name", "binary", "category", "backend", "package", "sudo",
		"profiles", "description", "install_cmd", "update_cmd", "remove_cmd", "source_dir",
		"build_cmd", "version_args", "version_regex", "disabled")
	profileKeys    = keySet("id", "name", "short", "description", "tools")
	profileMapKeys = keySet("id", "name", "short", "description", "tools")
)

func keySet(keys ...string) map[string]struct{} {
	set := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		set[key] = struct{}{}
	}
	return set
}

// checkUnknownKeys rejects keys outside the schema-1 grammar. It is applied
// to header-bearing (schema-bearing) user files and standalone catalogs.
func checkUnknownKeys(rawDoc map[string]any) error {
	if err := rejectUnknown(rawDoc, topLevelKeys, "catalog"); err != nil {
		return err
	}
	if table, ok := rawDoc["catalog"].(map[string]any); ok {
		if err := rejectUnknown(table, catalogKeys, "[catalog]"); err != nil {
			return err
		}
	}
	if table, ok := rawDoc["settings"].(map[string]any); ok {
		if err := rejectUnknown(table, settingsKeys, "[settings]"); err != nil {
			return err
		}
	}
	if err := checkTableList(rawDoc["categories"], categoryKeys, "[[categories]]"); err != nil {
		return err
	}
	if err := checkTableList(rawDoc["tools"], toolKeys, "[[tools]]"); err != nil {
		return err
	}
	switch profiles := rawDoc["profiles"].(type) {
	case map[string]any:
		ids := make([]string, 0, len(profiles))
		for id := range profiles {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			table, ok := profiles[id].(map[string]any)
			if !ok {
				continue
			}
			if err := rejectUnknown(table, profileMapKeys, fmt.Sprintf("[profiles.%s]", id)); err != nil {
				return err
			}
		}
	case []any:
		if err := checkTableList(profiles, profileKeys, "[[profiles]]"); err != nil {
			return err
		}
	}
	return nil
}

func checkTableList(raw any, allowed map[string]struct{}, section string) error {
	items, ok := raw.([]any)
	if !ok {
		return nil
	}
	for _, item := range items {
		table, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if err := rejectUnknown(table, allowed, section); err != nil {
			return err
		}
	}
	return nil
}

func rejectUnknown(table map[string]any, allowed map[string]struct{}, section string) error {
	keys := make([]string, 0, len(table))
	for key := range table {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if _, ok := allowed[key]; !ok {
			return fmt.Errorf("unknown key %q in %s", key, section)
		}
	}
	return nil
}
