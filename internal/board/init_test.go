// init_test.go — tests for the init scaffold (init.go).
//
// Covers: creating _lyx/config/board.yaml and .gitignore managed block,
// idempotency (re-run doesn't clobber or duplicate), and JSON output shape.

package board_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/board"
	"github.com/Knatte18/loomyard/internal/worktree"
)

// runInit invokes board.RunInit in-process and returns the exit code plus the
// JSON written to out. Caller must have called t.Chdir to set up the cwd.
func runInit(t *testing.T) (exitCode int, stdout string) {
	t.Helper()

	var buf bytes.Buffer
	code := board.RunInit(&buf, []string{})
	return code, buf.String()
}

// TestInitFirstRun tests the first-run init behavior: creates _lyx/config/
// with board.yaml + worktree.yaml (fully commented), .gitignore managed block,
// and returns the correct JSON envelope. Each check is a subtest with the original name.
//
// Folds: TestInitCreatesStructure, TestInitGitignoreBlock, TestInitJSONShape
func TestInitFirstRun(t *testing.T) {
	cwd := t.TempDir()
	t.Chdir(cwd)

	exitCode, stdout := runInit(t)

	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d; stdout: %s", exitCode, stdout)
	}

	t.Run("TestInitCreatesStructure", func(t *testing.T) {
		// Verify _lyx/ exists
		lyxDir := filepath.Join(cwd, "_lyx")
		info, err := os.Stat(lyxDir)
		if err != nil {
			t.Fatalf("_lyx directory not created: %v", err)
		}
		if !info.IsDir() {
			t.Fatalf("_lyx is not a directory")
		}

		// Verify _lyx/config/ directory exists
		configDir := filepath.Join(lyxDir, "config")
		info, err = os.Stat(configDir)
		if err != nil {
			t.Fatalf("_lyx/config directory not created: %v", err)
		}
		if !info.IsDir() {
			t.Fatalf("_lyx/config is not a directory")
		}

		// Verify board.yaml exists in _lyx/config/ and is fully commented
		boardYamlPath := filepath.Join(configDir, "board.yaml")
		content, err := os.ReadFile(boardYamlPath)
		if err != nil {
			t.Fatalf("board.yaml not created: %v", err)
		}

		// Assert written bytes equal board.ConfigTemplate()
		if string(content) != board.ConfigTemplate() {
			t.Errorf("board.yaml content = %q; want %q", string(content), board.ConfigTemplate())
		}

		lines := strings.Split(string(content), "\n")
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" {
				continue
			}
			if !strings.HasPrefix(trimmed, "#") {
				t.Errorf("expected all non-blank lines to start with #, found: %s", line)
			}
		}

		// Verify worktree.yaml exists in _lyx/config/ and is fully commented
		worktreeYamlPath := filepath.Join(configDir, "worktree.yaml")
		worktreeContent, err := os.ReadFile(worktreeYamlPath)
		if err != nil {
			t.Fatalf("worktree.yaml not created: %v", err)
		}

		// Assert written bytes equal worktree.ConfigTemplate()
		if string(worktreeContent) != worktree.ConfigTemplate() {
			t.Errorf("worktree.yaml content = %q; want %q", string(worktreeContent), worktree.ConfigTemplate())
		}

		lines = strings.Split(string(worktreeContent), "\n")
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" {
				continue
			}
			if !strings.HasPrefix(trimmed, "#") {
				t.Errorf("expected all non-blank lines to start with #, found: %s", line)
			}
		}
	})

	t.Run("TestInitGitignoreBlock", func(t *testing.T) {
		// Verify .gitignore exists with managed block
		gitignorePath := filepath.Join(cwd, ".gitignore")
		content, err := os.ReadFile(gitignorePath)
		if err != nil {
			t.Fatalf(".gitignore not created: %v", err)
		}

		contentStr := string(content)

		// Check for start marker
		if !strings.Contains(contentStr, "# === lyx-managed ===") {
			t.Errorf("expected start marker in .gitignore, got: %s", contentStr)
		}

		// Check for end marker
		if !strings.Contains(contentStr, "# === end lyx-managed ===") {
			t.Errorf("expected end marker in .gitignore, got: %s", contentStr)
		}

		// Check for .lyx/ entry
		if !strings.Contains(contentStr, ".lyx/") {
			t.Errorf("expected .lyx/ entry in .gitignore, got: %s", contentStr)
		}
	})

	t.Run("TestInitJSONShape", func(t *testing.T) {
		// Verify JSON output has the correct shape
		var result map[string]any
		if err := json.Unmarshal([]byte(stdout), &result); err != nil {
			t.Fatalf("failed to parse output JSON: %v; stdout: %s", err, stdout)
		}

		// Verify ok=true
		if ok, exists := result["ok"].(bool); !exists || !ok {
			t.Errorf("expected ok=true, got %v", result["ok"])
		}

		// Verify lyx_dir is "created"
		if lyxDir, ok := result["lyx_dir"].(string); !ok || lyxDir != "created" {
			t.Errorf("expected lyx_dir='created', got %v", result["lyx_dir"])
		}

		// Verify board_yaml is "created"
		if boardYaml, ok := result["board_yaml"].(string); !ok || boardYaml != "created" {
			t.Errorf("expected board_yaml='created', got %v", result["board_yaml"])
		}

		// Verify worktree_yaml is "created"
		if worktreeYaml, ok := result["worktree_yaml"].(string); !ok || worktreeYaml != "created" {
			t.Errorf("expected worktree_yaml='created', got %v", result["worktree_yaml"])
		}

		// Verify gitignore is "updated"
		if gitignore, ok := result["gitignore"].(string); !ok || gitignore != "updated" {
			t.Errorf("expected gitignore='updated', got %v", result["gitignore"])
		}

		// Verify no unexpected keys
		expectedKeys := map[string]bool{"ok": true, "lyx_dir": true, "board_yaml": true, "worktree_yaml": true, "gitignore": true}
		for key := range result {
			if !expectedKeys[key] {
				t.Errorf("unexpected key in JSON output: %s", key)
			}
		}
	})
}

// TestInitIdempotent tests that a second run doesn't clobber board.yaml or duplicate the gitignore block.
func TestInitIdempotent(t *testing.T) {
	cwd := t.TempDir()
	t.Chdir(cwd)

	// First run
	exitCode1, stdout1 := runInit(t)
	if exitCode1 != 0 {
		t.Fatalf("first run: expected exit 0, got %d; stdout: %s", exitCode1, stdout1)
	}

	// Capture board.yaml content and mtime
	boardYamlPath := filepath.Join(cwd, "_lyx", "config", "board.yaml")
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
	exitCode2, stdout2 := runInit(t)
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

	if worktreeYaml, ok := result["worktree_yaml"].(string); !ok || worktreeYaml != "exists" {
		t.Errorf("expected worktree_yaml='exists', got %v", result["worktree_yaml"])
	}

	if gitignoreStatus, ok := result["gitignore"].(string); !ok || gitignoreStatus != "unchanged" {
		t.Errorf("expected gitignore='unchanged', got %v", result["gitignore"])
	}

	if lyxDir, ok := result["lyx_dir"].(string); !ok || lyxDir != "exists" {
		t.Errorf("expected lyx_dir='exists', got %v", result["lyx_dir"])
	}
}
