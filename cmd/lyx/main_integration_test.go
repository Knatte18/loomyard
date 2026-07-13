//go:build integration

// main_integration_test.go holds the module-dispatcher tests that spawn
// gitexec.RunGit(["init"], …) to seed a real git repo so hubgeometry.Resolve
// succeeds, so this file is integration-tagged per the Test Tier Purity
// Invariant.

package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
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
	// (the board data dir is paths-owned), so only home/sidebar/proposal_prefix remain.
	boardConfig := "home: Home.md\nsidebar: _Sidebar.md\nproposal_prefix: proposal-\n"
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
	// (the board data dir is paths-owned), so only home/sidebar/proposal_prefix remain.
	boardConfig := "home: Home.md\nsidebar: _Sidebar.md\nproposal_prefix: proposal-\n"
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
