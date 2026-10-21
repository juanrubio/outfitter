package catalog

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/juanrubio/outfitter/internal/domain"
)

var validIDPattern = regexp.MustCompile(`^[a-z0-9]+(?:[-_][a-z0-9]+)*$`)

func isValidID(id string) bool {
	return validIDPattern.MatchString(id)
}

// checkDuplicateIDs rejects duplicate category, tool, and profile IDs in a
// standalone document. The merge helpers would otherwise collapse duplicate
// categories and profiles, so duplicates must be caught before building.
func checkDuplicateIDs(raw userFile) error {
	seenCategories := make(map[string]struct{}, len(raw.Categories))
	for _, item := range raw.Categories {
		category, err := decodeMap[userCategory](item)
		if err != nil {
			return fmt.Errorf("parse [[categories]]: %w", err)
		}
		if category.ID == nil || strings.TrimSpace(*category.ID) == "" {
			return errors.New("category id must not be empty")
		}
		id := strings.TrimSpace(*category.ID)
		if _, exists := seenCategories[id]; exists {
			return fmt.Errorf("duplicate category id %q", id)
		}
		seenCategories[id] = struct{}{}
	}

	seenTools := make(map[string]struct{}, len(raw.Tools))
	for _, item := range raw.Tools {
		tool, err := decodeMap[userTool](item)
		if err != nil {
			return fmt.Errorf("parse [[tools]]: %w", err)
		}
		if tool.ID == nil || strings.TrimSpace(*tool.ID) == "" {
			return errors.New("tool id must not be empty")
		}
		id := strings.TrimSpace(*tool.ID)
		if _, exists := seenTools[id]; exists {
			return fmt.Errorf("duplicate tool id %q", id)
		}
		seenTools[id] = struct{}{}
	}

	profiles, err := parseUserProfiles(raw.Profiles)
	if err != nil {
		return err
	}
	seenProfiles := make(map[string]struct{}, len(profiles))
	for _, profile := range profiles {
		if profile.ID == nil || strings.TrimSpace(*profile.ID) == "" {
			return errors.New("profile id must not be empty")
		}
		id := strings.TrimSpace(*profile.ID)
		if _, exists := seenProfiles[id]; exists {
			return fmt.Errorf("duplicate profile id %q", id)
		}
		seenProfiles[id] = struct{}{}
	}
	return nil
}

// validateStandaloneCatalog validates a replacement (or standalone) catalog
// after defaults and derived commands are applied but before profile
// synchronization can erase bad references.
func validateStandaloneCatalog(cat *Catalog) error {
	if strings.TrimSpace(cat.Settings.InstallDir) == "" {
		return errors.New("settings: install_dir must not be empty")
	}
	for _, category := range cat.Categories {
		if !isValidID(category.ID) {
			return fmt.Errorf("invalid category id %q", category.ID)
		}
	}

	profileIDs := make(map[string]struct{}, len(cat.Profiles))
	for _, profile := range cat.Profiles {
		if !isValidID(profile.ID) {
			return fmt.Errorf("invalid profile id %q", profile.ID)
		}
		profileIDs[profile.ID] = struct{}{}
	}

	activeIDs := make(map[string]struct{}, len(cat.Tools))
	allIDs := make(map[string]struct{}, len(cat.Tools))
	for _, tool := range cat.Tools {
		allIDs[tool.ID] = struct{}{}
		if !tool.Disabled {
			activeIDs[tool.ID] = struct{}{}
		}
	}

	for _, tool := range cat.Tools {
		if err := validateStandaloneTool(tool, profileIDs); err != nil {
			return err
		}
	}

	for _, profile := range cat.Profiles {
		for _, toolID := range profile.Tools {
			if _, ok := allIDs[toolID]; !ok {
				return fmt.Errorf("profile %q references unknown tool %q", profile.ID, toolID)
			}
			if _, ok := activeIDs[toolID]; !ok {
				return fmt.Errorf("profile %q references disabled tool %q", profile.ID, toolID)
			}
		}
	}

	if len(activeIDs) == 0 {
		return errors.New("catalog has no active tools: add at least one [[tools]] entry")
	}
	return nil
}

func validateStandaloneTool(
	tool domain.Tool,
	profileIDs map[string]struct{},
) error {
	if !isValidID(tool.ID) {
		return fmt.Errorf("invalid tool id %q", tool.ID)
	}
	if strings.TrimSpace(tool.Binary) == "" {
		return fmt.Errorf("tool %q: binary must not be empty", tool.ID)
	}
	if strings.ContainsAny(tool.Binary, " \t\n") {
		return fmt.Errorf("tool %q: invalid binary %q", tool.ID, tool.Binary)
	}
	if !tool.Backend.IsValid() {
		return fmt.Errorf("tool %q: invalid backend %q", tool.ID, tool.Backend)
	}
	if tool.VersionRegex != "" {
		if _, err := regexp.Compile(tool.VersionRegex); err != nil {
			return fmt.Errorf("tool %q: invalid version_regex %q: %w", tool.ID, tool.VersionRegex, err)
		}
	}
	for _, profileID := range tool.Profiles {
		if _, ok := profileIDs[profileID]; !ok {
			return fmt.Errorf("tool %q references unknown profile %q", tool.ID, profileID)
		}
	}
	if tool.Disabled {
		return nil
	}

	switch tool.Backend {
	case domain.BackendMiseAqua, domain.BackendMiseUbi, domain.BackendMiseGitHub,
		domain.BackendMiseNPM, domain.BackendUV, domain.BackendAPT:
		if strings.TrimSpace(tool.Package) == "" {
			return fmt.Errorf("tool %q: package is required for backend %q", tool.ID, tool.Backend)
		}
	case domain.BackendSource:
		if strings.TrimSpace(tool.SourceDir) == "" && strings.TrimSpace(tool.BuildCmd) == "" {
			return fmt.Errorf("tool %q: source backend requires source_dir or build_cmd", tool.ID)
		}
	case domain.BackendVendor:
		// Checked via effective commands below: vendor has no derived
		// commands, so all three must be explicit.
	}

	if tool.InstallCommand() == "" {
		return fmt.Errorf("tool %q: missing effective install command (set install_cmd)", tool.ID)
	}
	if tool.UpdateCommand() == "" {
		return fmt.Errorf("tool %q: missing effective update command (set update_cmd)", tool.ID)
	}
	if tool.RemoveCommand() == "" {
		return fmt.Errorf("tool %q: missing effective remove command (set remove_cmd)", tool.ID)
	}
	return nil
}
