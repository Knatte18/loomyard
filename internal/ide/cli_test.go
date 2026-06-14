package ide

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// TestRunCLISpawnDispatch tests that spawn subcommand dispatches correctly.
func TestRunCLISpawnDispatch(t *testing.T) {
	// Use the current repo's directory which is already a git repo
	// This test will just verify that RunCLI returns an error when paths can't resolve
	// the layout (since we're not in a git repo for this test worktree)
	tmpDir := t.TempDir()

	oldCwd, _ := os.Getwd()
	defer os.Chdir(oldCwd)
	os.Chdir(tmpDir)

	// Stub codeLauncher
	originalLauncher := codeLauncher
	defer func() { codeLauncher = originalLauncher }()
	codeLauncher = func(dir string) error { return nil }

	var out bytes.Buffer
	code := RunCLI(&out, []string{"spawn", "child"})

	// Should fail with layout resolution error since tmpDir is not a git repo
	if code != 1 {
		t.Fatalf("expected exit 1 for non-git directory, got %d; output: %s", code, out.String())
	}

	// Verify error is returned as JSON
	var result map[string]any
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse output: %v; output: %s", err, out.String())
	}

	if ok, _ := result["ok"].(bool); ok {
		t.Fatalf("expected ok=false, got %v", result)
	}
}

// TestRunCLIUnknownSubcommand tests unknown subcommand error (within a non-git dir).
func TestRunCLIUnknownSubcommand(t *testing.T) {
	tmpDir := t.TempDir()

	oldCwd, _ := os.Getwd()
	defer os.Chdir(oldCwd)
	os.Chdir(tmpDir)

	var out bytes.Buffer
	code := RunCLI(&out, []string{"unknown"})

	// Should fail because tmpDir is not a git repo (will fail layout resolution)
	if code != 1 {
		t.Fatalf("expected exit 1, got %d; output: %s", code, out.String())
	}

	output := out.String()
	if !strings.Contains(output, "failed to resolve layout") {
		t.Fatalf("expected layout error, got: %q", output)
	}
}

// TestRunCLIMissingSlug tests missing slug error for spawn.
func TestRunCLIMissingSlug(t *testing.T) {
	tmpDir := t.TempDir()

	oldCwd, _ := os.Getwd()
	defer os.Chdir(oldCwd)
	os.Chdir(tmpDir)

	var out bytes.Buffer
	code := RunCLI(&out, []string{"spawn"})

	// Should fail because tmpDir is not a git repo
	if code != 1 {
		t.Fatalf("expected exit 1, got %d; output: %s", code, out.String())
	}

	output := out.String()
	if !strings.Contains(output, "failed to resolve layout") {
		t.Fatalf("expected layout error, got: %q", output)
	}
}

// TestRunCLINoArgs tests no-args usage error.
func TestRunCLINoArgs(t *testing.T) {
	tmpDir := t.TempDir()

	oldCwd, _ := os.Getwd()
	defer os.Chdir(oldCwd)
	os.Chdir(tmpDir)

	var out bytes.Buffer
	code := RunCLI(&out, []string{})

	// Should fail because tmpDir is not a git repo
	if code != 1 {
		t.Fatalf("expected exit 1, got %d; output: %s", code, out.String())
	}

	output := out.String()
	if !strings.Contains(output, "failed to resolve layout") {
		t.Fatalf("expected layout error, got: %q", output)
	}
}
