// vscode_test.go covers config generation and its non-clobbering behavior when
// .vscode files already exist.

package ide

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestWriteVSCodeConfigCreatesFilesWhenAbsent tests both files are created when absent.
func TestWriteVSCodeConfigCreatesFilesWhenAbsent(t *testing.T) {
	tmpDir := t.TempDir()
	worktreeDir := tmpDir
	relpath := "."
	slug := "test-slug"
	color := "#2d7d46"

	err := writeVSCodeConfig(worktreeDir, relpath, slug, color)
	if err != nil {
		t.Fatalf("writeVSCodeConfig failed: %v", err)
	}

	// Check settings.json exists and is valid
	settingsPath := filepath.Join(worktreeDir, relpath, ".vscode", "settings.json")
	if _, err := os.Stat(settingsPath); err != nil {
		t.Fatalf("settings.json not created: %v", err)
	}

	settingsData, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("failed to read settings.json: %v", err)
	}

	var settings map[string]any
	if err := json.Unmarshal(settingsData, &settings); err != nil {
		t.Fatalf("settings.json is not valid JSON: %v", err)
	}

	// Verify expected keys
	if _, ok := settings["workbench.colorCustomizations"]; !ok {
		t.Fatalf("missing workbench.colorCustomizations in settings.json")
	}
	if _, ok := settings["window.title"]; !ok {
		t.Fatalf("missing window.title in settings.json")
	}

	// Check tasks.json exists and is valid
	tasksPath := filepath.Join(worktreeDir, relpath, ".vscode", "tasks.json")
	if _, err := os.Stat(tasksPath); err != nil {
		t.Fatalf("tasks.json not created: %v", err)
	}

	tasksData, err := os.ReadFile(tasksPath)
	if err != nil {
		t.Fatalf("failed to read tasks.json: %v", err)
	}

	var tasks map[string]any
	if err := json.Unmarshal(tasksData, &tasks); err != nil {
		t.Fatalf("tasks.json is not valid JSON: %v", err)
	}

	// Verify tasks structure
	tasksList, ok := tasks["tasks"].([]any)
	if !ok {
		t.Fatalf("tasks.json missing tasks array")
	}

	if len(tasksList) == 0 {
		t.Fatalf("tasks.json has no tasks")
	}

	// Verify the first task has "Start Claude" label
	firstTask, ok := tasksList[0].(map[string]any)
	if !ok {
		t.Fatalf("first task is not a map")
	}

	if label, ok := firstTask["label"].(string); !ok || label != "Start Claude" {
		t.Fatalf("expected label 'Start Claude', got %v", firstTask["label"])
	}

	// Verify folderOpen task
	if runOptions, ok := firstTask["runOptions"].(map[string]any); !ok {
		t.Fatalf("missing runOptions in task")
	} else {
		if runOn, ok := runOptions["runOn"].(string); !ok || runOn != "folderOpen" {
			t.Fatalf("expected runOn 'folderOpen', got %v", runOptions["runOn"])
		}
	}
}

// TestWriteVSCodeConfigDoesNotClobber tests that existing files are not clobbered.
func TestWriteVSCodeConfigDoesNotClobber(t *testing.T) {
	tmpDir := t.TempDir()
	worktreeDir := tmpDir
	relpath := "."
	slug := "test-slug"
	color := "#2d7d46"

	// Create .vscode directory and existing settings.json
	vscodePath := filepath.Join(worktreeDir, relpath, ".vscode")
	if err := os.MkdirAll(vscodePath, 0o755); err != nil {
		t.Fatalf("failed to create .vscode: %v", err)
	}

	// Write a settings.json with custom content
	originalSettings := map[string]any{"custom": "value"}
	originalData, _ := json.Marshal(originalSettings)
	settingsPath := filepath.Join(vscodePath, "settings.json")
	if err := os.WriteFile(settingsPath, originalData, 0o644); err != nil {
		t.Fatalf("failed to write original settings.json: %v", err)
	}

	// Write a tasks.json with custom content
	originalTasks := map[string]any{"version": "999.0.0"}
	originalTasksData, _ := json.Marshal(originalTasks)
	tasksPath := filepath.Join(vscodePath, "tasks.json")
	if err := os.WriteFile(tasksPath, originalTasksData, 0o644); err != nil {
		t.Fatalf("failed to write original tasks.json: %v", err)
	}

	// Call writeVSCodeConfig
	err := writeVSCodeConfig(worktreeDir, relpath, slug, color)
	if err != nil {
		t.Fatalf("writeVSCodeConfig failed: %v", err)
	}

	// Verify settings.json was not modified
	settingsData, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("failed to read settings.json: %v", err)
	}

	var settings map[string]any
	if err := json.Unmarshal(settingsData, &settings); err != nil {
		t.Fatalf("settings.json is not valid JSON: %v", err)
	}

	if settings["custom"] != "value" {
		t.Fatalf("settings.json was clobbered")
	}

	// Verify tasks.json was not modified
	tasksData, err := os.ReadFile(tasksPath)
	if err != nil {
		t.Fatalf("failed to read tasks.json: %v", err)
	}

	var tasks map[string]any
	if err := json.Unmarshal(tasksData, &tasks); err != nil {
		t.Fatalf("tasks.json is not valid JSON: %v", err)
	}

	if tasks["version"] != "999.0.0" {
		t.Fatalf("tasks.json was clobbered")
	}
}

// TestWriteVSCodeConfigRegistersInGitignore tests .vscode/ is registered in .gitignore.
func TestWriteVSCodeConfigRegistersInGitignore(t *testing.T) {
	tmpDir := t.TempDir()
	worktreeDir := tmpDir
	relpath := "."
	slug := "test-slug"
	color := "#2d7d46"

	err := writeVSCodeConfig(worktreeDir, relpath, slug, color)
	if err != nil {
		t.Fatalf("writeVSCodeConfig failed: %v", err)
	}

	// Check .gitignore exists and contains .vscode/
	gitignorePath := filepath.Join(worktreeDir, relpath, ".gitignore")
	if _, err := os.Stat(gitignorePath); err != nil {
		t.Fatalf(".gitignore not created: %v", err)
	}

	gitignoreContent, err := os.ReadFile(gitignorePath)
	if err != nil {
		t.Fatalf("failed to read .gitignore: %v", err)
	}

	content := string(gitignoreContent)
	if !strings.Contains(content, ".vscode/") {
		t.Fatalf(".gitignore does not contain '.vscode/' entry")
	}
}
