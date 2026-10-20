package engine

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/juanrubio/outfitter/internal/adapter"
	"github.com/juanrubio/outfitter/internal/domain"
)

func vendorPlan(tools ...domain.Tool) domain.ActionPlan {
	steps := make([]domain.PlanStep, 0, len(tools))
	for _, tool := range tools {
		steps = append(steps, domain.PlanStep{
			Tool:    tool,
			Action:  domain.ActionInstall,
			Command: tool.InstallCommand(),
		})
	}
	return domain.ActionPlan{Installations: steps}
}

func vendorTool(id, command string) domain.Tool {
	return domain.Tool{
		ID:         id,
		Name:       id,
		Binary:     id,
		Backend:    domain.BackendVendor,
		InstallCmd: command,
	}
}

func TestRunnerStreamsOutputAndWritesStructuredLog(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	logDir := t.TempDir()
	runner := NewRunner(RunnerOptions{HomeDir: home, LogDir: logDir})
	tool := vendorTool("demo", "printf 'stdout-line\\n'; printf 'stderr-line\\n' >&2")

	var events []StepEvent
	result := runner.Run(context.Background(), vendorPlan(tool), func(event StepEvent) {
		events = append(events, event)
	})
	if result.HasFailures() {
		t.Fatalf("plan failed: %+v", result.Steps)
	}
	if result.Succeeded() != 1 || result.Failed() != 0 {
		t.Fatalf("succeeded=%d failed=%d", result.Succeeded(), result.Failed())
	}

	output := strings.Join(result.Steps[0].Output, "\n")
	for _, expected := range []string{"stdout-line", "stderr-line"} {
		if !strings.Contains(output, expected) {
			t.Fatalf("output %q is missing %q", output, expected)
		}
	}

	var streamed []string
	running := 0
	for _, event := range events {
		if event.Status == string(StepStatusRunning) {
			running++
		}
		if event.Line != "" {
			streamed = append(streamed, event.Line)
		}
	}
	if running < 2 {
		t.Fatalf("running events = %d, want start plus streamed lines", running)
	}
	streamedText := strings.Join(streamed, "\n")
	if !strings.Contains(streamedText, "stdout-line") || !strings.Contains(streamedText, "stderr-line") {
		t.Fatalf("streamed events = %v", streamed)
	}

	if result.LogPath == "" {
		t.Fatal("PlanResult.LogPath is empty")
	}
	if !strings.HasPrefix(result.LogPath, logDir+string(filepath.Separator)) {
		t.Fatalf("log path = %q, want under %q", result.LogPath, logDir)
	}
	if info, err := os.Stat(result.LogPath); err != nil {
		t.Fatalf("stat log: %v", err)
	} else if info.Mode().Perm() != 0o644 {
		t.Fatalf("log mode = %o, want 0644", info.Mode().Perm())
	}
	matches, err := filepath.Glob(filepath.Join(logDir, "run-*.log"))
	if err != nil || len(matches) != 1 {
		t.Fatalf("run logs = %v, err=%v", matches, err)
	}
	logged, err := os.ReadFile(result.LogPath)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	logText := string(logged)
	for _, expected := range []string{
		"# Outfitter execution log", "tool=demo", "stdout-line", "stderr-line",
		"status=done", "finished_at=",
	} {
		if !strings.Contains(logText, expected) {
			t.Errorf("log is missing %q:\n%s", expected, logText)
		}
	}
}

// TestRunnerRemoveStraysOnlyForSucceededSteps verifies the opt-in stray
// phase: strays of succeeded steps are deleted and reported, strays of
// failed steps are left alone.
func TestRunnerRemoveStraysOnlyForSucceededSteps(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	strayOK := filepath.Join(home, "old-lazygit")
	strayFail := filepath.Join(home, "old-fzf")
	writeTestFile(t, strayOK, "binary", 0o755)
	writeTestFile(t, strayFail, "binary", 0o755)

	executor := CommandExecutorFunc(func(
		_ context.Context, spec adapter.CommandSpec, _ func(string),
	) (int, error) {
		if strings.Contains(spec.Display, "fail-tool") {
			return 1, errors.New("boom")
		}
		return 0, nil
	})
	runner := NewRunner(RunnerOptions{
		HomeDir: home, LogDir: t.TempDir(), CommandExecutor: executor,
		LookPath: func(string) (string, error) { return "/usr/bin/mise", nil },
	})

	okStep := domain.PlanStep{
		Tool: vendorTool("lazygit", "true"), Action: domain.ActionUpdate,
		Command: "true", Migrate: true, StrayBinaryPath: strayOK,
	}
	failStep := domain.PlanStep{
		Tool: vendorTool("fail-tool", "fail-tool"), Action: domain.ActionUpdate,
		Command: "fail-tool", Migrate: true, StrayBinaryPath: strayFail,
	}
	plan := domain.ActionPlan{
		Updates:      []domain.PlanStep{okStep, failStep},
		RemoveStrays: true,
	}

	var events []StepEvent
	result := runner.Run(context.Background(), plan, func(event StepEvent) {
		events = append(events, event)
	})
	// The stray phase runs after the step finished: its line events must
	// repeat the terminal status, never resurrect the step as running.
	seenDone := false
	strayLineStatus := ""
	for _, event := range events {
		if event.StepIndex != 0 {
			continue
		}
		if event.Status == string(StepStatusDone) && event.Line == "" {
			seenDone = true
			continue
		}
		if seenDone && event.Status == string(StepStatusRunning) {
			t.Fatalf("step 0 emitted running event after done: %+v", event)
		}
		if strings.Contains(event.Line, "Removed stray binary") {
			strayLineStatus = event.Status
		}
	}
	if strayLineStatus != string(StepStatusDone) {
		t.Fatalf("stray line event status = %q, want done", strayLineStatus)
	}
	if len(result.Strays) != 1 {
		t.Fatalf("stray results = %+v, want exactly the succeeded step's stray", result.Strays)
	}
	if !result.Strays[0].Removed || result.Strays[0].Path != strayOK || result.Strays[0].ToolID != "lazygit" {
		t.Fatalf("stray result = %+v", result.Strays[0])
	}
	if _, err := os.Stat(strayOK); !os.IsNotExist(err) {
		t.Errorf("succeeded step's stray still exists (stat err = %v)", err)
	}
	if _, err := os.Stat(strayFail); err != nil {
		t.Errorf("failed step's stray must remain: %v", err)
	}

	// Without the opt-in, strays are never touched.
	writeTestFile(t, strayOK, "binary", 0o755)
	plan.RemoveStrays = false
	result = runner.Run(context.Background(), plan, nil)
	if len(result.Strays) != 0 {
		t.Fatalf("strays without opt-in = %+v, want none", result.Strays)
	}
	if _, err := os.Stat(strayOK); err != nil {
		t.Errorf("stray removed without opt-in: %v", err)
	}
}

func TestRunnerIsolatesPerToolFailures(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	logDir := t.TempDir()
	var executed []string
	executor := CommandExecutorFunc(func(
		_ context.Context,
		spec adapter.CommandSpec,
		onLine func(string),
	) (int, error) {
		executed = append(executed, spec.Display)
		onLine("running " + spec.Display)
		if strings.Contains(spec.Display, "failing-tool") {
			return 7, errors.New("exit status 7")
		}
		return 0, nil
	})
	runner := NewRunner(RunnerOptions{
		HomeDir:         home,
		LogDir:          logDir,
		CommandExecutor: executor,
	})
	plan := vendorPlan(vendorTool("failing", "failing-tool"), vendorTool("working", "working-tool"))

	var failedEvent *StepEvent
	result := runner.Run(context.Background(), plan, func(event StepEvent) {
		if event.Status == string(StepStatusFailed) {
			copied := event
			failedEvent = &copied
		}
	})

	if result.Succeeded() != 1 || result.Failed() != 1 {
		t.Fatalf("succeeded=%d failed=%d steps=%+v", result.Succeeded(), result.Failed(), result.Steps)
	}
	if len(result.Steps) != 2 {
		t.Fatalf("steps = %d, want 2", len(result.Steps))
	}
	if result.Steps[0].Success || result.Steps[0].ExitCode != 7 {
		t.Fatalf("first step = %+v, want failure with exit code 7", result.Steps[0])
	}
	if !result.Steps[1].Success || result.Steps[1].ExitCode != 0 {
		t.Fatalf("second step = %+v, want success", result.Steps[1])
	}
	if len(executed) != 2 || executed[0] != "failing-tool" || executed[1] != "working-tool" {
		t.Fatalf("executed = %v, want both tools in order", executed)
	}
	if failedEvent == nil || failedEvent.ExitCode != 7 || failedEvent.Err == nil {
		t.Fatalf("failed event = %+v", failedEvent)
	}

	logged, err := os.ReadFile(result.LogPath)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	logText := string(logged)
	for _, expected := range []string{"exit_code=7", "working-tool", "failed=1"} {
		if !strings.Contains(logText, expected) {
			t.Errorf("log is missing %q:\n%s", expected, logText)
		}
	}
}

func TestRunnerFailsDependentMiseStepsAfterBootstrapFailure(t *testing.T) {
	t.Parallel()
	var executed []string
	executor := CommandExecutorFunc(func(
		_ context.Context,
		spec adapter.CommandSpec,
		_ func(string),
	) (int, error) {
		executed = append(executed, spec.Display)
		return 0, nil
	})
	runner := NewRunner(RunnerOptions{
		HomeDir:         t.TempDir(),
		LogDir:          t.TempDir(),
		CommandExecutor: executor,
		Bootstrap: func(context.Context, BootstrapOptions) (string, error) {
			return "", errors.New("release download unavailable")
		},
	})

	aptTool := domain.Tool{
		ID: "tmux", Name: "tmux", Binary: "tmux", Backend: domain.BackendAPT,
		Package: "tmux", Sudo: true,
	}
	miseTool := domain.Tool{
		ID: "fzf", Name: "fzf", Binary: "fzf", Backend: domain.BackendMiseAqua,
		Package: "junegunn/fzf",
	}
	plan := domain.ActionPlan{
		BootstrapStep: &domain.PlanStep{
			Tool:    domain.Tool{ID: "mise", Name: "mise", Binary: "mise"},
			Action:  domain.ActionInstall,
			Command: domain.BootstrapMiseCommand,
		},
		Installations: []domain.PlanStep{
			{Tool: aptTool, Action: domain.ActionInstall, Command: aptTool.InstallCommand()},
			{Tool: miseTool, Action: domain.ActionInstall, Command: miseTool.InstallCommand()},
		},
	}

	result := runner.Run(context.Background(), plan, nil)
	if len(result.Steps) != 3 {
		t.Fatalf("steps = %d, want bootstrap + apt + mise", len(result.Steps))
	}
	if result.Steps[0].Success || !strings.Contains(result.Steps[0].Err.Error(), "bootstrap mise") {
		t.Fatalf("bootstrap step = %+v", result.Steps[0])
	}
	if !result.Steps[1].Success {
		t.Fatalf("independent apt step should succeed: %+v", result.Steps[1])
	}
	if result.Steps[2].Success || !strings.Contains(result.Steps[2].Err.Error(), "requires bootstrap") {
		t.Fatalf("dependent mise step = %+v", result.Steps[2])
	}
	if len(executed) != 1 || !strings.Contains(executed[0], "apt-get") {
		t.Fatalf("executed = %v, want only apt", executed)
	}
}

func TestProcessExecutorCancelsProcessGroup(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("process-group cleanup is POSIX-specific")
	}
	t.Parallel()
	home := t.TempDir()
	runner := NewRunner(RunnerOptions{HomeDir: home, LogDir: t.TempDir()})
	tool := vendorTool("group", "sleep 30 & echo $!; wait")
	plan := vendorPlan(tool)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pidCh := make(chan int, 1)
	resultCh := make(chan PlanResult, 1)
	started := time.Now()
	go func() {
		resultCh <- runner.Run(ctx, plan, func(event StepEvent) {
			if event.Line == "" {
				return
			}
			pid, err := strconv.Atoi(strings.TrimSpace(event.Line))
			if err != nil {
				return
			}
			select {
			case pidCh <- pid:
			default:
			}
		})
	}()

	var grandchild int
	select {
	case grandchild = <-pidCh:
	case <-time.After(10 * time.Second):
		cancel()
		t.Fatal("timed out waiting for the grandchild PID")
	}

	cancel()
	var result PlanResult
	select {
	case result = <-resultCh:
	case <-time.After(8 * time.Second):
		t.Fatal("runner did not return after context cancellation")
	}
	if elapsed := time.Since(started); elapsed > 8*time.Second {
		t.Fatalf("cancellation took %s", elapsed)
	}
	if !result.HasFailures() {
		t.Fatalf("cancelled run should record a failure: %+v", result.Steps)
	}

	deadline := time.Now().Add(3 * time.Second)
	for {
		err := syscall.Kill(grandchild, 0)
		if errors.Is(err, syscall.ESRCH) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("grandchild process %d is still alive (kill error: %v)", grandchild, err)
		}
		time.Sleep(25 * time.Millisecond)
	}
}

func TestPlanResultHelpers(t *testing.T) {
	t.Parallel()
	if (PlanResult{}).HasFailures() {
		t.Fatal("empty result should not report failures")
	}
	result := PlanResult{Steps: []StepResult{
		{Success: true},
		{Success: false},
		{Success: true},
	}}
	if result.Succeeded() != 2 || result.Failed() != 1 || !result.HasFailures() {
		t.Fatalf("helpers = (%d, %d, %v)", result.Succeeded(), result.Failed(), result.HasFailures())
	}
}

func TestProcessExecutorPrependsMiseEnvironment(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	executor := &ProcessExecutor{
		HomeDir: home,
		BaseEnv: []string{"PATH=/usr/bin:/bin", "HOME=" + home},
	}
	var lines []string
	exitCode, err := executor.Execute(context.Background(), adapter.CommandSpec{
		Program: "sh",
		Args:    []string{"-c", `printf '%s' "$PATH"`},
		Display: "sh -c path",
	}, func(line string) {
		lines = append(lines, line)
	})
	if err != nil || exitCode != 0 {
		t.Fatalf("Execute: exit=%d err=%v", exitCode, err)
	}
	expected := strings.Join([]string{
		filepath.Join(home, ".local", "bin"),
		filepath.Join(home, ".local", "share", "mise", "shims"),
		"/usr/bin:/bin",
	}, ":")
	if got := strings.Join(lines, ""); got != expected {
		t.Fatalf("PATH = %q, want %q", got, expected)
	}
}

func TestMergeEnvReplacesKeys(t *testing.T) {
	t.Parallel()
	merged := mergeEnv([]string{"PATH=/bin", "HOME=/home/test"}, []string{"PATH=/custom", "EXTRA=1"})
	joined := strings.Join(merged, "\n")
	if strings.Count(joined, "PATH=") != 1 || !strings.Contains(joined, "PATH=/custom") {
		t.Fatalf("merged environment = %v", merged)
	}
	if !strings.Contains(joined, "EXTRA=1") || !strings.Contains(joined, "HOME=/home/test") {
		t.Fatalf("merged environment = %v", merged)
	}
}
