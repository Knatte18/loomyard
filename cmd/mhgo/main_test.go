package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

var binaryPath string

func TestMain(m *testing.M) {
	// Build the binary once for all tests
	tmpDir := os.TempDir()
	binaryPath = filepath.Join(tmpDir, "mhgo-test")
	if os.PathSeparator == '\\' {
		binaryPath += ".exe"
	}

	cmd := exec.Command("go", "build", "-o", binaryPath, "./cmd/mhgo")
	cmd.Dir = "."
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to build binary: %v\n", err)
		os.Exit(1)
	}
	defer os.Remove(binaryPath)

	code := m.Run()
	os.Exit(code)
}

func runMhgo(t *testing.T, wikiPath string, args ...string) (exitCode int, stdout string) {
	t.Helper()

	allArgs := append([]string{"-wiki-path", wikiPath, "wiki"}, args...)
	cmd := exec.Command(binaryPath, allArgs...)
	cmd.Dir = "."
	cmd.Env = append(os.Environ(), "WIKI_SKIP_GIT=1")

	output, err := cmd.CombinedOutput()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return exitErr.ExitCode(), string(output)
		}
		return 1, string(output)
	}
	return 0, string(output)
}

func TestUpsertTask(t *testing.T) {
	wikiPath := t.TempDir()

	// (a) upsert creates a task and returns {"ok":true,"task":{...}}
	payload := `{"slug":"foo","title":"Foo task"}`
	exitCode, stdout := runMhgo(t, wikiPath, "upsert", payload)

	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d; stdout: %s", exitCode, stdout)
	}

	var result map[string]interface{}
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

func TestListTasks(t *testing.T) {
	wikiPath := t.TempDir()

	// First upsert a task
	payload := `{"slug":"foo","title":"Foo task"}`
	exitCode, _ := runMhgo(t, wikiPath, "upsert", payload)
	if exitCode != 0 {
		t.Fatalf("upsert failed")
	}

	// (b) list returns tasks array with layer and has_proposal fields
	exitCode, stdout := runMhgo(t, wikiPath, "list")

	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d; stdout: %s", exitCode, stdout)
	}

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("failed to parse output: %v; stdout: %s", err, stdout)
	}

	if ok, exists := result["ok"].(bool); !exists || !ok {
		t.Fatalf("expected ok=true, got %v", result)
	}

	tasks, ok := result["tasks"].([]interface{})
	if !ok || len(tasks) == 0 {
		t.Fatalf("expected non-empty tasks array, got %v", result)
	}

	// Check first task has layer and has_proposal fields
	taskMap, ok := tasks[0].(map[string]interface{})
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

func TestGetTask(t *testing.T) {
	wikiPath := t.TempDir()

	// First upsert a task
	payload := `{"slug":"foo","title":"Foo task"}`
	runMhgo(t, wikiPath, "upsert", payload)

	// (c) get with existing slug returns task
	exitCode, stdout := runMhgo(t, wikiPath, "get", `{"id_or_slug":"foo"}`)

	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d; stdout: %s", exitCode, stdout)
	}

	var result map[string]interface{}
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

func TestGetNonexistentTask(t *testing.T) {
	wikiPath := t.TempDir()

	// (d) get with nonexistent slug returns null task
	exitCode, stdout := runMhgo(t, wikiPath, "get", `{"id_or_slug":"nonexistent"}`)

	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d; stdout: %s", exitCode, stdout)
	}

	var result map[string]interface{}
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

func TestRemoveNonexistentTask(t *testing.T) {
	wikiPath := t.TempDir()

	// (e) remove nonexistent task returns error with exit 1
	exitCode, stdout := runMhgo(t, wikiPath, "remove", `{"id_or_slug":"nonexistent"}`)

	if exitCode != 1 {
		t.Fatalf("expected exit 1, got %d; stdout: %s", exitCode, stdout)
	}

	var result map[string]interface{}
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

func TestSetPhase(t *testing.T) {
	wikiPath := t.TempDir()

	// First upsert a task
	payload := `{"slug":"foo","title":"Foo task"}`
	runMhgo(t, wikiPath, "upsert", payload)

	// (f) set-phase returns exit 0
	exitCode, stdout := runMhgo(t, wikiPath, "set-phase", `{"id_or_slug":"foo","phase":"active"}`)

	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d; stdout: %s", exitCode, stdout)
	}

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("failed to parse output: %v; stdout: %s", err, stdout)
	}

	if ok, exists := result["ok"].(bool); !exists || !ok {
		t.Fatalf("expected ok=true, got %v", result)
	}
}

func TestRerender(t *testing.T) {
	wikiPath := t.TempDir()

	// (g) rerender returns exit 0 and creates Home.md
	exitCode, stdout := runMhgo(t, wikiPath, "rerender")

	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d; stdout: %s", exitCode, stdout)
	}

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("failed to parse output: %v; stdout: %s", err, stdout)
	}

	if ok, exists := result["ok"].(bool); !exists || !ok {
		t.Fatalf("expected ok=true, got %v", result)
	}

	// Check Home.md exists
	homePath := filepath.Join(wikiPath, "Home.md")
	if _, err := os.Stat(homePath); err != nil {
		t.Fatalf("Home.md not created: %v", err)
	}
}
