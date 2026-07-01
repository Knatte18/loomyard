// reconcile_test.go — tests for the lyx config reconcile subcommand.
//
// Migrated from internal/update/update_test.go; drives the same two scenarios
// (dry-run and --apply) through the new configcli seam.

package configcli

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/hubgeometry"
)

// TestReconcile_DryRun verifies that "lyx config reconcile" without --apply writes
// no files and returns a JSON envelope with ok=true, applied=false, and a non-empty
// modules array whose entries carry module/added/removed/applied fields.
func TestReconcile_DryRun(t *testing.T) {
	tmpDir := t.TempDir()

	// Initialize a minimal git repo so hubgeometry.Resolve works.
	_, _, exitCode, err := gitexec.RunGit([]string{"init"}, tmpDir)
	if err != nil || exitCode != 0 {
		t.Fatalf("git init failed: %v (exit code %d)", err, exitCode)
	}

	// Create config directory with a sample board file.
	configDir := hubgeometry.ConfigDir(tmpDir)
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("mkdir config: %v", err)
	}

	boardPath := hubgeometry.ConfigFile(tmpDir, "board")
	originalContent := "path: board\nstale_key: old_value\n"
	if err := os.WriteFile(boardPath, []byte(originalContent), 0o644); err != nil {
		t.Fatalf("write board.yaml: %v", err)
	}

	// Chdir into the temp repo so hubgeometry.Getwd inside RunCLI resolves to a git repo.
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

	// Verify board.yaml was not modified.
	content, err := os.ReadFile(boardPath)
	if err != nil {
		t.Fatalf("read board.yaml: %v", err)
	}
	if string(content) != originalContent {
		t.Error("board.yaml was modified during dry-run; should be unchanged")
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

// TestReconcile_NotAGitRepo verifies that "lyx config reconcile" run from a
// non-git temp directory surfaces hubgeometry's bare ErrNotAGitRepo sentinel
// with no "resolve layout:" prefix and no raw "fatal:" git stderr.
func TestReconcile_NotAGitRepo(t *testing.T) {
	tmpDir := t.TempDir()

	// Chdir into the non-git temp dir so hubgeometry.Getwd inside RunCLI resolves there.
	oldCwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer os.Chdir(oldCwd) //nolint:errcheck

	var buf bytes.Buffer
	runExitCode := RunCLI(&buf, []string{"reconcile"})

	if runExitCode != 1 {
		t.Errorf("RunCLI(reconcile) in non-git dir = %d; want 1", runExitCode)
	}

	var result map[string]any
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("parse JSON: %v, output: %s", err, buf.String())
	}
	errMsg, _ := result["error"].(string)
	if errMsg != "not a git repository" {
		t.Errorf("RunCLI(reconcile) error = %q; want exactly \"not a git repository\"", errMsg)
	}
}

// TestReconcile_Apply verifies that "lyx config reconcile --apply" writes config
// files to disk and returns a JSON envelope with ok=true and applied=true.
func TestReconcile_Apply(t *testing.T) {
	tmpDir := t.TempDir()

	// Initialize a minimal git repo.
	_, _, exitCode, err := gitexec.RunGit([]string{"init"}, tmpDir)
	if err != nil || exitCode != 0 {
		t.Fatalf("git init failed: %v (exit code %d)", err, exitCode)
	}

	// Create config directory.
	configDir := hubgeometry.ConfigDir(tmpDir)
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("mkdir config: %v", err)
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

	// Verify weft.yaml was created on disk.
	weftPath := hubgeometry.ConfigFile(tmpDir, "weft")
	if _, err := os.Stat(weftPath); err != nil {
		t.Errorf("weft.yaml not created: %v", err)
	}
}
