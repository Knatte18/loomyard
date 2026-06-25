// initcli_test.go — tests for the lyx init command.

package initcli_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/board"
	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/initcli"
	"github.com/Knatte18/loomyard/internal/paths"
	"github.com/Knatte18/loomyard/internal/warp"
	"github.com/Knatte18/loomyard/internal/weft"
)

func TestRunInit_FirstRun(t *testing.T) {
	tmpDir := t.TempDir()

	// Initialize a git repo so paths.Resolve works
	_, _, exitCode, initErr := gitexec.RunGit([]string{"init"}, tmpDir)
	if initErr != nil || exitCode != 0 {
		t.Fatalf("git init failed: %v (exit code %d)", initErr, exitCode)
	}

	oldCwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer os.Chdir(oldCwd)

	// Run init
	var buf bytes.Buffer
	runExitCode := initcli.RunInit(&buf, []string{})

	if runExitCode != 0 {
		t.Fatalf("RunInit() = %d; want 0, output: %s", runExitCode, buf.String())
	}

	// Parse and verify JSON
	var result map[string]any
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("parse JSON: %v, output: %s", err, buf.String())
	}

	if ok, _ := result["ok"].(bool); !ok {
		t.Error("ok flag is not true")
	}

	// Verify _lyx/config/ directories exist
	configDir := paths.ConfigDir(tmpDir)
	if _, err := os.Stat(configDir); err != nil {
		t.Fatalf("_lyx/config not created: %v", err)
	}

	// Verify all three config files exist
	for _, module := range []string{"board", "warp", "weft"} {
		cfgPath := paths.ConfigFile(tmpDir, module)
		if _, err := os.Stat(cfgPath); err != nil {
			t.Errorf("%s.yaml not created: %v", module, err)
		}
	}

	// Verify .gitignore has the managed block
	gitignorePath := filepath.Join(tmpDir, ".gitignore")
	content, err := os.ReadFile(gitignorePath)
	if err != nil {
		t.Fatalf(".gitignore not created: %v", err)
	}
	contentStr := string(content)
	if !strings.Contains(contentStr, "# === lyx-managed ===") {
		t.Error(".gitignore missing start marker")
	}
	if !strings.Contains(contentStr, ".lyx/") {
		t.Error(".gitignore missing .lyx/ entry")
	}

	// Verify strict loads pass
	t.Run("StrictLoadsPass", func(t *testing.T) {
		_, err := board.LoadConfig(tmpDir, "board")
		if err != nil {
			t.Errorf("board.LoadConfig failed: %v", err)
		}

		_, err = warp.LoadConfig(tmpDir, "warp")
		if err != nil {
			t.Errorf("warp.LoadConfig failed: %v", err)
		}

		// Weft loads from the same directory in this test
		_, err = weft.LoadConfig(tmpDir)
		if err != nil {
			t.Errorf("weft.LoadConfig failed: %v", err)
		}
	})
}

func TestRunInit_Idempotent(t *testing.T) {
	tmpDir := t.TempDir()

	// Initialize git repo
	_, _, exitCode, initErr := gitexec.RunGit([]string{"init"}, tmpDir)
	if initErr != nil || exitCode != 0 {
		t.Fatalf("git init failed: %v (exit code %d)", initErr, exitCode)
	}

	oldCwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer os.Chdir(oldCwd)

	// First run
	var buf1 bytes.Buffer
	exitCode1 := initcli.RunInit(&buf1, []string{})
	if exitCode1 != 0 {
		t.Fatalf("first RunInit() = %d; want 0, output: %s", exitCode1, buf1.String())
	}

	// Capture files and gitignore after first run
	boardPath := paths.ConfigFile(tmpDir, "board")
	content1, err := os.ReadFile(boardPath)
	if err != nil {
		t.Fatalf("read board.yaml: %v", err)
	}

	gitignorePath := filepath.Join(tmpDir, ".gitignore")
	gitignore1, err := os.ReadFile(gitignorePath)
	if err != nil {
		t.Fatalf("read .gitignore: %v", err)
	}

	// Second run
	var buf2 bytes.Buffer
	exitCode2 := initcli.RunInit(&buf2, []string{})
	if exitCode2 != 0 {
		t.Fatalf("second RunInit() = %d; want 0, output: %s", exitCode2, buf2.String())
	}

	// Verify files unchanged
	content2, err := os.ReadFile(boardPath)
	if err != nil {
		t.Fatalf("read board.yaml after second run: %v", err)
	}
	if string(content1) != string(content2) {
		t.Error("board.yaml changed on second run (should be idempotent)")
	}

	// Verify gitignore unchanged
	gitignore2, err := os.ReadFile(gitignorePath)
	if err != nil {
		t.Fatalf("read .gitignore after second run: %v", err)
	}
	if string(gitignore1) != string(gitignore2) {
		t.Error(".gitignore changed on second run (should be idempotent)")
	}

	// Verify JSON output indicates no changes
	var result map[string]any
	if err := json.Unmarshal(buf2.Bytes(), &result); err != nil {
		t.Fatalf("parse second run JSON: %v", err)
	}

	modules, ok := result["modules"].([]any)
	if !ok {
		t.Error("modules is not an array")
	} else {
		for _, mod := range modules {
			if m, ok := mod.(map[string]any); ok {
				if applied, _ := m["applied"].(bool); applied {
					moduleName, _ := m["module"].(string)
					t.Errorf("module %s reports applied=true on second run (should be idempotent)", moduleName)
				}
			}
		}
	}
}
