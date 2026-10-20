// Package adapter translates backend-agnostic plan steps into concrete command
// specifications for mise, apt, vendor, and local source builds. It performs no
// process execution itself; the engine runner consumes the returned specs so
// every side effect stays observable and testable.
package adapter

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/juanrubio/outfitter/internal/domain"
)

// CommandSpec describes one command invocation for a plan step. CustomFunc is
// used when a step is executed natively in-process (for example the verified
// mise bootstrap or the CodexBar bundle installer).
type CommandSpec struct {
	Program    string
	Args       []string
	Dir        string
	Env        []string
	Display    string
	CustomFunc func(ctx context.Context, onLine func(string)) error
}

// Options configures an Adapter.
type Options struct {
	// HomeDir expands "~/" paths and locates per-user binaries.
	HomeDir string
	// LookPath resolves mise on PATH. Nil uses exec.LookPath.
	LookPath func(string) (string, error)
	// CodexBar overrides the native CodexBar release downloader.
	CodexBar CodexBarOptions
}

// Adapter builds command specifications for one execution run. It remembers
// which mise runtime prerequisites have already been queued during the current
// run so node and uv are ensured once rather than once per tool.
type Adapter struct {
	homeDir  string
	lookPath func(string) (string, error)
	codexBar CodexBarOptions

	mu      sync.Mutex
	ensured map[string]bool
}

// New returns an adapter with production defaults.
func New(opts Options) *Adapter {
	lookPath := opts.LookPath
	if lookPath == nil {
		lookPath = exec.LookPath
	}
	homeDir := opts.HomeDir
	if homeDir == "" {
		if home, err := os.UserHomeDir(); err == nil {
			homeDir = home
		}
	}

	codexBar := opts.CodexBar
	if codexBar.HomeDir == "" {
		codexBar.HomeDir = homeDir
	}

	return &Adapter{
		homeDir:  homeDir,
		lookPath: lookPath,
		codexBar: codexBar,
		ensured:  make(map[string]bool),
	}
}

// Reset clears the per-run runtime prerequisites.
func (a *Adapter) Reset() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.ensured = make(map[string]bool)
}

// Commands returns the ordered command specifications for a plan step. Runtime
// prerequisites are prepended for mise:npm and uv installs and updates.
func (a *Adapter) Commands(step domain.PlanStep) ([]CommandSpec, error) {
	backend := step.Tool.Backend
	if !backend.IsValid() {
		return nil, fmt.Errorf("tool %q: unsupported backend %q", step.Tool.ID, backend)
	}
	if !validAction(step.Action) {
		return nil, fmt.Errorf("tool %q: unsupported action %q", step.Tool.ID, step.Action)
	}
	if step.Blocked != "" {
		return nil, fmt.Errorf("tool %q: %s", step.Tool.ID, step.Blocked)
	}
	if step.RemovePath != "" {
		command := step.Command
		if command == "" {
			command = domain.RemovalCommand(step.RemovePath)
		}
		return []CommandSpec{shellSpec(command)}, nil
	}

	switch backend {
	case domain.BackendMiseAqua, domain.BackendMiseUbi, domain.BackendMiseGitHub,
		domain.BackendMiseNPM, domain.BackendUV:
		return a.miseCommands(step)
	case domain.BackendAPT:
		return a.aptCommands(step)
	case domain.BackendVendor:
		return a.vendorCommands(step)
	case domain.BackendSource:
		return a.sourceCommands(step)
	default:
		return nil, fmt.Errorf("tool %q: unsupported backend %q", step.Tool.ID, backend)
	}
}

func validAction(action domain.DesiredAction) bool {
	switch action {
	case domain.ActionInstall, domain.ActionUpdate, domain.ActionRemove:
		return true
	default:
		return false
	}
}

func (a *Adapter) expandHome(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || a.homeDir == "" {
		return value
	}
	if value == "~" {
		return a.homeDir
	}
	if strings.HasPrefix(value, "~/") {
		return filepath.Join(a.homeDir, strings.TrimPrefix(value, "~/"))
	}
	return value
}

func (a *Adapter) ensureOnce(key string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.ensured[key] {
		return false
	}
	a.ensured[key] = true
	return true
}

func displayCommand(program string, args ...string) string {
	if len(args) == 0 {
		return program
	}
	return program + " " + strings.Join(args, " ")
}

func shellSpec(command string) CommandSpec {
	command = strings.TrimSpace(command)
	if command == "" {
		return CommandSpec{}
	}
	return CommandSpec{
		Program: "bash",
		Args:    []string{"-o", "pipefail", "-c", command},
		Display: command,
	}
}

var errNoSourceDir = errors.New("source backend requires source_dir or a custom build command")
