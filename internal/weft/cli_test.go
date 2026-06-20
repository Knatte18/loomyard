// cli_test.go — tests for weft CLI routing.

package weft

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestRunCLI_UnknownSubcommand(t *testing.T) {
	tmpDir := t.TempDir()
	t.Chdir(tmpDir)

	var out bytes.Buffer
	exitCode := RunCLI(&out, []string{"unknown"})

	if exitCode != 1 {
		t.Errorf("RunCLI with unknown subcommand returned %d; want 1", exitCode)
	}
}

func TestRunCLI_WeftPathPushOnly(t *testing.T) {
	tmpDir := t.TempDir()

	var out bytes.Buffer
	// Call with --weft-path and a non-push subcommand
	exitCode := RunCLI(&out, []string{"--weft-path", tmpDir, "status"})

	if exitCode != 1 {
		t.Errorf("RunCLI --weft-path with non-push returned %d; want 1", exitCode)
	}

	// Check that the error message is about worktree context
	output := out.String()
	var jsonOut map[string]any
	if err := json.Unmarshal([]byte(output), &jsonOut); err != nil {
		t.Fatalf("failed to unmarshal JSON output: %v", err)
	}

	if ok, _ := jsonOut["ok"].(bool); ok {
		t.Errorf("ok should be false for error; got true")
	}

	if errMsg, ok := jsonOut["error"].(string); ok {
		if errMsg != "subcommand requires a worktree context" {
			t.Errorf("error message = %q; want %q", errMsg, "subcommand requires a worktree context")
		}
	} else {
		t.Errorf("error field missing or not a string")
	}
}

func TestRunCLI_StatusWithMinimalFixture(t *testing.T) {
	// Create a temporary fixture with minimal structure
	tmpDir := t.TempDir()
	t.Setenv("WEFT_SKIP_GIT", "1")

	// Initialize a git repo
	mustRun(t, tmpDir, "git", "init", "-b", "main")
	mustRun(t, tmpDir, "git", "config", "user.email", "test@test.com")
	mustRun(t, tmpDir, "git", "config", "user.name", "Test")

	// Create _lyx directory structure
	lyxDir := filepath.Join(tmpDir, "_lyx")
	if err := os.MkdirAll(filepath.Join(lyxDir, "config"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	// Create initial commit
	readmeFile := filepath.Join(tmpDir, "README")
	if err := os.WriteFile(readmeFile, []byte("test"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	mustRun(t, tmpDir, "git", "add", ".")
	mustRun(t, tmpDir, "git", "commit", "-m", "init")

	// Create weft structure in hub
	hub := filepath.Dir(tmpDir)
	weftName := filepath.Base(tmpDir) + "-weft"
	weftPath := filepath.Join(hub, weftName)

	if err := os.MkdirAll(weftPath, 0o755); err != nil {
		t.Fatalf("MkdirAll weft: %v", err)
	}

	// Initialize weft as a git repo
	mustRun(t, weftPath, "git", "init", "-b", "main")
	mustRun(t, weftPath, "git", "config", "user.email", "test@test.com")
	mustRun(t, weftPath, "git", "config", "user.name", "Test")

	// Create _lyx in weft
	weftLyxDir := filepath.Join(weftPath, "_lyx")
	if err := os.MkdirAll(filepath.Join(weftLyxDir, "config"), 0o755); err != nil {
		t.Fatalf("MkdirAll weft _lyx: %v", err)
	}

	// Create initial weft commit
	weftFile := filepath.Join(weftLyxDir, "config.yaml")
	if err := os.WriteFile(weftFile, []byte("test"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	mustRun(t, weftPath, "git", "add", ".")
	mustRun(t, weftPath, "git", "commit", "-m", "init")

	// Change to the host repo
	t.Chdir(tmpDir)

	// Call status
	var out bytes.Buffer
	exitCode := RunCLI(&out, []string{"status"})

	if exitCode != 0 {
		t.Errorf("RunCLI status returned %d; want 0", exitCode)
		t.Logf("output: %s", out.String())
	}

	// Parse JSON output
	var jsonOut map[string]any
	if err := json.Unmarshal(out.Bytes(), &jsonOut); err != nil {
		t.Fatalf("failed to unmarshal JSON output: %v", err)
	}

	if ok, _ := jsonOut["ok"].(bool); !ok {
		t.Errorf("ok should be true; got false. Error: %v", jsonOut["error"])
	}

	// Check for junction_ok field
	if _, hasJunction := jsonOut["junction_ok"]; !hasJunction {
		t.Errorf("junction_ok field missing from status output")
	}
}
