// init_test.go — tests for the init scaffold (init.go).
//
// Covers: creating _mhgo/board.yaml and .gitignore managed block,
// idempotency (re-run doesn't clobber or duplicate), and JSON output shape.

package board_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/mhgo/internal/board"
)

// runInit invokes board.RunInit in-process from a temp cwd and returns the exit
// code plus the JSON written to out.
func runInit(t *testing.T, cwd string) (exitCode int, stdout string) {
	t.Helper()

	// Save original cwd
	origCwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get original cwd: %v", err)
	}
	defer func() {
		if err := os.Chdir(origCwd); err != nil {
			t.Errorf("failed to restore cwd: %v", err)
		}
	}()

	// Change to test cwd
	if err := os.Chdir(cwd); err != nil {
		t.Fatalf("failed to chdir to temp dir: %v", err)
	}

	var buf bytes.Buffer
	code := board.RunInit(&buf, []string{})
	return code, buf.String()
}

// TestInitCreatesStructure tests that first run creates _mhgo/board.yaml.
func TestInitCreatesStructure(t *testing.T) {
	cwd := t.TempDir()

	exitCode, stdout := runInit(t, cwd)

	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d; stdout: %s", exitCode, stdout)
	}

	// Verify _mhgo/ exists
	mhgoDir := filepath.Join(cwd, "_mhgo")
	info, err := os.Stat(mhgoDir)
	if err != nil {
		t.Fatalf("_mhgo directory not created: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("_mhgo is not a directory")
	}

	// Verify board.yaml exists
	boardYamlPath := filepath.Join(mhgoDir, "board.yaml")
	content, err := os.ReadFile(boardYamlPath)
	if err != nil {
		t.Fatalf("board.yaml not created: %v", err)
	}

	// Verify board.yaml is fully commented (no uncommented lines except blank/comment-only)
	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			// Blank line is OK
			continue
		}
		if !strings.HasPrefix(trimmed, "#") {
			t.Errorf("expected all non-blank lines to start with #, found: %s", line)
		}
	}
}

// TestInitGitignoreBlock tests that .gitignore managed block is created.
func TestInitGitignoreBlock(t *testing.T) {
	cwd := t.TempDir()

	exitCode, stdout := runInit(t, cwd)

	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d; stdout: %s", exitCode, stdout)
	}

	// Verify .gitignore exists
	gitignorePath := filepath.Join(cwd, ".gitignore")
	content, err := os.ReadFile(gitignorePath)
	if err != nil {
		t.Fatalf(".gitignore not created: %v", err)
	}

	contentStr := string(content)

	// Check for start marker
	if !strings.Contains(contentStr, "# === mhgo-managed ===") {
		t.Errorf("expected start marker in .gitignore, got: %s", contentStr)
	}

	// Check for end marker
	if !strings.Contains(contentStr, "# === end mhgo-managed ===") {
		t.Errorf("expected end marker in .gitignore, got: %s", contentStr)
	}

	// Check for .mhgo/ entry
	if !strings.Contains(contentStr, ".mhgo/") {
		t.Errorf("expected .mhgo/ entry in .gitignore, got: %s", contentStr)
	}
}

// TestInitIdempotent tests that a second run doesn't clobber board.yaml or duplicate the gitignore block.
func TestInitIdempotent(t *testing.T) {
	cwd := t.TempDir()

	// First run
	exitCode1, stdout1 := runInit(t, cwd)
	if exitCode1 != 0 {
		t.Fatalf("first run: expected exit 0, got %d; stdout: %s", exitCode1, stdout1)
	}

	// Capture board.yaml content and mtime
	boardYamlPath := filepath.Join(cwd, "_mhgo", "board.yaml")
	content1, err := os.ReadFile(boardYamlPath)
	if err != nil {
		t.Fatalf("failed to read board.yaml after first run: %v", err)
	}

	_, err = os.Stat(boardYamlPath)
	if err != nil {
		t.Fatalf("failed to stat board.yaml: %v", err)
	}

	// Capture .gitignore content
	gitignorePath := filepath.Join(cwd, ".gitignore")
	gitignore1, err := os.ReadFile(gitignorePath)
	if err != nil {
		t.Fatalf("failed to read .gitignore after first run: %v", err)
	}

	// Second run
	exitCode2, stdout2 := runInit(t, cwd)
	if exitCode2 != 0 {
		t.Fatalf("second run: expected exit 0, got %d; stdout: %s", exitCode2, stdout2)
	}

	// Verify board.yaml unchanged
	content2, err := os.ReadFile(boardYamlPath)
	if err != nil {
		t.Fatalf("failed to read board.yaml after second run: %v", err)
	}

	if string(content1) != string(content2) {
		t.Errorf("board.yaml was modified on second run")
	}

	// Verify .gitignore not duplicated (same content)
	gitignore2, err := os.ReadFile(gitignorePath)
	if err != nil {
		t.Fatalf("failed to read .gitignore after second run: %v", err)
	}

	if string(gitignore1) != string(gitignore2) {
		t.Errorf(".gitignore was modified on second run (block may have been duplicated)")
	}

	// Parse JSON output and verify "exists" and "unchanged" statuses
	var result map[string]any
	if err := json.Unmarshal([]byte(stdout2), &result); err != nil {
		t.Fatalf("failed to parse second run output: %v; stdout: %s", err, stdout2)
	}

	if boardYaml, ok := result["board_yaml"].(string); !ok || boardYaml != "exists" {
		t.Errorf("expected board_yaml='exists', got %v", result["board_yaml"])
	}

	if gitignoreStatus, ok := result["gitignore"].(string); !ok || gitignoreStatus != "unchanged" {
		t.Errorf("expected gitignore='unchanged', got %v", result["gitignore"])
	}

	if mhgoDir, ok := result["mhgo_dir"].(string); !ok || mhgoDir != "exists" {
		t.Errorf("expected mhgo_dir='exists', got %v", result["mhgo_dir"])
	}
}

// TestInitJSONShape tests that the JSON output has the correct shape on first run.
func TestInitJSONShape(t *testing.T) {
	cwd := t.TempDir()

	exitCode, stdout := runInit(t, cwd)

	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d; stdout: %s", exitCode, stdout)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("failed to parse output JSON: %v; stdout: %s", err, stdout)
	}

	// Verify ok=true
	if ok, exists := result["ok"].(bool); !exists || !ok {
		t.Errorf("expected ok=true, got %v", result["ok"])
	}

	// Verify mhgo_dir is "created"
	if mhgoDir, ok := result["mhgo_dir"].(string); !ok || mhgoDir != "created" {
		t.Errorf("expected mhgo_dir='created', got %v", result["mhgo_dir"])
	}

	// Verify board_yaml is "created"
	if boardYaml, ok := result["board_yaml"].(string); !ok || boardYaml != "created" {
		t.Errorf("expected board_yaml='created', got %v", result["board_yaml"])
	}

	// Verify gitignore is "updated"
	if gitignore, ok := result["gitignore"].(string); !ok || gitignore != "updated" {
		t.Errorf("expected gitignore='updated', got %v", result["gitignore"])
	}

	// Verify no unexpected keys
	expectedKeys := map[string]bool{"ok": true, "mhgo_dir": true, "board_yaml": true, "gitignore": true}
	for key := range result {
		if !expectedKeys[key] {
			t.Errorf("unexpected key in JSON output: %s", key)
		}
	}
}
