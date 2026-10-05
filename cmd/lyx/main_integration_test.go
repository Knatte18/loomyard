//go:build integration

// main_integration_test.go holds the module-dispatcher tests that spawn
// gitkit.Git(t, cwd, "init") to seed a real git repo so lyxcwd.Resolve
// succeeds, so this file is integration-tagged per the Test Tier Purity
// Invariant.

package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
	"github.com/Knatte18/loomyard/internal/testkit/envelope"
	"github.com/Knatte18/loomyard/internal/testkit/lyxbin"
)

func TestRunDispatchesToBoard(t *testing.T) {
	t.Setenv("BOARD_SKIP_GIT", "1")
	// Create temp cwd with _lyx/config/board.yaml
	cwd := t.TempDir()

	// Initialize a git repo so lyxcwd.Resolve succeeds.
	gitkit.Git(t, cwd, "init")

	// The board config is the hub's: the hub is cwd's parent.
	boardDir := fabricengine.BoardDir(filepath.Dir(cwd))
	configDir := configengine.ConfigDir(boardDir)
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("failed to create hub board config dir: %v", err)
	}
	configPath := configengine.ConfigFile(boardDir, "board")
	// Write a template-complete board config. path: is no longer a template key
	// (the board data dir is paths-owned), so only readme/design_prefix remain.
	boardConfig := "readme: Home.md\ndesign_prefix: proposal-\n"
	if err := os.WriteFile(configPath, []byte(boardConfig), 0o644); err != nil {
		t.Fatalf("failed to write hub board.yaml: %v", err)
	}
	t.Chdir(cwd)

	var out bytes.Buffer
	code := run([]string{"board", "rerender"}, &out)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d; output: %s", code, out.String())
	}

	envelope.RequireOK(t, out.String())
}

func TestRunBoardErrorPropagatesExitCode(t *testing.T) {
	t.Setenv("BOARD_SKIP_GIT", "1")
	// Create temp cwd with _lyx/config/board.yaml
	cwd := t.TempDir()

	// Initialize a git repo so lyxcwd.Resolve succeeds.
	gitkit.Git(t, cwd, "init")

	// The board config is the hub's: the hub is cwd's parent.
	boardDir := fabricengine.BoardDir(filepath.Dir(cwd))
	configDir := configengine.ConfigDir(boardDir)
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("failed to create hub board config dir: %v", err)
	}
	configPath := configengine.ConfigFile(boardDir, "board")
	// Write a template-complete board config.
	boardConfig := "readme: Home.md\ndesign_prefix: proposal-\n"
	if err := os.WriteFile(configPath, []byte(boardConfig), 0o644); err != nil {
		t.Fatalf("failed to write hub board.yaml: %v", err)
	}
	t.Chdir(cwd)

	var out bytes.Buffer
	code := run([]string{"board", "remove", `{"slug":"nope"}`}, &out)
	if code != 1 {
		t.Fatalf("expected exit 1 from failing board command, got %d; output: %s", code, out.String())
	}
	if !strings.Contains(out.String(), `"ok":false`) {
		t.Fatalf("expected error JSON on out, got %q", out.String())
	}
}

// traceFilenamePattern matches the durable sink's trace-file naming: "trace-<UTC timestamp>-<TraceID>-<PID>.log".
var traceFilenamePattern = regexp.MustCompile(`^trace-\d{8}T\d{6}Z-[0-9a-f]{16}-\d+\.log$`)

// TestRootHookWritesTraceFileOnNonZeroExit verifies the root hook writes trace files on failure.
func TestRootHookWritesTraceFileOnNonZeroExit(t *testing.T) {
	lyxExe := lyxbin.Build(t)

	// Initialize a git repo so the spawned process's lyxcwd.Resolve succeeds.
	cwd := t.TempDir()
	gitkit.Git(t, cwd, "init")

	// Mark cwd as a worktree lyx owns (presence of the durable _lyx tree) so the
	// durable sink's cwd-anchored fallback is allowed to arm per isLyxWorktree
	// (internal/logger/sink.go) -- a plain git checkout without _lyx is exactly
	// the case R6-6 made that fallback refuse, on purpose.
	lyxDir := filepath.Join(cwd, lyxdirs.LyxDirName)
	if err := os.MkdirAll(lyxDir, 0o755); err != nil {
		t.Fatalf("failed to create _lyx: %v", err)
	}

	cmd := exec.Command(lyxExe, "bogus-subcommand")
	cmd.Dir = cwd
	out, runErr := cmd.CombinedOutput()

	var exitErr *exec.ExitError
	if runErr == nil {
		t.Fatalf("expected lyx bogus-subcommand to exit non-zero, got exit 0; output: %s", out)
	}
	if !errors.As(runErr, &exitErr) {
		t.Fatalf("expected an *exec.ExitError from lyx bogus-subcommand, got %v; output: %s", runErr, out)
	}
	if exitErr.ExitCode() == 0 {
		t.Fatalf("expected a non-zero exit code from lyx bogus-subcommand, got 0; output: %s", out)
	}

	logsDir := filepath.Join(cwd, ".lyx", "logs")
	entries, err := os.ReadDir(logsDir)
	if err != nil {
		t.Fatalf("ReadDir(%q): %v; lyx output: %s", logsDir, err, out)
	}

	var traceFile string
	for _, entry := range entries {
		if traceFilenamePattern.MatchString(entry.Name()) {
			traceFile = entry.Name()
			break
		}
	}
	if traceFile == "" {
		names := make([]string, len(entries))
		for i, entry := range entries {
			names[i] = entry.Name()
		}
		t.Fatalf("no file in %q matches %s; found: %v", logsDir, traceFilenamePattern, names)
	}

	content, err := os.ReadFile(filepath.Join(logsDir, traceFile))
	if err != nil {
		t.Fatalf("ReadFile(%q): %v", traceFile, err)
	}
	firstLine := strings.SplitN(string(content), "\n", 2)[0]
	if !strings.Contains(firstLine, "bogus-subcommand") {
		t.Errorf("trace file header line = %q; want it to name the spawned command %q", firstLine, "bogus-subcommand")
	}
	if !strings.Contains(firstLine, "trace=") {
		t.Errorf("trace file header line = %q; want a trace= field naming the trace ID", firstLine)
	}
}

func TestRunDispatchesToConfigReconcile(t *testing.T) {
	// Create temp cwd with git repo and _lyx/config to allow config reconcile to work.
	// configcli.RunCLI should recognize the subcommand and produce JSON output.
	cwd := t.TempDir()

	// Initialize git repo so lyxcwd.Resolve succeeds.
	gitkit.Git(t, cwd, "init")

	lyxDir := filepath.Join(cwd, lyxdirs.LyxDirName)
	if err := os.MkdirAll(lyxDir, 0o755); err != nil {
		t.Fatalf("failed to create _lyx: %v", err)
	}
	configDir := configengine.ConfigDir(cwd)
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("failed to create _lyx/config: %v", err)
	}
	t.Chdir(cwd)

	var out bytes.Buffer
	code := run([]string{"config", "reconcile"}, &out)
	if code != 0 {
		t.Fatalf("expected exit 0 for config reconcile, got %d; output: %s", code, out.String())
	}

	envelope.RequireOK(t, out.String())
}

// exitSweepFixture is a git repo carrying _lyx/ (so the cwd-anchored sink may arm) and a .lyx/logs directory pre-seeded with dead-pid traces.
type exitSweepFixture struct {
	cwd    string
	logs   string
	seeded []string // file names, newest mtime first
}

// newExitSweepFixture seeds four dead-pid traces whose mtimes are 1..4 days old, and writes logger.yaml when loggerYAML is non-empty.
func newExitSweepFixture(t *testing.T, loggerYAML string) exitSweepFixture {
	t.Helper()
	cwd := t.TempDir()
	gitkit.Git(t, cwd, "init")
	if err := os.MkdirAll(configengine.ConfigDir(cwd), 0o755); err != nil {
		t.Fatalf("failed to create _lyx/config: %v", err)
	}
	if loggerYAML != "" {
		if err := os.WriteFile(configengine.ConfigFile(cwd, "logger"), []byte(loggerYAML), 0o644); err != nil {
			t.Fatalf("failed to write logger.yaml: %v", err)
		}
	}
	logs := filepath.Join(cwd, ".lyx", "logs")
	if err := os.MkdirAll(logs, 0o755); err != nil {
		t.Fatalf("failed to create logs dir: %v", err)
	}
	fx := exitSweepFixture{cwd: cwd, logs: logs}
	for i := 1; i <= 4; i++ {
		mtime := time.Now().Add(-time.Duration(i) * 24 * time.Hour)
		name := fmt.Sprintf("trace-%s-%016x-999999999.log", mtime.UTC().Format("20060102T150405Z"), i)
		path := filepath.Join(logs, name)
		if err := os.WriteFile(path, []byte("seeded"), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
		if err := os.Chtimes(path, mtime, mtime); err != nil {
			t.Fatalf("Chtimes(%s) = %v", path, err)
		}
		fx.seeded = append(fx.seeded, name)
	}
	return fx
}

// run executes lyx in the fixture and returns its exit code and combined output.
func (fx exitSweepFixture) run(t *testing.T, lyxExe string, args ...string) (int, string) {
	t.Helper()
	cmd := exec.Command(lyxExe, args...)
	cmd.Dir = fx.cwd
	out, err := cmd.CombinedOutput()
	if err == nil {
		return 0, string(out)
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("lyx %v: %v; output: %s", args, err, out)
	}
	return exitErr.ExitCode(), string(out)
}

// logNames lists the file names in the fixture's logs directory.
func (fx exitSweepFixture) logNames(t *testing.T) map[string]bool {
	t.Helper()
	entries, err := os.ReadDir(fx.logs)
	if err != nil {
		t.Fatalf("ReadDir(%q): %v", fx.logs, err)
	}
	names := map[string]bool{}
	for _, e := range entries {
		names[e.Name()] = true
	}
	return names
}

func TestExitSweep_ConfiguredCountKeepsNewestSeededTraces(t *testing.T) {
	lyxExe := lyxbin.Build(t)
	fx := newExitSweepFixture(t, "trace_retention_count: 2\ntrace_retention_days: 14\n")

	code, out := fx.run(t, lyxExe, "bogus-subcommand")
	if code == 0 {
		t.Fatalf("expected non-zero exit; output: %s", out)
	}

	names := fx.logNames(t)
	if len(names) != 3 {
		t.Fatalf("logs dir holds %v; want this process's own trace plus two seeded", names)
	}
	for _, kept := range fx.seeded[:2] {
		if !names[kept] {
			t.Errorf("newest seeded trace %s was deleted; logs: %v", kept, names)
		}
	}
	for _, gone := range fx.seeded[2:] {
		if names[gone] {
			t.Errorf("older seeded trace %s survived; logs: %v", gone, names)
		}
	}
}

func TestExitSweep_InvalidConfigWarnsAndKeepsExitCode(t *testing.T) {
	lyxExe := lyxbin.Build(t)
	baseline := newExitSweepFixture(t, "")
	wantCode, _ := baseline.run(t, lyxExe, "bogus-subcommand")

	fx := newExitSweepFixture(t, "trace_retention_count: 0\ntrace_retention_days: 14\n")
	code, out := fx.run(t, lyxExe, "bogus-subcommand")

	if code != wantCode {
		t.Errorf("exit code = %d; want %d (the code without any logger.yaml)", code, wantCode)
	}
	warns := 0
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "logger.yaml") && strings.Contains(line, "trace_retention_count") {
			warns++
		}
	}
	if warns != 1 {
		t.Errorf("Warn lines naming logger.yaml and trace_retention_count = %d; want 1; output: %s", warns, out)
	}
	names := fx.logNames(t)
	for _, seeded := range fx.seeded {
		if !names[seeded] {
			t.Errorf("seeded trace %s was deleted under the default bounds; logs: %v", seeded, names)
		}
	}
}

func TestExitSweep_QuietZeroExitNeitherArmsNorSweeps(t *testing.T) {
	lyxExe := lyxbin.Build(t)
	for name, loggerYAML := range map[string]string{
		"absent":  "",
		"invalid": "trace_retention_count: 0\ntrace_retention_days: 14\n",
	} {
		t.Run(name, func(t *testing.T) {
			fx := newExitSweepFixture(t, loggerYAML)

			code, out := fx.run(t, lyxExe, "--help")
			if code != 0 {
				t.Fatalf("lyx --help exit code = %d; output: %s", code, out)
			}
			if strings.Contains(out, "level=WARN") {
				t.Errorf("output carries a Warn; want none: %s", out)
			}
			names := fx.logNames(t)
			if len(names) != len(fx.seeded) {
				t.Errorf("logs dir holds %v; want exactly the seeded traces", names)
			}
			for _, seeded := range fx.seeded {
				if !names[seeded] {
					t.Errorf("seeded trace %s missing; logs: %v", seeded, names)
				}
			}
		})
	}
}
