package engine

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/juanrubio/outfitter/internal/adapter"
	"github.com/juanrubio/outfitter/internal/domain"
)

// StepStatus is the lifecycle state of one executed plan step.
type StepStatus string

// Supported execution step states.
const (
	StepStatusRunning StepStatus = "running"
	StepStatusDone    StepStatus = "done"
	StepStatusFailed  StepStatus = "failed"
)

// StepEvent is emitted while a plan is executing. Line events carry one
// streamed stdout/stderr line and repeat the step's current status —
// the terminal status for post-step lines such as stray removals.
type StepEvent struct {
	StepIndex  int
	TotalSteps int
	Step       domain.PlanStep
	Status     string
	Line       string
	Duration   time.Duration
	Err        error
	ExitCode   int
}

// StepResult is the recorded outcome of one plan step.
type StepResult struct {
	Step     domain.PlanStep
	Success  bool
	ExitCode int
	Duration time.Duration
	Output   []string
	Err      error
}

// StrayResult is the outcome of one stray-binary removal.
type StrayResult struct {
	ToolID  string
	Path    string
	Removed bool
	Err     error
}

// PlanResult is the complete outcome of one execution run.
type PlanResult struct {
	StartedAt  time.Time
	FinishedAt time.Time
	Duration   time.Duration
	Steps      []StepResult
	Strays     []StrayResult
	LogPath    string
}

// Succeeded returns the number of successful steps.
func (r PlanResult) Succeeded() int {
	count := 0
	for _, step := range r.Steps {
		if step.Success {
			count++
		}
	}
	return count
}

// Failed returns the number of failed steps.
func (r PlanResult) Failed() int {
	count := 0
	for _, step := range r.Steps {
		if !step.Success {
			count++
		}
	}
	return count
}

// HasFailures reports whether any step failed.
func (r PlanResult) HasFailures() bool {
	return r.Failed() > 0
}

// CommandExecutor runs one command specification and streams its combined
// output line by line. Tests can inject a fake executor to avoid subprocesses.
type CommandExecutor interface {
	Execute(ctx context.Context, spec adapter.CommandSpec, onLine func(string)) (int, error)
}

// CommandExecutorFunc adapts a function to CommandExecutor.
type CommandExecutorFunc func(ctx context.Context, spec adapter.CommandSpec, onLine func(string)) (int, error)

// Execute implements CommandExecutor.
func (f CommandExecutorFunc) Execute(
	ctx context.Context,
	spec adapter.CommandSpec,
	onLine func(string),
) (int, error) {
	return f(ctx, spec, onLine)
}

// RunnerOptions configures a Runner. Zero values select production defaults.
type RunnerOptions struct {
	HomeDir         string
	LogDir          string
	BootstrapOpts   BootstrapOptions
	CodexBarOpts    adapter.CodexBarOptions
	CommandExecutor CommandExecutor
	LookPath        func(string) (string, error)
	Bootstrap       func(ctx context.Context, opts BootstrapOptions) (string, error)
}

// Runner executes an ActionPlan with per-tool failure isolation, process-group
// cancellation, live line streaming, and structured logs.
type Runner struct {
	opts RunnerOptions
}

// NewRunner returns a Runner with production defaults and the supplied options.
func NewRunner(opts RunnerOptions) *Runner {
	if opts.HomeDir == "" {
		if home, err := os.UserHomeDir(); err == nil {
			opts.HomeDir = home
		}
	}
	if opts.HomeDir != "" {
		if opts.LogDir == "" {
			opts.LogDir = filepath.Join(opts.HomeDir, ".local", "state", "outfitter", "logs")
		}
		if opts.BootstrapOpts.HomeDir == "" {
			opts.BootstrapOpts.HomeDir = opts.HomeDir
		}
		if opts.CodexBarOpts.HomeDir == "" {
			opts.CodexBarOpts.HomeDir = opts.HomeDir
		}
	}
	if opts.CommandExecutor == nil {
		opts.CommandExecutor = &ProcessExecutor{HomeDir: opts.HomeDir}
	}
	if opts.Bootstrap == nil {
		opts.Bootstrap = BootstrapMise
	}
	return &Runner{opts: opts}
}

// Run executes every step in the plan in order. A failed tool does not stop
// independent tools; a failed mise bootstrap fails dependent mise-backed steps
// fast while apt, vendor, and source steps continue.
func (r *Runner) Run(
	ctx context.Context,
	plan domain.ActionPlan,
	onEvent func(StepEvent),
) PlanResult {
	started := time.Now()
	result := PlanResult{StartedAt: started}
	logger, _ := newRunLogger(r.opts.LogDir, started)
	if logger != nil {
		defer logger.Close()
		result.LogPath = logger.Path()
	}

	steps := plan.Steps()
	result.Steps = make([]StepResult, 0, len(steps))
	if logger != nil {
		logger.Line("started_at=%s steps=%d", started.Format(time.RFC3339Nano), len(steps))
	}

	commands := adapter.New(adapter.Options{
		HomeDir:  r.opts.HomeDir,
		LookPath: r.opts.LookPath,
		CodexBar: r.opts.CodexBarOpts,
	})
	miseReady := plan.BootstrapStep == nil
	var bootstrapErr error

	for index, step := range steps {
		if ctx.Err() != nil {
			break
		}

		stepStarted := time.Now()
		emitStepEvent(onEvent, StepEvent{
			StepIndex:  index,
			TotalSteps: len(steps),
			Step:       step,
			Status:     string(StepStatusRunning),
		})
		if logger != nil {
			logger.Line("step=%d/%d tool=%s action=%s status=%s",
				index+1, len(steps), step.Tool.ID, step.Action, StepStatusRunning)
		}

		var recorded StepResult
		switch {
		case step.Command == domain.BootstrapMiseCommand:
			recorded = r.runBootstrap(ctx, index, len(steps), step, onEvent, logger)
			miseReady = recorded.Success
			if !recorded.Success {
				bootstrapErr = recorded.Err
			}
		case step.Tool.Backend.RequiresMise() && !miseReady:
			err := dependencyError(bootstrapErr)
			recorded = StepResult{Step: step, Err: err, ExitCode: 1}
			if logger != nil {
				logger.Line("error: %v", err)
			}
		default:
			recorded = r.runToolStep(ctx, index, len(steps), step, commands, onEvent, logger)
		}

		recorded.Duration = time.Since(stepStarted)
		result.Steps = append(result.Steps, recorded)

		status := StepStatusDone
		if !recorded.Success {
			status = StepStatusFailed
		}
		emitStepEvent(onEvent, StepEvent{
			StepIndex:  index,
			TotalSteps: len(steps),
			Step:       step,
			Status:     string(status),
			Duration:   recorded.Duration,
			Err:        recorded.Err,
			ExitCode:   recorded.ExitCode,
		})
		if logger != nil {
			logger.Line("step=%d/%d tool=%s status=%s duration=%s exit_code=%d",
				index+1, len(steps), step.Tool.ID, status,
				recorded.Duration.Round(time.Millisecond), recorded.ExitCode)
		}
		if ctx.Err() != nil {
			break
		}
	}

	if plan.RemoveStrays && ctx.Err() == nil {
		result.Strays = r.removeStrays(result.Steps, len(steps), onEvent, logger)
	}

	result.FinishedAt = time.Now()
	result.Duration = result.FinishedAt.Sub(result.StartedAt)
	if logger != nil {
		logger.Line("finished_at=%s duration=%s succeeded=%d failed=%d",
			result.FinishedAt.Format(time.RFC3339Nano),
			result.Duration.Round(time.Millisecond), result.Succeeded(), result.Failed())
	}
	return result
}

// removeStrays deletes the stray binaries of succeeded steps. A failed
// step keeps its stray: it may be the only working copy left.
func (r *Runner) removeStrays(
	steps []StepResult, total int, onEvent func(StepEvent), logger *runLogger,
) []StrayResult {
	type pending struct {
		index    int
		step     domain.PlanStep
		duration time.Duration
	}
	var pendingStrays []pending
	var paths []string
	for index, recorded := range steps {
		if recorded.Success && recorded.Step.StrayBinaryPath != "" {
			pendingStrays = append(pendingStrays, pending{index, recorded.Step, recorded.Duration})
			paths = append(paths, recorded.Step.StrayBinaryPath)
		}
	}
	if len(paths) == 0 {
		return nil
	}

	removed, removeErr := RemoveShadowedBinaries(paths)
	gone := make(map[string]bool, len(removed))
	for _, path := range removed {
		gone[path] = true
	}

	results := make([]StrayResult, 0, len(pendingStrays))
	for _, stray := range pendingStrays {
		outcome := StrayResult{
			ToolID:  stray.step.Tool.ID,
			Path:    stray.step.StrayBinaryPath,
			Removed: gone[stray.step.StrayBinaryPath],
		}
		var line string
		if outcome.Removed {
			line = "Removed stray binary " + outcome.Path
		} else {
			outcome.Err = removeErr
			line = fmt.Sprintf("Failed to remove stray binary %s: %v", outcome.Path, removeErr)
		}
		results = append(results, outcome)
		// The step already finished: repeat its terminal status and
		// duration so consumers never resurrect it as running.
		emitStepEvent(onEvent, StepEvent{
			StepIndex:  stray.index,
			TotalSteps: total,
			Step:       stray.step,
			Status:     string(StepStatusDone),
			Line:       line,
			Duration:   stray.duration,
			Err:        outcome.Err,
		})
		if logger != nil {
			logger.Line("stray tool=%s path=%s removed=%v", outcome.ToolID, outcome.Path, outcome.Removed)
		}
	}
	return results
}

func (r *Runner) runBootstrap(
	ctx context.Context,
	index, total int,
	step domain.PlanStep,
	onEvent func(StepEvent),
	logger *runLogger,
) StepResult {
	result := StepResult{Step: step}
	if logger != nil {
		logger.Line("$ %s", step.Command)
	}

	opts := r.opts.BootstrapOpts
	if opts.HomeDir == "" {
		opts.HomeDir = r.opts.HomeDir
	}
	path, err := r.opts.Bootstrap(ctx, opts)
	if err != nil {
		result.Err = fmt.Errorf("bootstrap mise: %w", err)
		result.ExitCode = 1
		if logger != nil {
			logger.Line("error: %v", result.Err)
		}
		return result
	}

	line := "mise installed at " + path
	result.Output = append(result.Output, line)
	result.Success = true
	emitLineEvent(onEvent, index, total, step, line)
	if logger != nil {
		logger.Line("| %s", line)
	}
	return result
}

func (r *Runner) runToolStep(
	ctx context.Context,
	index, total int,
	step domain.PlanStep,
	commands *adapter.Adapter,
	onEvent func(StepEvent),
	logger *runLogger,
) StepResult {
	result := StepResult{Step: step}

	specs, err := commands.Commands(step)
	if err != nil {
		result.Err = err
		result.ExitCode = 1
		if logger != nil {
			logger.Line("error: %v", err)
		}
		return result
	}
	if len(specs) == 0 {
		result.Err = fmt.Errorf("tool %q: adapter returned no commands", step.Tool.ID)
		result.ExitCode = 1
		if logger != nil {
			logger.Line("error: %v", result.Err)
		}
		return result
	}

	lineCallback := func(line string) {
		result.Output = append(result.Output, line)
		emitLineEvent(onEvent, index, total, step, line)
		if logger != nil {
			logger.Line("| %s", line)
		}
	}

	for _, spec := range specs {
		if logger != nil {
			logger.Line("$ %s", spec.Display)
		}
		exitCode, runErr := r.executeSpec(ctx, spec, lineCallback)
		result.ExitCode = exitCode
		if runErr == nil && exitCode != 0 {
			runErr = fmt.Errorf("%s exited with code %d", spec.Display, exitCode)
		}
		if runErr != nil {
			result.Err = runErr
			if logger != nil {
				logger.Line("error: %v", runErr)
			}
			return result
		}
		if ctx.Err() != nil {
			result.Err = ctx.Err()
			result.ExitCode = -1
			if logger != nil {
				logger.Line("error: %v", result.Err)
			}
			return result
		}
	}

	result.Success = true
	return result
}

func (r *Runner) executeSpec(
	ctx context.Context,
	spec adapter.CommandSpec,
	onLine func(string),
) (int, error) {
	if spec.CustomFunc != nil {
		if err := spec.CustomFunc(ctx, onLine); err != nil {
			return 1, err
		}
		return 0, nil
	}
	return r.opts.CommandExecutor.Execute(ctx, spec, onLine)
}

func dependencyError(cause error) error {
	if cause == nil {
		cause = errors.New("mise bootstrap did not complete")
	}
	return fmt.Errorf("skipped: dependent mise step requires bootstrap: %w", cause)
}

func emitStepEvent(onEvent func(StepEvent), event StepEvent) {
	if onEvent != nil {
		onEvent(event)
	}
}

func emitLineEvent(
	onEvent func(StepEvent),
	index, total int,
	step domain.PlanStep,
	line string,
) {
	emitStepEvent(onEvent, StepEvent{
		StepIndex:  index,
		TotalSteps: total,
		Step:       step,
		Status:     string(StepStatusRunning),
		Line:       line,
	})
}

// ProcessExecutor is the production command executor. Commands run in their
// own process group so context cancellation terminates children and
// grandchildren as well as the direct child.
type ProcessExecutor struct {
	HomeDir string
	BaseEnv []string
}

// Execute runs the command, streams combined stdout/stderr line by line, and
// returns its exit code.
func (e *ProcessExecutor) Execute(
	ctx context.Context,
	spec adapter.CommandSpec,
	onLine func(string),
) (int, error) {
	cmd := exec.CommandContext(ctx, spec.Program, spec.Args...)
	baseEnv := e.BaseEnv
	if baseEnv == nil {
		baseEnv = os.Environ()
	}
	cmd.Env = mergeEnv(MiseEnv(baseEnv, e.HomeDir), spec.Env)
	cmd.Dir = spec.Dir
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if errors.Is(err, syscall.ESRCH) {
			return nil
		}
		return err
	}
	cmd.WaitDelay = 5 * time.Second

	reader, writer := io.Pipe()
	cmd.Stdout = writer
	cmd.Stderr = writer

	if err := cmd.Start(); err != nil {
		_ = writer.Close()
		_ = reader.Close()
		if ctx.Err() != nil {
			return -1, fmt.Errorf("run %s: %w", spec.Display, ctx.Err())
		}
		return -1, fmt.Errorf("run %s: %w", spec.Display, err)
	}

	scanDone := make(chan struct{})
	go func() {
		defer close(scanDone)
		defer func() { _ = reader.Close() }()
		buffered := bufio.NewReader(reader)
		for {
			line, readErr := buffered.ReadString('\n')
			if len(line) > 0 {
				line = strings.TrimSuffix(line, "\n")
				line = strings.TrimSuffix(line, "\r")
				if onLine != nil {
					onLine(line)
				}
			}
			if readErr != nil {
				return
			}
		}
	}()

	waitErr := cmd.Wait()
	_ = writer.Close()
	<-scanDone

	exitCode := 0
	if waitErr != nil {
		exitCode = -1
		var exitErr *exec.ExitError
		if errors.As(waitErr, &exitErr) {
			exitCode = exitErr.ExitCode()
		}
		if ctx.Err() != nil {
			return exitCode, fmt.Errorf("run %s: %w", spec.Display, ctx.Err())
		}
		return exitCode, fmt.Errorf("run %s: %w", spec.Display, waitErr)
	}
	return exitCode, nil
}

func mergeEnv(base, extra []string) []string {
	if len(extra) == 0 {
		return append([]string(nil), base...)
	}
	merged := append([]string(nil), base...)
	for _, entry := range extra {
		key, _, ok := strings.Cut(entry, "=")
		if !ok || key == "" {
			merged = append(merged, entry)
			continue
		}
		replaced := false
		for index, existing := range merged {
			existingKey, _, hasValue := strings.Cut(existing, "=")
			if hasValue && existingKey == key {
				merged[index] = entry
				replaced = true
				break
			}
		}
		if !replaced {
			merged = append(merged, entry)
		}
	}
	return merged
}

type runLogger struct {
	file *os.File
	path string
	mu   sync.Mutex
}

func newRunLogger(logDir string, started time.Time) (*runLogger, error) {
	if strings.TrimSpace(logDir) == "" {
		return nil, errors.New("log directory is empty")
	}
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return nil, fmt.Errorf("create log directory: %w", err)
	}

	base := "run-" + started.Format("20060102-150405")
	for attempt := 0; attempt < 1000; attempt++ {
		name := base + ".log"
		if attempt > 0 {
			name = fmt.Sprintf("%s-%d.log", base, attempt)
		}
		path := filepath.Join(logDir, name)
		file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if errors.Is(err, os.ErrExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("create run log: %w", err)
		}
		if err := file.Chmod(0o644); err != nil {
			_ = file.Close()
			_ = os.Remove(path)
			return nil, fmt.Errorf("set run log permissions: %w", err)
		}
		logger := &runLogger{file: file, path: path}
		logger.Line("# Outfitter execution log")
		return logger, nil
	}
	return nil, errors.New("could not allocate a unique run log file")
}

// Path returns the log file path.
func (l *runLogger) Path() string {
	if l == nil {
		return ""
	}
	return l.path
}

// Line writes one timestamped log line.
func (l *runLogger) Line(format string, args ...any) {
	if l == nil || l.file == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	_, _ = fmt.Fprintf(l.file, "[%s] ", time.Now().Format(time.RFC3339Nano))
	_, _ = fmt.Fprintf(l.file, format, args...)
	_, _ = fmt.Fprintln(l.file)
}

// Close flushes and closes the log file.
func (l *runLogger) Close() {
	if l == nil || l.file == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	_ = l.file.Sync()
	_ = l.file.Close()
	l.file = nil
}
