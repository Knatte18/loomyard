//go:build integration

// main_integration_test.go holds the module-dispatcher tests that spawn
// gitexec.RunGit(["init"], …) to seed a real git repo so hubgeometry.Resolve
// succeeds, so this file is integration-tagged per the Test Tier Purity
// Invariant.

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/hubgeometry"
)

func TestRunDispatchesToBoard(t *testing.T) {
	t.Setenv("BOARD_SKIP_GIT", "1")
	// Create temp cwd with _lyx/config/board.yaml
	cwd := t.TempDir()

	// Initialize a git repo so the board's PersistentPreRunE can call hubgeometry.Resolve
	// without error. The board data dir is now geometry (hubgeometry.BoardDir(Hub)) rather
	// than a config key, so the dispatched command resolves the worktree layout.
	if _, _, exitCode, err := gitexec.RunGit([]string{"init"}, cwd); err != nil || exitCode != 0 {
		t.Fatalf("git init failed: %v (exit code %d)", err, exitCode)
	}

	lyxDir := filepath.Join(cwd, hubgeometry.LyxDirName)
	if err := os.MkdirAll(lyxDir, 0o755); err != nil {
		t.Fatalf("failed to create _lyx: %v", err)
	}
	configDir := hubgeometry.ConfigDir(cwd)
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("failed to create _lyx/config: %v", err)
	}
	configPath := hubgeometry.ConfigFile(cwd, "board")
	// Write a template-complete board config. path: is no longer a template key
	// (the board data dir is paths-owned), so only readme/design_prefix remain.
	boardConfig := "readme: Home.md\ndesign_prefix: proposal-\n"
	if err := os.WriteFile(configPath, []byte(boardConfig), 0o644); err != nil {
		t.Fatalf("failed to write board.yaml: %v", err)
	}
	t.Chdir(cwd)

	var out bytes.Buffer
	code := run([]string{"board", "rerender"}, &out)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d; output: %s", code, out.String())
	}

	// run must forward the board module's JSON to out unchanged.
	var result map[string]any
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse board output: %v; output: %s", err, out.String())
	}
	if ok, _ := result["ok"].(bool); !ok {
		t.Fatalf("expected ok=true from dispatched board command, got %v", result)
	}
}

func TestRunBoardErrorPropagatesExitCode(t *testing.T) {
	t.Setenv("BOARD_SKIP_GIT", "1")
	// Create temp cwd with _lyx/config/board.yaml
	cwd := t.TempDir()

	// Initialize a git repo so PersistentPreRunE's hubgeometry.Resolve succeeds; this
	// ensures the exit-1 assertion below tests the board command's own failure
	// (removing a nonexistent task), not an upstream layout-resolution error.
	if _, _, exitCode, err := gitexec.RunGit([]string{"init"}, cwd); err != nil || exitCode != 0 {
		t.Fatalf("git init failed: %v (exit code %d)", err, exitCode)
	}

	lyxDir := filepath.Join(cwd, hubgeometry.LyxDirName)
	if err := os.MkdirAll(lyxDir, 0o755); err != nil {
		t.Fatalf("failed to create _lyx: %v", err)
	}
	configDir := hubgeometry.ConfigDir(cwd)
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("failed to create _lyx/config: %v", err)
	}
	configPath := hubgeometry.ConfigFile(cwd, "board")
	// Write a template-complete board config. path: is no longer a template key
	// (the board data dir is paths-owned), so only readme/design_prefix remain.
	boardConfig := "readme: Home.md\ndesign_prefix: proposal-\n"
	if err := os.WriteFile(configPath, []byte(boardConfig), 0o644); err != nil {
		t.Fatalf("failed to write board.yaml: %v", err)
	}
	t.Chdir(cwd)

	// remove of a nonexistent task fails — exit code must bubble up through run.
	var out bytes.Buffer
	code := run([]string{"board", "remove", `{"slug":"nope"}`}, &out)
	if code != 1 {
		t.Fatalf("expected exit 1 from failing board command, got %d; output: %s", code, out.String())
	}
	if !strings.Contains(out.String(), `"ok":false`) {
		t.Fatalf("expected error JSON on out, got %q", out.String())
	}
}

// integrationTestFile is this source file's own absolute path, captured at
// compile time by runtime.Caller(0) — used by buildLyxBinary to locate the
// repo root independent of the process's runtime cwd, following
// internal/reedcli/smoke_test.go's buildLyxBinary precedent: a
// pre-compiled test binary run from elsewhere has no automatic cwd, so a
// relative parent-directory walk from os.Getwd() is not reliable, while a
// build-time-baked path is immune to that.
var _, integrationTestFile, _, _ = runtime.Caller(0)

// traceFilenamePattern matches the durable sink's trace-file naming
// grammar exactly as sink.go's ensureDurableSink composes it:
// "trace-<UTC timestamp>-<TraceID>-<PID>.log".
var traceFilenamePattern = regexp.MustCompile(`^trace-\d{8}T\d{6}Z-[0-9a-f]{16}-\d+\.log$`)

// buildLyxBinary compiles the working tree's cmd/lyx into a temp dir and
// returns its path, so the test below spawns a real lyx process rather than
// calling run() in-process — testing.Testing() is true for any in-process
// call, which would self-suppress the root hook this test is proving works.
func buildLyxBinary(t *testing.T) string {
	t.Helper()
	repoRoot, err := filepath.Abs(filepath.Join(filepath.Dir(integrationTestFile), "..", ".."))
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	lyxExe := filepath.Join(t.TempDir(), "lyx.exe")
	cmd := exec.Command("go", "build", "-o", lyxExe, "./cmd/lyx")
	cmd.Dir = repoRoot
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build ./cmd/lyx: %v\n%s", err, out)
	}
	return lyxExe
}

// TestRootHookWritesTraceFileOnNonZeroExit spawns a real lyx binary against
// a fixture worktree and drives it to a non-zero exit via an unknown
// subcommand, which clihelp.GroupRunE rejects cheaply and deterministically
// (discussion.md's sink-open-triggers "This also settles the integration
// test's trigger" paragraph). It is the only test that proves geometry
// resolution, the root-hook wiring (Card 27), and the real file path
// together: an in-process call to run() cannot, because testing.Testing()
// is true there and the root hook self-suppresses.
func TestRootHookWritesTraceFileOnNonZeroExit(t *testing.T) {
	lyxExe := buildLyxBinary(t)

	// A real git repo so the spawned process's own hubgeometry.Resolve call
	// succeeds, mirroring this file's other tests' fixture setup.
	cwd := t.TempDir()
	if _, _, exitCode, err := gitexec.RunGit([]string{"init"}, cwd); err != nil || exitCode != 0 {
		t.Fatalf("git init failed: %v (exit code %d)", err, exitCode)
	}

	cmd := exec.Command(lyxExe, "bogus-subcommand")
	cmd.Dir = cwd
	out, runErr := cmd.CombinedOutput()

	// A non-zero exit is expected and required — it is the trigger under
	// test. Any error type other than *exec.ExitError (e.g. the binary
	// failing to start) is a real test failure, not the condition we want.
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

	// hubgeometry's WorktreeLogsDir returns filepath.Join(WorktreeRoot,
	// ".lyx", "logs"); this literal join is a review-only rule for test
	// files (TestEnforcement_GeometryLiterals excludes *_test.go), and
	// asserting against the real produced path is the point of this test.
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

	// Initialize git repo so hubgeometry.Resolve succeeds.
	_, _, exitCode, err := gitexec.RunGit([]string{"init"}, cwd)
	if err != nil || exitCode != 0 {
		t.Fatalf("git init failed: %v (exit code %d)", err, exitCode)
	}

	lyxDir := filepath.Join(cwd, hubgeometry.LyxDirName)
	if err := os.MkdirAll(lyxDir, 0o755); err != nil {
		t.Fatalf("failed to create _lyx: %v", err)
	}
	configDir := hubgeometry.ConfigDir(cwd)
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("failed to create _lyx/config: %v", err)
	}
	t.Chdir(cwd)

	var out bytes.Buffer
	code := run([]string{"config", "reconcile"}, &out)
	if code != 0 {
		t.Fatalf("expected exit 0 for config reconcile, got %d; output: %s", code, out.String())
	}

	// Verify JSON output with ok=true.
	var result map[string]any
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse config reconcile output: %v; output: %s", err, out.String())
	}
	if ok, _ := result["ok"].(bool); !ok {
		t.Fatalf("expected ok=true from config reconcile command, got %v", result)
	}
}
