//go:build integration

// reconcile_integration_test.go holds the reconcile scenarios that spawn
// gitexec.RunGit(["init"], …) to seed a real git repo: the dry-run and
// --apply round trips. This file is integration-tagged per the Test Tier
// Purity Invariant; the spawn-free not-a-git-repo assertion stays in
// reconcile_test.go.

package configcli

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

// TestReconcile_DryRun verifies that "lyx config reconcile" without --apply writes no files and
// returns a JSON envelope with ok=true, applied=false, and a non-empty modules array whose entries
// carry module/added/removed/applied fields.
func TestReconcile_DryRun(t *testing.T) {
	tmpDir := t.TempDir()

	// Initialize a minimal git repo so lyxcwd.Resolve works.
	_, _, exitCode, err := gitexec.RunGit([]string{"init"}, tmpDir)
	if err != nil || exitCode != 0 {
		t.Fatalf("git init failed: %v (exit code %d)", err, exitCode)
	}

	// Create config directory with a sample loom file.
	configDir := configengine.ConfigDir(tmpDir)
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("mkdir config: %v", err)
	}

	loomPath := configengine.ConfigFile(tmpDir, "loom")
	originalContent := "discussion_timeout_min: 480\nstale_key: old_value\n"
	if err := os.WriteFile(loomPath, []byte(originalContent), 0o644); err != nil {
		t.Fatalf("write loom.yaml: %v", err)
	}

	// Chdir into the temp repo so lyxcwd.Getwd inside RunCLI resolves to a git repo.
	oldCwd, err2 := os.Getwd()
	if err2 != nil {
		t.Fatalf("getwd: %v", err2)
	}
	if err2 := os.Chdir(tmpDir); err2 != nil {
		t.Fatalf("chdir: %v", err2)
	}
	defer os.Chdir(oldCwd) //nolint:errcheck

	// Run dry-run (no --apply).
	var buf bytes.Buffer
	runExitCode := RunCLI(&buf, []string{"reconcile"})

	if runExitCode != 0 {
		t.Errorf("RunCLI(reconcile) = %d; want 0, output: %s", runExitCode, buf.String())
	}

	// Parse JSON output.
	var result map[string]any
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("parse JSON: %v, output: %s", err, buf.String())
	}

	if ok, _ := result["ok"].(bool); !ok {
		t.Error("ok flag is not true")
	}

	// Check applied=false (dry-run).
	if applied, _ := result["applied"].(bool); applied {
		t.Error("applied is true; want false (dry-run)")
	}

	// Verify loom.yaml was not modified.
	content, err := os.ReadFile(loomPath)
	if err != nil {
		t.Fatalf("read loom.yaml: %v", err)
	}
	if string(content) != originalContent {
		t.Error("loom.yaml was modified during dry-run; should be unchanged")
	}

	// Check modules array exists and contains per-module info.
	modules, ok := result["modules"].([]any)
	if !ok {
		t.Error("modules is not an array")
	} else if len(modules) == 0 {
		t.Error("modules array is empty; want non-empty")
	} else {
		// Verify first module has the expected shape.
		if mod, ok := modules[0].(map[string]any); ok {
			if _, hasModule := mod["module"]; !hasModule {
				t.Error("module entry missing 'module' field")
			}
			if _, hasAdded := mod["added"]; !hasAdded {
				t.Error("module entry missing 'added' field")
			}
			if _, hasRemoved := mod["removed"]; !hasRemoved {
				t.Error("module entry missing 'removed' field")
			}
			if _, hasApplied := mod["applied"]; !hasApplied {
				t.Error("module entry missing 'applied' field")
			}
		}
	}
}

// TestReconcile_Apply verifies that "lyx config reconcile --apply" writes config files to disk and
// returns a JSON envelope with ok=true and applied=true.
func TestReconcile_Apply(t *testing.T) {
	tmpDir := t.TempDir()

	// Initialize a minimal git repo.
	_, _, exitCode, err := gitexec.RunGit([]string{"init"}, tmpDir)
	if err != nil || exitCode != 0 {
		t.Fatalf("git init failed: %v (exit code %d)", err, exitCode)
	}

	// Create config directory, with a per-worktree board.yaml the hub file below subsumes.
	configDir := configengine.ConfigDir(tmpDir)
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("mkdir config: %v", err)
	}
	const subsumedBoard = "labels:\n  area-x: Area X\n"
	boardPath := configengine.ConfigFile(tmpDir, "board")
	if err := os.WriteFile(boardPath, []byte(subsumedBoard), 0o644); err != nil {
		t.Fatalf("write board.yaml: %v", err)
	}
	location, err := lyxcwd.Resolve(tmpDir)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	hubBoardDir := fabricengine.BoardDir(location.HubPath)
	if err := os.MkdirAll(configengine.ConfigDir(hubBoardDir), 0o755); err != nil {
		t.Fatalf("mkdir hub config: %v", err)
	}
	if err := os.WriteFile(configengine.ConfigFile(hubBoardDir, "board"), []byte("labels:\n  area-x: Area X\n  area-y: Area Y\n"), 0o644); err != nil {
		t.Fatalf("write hub board.yaml: %v", err)
	}

	// Chdir into the temp repo.
	oldCwd, err2 := os.Getwd()
	if err2 != nil {
		t.Fatalf("getwd: %v", err2)
	}
	if err2 := os.Chdir(tmpDir); err2 != nil {
		t.Fatalf("chdir: %v", err2)
	}
	defer os.Chdir(oldCwd) //nolint:errcheck

	// Run with --apply.
	var buf bytes.Buffer
	runExitCode := RunCLI(&buf, []string{"reconcile", "--apply"})

	if runExitCode != 0 {
		t.Errorf("RunCLI(reconcile --apply) = %d; want 0, output: %s", runExitCode, buf.String())
	}

	// Parse JSON output.
	var result map[string]any
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("parse JSON: %v, output: %s", err, buf.String())
	}

	if ok, _ := result["ok"].(bool); !ok {
		t.Error("ok flag is not true")
	}

	// Check applied=true.
	if applied, _ := result["applied"].(bool); !applied {
		t.Error("applied is false; want true")
	}

	// Verify loom.yaml was created on disk. Hub-wide modules are never written per-worktree, so
	// "loom" is the generic module this reconcile-writes-to-disk assertion exercises.
	loomPath := configengine.ConfigFile(tmpDir, "loom")
	if _, err := os.Stat(loomPath); err != nil {
		t.Errorf("loom.yaml not created: %v", err)
	}

	// The per-worktree board.yaml the hub file subsumes is reported retired and deleted.
	modules, _ := result["modules"].([]any)
	retired := false
	for _, m := range modules {
		if mod, ok := m.(map[string]any); ok && mod["module"] == "board" {
			retired, _ = mod["retired"].(bool)
		}
	}
	if !retired {
		t.Errorf("board module not reported retired; modules = %v", modules)
	}
	if _, err := os.Stat(boardPath); !os.IsNotExist(err) {
		t.Errorf("per-worktree board.yaml still present after --apply (stat err = %v)", err)
	}
}
