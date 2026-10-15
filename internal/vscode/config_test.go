// config_test.go covers config generation and its non-clobbering behavior when .vscode files
// already exist.

package vscode

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestWriteVSCodeConfigCreatesFilesWhenAbsent pins the full generated config: the settings.json keys and the single on-demand reed attach task with its command, args and presentation, and no folder-open trigger.
//
//testtiming:keep pins the whole settings.json and the attach task's shape, which no covering test asserts
func TestWriteVSCodeConfigCreatesFilesWhenAbsent(t *testing.T) {
	dir := t.TempDir()
	lyxPath := "/opt/lyx/bin/lyx"

	if err := WriteConfig(dir, ".", "test-slug", "#2d7d46", lyxPath); err != nil {
		t.Fatalf("WriteConfig failed: %v", err)
	}

	settingsData, err := os.ReadFile(filepath.Join(dir, ".vscode", "settings.json"))
	if err != nil {
		t.Fatalf("failed to read settings.json: %v", err)
	}
	var settings map[string]any
	if err := json.Unmarshal(settingsData, &settings); err != nil {
		t.Fatalf("settings.json is not valid JSON: %v", err)
	}
	if _, ok := settings["workbench.colorCustomizations"]; !ok {
		t.Fatalf("missing workbench.colorCustomizations in settings.json")
	}
	if _, ok := settings["window.title"]; !ok {
		t.Fatalf("missing window.title in settings.json")
	}
	if got := settings["terminal.integrated.defaultLocation"]; got != "editor" {
		t.Errorf("terminal.integrated.defaultLocation = %v; want \"editor\" so the reed attach terminal opens in the main area", got)
	}
	watcherExclude, ok := settings["files.watcherExclude"].(map[string]any)
	if !ok {
		t.Fatalf("files.watcherExclude not found or not a map in settings.json")
	}
	if excludeLyx := watcherExclude["**/_lyx/**"]; excludeLyx != true {
		t.Fatalf("files.watcherExclude[**/_lyx/**] = %v; want true", excludeLyx)
	}

	tasks := readTasksFile(t, dir).Tasks
	if len(tasks) != 1 {
		t.Fatalf("got %d tasks, want exactly 1: %v", len(tasks), tasks)
	}
	task := tasks[0]
	if task["label"] != "reed attach" {
		t.Errorf("label = %v; want reed attach", task["label"])
	}
	if task["command"] != lyxPath {
		t.Errorf("command = %v; want %q", task["command"], lyxPath)
	}
	args, _ := task["args"].([]any)
	if len(args) != 2 || args[0] != "reed" || args[1] != "attach" {
		t.Errorf("args = %v; want [reed attach]", task["args"])
	}
	for _, key := range []string{"runOptions", "dependsOn"} {
		if _, ok := task[key]; ok {
			t.Errorf("task carries %s = %v; want none, so opening the folder runs nothing", key, task[key])
		}
	}
	presentation, ok := task["presentation"].(map[string]any)
	if !ok {
		t.Fatalf("task missing presentation")
	}
	wantPresentation := map[string]any{"reveal": "always", "panel": "new", "echo": true, "focus": true}
	for key, want := range wantPresentation {
		if presentation[key] != want {
			t.Errorf("presentation.%s = %v; want %v", key, presentation[key], want)
		}
	}
}

// tasksFile is the decoded shape of a generated tasks.json.
type tasksFile struct {
	Version string           `json:"version"`
	Tasks   []map[string]any `json:"tasks"`
}

// readTasksFile reads and decodes the tasks.json that WriteConfig wrote under dir.
func readTasksFile(t *testing.T, dir string) tasksFile {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, ".vscode", "tasks.json"))
	if err != nil {
		t.Fatalf("failed to read tasks.json: %v", err)
	}
	var tasks tasksFile
	if err := json.Unmarshal(data, &tasks); err != nil {
		t.Fatalf("tasks.json is not valid JSON: %v", err)
	}
	return tasks
}

// TestWriteConfigFallsBackToBareName pins that an empty lyx path degrades to the bare binary name in the generated task.
func TestWriteConfigFallsBackToBareName(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	if err := WriteConfig(dir, ".", "test-slug", "#2d7d46", ""); err != nil {
		t.Fatalf("WriteConfig failed: %v", err)
	}

	tasks := readTasksFile(t, dir).Tasks
	if len(tasks) != 1 {
		t.Fatalf("got %d tasks, want exactly 1: %v", len(tasks), tasks)
	}
	if tasks[0]["command"] != "lyx" {
		t.Errorf("command = %v; want lyx", tasks[0]["command"])
	}
}

// TestWriteConfigKeepsSettingsAndOverwritesTasks pins that a rerun leaves an existing settings.json untouched and replaces an existing tasks.json.
//
//testtiming:keep pins the untouched settings.json and the overwritten tasks.json, which the gitignore test does not assert
func TestWriteConfigKeepsSettingsAndOverwritesTasks(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	vscodePath := filepath.Join(dir, ".vscode")
	if err := os.MkdirAll(vscodePath, 0o755); err != nil {
		t.Fatalf("failed to create .vscode: %v", err)
	}
	settingsPath := filepath.Join(vscodePath, "settings.json")
	custom := []byte(`{"custom":"value"}`)
	if err := os.WriteFile(settingsPath, custom, 0o644); err != nil {
		t.Fatalf("failed to write original settings.json: %v", err)
	}
	if err := os.WriteFile(filepath.Join(vscodePath, "tasks.json"), []byte(`{"version":"999.0.0"}`), 0o644); err != nil {
		t.Fatalf("failed to write original tasks.json: %v", err)
	}

	if err := WriteConfig(dir, ".", "slug", "#2d7d46", "/opt/lyx/bin/lyx"); err != nil {
		t.Fatalf("WriteConfig failed: %v", err)
	}

	got, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("failed to read settings.json: %v", err)
	}
	if string(got) != string(custom) {
		t.Errorf("settings.json was clobbered: %q", got)
	}
	tasks := readTasksFile(t, dir)
	if tasks.Version != "2.0.0" {
		t.Errorf("tasks.json was not overwritten: version = %v", tasks.Version)
	}
	if len(tasks.Tasks) != 1 || tasks.Tasks[0]["label"] != "reed attach" {
		t.Errorf("tasks = %v; want the single reed attach task", tasks.Tasks)
	}
}

// TestWriteConfigLeavesGitignoreAlone pins that WriteConfig neither creates a .gitignore nor modifies an existing one.
//
//testtiming:keep pins that no .gitignore is created or modified, which its covering tests do not assert
func TestWriteConfigLeavesGitignoreAlone(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	if err := WriteConfig(dir, ".", "slug", "#2d7d46", "/opt/lyx/bin/lyx"); err != nil {
		t.Fatalf("WriteConfig failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".gitignore")); !os.IsNotExist(err) {
		t.Errorf(".gitignore should not exist, stat err = %v", err)
	}

	existing := []byte("build/\n")
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), existing, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := WriteConfig(dir, ".", "slug", "#2d7d46", "/opt/lyx/bin/lyx"); err != nil {
		t.Fatalf("WriteConfig failed: %v", err)
	}
	got, _ := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if string(got) != string(existing) {
		t.Errorf(".gitignore modified: %q", got)
	}
}
