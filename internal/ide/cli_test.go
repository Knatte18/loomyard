package ide

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRunCLISpawnDispatch tests that spawn subcommand dispatches correctly.
func TestRunCLISpawnDispatch(t *testing.T) {
	tmpDir := t.TempDir()
	container := tmpDir
	mainWorktreePath := filepath.Join(container, "main")
	childWorktreePath := filepath.Join(container, "child")

	// Create main with _mhgo
	if err := os.MkdirAll(filepath.Join(mainWorktreePath, "_mhgo"), 0o755); err != nil {
		t.Fatalf("failed to create main _mhgo: %v", err)
	}

	// Create child
	if err := os.MkdirAll(childWorktreePath, 0o755); err != nil {
		t.Fatalf("failed to create child: %v", err)
	}

	// Change to main worktree directory
	oldCwd, _ := os.Getwd()
	defer os.Chdir(oldCwd)
	os.Chdir(mainWorktreePath)

	// Stub codeLauncher to avoid launching real VS Code
	originalLauncher := codeLauncher
	defer func() { codeLauncher = originalLauncher }()
	codeLauncher = func(dir string) error { return nil }

	var out bytes.Buffer
	code := RunCLI(&out, []string{"spawn", "child"})

	if code != 0 {
		t.Fatalf("expected exit 0, got %d; output: %s", code, out.String())
	}

	// Verify output is valid JSON with ok=true
	var result map[string]any
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse output: %v; output: %s", err, out.String())
	}

	if ok, _ := result["ok"].(bool); !ok {
		t.Fatalf("expected ok=true, got %v", result)
	}
}

// TestRunCLIUnknownSubcommand tests unknown subcommand error.
func TestRunCLIUnknownSubcommand(t *testing.T) {
	tmpDir := t.TempDir()
	mainWorktreePath := filepath.Join(tmpDir, "main")

	if err := os.MkdirAll(filepath.Join(mainWorktreePath, "_mhgo"), 0o755); err != nil {
		t.Fatalf("failed to create main _mhgo: %v", err)
	}

	oldCwd, _ := os.Getwd()
	defer os.Chdir(oldCwd)
	os.Chdir(mainWorktreePath)

	var out bytes.Buffer
	code := RunCLI(&out, []string{"unknown"})

	if code != 1 {
		t.Fatalf("expected exit 1, got %d; output: %s", code, out.String())
	}

	output := out.String()
	if !strings.Contains(output, "unknown subcommand") {
		t.Fatalf("expected 'unknown subcommand' error, got: %q", output)
	}
}

// TestRunCLIMissingSlug tests missing slug error for spawn.
func TestRunCLIMissingSlug(t *testing.T) {
	tmpDir := t.TempDir()
	mainWorktreePath := filepath.Join(tmpDir, "main")

	if err := os.MkdirAll(filepath.Join(mainWorktreePath, "_mhgo"), 0o755); err != nil {
		t.Fatalf("failed to create main _mhgo: %v", err)
	}

	oldCwd, _ := os.Getwd()
	defer os.Chdir(oldCwd)
	os.Chdir(mainWorktreePath)

	var out bytes.Buffer
	code := RunCLI(&out, []string{"spawn"})

	if code != 1 {
		t.Fatalf("expected exit 1, got %d; output: %s", code, out.String())
	}

	output := out.String()
	if !strings.Contains(output, "usage:") {
		t.Fatalf("expected usage error, got: %q", output)
	}
}

// TestRunCLINoArgs tests no-args usage error.
func TestRunCLINoArgs(t *testing.T) {
	tmpDir := t.TempDir()
	mainWorktreePath := filepath.Join(tmpDir, "main")

	if err := os.MkdirAll(filepath.Join(mainWorktreePath, "_mhgo"), 0o755); err != nil {
		t.Fatalf("failed to create main _mhgo: %v", err)
	}

	oldCwd, _ := os.Getwd()
	defer os.Chdir(oldCwd)
	os.Chdir(mainWorktreePath)

	var out bytes.Buffer
	code := RunCLI(&out, []string{})

	if code != 1 {
		t.Fatalf("expected exit 1, got %d; output: %s", code, out.String())
	}

	output := out.String()
	if !strings.Contains(output, "usage:") {
		t.Fatalf("expected usage error, got: %q", output)
	}
}
