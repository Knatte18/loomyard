// update_test.go — tests for the lyx update command.

package update

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/git"
)

func TestRunCLI_DryRun(t *testing.T) {
	tmpDir := t.TempDir()

	// Initialize a minimal git repo so paths.Resolve works
	_, _, exitCode, err := git.RunGit([]string{"init"}, tmpDir)
	if err != nil || exitCode != 0 {
		t.Fatalf("git init failed: %v (exit code %d)", err, exitCode)
	}

	// Create config directory with a sample board file
	configDir := filepath.Join(tmpDir, "_lyx", "config")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("mkdir config: %v", err)
	}

	boardPath := filepath.Join(configDir, "board.yaml")
	originalContent := "path: board\nstale_key: old_value\n"
	if err := os.WriteFile(boardPath, []byte(originalContent), 0o644); err != nil {
		t.Fatalf("write board.yaml: %v", err)
	}

	// Change to tmpDir
	oldCwd, err2 := os.Getwd()
	if err2 != nil {
		t.Fatalf("getwd: %v", err2)
	}
	if err2 := os.Chdir(tmpDir); err2 != nil {
		t.Fatalf("chdir: %v", err2)
	}
	defer os.Chdir(oldCwd)

	// Run dry-run (no --apply)
	var buf bytes.Buffer
	runExitCode := RunCLI(&buf, []string{})

	if runExitCode != 0 {
		t.Errorf("RunCLI() = %d; want 0, output: %s", runExitCode, buf.String())
	}

	// Parse JSON output
	var result map[string]any
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("parse JSON: %v, output: %s", err, buf.String())
	}

	if ok, _ := result["ok"].(bool); !ok {
		t.Error("ok flag is not true")
	}

	// Check applied=false (dry-run)
	if applied, _ := result["applied"].(bool); applied {
		t.Error("applied is true; want false (dry-run)")
	}

	// Verify file was not written
	content, err := os.ReadFile(boardPath)
	if err != nil {
		t.Fatalf("read board.yaml: %v", err)
	}
	if string(content) != originalContent {
		t.Error("board.yaml was modified during dry-run; should be unchanged")
	}

	// Check modules array exists and contains per-module info
	modules, ok := result["modules"].([]any)
	if !ok {
		t.Error("modules is not an array")
	} else if len(modules) == 0 {
		t.Error("modules array is empty; want non-empty")
	} else {
		// Verify first module has the expected shape
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

func TestRunCLI_Apply(t *testing.T) {
	tmpDir := t.TempDir()

	// Initialize a minimal git repo
	_, _, exitCode, err := git.RunGit([]string{"init"}, tmpDir)
	if err != nil || exitCode != 0 {
		t.Fatalf("git init failed: %v (exit code %d)", err, exitCode)
	}

	// Create config directory
	configDir := filepath.Join(tmpDir, "_lyx", "config")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("mkdir config: %v", err)
	}

	// Change to tmpDir
	oldCwd, err2 := os.Getwd()
	if err2 != nil {
		t.Fatalf("getwd: %v", err2)
	}
	if err2 := os.Chdir(tmpDir); err2 != nil {
		t.Fatalf("chdir: %v", err2)
	}
	defer os.Chdir(oldCwd)

	// Run with --apply
	var buf bytes.Buffer
	runExitCode := RunCLI(&buf, []string{"--apply"})

	if runExitCode != 0 {
		t.Errorf("RunCLI(--apply) = %d; want 0, output: %s", runExitCode, buf.String())
	}

	// Parse JSON output
	var result map[string]any
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("parse JSON: %v, output: %s", err, buf.String())
	}

	if ok, _ := result["ok"].(bool); !ok {
		t.Error("ok flag is not true")
	}

	// Check applied=true
	if applied, _ := result["applied"].(bool); !applied {
		t.Error("applied is false; want true")
	}

	// Verify files were created
	weftPath := filepath.Join(configDir, "weft.yaml")
	if _, err := os.Stat(weftPath); err != nil {
		t.Errorf("weft.yaml not created: %v", err)
	}
}
