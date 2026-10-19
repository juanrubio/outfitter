// Package catalog loads Outfitter's embedded catalog and merges user overrides
// from catalog.toml.
package catalog

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/pelletier/go-toml/v2"

	"github.com/juanrubio/outfitter/internal/domain"
)

//go:embed default_catalog.toml
var defaultCatalogTOML string

// Catalog is the merged, active catalog used by the TUI and CLI.
type Catalog struct {
	Settings   domain.Settings   `json:"settings"`
	Categories []domain.Category `json:"categories"`
	Profiles   []domain.Profile  `json:"profiles"`
	Tools      []domain.Tool     `json:"tools"`
}

// ToolByID returns the active tool with the given ID.
func (c *Catalog) ToolByID(id string) (domain.Tool, bool) {
	for _, tool := range c.Tools {
		if tool.ID == id {
			return tool, true
		}
	}
	return domain.Tool{}, false
}

// ProfileByID returns the profile with the given ID.
func (c *Catalog) ProfileByID(id string) (domain.Profile, bool) {
	for _, profile := range c.Profiles {
		if profile.ID == id {
			return profile, true
		}
	}
	return domain.Profile{}, false
}

// ToolsByCategory returns the active tools in a category, preserving catalog
// order.
func (c *Catalog) ToolsByCategory(categoryID string) []domain.Tool {
	tools := make([]domain.Tool, 0)
	for _, tool := range c.Tools {
		if tool.CategoryID == categoryID {
			tools = append(tools, tool)
		}
	}
	return tools
}

// DefaultUserCatalogPath returns the conventional per-user catalog path.
func DefaultUserCatalogPath() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ".config", "outfitter", "catalog.toml")
}

// ExportDefault returns the raw embedded default catalog.
func ExportDefault() string {
	return defaultCatalogTOML
}

// LoadDefault parses and returns the embedded catalog without user overrides.
func LoadDefault() (*Catalog, error) {
	cat, err := ParseCatalog([]byte(defaultCatalogTOML))
	if err != nil {
		return nil, fmt.Errorf("parse embedded catalog: %w", err)
	}
	return cat, nil
}

// ParseCatalog parses a complete catalog TOML document, applies defaults, and
// validates the resulting catalog. It never merges the embedded defaults.
func ParseCatalog(data []byte) (*Catalog, error) {
	var raw defaultFile
	if err := toml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse catalog: %w", err)
	}

	settings := domain.DefaultSettings()
	if raw.Settings != nil {
		settings = *raw.Settings
		if settings.InstallDir == "" {
			settings.InstallDir = domain.DefaultInstallDir
		}
	}

	cat := &Catalog{
		Settings:   settings,
		Categories: append([]domain.Category(nil), raw.Categories...),
		Profiles:   append([]domain.Profile(nil), raw.Profiles...),
		Tools:      append([]domain.Tool(nil), raw.Tools...),
	}
	if err := finalizeCatalog(cat); err != nil {
		return nil, err
	}
	return cat, nil
}

// Load loads the embedded catalog and, when present, merges a user catalog.
// An empty path uses ~/.config/outfitter/catalog.toml if it exists.
func Load(userCatalogPath string) (*Catalog, error) {
	if userCatalogPath == "" {
		path := DefaultUserCatalogPath()
		if path == "" {
			return LoadDefault()
		}
		data, err := os.ReadFile(path)
		if errors.Is(err, os.ErrNotExist) {
			return LoadDefault()
		}
		if err != nil {
			return nil, fmt.Errorf("read user catalog %s: %w", path, err)
		}
		return LoadFromBytes(data)
	}

	data, err := os.ReadFile(userCatalogPath)
	if err != nil {
		return nil, fmt.Errorf("read user catalog %s: %w", userCatalogPath, err)
	}
	return LoadFromBytes(data)
}

// LoadFromBytes loads the embedded catalog and merges the supplied user TOML.
func LoadFromBytes(userTOML []byte) (*Catalog, error) {
	cat, err := LoadDefault()
	if err != nil {
		return nil, err
	}
	if len(bytes.TrimSpace(userTOML)) == 0 {
		return cat, nil
	}

	var raw userFile
	if err := toml.Unmarshal(userTOML, &raw); err != nil {
		return nil, fmt.Errorf("parse user catalog: %w", err)
	}
	if err := mergeUserCatalog(cat, raw); err != nil {
		return nil, err
	}
	if err := finalizeCatalog(cat); err != nil {
		return nil, err
	}
	return cat, nil
}

type defaultFile struct {
	Settings   *domain.Settings  `toml:"settings"`
	Categories []domain.Category `toml:"categories"`
	Profiles   []domain.Profile  `toml:"profiles"`
	Tools      []domain.Tool     `toml:"tools"`
}

type userFile struct {
	Settings   map[string]any   `toml:"settings"`
	Categories []map[string]any `toml:"categories"`
	Tools      []map[string]any `toml:"tools"`
	Profiles   any              `toml:"profiles"`
}

type userSettings struct {
	InstallDir        *string `json:"install_dir"`
	AutoBootstrapMise *bool   `json:"auto_bootstrap_mise"`
	GitHubTokenFromGH *bool   `json:"github_token_from_gh"`
}

type userCategory struct {
	ID    *string `json:"id"`
	Num   *int    `json:"num"`
	Title *string `json:"title"`
}

type userTool struct {
	ID           *string   `json:"id"`
	Name         *string   `json:"name"`
	Binary       *string   `json:"binary"`
	Category     *string   `json:"category"`
	Backend      *string   `json:"backend"`
	Package      *string   `json:"package"`
	Sudo         *bool     `json:"sudo"`
	Profiles     *[]string `json:"profiles"`
	Description  *string   `json:"description"`
	InstallCmd   *string   `json:"install_cmd"`
	UpdateCmd    *string   `json:"update_cmd"`
	RemoveCmd    *string   `json:"remove_cmd"`
	SourceDir    *string   `json:"source_dir"`
	BuildCmd     *string   `json:"build_cmd"`
	VersionArgs  *[]string `json:"version_args"`
	VersionRegex *string   `json:"version_regex"`
	Disabled     *bool     `json:"disabled"`
}

type userProfile struct {
	ID          *string   `json:"id"`
	Name        *string   `json:"name"`
	Short       *string   `json:"short"`
	Description *string   `json:"description"`
	Tools       *[]string `json:"tools"`
}

type membershipOverride struct {
	toolID   string
	profiles []string
}

func mergeUserCatalog(cat *Catalog, raw userFile) error {
	if err := applyUserSettings(cat, raw.Settings); err != nil {
		return err
	}
	if err := applyUserCategories(cat, raw.Categories); err != nil {
		return err
	}

	overrides, err := applyUserTools(cat, raw.Tools)
	if err != nil {
		return err
	}

	profiles, err := parseUserProfiles(raw.Profiles)
	if err != nil {
		return err
	}
	if err := applyUserProfiles(cat, profiles); err != nil {
		return err
	}

	for _, override := range overrides {
		applyToolProfiles(cat, override.toolID, override.profiles)
	}
	return nil
}

func applyUserSettings(cat *Catalog, raw map[string]any) error {
	if raw == nil {
		return nil
	}
	settings, err := decodeMap[userSettings](raw)
	if err != nil {
		return fmt.Errorf("parse [settings]: %w", err)
	}
	if settings.InstallDir != nil {
		cat.Settings.InstallDir = *settings.InstallDir
	}
	if settings.AutoBootstrapMise != nil {
		cat.Settings.AutoBootstrapMise = *settings.AutoBootstrapMise
	}
	if settings.GitHubTokenFromGH != nil {
		cat.Settings.GitHubTokenFromGH = *settings.GitHubTokenFromGH
	}
	return nil
}

func applyUserCategories(cat *Catalog, raw []map[string]any) error {
	for _, item := range raw {
		category, err := decodeMap[userCategory](item)
		if err != nil {
			return fmt.Errorf("parse [[categories]]: %w", err)
		}
		if category.ID == nil || strings.TrimSpace(*category.ID) == "" {
			return errors.New("category id must not be empty")
		}
		id := strings.TrimSpace(*category.ID)

		index := categoryIndex(cat, id)
		if index < 0 {
			entry := domain.Category{
				ID:    id,
				Num:   nextCategoryNum(cat),
				Title: titleFromID(id),
			}
			if category.Num != nil && *category.Num > 0 {
				entry.Num = *category.Num
			}
			if category.Title != nil && strings.TrimSpace(*category.Title) != "" {
				entry.Title = *category.Title
			}
			cat.Categories = append(cat.Categories, entry)
			continue
		}

		entry := cat.Categories[index]
		if category.Num != nil && *category.Num > 0 {
			entry.Num = *category.Num
		}
		if category.Title != nil && strings.TrimSpace(*category.Title) != "" {
			entry.Title = *category.Title
		}
		cat.Categories[index] = entry
	}
	return nil
}

func applyUserTools(cat *Catalog, raw []map[string]any) ([]membershipOverride, error) {
	seen := make(map[string]struct{}, len(raw))
	overrides := make([]membershipOverride, 0, len(raw))

	for _, item := range raw {
		tool, err := decodeMap[userTool](item)
		if err != nil {
			return nil, fmt.Errorf("parse [[tools]]: %w", err)
		}
		if tool.ID == nil || strings.TrimSpace(*tool.ID) == "" {
			return nil, errors.New("tool id must not be empty")
		}
		id := strings.TrimSpace(*tool.ID)
		if _, exists := seen[id]; exists {
			return nil, fmt.Errorf("duplicate tool id %q", id)
		}
		seen[id] = struct{}{}

		index := toolIndex(cat, id)
		if index < 0 {
			added, err := addUserTool(cat, id, tool)
			if err != nil {
				return nil, err
			}
			cat.Tools = append(cat.Tools, added)
		} else {
			updated, err := updateUserTool(cat, cat.Tools[index], id, tool)
			if err != nil {
				return nil, err
			}
			cat.Tools[index] = updated
		}

		if tool.Profiles != nil {
			overrides = append(overrides, membershipOverride{
				toolID:   id,
				profiles: dedupeStrings(*tool.Profiles),
			})
		}
	}
	return overrides, nil
}

func addUserTool(cat *Catalog, id string, raw userTool) (domain.Tool, error) {
	if raw.Binary == nil || strings.TrimSpace(*raw.Binary) == "" {
		return domain.Tool{}, fmt.Errorf("tool %q: binary must not be empty", id)
	}
	if raw.Backend == nil || strings.TrimSpace(*raw.Backend) == "" {
		return domain.Tool{}, fmt.Errorf("tool %q: backend must not be empty", id)
	}

	backend := domain.BackendType(strings.TrimSpace(*raw.Backend))
	if !backend.IsValid() {
		return domain.Tool{}, fmt.Errorf("tool %q: invalid backend %q", id, *raw.Backend)
	}

	category := "utilities"
	if raw.Category != nil && strings.TrimSpace(*raw.Category) != "" {
		category = strings.TrimSpace(*raw.Category)
	}
	ensureCategory(cat, category)

	tool := domain.Tool{
		ID:         id,
		Name:       id,
		Binary:     strings.TrimSpace(*raw.Binary),
		CategoryID: category,
		Backend:    backend,
		Sudo:       backend == domain.BackendAPT,
	}
	applyOptionalFields(&tool, raw)
	finalizeToolCommands(&tool, raw, true, true)
	return tool, nil
}

func updateUserTool(cat *Catalog, tool domain.Tool, id string, raw userTool) (domain.Tool, error) {
	oldBackend := tool.Backend
	oldPackage := tool.Package
	oldSourceDir := tool.SourceDir
	oldBuildCmd := tool.BuildCmd

	if raw.Binary != nil {
		binary := strings.TrimSpace(*raw.Binary)
		if binary == "" {
			return domain.Tool{}, fmt.Errorf("tool %q: binary must not be empty", id)
		}
		tool.Binary = binary
	}
	if raw.Category != nil && strings.TrimSpace(*raw.Category) != "" {
		category := strings.TrimSpace(*raw.Category)
		ensureCategory(cat, category)
		tool.CategoryID = category
	}
	if raw.Backend != nil {
		backend := domain.BackendType(strings.TrimSpace(*raw.Backend))
		if !backend.IsValid() {
			return domain.Tool{}, fmt.Errorf("tool %q: invalid backend %q", id, *raw.Backend)
		}
		tool.Backend = backend
	}
	applyOptionalFields(&tool, raw)

	backendChanged := tool.Backend != oldBackend
	packageChanged := tool.Package != oldPackage
	sourceDirChanged := tool.SourceDir != oldSourceDir
	buildCmdChanged := tool.BuildCmd != oldBuildCmd
	recompute := backendChanged || packageChanged || sourceDirChanged || buildCmdChanged
	finalizeToolCommands(&tool, raw, recompute, false)
	return tool, nil
}

func applyOptionalFields(tool *domain.Tool, raw userTool) {
	if raw.Name != nil && strings.TrimSpace(*raw.Name) != "" {
		tool.Name = *raw.Name
	}
	if raw.Package != nil && strings.TrimSpace(*raw.Package) != "" {
		tool.Package = *raw.Package
	}
	if raw.Sudo != nil {
		tool.Sudo = *raw.Sudo
	}
	if raw.Profiles != nil {
		tool.Profiles = dedupeStrings(*raw.Profiles)
	}
	if raw.Description != nil && strings.TrimSpace(*raw.Description) != "" {
		tool.Description = *raw.Description
	}
	if raw.InstallCmd != nil && strings.TrimSpace(*raw.InstallCmd) != "" {
		tool.InstallCmd = *raw.InstallCmd
	}
	if raw.UpdateCmd != nil && strings.TrimSpace(*raw.UpdateCmd) != "" {
		tool.UpdateCmd = *raw.UpdateCmd
	}
	if raw.RemoveCmd != nil && strings.TrimSpace(*raw.RemoveCmd) != "" {
		tool.RemoveCmd = *raw.RemoveCmd
	}
	if raw.SourceDir != nil && strings.TrimSpace(*raw.SourceDir) != "" {
		tool.SourceDir = *raw.SourceDir
	}
	if raw.BuildCmd != nil && strings.TrimSpace(*raw.BuildCmd) != "" {
		tool.BuildCmd = *raw.BuildCmd
	}
	if raw.VersionArgs != nil {
		tool.VersionArgs = append([]string(nil), (*raw.VersionArgs)...)
	}
	if raw.VersionRegex != nil && strings.TrimSpace(*raw.VersionRegex) != "" {
		tool.VersionRegex = *raw.VersionRegex
	}
	if raw.Disabled != nil {
		tool.Disabled = *raw.Disabled
	}
}

func finalizeToolCommands(tool *domain.Tool, raw userTool, recompute, isNew bool) {
	if tool.Backend == domain.BackendSource && tool.BuildCmd != "" {
		if raw.InstallCmd == nil {
			tool.InstallCmd = tool.BuildCmd
		}
		if raw.UpdateCmd == nil {
			tool.UpdateCmd = tool.BuildCmd
		}
	}

	if isNew || recompute {
		if raw.InstallCmd == nil {
			tool.InstallCmd = domain.DefaultInstallCommand(*tool)
		}
		if raw.UpdateCmd == nil {
			tool.UpdateCmd = domain.DefaultUpdateCommand(*tool)
		}
		if raw.RemoveCmd == nil {
			tool.RemoveCmd = domain.DefaultRemoveCommand(*tool)
		}
	}
}

func parseUserProfiles(raw any) ([]userProfile, error) {
	switch value := raw.(type) {
	case nil:
		return nil, nil
	case map[string]any:
		keys := make([]string, 0, len(value))
		for key := range value {
			keys = append(keys, key)
		}
		sort.Strings(keys)

		profiles := make([]userProfile, 0, len(keys))
		for _, id := range keys {
			item, ok := value[id].(map[string]any)
			if !ok {
				return nil, fmt.Errorf("profile %q must be a table", id)
			}
			copied := make(map[string]any, len(item)+1)
			for key, field := range item {
				copied[key] = field
			}
			copied["id"] = id

			profile, err := decodeMap[userProfile](copied)
			if err != nil {
				return nil, fmt.Errorf("parse profile %q: %w", id, err)
			}
			profiles = append(profiles, profile)
		}
		return profiles, nil
	case []any:
		profiles := make([]userProfile, 0, len(value))
		for _, entry := range value {
			item, ok := entry.(map[string]any)
			if !ok {
				return nil, errors.New("[[profiles]] entries must be tables")
			}
			profile, err := decodeMap[userProfile](item)
			if err != nil {
				return nil, fmt.Errorf("parse [[profiles]]: %w", err)
			}
			profiles = append(profiles, profile)
		}
		return profiles, nil
	default:
		return nil, fmt.Errorf("profiles must be a table or array, got %T", raw)
	}
}

func applyUserProfiles(cat *Catalog, profiles []userProfile) error {
	for _, raw := range profiles {
		if raw.ID == nil || strings.TrimSpace(*raw.ID) == "" {
			return errors.New("profile id must not be empty")
		}
		id := strings.TrimSpace(*raw.ID)

		index := profileIndex(cat, id)
		if index < 0 {
			profile := domain.Profile{ID: id, Name: id, Short: id}
			applyProfileFields(&profile, raw)
			cat.Profiles = append(cat.Profiles, profile)
			continue
		}

		profile := cat.Profiles[index]
		applyProfileFields(&profile, raw)
		cat.Profiles[index] = profile
	}
	return nil
}

func applyProfileFields(profile *domain.Profile, raw userProfile) {
	if raw.Name != nil && strings.TrimSpace(*raw.Name) != "" {
		profile.Name = *raw.Name
		if raw.Short == nil {
			profile.Short = *raw.Name
		}
	}
	if raw.Short != nil && strings.TrimSpace(*raw.Short) != "" {
		profile.Short = *raw.Short
	}
	if raw.Description != nil && strings.TrimSpace(*raw.Description) != "" {
		profile.Description = *raw.Description
	}
	if raw.Tools != nil {
		profile.Tools = dedupeStrings(*raw.Tools)
	}
}

func applyToolProfiles(cat *Catalog, toolID string, profiles []string) {
	desired := make(map[string]struct{}, len(profiles))
	for _, id := range profiles {
		desired[id] = struct{}{}
	}

	for index := range cat.Profiles {
		profile := &cat.Profiles[index]
		_, wanted := desired[profile.ID]
		hasTool := containsString(profile.Tools, toolID)

		switch {
		case wanted && !hasTool:
			profile.Tools = append(profile.Tools, toolID)
		case !wanted && hasTool:
			profile.Tools = removeString(profile.Tools, toolID)
		}
	}
}

func finalizeCatalog(cat *Catalog) error {
	active := make([]domain.Tool, 0, len(cat.Tools))
	for _, tool := range cat.Tools {
		if tool.Disabled {
			continue
		}
		if tool.CategoryID == "" {
			tool.CategoryID = "utilities"
		}
		ensureCategory(cat, tool.CategoryID)
		active = append(active, tool)
	}
	cat.Tools = active

	sort.SliceStable(cat.Categories, func(i, j int) bool {
		if cat.Categories[i].Num == cat.Categories[j].Num {
			return cat.Categories[i].ID < cat.Categories[j].ID
		}
		return cat.Categories[i].Num < cat.Categories[j].Num
	})

	synchronizeProfiles(cat)

	categoryOrder := make(map[string]int, len(cat.Categories))
	for index, category := range cat.Categories {
		categoryOrder[category.ID] = index
	}
	sort.SliceStable(cat.Tools, func(i, j int) bool {
		left, leftOK := categoryOrder[cat.Tools[i].CategoryID]
		right, rightOK := categoryOrder[cat.Tools[j].CategoryID]
		if !leftOK {
			left = len(cat.Categories)
		}
		if !rightOK {
			right = len(cat.Categories)
		}
		return left < right
	})

	return validateCatalog(cat)
}

func synchronizeProfiles(cat *Catalog) {
	active := make(map[string]struct{}, len(cat.Tools))
	for _, tool := range cat.Tools {
		active[tool.ID] = struct{}{}
	}

	for index := range cat.Profiles {
		profile := &cat.Profiles[index]
		seen := make(map[string]struct{}, len(profile.Tools))
		tools := make([]string, 0, len(profile.Tools))
		for _, id := range profile.Tools {
			if _, ok := active[id]; !ok {
				continue
			}
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			tools = append(tools, id)
		}
		profile.Tools = tools
	}

	for index := range cat.Tools {
		profiles := make([]string, 0, len(cat.Profiles))
		for _, profile := range cat.Profiles {
			if containsString(profile.Tools, cat.Tools[index].ID) {
				profiles = append(profiles, profile.ID)
			}
		}
		cat.Tools[index].Profiles = profiles
	}
}

func validateCatalog(cat *Catalog) error {
	categories := make(map[string]struct{}, len(cat.Categories))
	for _, category := range cat.Categories {
		if strings.TrimSpace(category.ID) == "" {
			return errors.New("category id must not be empty")
		}
		if _, exists := categories[category.ID]; exists {
			return fmt.Errorf("duplicate category id %q", category.ID)
		}
		categories[category.ID] = struct{}{}
	}

	profiles := make(map[string]struct{}, len(cat.Profiles))
	for _, profile := range cat.Profiles {
		if strings.TrimSpace(profile.ID) == "" {
			return errors.New("profile id must not be empty")
		}
		if _, exists := profiles[profile.ID]; exists {
			return fmt.Errorf("duplicate profile id %q", profile.ID)
		}
		profiles[profile.ID] = struct{}{}
	}

	tools := make(map[string]struct{}, len(cat.Tools))
	for _, tool := range cat.Tools {
		if strings.TrimSpace(tool.ID) == "" {
			return errors.New("tool id must not be empty")
		}
		if _, exists := tools[tool.ID]; exists {
			return fmt.Errorf("duplicate tool id %q", tool.ID)
		}
		tools[tool.ID] = struct{}{}

		if strings.TrimSpace(tool.Binary) == "" {
			return fmt.Errorf("tool %q: binary must not be empty", tool.ID)
		}
		if !tool.Backend.IsValid() {
			return fmt.Errorf("tool %q: invalid backend %q", tool.ID, tool.Backend)
		}
	}
	return nil
}

func decodeMap[T any](raw map[string]any) (T, error) {
	var value T
	data, err := json.Marshal(raw)
	if err != nil {
		return value, err
	}
	if err := json.Unmarshal(data, &value); err != nil {
		return value, err
	}
	return value, nil
}

func toolIndex(cat *Catalog, id string) int {
	for index, tool := range cat.Tools {
		if tool.ID == id {
			return index
		}
	}
	return -1
}

func categoryIndex(cat *Catalog, id string) int {
	for index, category := range cat.Categories {
		if category.ID == id {
			return index
		}
	}
	return -1
}

func profileIndex(cat *Catalog, id string) int {
	for index, profile := range cat.Profiles {
		if profile.ID == id {
			return index
		}
	}
	return -1
}

func ensureCategory(cat *Catalog, id string) {
	if id == "" || categoryIndex(cat, id) >= 0 {
		return
	}
	cat.Categories = append(cat.Categories, domain.Category{
		ID:    id,
		Num:   nextCategoryNum(cat),
		Title: titleFromID(id),
	})
}

func nextCategoryNum(cat *Catalog) int {
	maxNum := 0
	for _, category := range cat.Categories {
		if category.Num > maxNum {
			maxNum = category.Num
		}
	}
	return maxNum + 1
}

func titleFromID(id string) string {
	return strings.ToUpper(strings.ReplaceAll(id, "-", " "))
}

func dedupeStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func containsString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func removeString(values []string, unwanted string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value != unwanted {
			result = append(result, value)
		}
	}
	return result
}
