// cli_test.go — tests for the board CLI (cli.go).
//
// Drives RunCLI in-process and asserts the JSON + exit-code contract for each
// subcommand (upsert, list, get, set-phase, remove, rerender).

package board_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/board"
)

// seedCwd creates a temp directory with _lyx/config/board.yaml seeded (path: board),
// changes to that directory, and returns the cwd path. The caller must restore
// the original cwd after the test (or use t.Chdir).
func seedCwd(t *testing.T) string {
	t.Helper()

	cwd := t.TempDir()
	lyxDir := filepath.Join(cwd, "_lyx")
	if err := os.MkdirAll(lyxDir, 0o755); err != nil {
		t.Fatalf("failed to create _lyx: %v", err)
	}

	configDir := filepath.Join(lyxDir, "config")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("failed to create _lyx/config: %v", err)
	}

	configPath := filepath.Join(configDir, "board.yaml")
	if err := os.WriteFile(configPath, []byte("path: board\n"), 0o644); err != nil {
		t.Fatalf("failed to write board.yaml: %v", err)
	}

	t.Chdir(cwd)
	return cwd
}

// runCLI invokes board.RunCLI in-process and returns the exit code plus the JSON
// written to out. Caller must have called seedCwd and t.Chdir to set up the cwd.
// BOARD_SKIP_GIT must be set by the caller.
func runCLI(t *testing.T, args ...string) (exitCode int, stdout string) {
	t.Helper()

	var buf bytes.Buffer
	code := board.RunCLI(&buf, args)
	return code, buf.String()
}

func TestCLIUpsertTask(t *testing.T) {
	t.Setenv("BOARD_SKIP_GIT", "1")
	seedCwd(t)

	// (a) upsert creates a task and returns {"ok":true,"task":{...}}
	payload := `{"slug":"foo","title":"Foo task"}`
	exitCode, stdout := runCLI(t, "upsert", payload)

	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d; stdout: %s", exitCode, stdout)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("failed to parse output: %v; stdout: %s", err, stdout)
	}

	if ok, exists := result["ok"].(bool); !exists || !ok {
		t.Fatalf("expected ok=true, got %v", result)
	}

	if _, exists := result["task"]; !exists {
		t.Fatalf("expected task in result, got %v", result)
	}
}

func TestCLIListTasks(t *testing.T) {
	t.Setenv("BOARD_SKIP_GIT", "1")
	seedCwd(t)

	// First upsert a task
	payload := `{"slug":"foo","title":"Foo task"}`
	exitCode, _ := runCLI(t, "upsert", payload)
	if exitCode != 0 {
		t.Fatalf("upsert failed")
	}

	// (b) list returns tasks array with layer and has_proposal fields
	exitCode, stdout := runCLI(t, "list")

	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d; stdout: %s", exitCode, stdout)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("failed to parse output: %v; stdout: %s", err, stdout)
	}

	if ok, exists := result["ok"].(bool); !exists || !ok {
		t.Fatalf("expected ok=true, got %v", result)
	}

	tasks, ok := result["tasks"].([]any)
	if !ok || len(tasks) == 0 {
		t.Fatalf("expected non-empty tasks array, got %v", result)
	}

	// Check first task has layer and has_proposal fields
	taskMap, ok := tasks[0].(map[string]any)
	if !ok {
		t.Fatalf("expected task to be map, got %T", tasks[0])
	}

	if _, exists := taskMap["layer"]; !exists {
		t.Fatalf("expected layer field, got %v", taskMap)
	}

	if _, exists := taskMap["has_proposal"]; !exists {
		t.Fatalf("expected has_proposal field, got %v", taskMap)
	}
}

func TestCLIGetTask(t *testing.T) {
	t.Setenv("BOARD_SKIP_GIT", "1")
	seedCwd(t)

	// First upsert a task
	payload := `{"slug":"foo","title":"Foo task"}`
	runCLI(t, "upsert", payload)

	// (c) get with existing slug returns task
	exitCode, stdout := runCLI(t, "get", `{"id_or_slug":"foo"}`)

	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d; stdout: %s", exitCode, stdout)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("failed to parse output: %v; stdout: %s", err, stdout)
	}

	if ok, exists := result["ok"].(bool); !exists || !ok {
		t.Fatalf("expected ok=true, got %v", result)
	}

	task, ok := result["task"]
	if !ok || task == nil {
		t.Fatalf("expected non-null task, got %v", result)
	}
}

func TestCLIGetNonexistentTask(t *testing.T) {
	t.Setenv("BOARD_SKIP_GIT", "1")
	seedCwd(t)

	// (d) get with nonexistent slug returns null task
	exitCode, stdout := runCLI(t, "get", `{"id_or_slug":"nonexistent"}`)

	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d; stdout: %s", exitCode, stdout)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("failed to parse output: %v; stdout: %s", err, stdout)
	}

	if ok, exists := result["ok"].(bool); !exists || !ok {
		t.Fatalf("expected ok=true, got %v", result)
	}

	if task, exists := result["task"]; !exists || task != nil {
		t.Fatalf("expected null task, got %v", result)
	}
}

func TestCLIRemoveNonexistentTask(t *testing.T) {
	t.Setenv("BOARD_SKIP_GIT", "1")
	seedCwd(t)

	// (e) remove nonexistent task returns error with exit 1
	exitCode, stdout := runCLI(t, "remove", `{"id_or_slug":"nonexistent"}`)

	if exitCode != 1 {
		t.Fatalf("expected exit 1, got %d; stdout: %s", exitCode, stdout)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("failed to parse output: %v; stdout: %s", err, stdout)
	}

	if ok, exists := result["ok"].(bool); !exists || ok {
		t.Fatalf("expected ok=false, got %v", result)
	}

	if errMsg, exists := result["error"]; !exists || errMsg == nil {
		t.Fatalf("expected error message, got %v", result)
	}
}

func TestCLISetPhase(t *testing.T) {
	t.Setenv("BOARD_SKIP_GIT", "1")
	seedCwd(t)

	// First upsert a task
	payload := `{"slug":"foo","title":"Foo task"}`
	runCLI(t, "upsert", payload)

	// (f) set-phase returns exit 0
	exitCode, stdout := runCLI(t, "set-phase", `{"id_or_slug":"foo","phase":"active"}`)

	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d; stdout: %s", exitCode, stdout)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("failed to parse output: %v; stdout: %s", err, stdout)
	}

	if ok, exists := result["ok"].(bool); !exists || !ok {
		t.Fatalf("expected ok=true, got %v", result)
	}
}

func TestCLIRerender(t *testing.T) {
	t.Setenv("BOARD_SKIP_GIT", "1")
	cwd := seedCwd(t)

	// (g) rerender returns exit 0 and creates Home.md
	exitCode, stdout := runCLI(t, "rerender")

	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d; stdout: %s", exitCode, stdout)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("failed to parse output: %v; stdout: %s", err, stdout)
	}

	if ok, exists := result["ok"].(bool); !exists || !ok {
		t.Fatalf("expected ok=true, got %v", result)
	}

	// Check Home.md exists in <cwd>/board/
	homePath := filepath.Join(cwd, "board", "Home.md")
	if _, err := os.Stat(homePath); err != nil {
		t.Fatalf("Home.md not created: %v", err)
	}
}

func TestCLINotInitialized(t *testing.T) {
	t.Setenv("BOARD_SKIP_GIT", "1")
	// Do NOT call seedCwd: cwd has no _lyx/
	cwd := t.TempDir()
	t.Chdir(cwd)

	// (h) Running a command from cwd without _lyx/ returns exit 1 with error
	exitCode, stdout := runCLI(t, "list")

	if exitCode != 1 {
		t.Fatalf("expected exit 1, got %d; stdout: %s", exitCode, stdout)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("failed to parse output: %v; stdout: %s", err, stdout)
	}

	if ok, exists := result["ok"].(bool); !exists || ok {
		t.Fatalf("expected ok=false, got %v", result)
	}

	if errMsg, exists := result["error"].(string); !exists {
		t.Fatalf("expected error message, got %v", result)
	} else if !strings.Contains(errMsg, "not initialized") {
		t.Fatalf("expected error to contain 'not initialized', got %q", errMsg)
	}
}
