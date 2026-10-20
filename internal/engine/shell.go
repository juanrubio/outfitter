package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ShellKind identifies a login shell family Outfitter understands.
type ShellKind string

// Supported shell families. Only bash and zsh can be fixed automatically;
// fish users get the manual line instead.
const (
	ShellBash  ShellKind = "bash"
	ShellZsh   ShellKind = "zsh"
	ShellFish  ShellKind = "fish"
	ShellOther ShellKind = "other"
)

// Markers delimiting the block Outfitter manages inside a shell rc file.
const (
	shellBlockStart = "# >>> outfitter mise >>>"
	shellBlockEnd   = "# <<< outfitter mise <<<"
)

// ShellIntegration describes whether mise-managed tools resolve in the
// user's interactive shell.
type ShellIntegration struct {
	Shell      ShellKind `json:"shell"`
	RCFile     string    `json:"rc_file,omitempty"`
	Integrated bool      `json:"integrated"`
	Fixable    bool      `json:"fixable"`
}

// ShellSetup detects and repairs shell integration for mise. File access
// uses the real filesystem under HomeDir; environment reads go through
// Getenv so tests stay host-independent.
type ShellSetup struct {
	HomeDir string
	Getenv  func(string) string
}

func (s *ShellSetup) getenv(key string) string {
	if s.Getenv != nil {
		return s.Getenv(key)
	}
	return os.Getenv(key)
}

func (s *ShellSetup) homeDir() string {
	if s.HomeDir != "" {
		return s.HomeDir
	}
	home, _ := os.UserHomeDir()
	return home
}

func (s *ShellSetup) shimsDir() string {
	return filepath.Join(s.homeDir(), ".local", "share", "mise", "shims")
}

func shellKind(name string) ShellKind {
	switch ShellKind(filepath.Base(strings.TrimSpace(name))) {
	case ShellBash:
		return ShellBash
	case ShellZsh:
		return ShellZsh
	case ShellFish:
		return ShellFish
	default:
		return ShellOther
	}
}

func (s *ShellSetup) rcFile(shell ShellKind) string {
	home := s.homeDir()
	switch shell {
	case ShellBash:
		return filepath.Join(home, ".bashrc")
	case ShellZsh:
		return filepath.Join(home, ".zshrc")
	case ShellFish:
		return filepath.Join(home, ".config", "fish", "config.fish")
	default:
		return ""
	}
}

// Detect reports the shell integration state. A non-empty shellOverride
// (a shell name like "zsh") replaces $SHELL detection.
func (s *ShellSetup) Detect(shellOverride string) ShellIntegration {
	shell := shellKind(shellOverride)
	if shell == ShellOther && strings.TrimSpace(shellOverride) == "" {
		shell = shellKind(s.getenv("SHELL"))
	}
	integration := ShellIntegration{
		Shell:   shell,
		RCFile:  s.rcFile(shell),
		Fixable: shell == ShellBash || shell == ShellZsh,
	}

	if s.getenv("MISE_SHELL") != "" {
		integration.Integrated = true
		return integration
	}
	for _, dir := range strings.Split(s.getenv("PATH"), ":") {
		if dir == s.shimsDir() {
			integration.Integrated = true
			return integration
		}
	}
	if integration.RCFile != "" {
		if data, err := os.ReadFile(integration.RCFile); err == nil && rcIntegratesMise(string(data)) {
			integration.Integrated = true
		}
	}
	return integration
}

func rcIntegratesMise(content string) bool {
	return strings.Contains(content, "mise activate") ||
		strings.Contains(content, "mise/shims") ||
		strings.Contains(content, shellBlockStart)
}

// IntegrationLine returns the rc line that integrates mise for a shell:
// the activate hook by default, or a shims PATH entry when shims is true.
func (s *ShellSetup) IntegrationLine(shell ShellKind, misePath string, shims bool) string {
	if strings.TrimSpace(misePath) == "" {
		misePath = "mise"
	}
	if shims {
		if shell == ShellFish {
			return "fish_add_path ~/.local/share/mise/shims"
		}
		return `export PATH="$HOME/.local/share/mise/shims:$PATH"`
	}
	switch shell {
	case ShellBash:
		return fmt.Sprintf("eval \"$(%s activate bash)\"", misePath)
	case ShellZsh:
		return fmt.Sprintf("eval \"$(%s activate zsh)\"", misePath)
	case ShellFish:
		return misePath + " activate fish | source"
	default:
		return ""
	}
}

// IntegrationBlock returns the marked rc block Outfitter manages.
func (s *ShellSetup) IntegrationBlock(shell ShellKind, misePath string, shims bool) []string {
	line := s.IntegrationLine(shell, misePath, shims)
	if line == "" {
		return nil
	}
	return []string{shellBlockStart, line, shellBlockEnd}
}

// Apply writes the integration block into the shell's rc file and reports
// whether the file changed. It never touches an rc that already
// integrates mise outside the managed block, replaces the managed block
// when the requested mechanism changes, and writes through symlinks
// without replacing them. Only bash and zsh are supported; other shells
// get an error carrying the manual line.
func (s *ShellSetup) Apply(shellOverride, misePath string, shims bool) (bool, error) {
	integration := s.Detect(shellOverride)
	if !integration.Fixable {
		line := s.IntegrationLine(integration.Shell, misePath, shims)
		if line == "" {
			return false, fmt.Errorf("automatic shell fix supports bash and zsh; shell %q is not supported", integration.Shell)
		}
		return false, fmt.Errorf("automatic shell fix supports bash and zsh; for %s add this line to %s manually: %s",
			integration.Shell, integration.RCFile, line)
	}

	block := s.IntegrationBlock(integration.Shell, misePath, shims)
	content := ""
	if data, err := os.ReadFile(integration.RCFile); err == nil {
		content = string(data)
	} else if !os.IsNotExist(err) {
		return false, err
	}

	updated, changed := spliceShellBlock(content, block)
	if !changed {
		return false, nil
	}
	if err := writeFileThroughSymlink(integration.RCFile, updated); err != nil {
		return false, err
	}
	return true, nil
}

// spliceShellBlock inserts or replaces the managed block in rc content.
// Content that already integrates mise outside the block is left alone.
func spliceShellBlock(content string, block []string) (string, bool) {
	lines := strings.Split(content, "\n")
	start, end := -1, -1
	for i, line := range lines {
		if strings.TrimSpace(line) == shellBlockStart {
			start = i
		}
		if strings.TrimSpace(line) == shellBlockEnd && start >= 0 {
			end = i
			break
		}
	}
	if start >= 0 && end > start {
		existing := lines[start : end+1]
		if strings.Join(existing, "\n") == strings.Join(block, "\n") {
			return content, false
		}
		merged := make([]string, 0, len(lines)+len(block))
		merged = append(merged, lines[:start]...)
		merged = append(merged, block...)
		merged = append(merged, lines[end+1:]...)
		return strings.Join(merged, "\n"), true
	}
	if rcIntegratesMise(content) {
		return content, false
	}
	trimmed := strings.TrimRight(content, "\n")
	var b strings.Builder
	if trimmed != "" {
		b.WriteString(trimmed)
		b.WriteString("\n\n")
	}
	b.WriteString(strings.Join(block, "\n"))
	b.WriteString("\n")
	return b.String(), true
}

// writeFileThroughSymlink truncates and rewrites path in place so a
// symlinked rc file keeps pointing at its target.
func writeFileThroughSymlink(path, content string) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644) //nolint:gosec // rc file path is derived from the user's home directory
	if err != nil {
		return err
	}
	if _, err := file.WriteString(content); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}
