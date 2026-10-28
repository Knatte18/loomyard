//go:build tier0

// config_test.go covers config generation: the settings.json written fresh, lym's own keys set in an existing one, and the tasks.json overwritten.

package vscode

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// testLook is a look with a light worktree colour, so the two bars take different text colours.
var testLook = Look{Title: "TST: test-slug", ProjectColor: PrimeColor, WorktreeColor: "#ffeb3b"}

// assertLook asserts settings carry testLook's window title and exactly lym's title-bar and activity-bar colour keys.
func assertLook(t *testing.T, settings map[string]any) {
	t.Helper()
	if got := settings["window.title"]; got != testLook.Title {
		t.Errorf("window.title = %v; want %q", got, testLook.Title)
	}
	want := map[string]any{
		"titleBar.activeBackground":      PrimeColor,
		"titleBar.inactiveBackground":    PrimeColor,
		"titleBar.activeForeground":      "#ffffff",
		"titleBar.inactiveForeground":    "#ffffffaa",
		"activityBar.background":         "#ffeb3b",
		"activityBar.foreground":         "#000000",
		"activityBar.inactiveForeground": "#000000aa",
	}
	if got := settings["workbench.colorCustomizations"]; !reflect.DeepEqual(got, want) {
		t.Errorf("workbench.colorCustomizations = %v; want %v", got, want)
	}
}

// TestWriteVSCodeConfigCreatesFilesWhenAbsent pins the full generated config: the settings.json keys and the single on-demand reed attach task with its command, args and presentation, and no folder-open trigger.
//
//testtiming:keep pins the whole settings.json and the attach task's shape, which no covering test asserts
func TestWriteVSCodeConfigCreatesFilesWhenAbsent(t *testing.T) {
	dir := t.TempDir()
	lymPath := "/opt/lym/bin/lym"

	if err := WriteConfig(dir, ".", testLook, lymPath); err != nil {
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
	assertLook(t, settings)
	if got := settings["terminal.integrated.defaultLocation"]; got != "editor" {
		t.Errorf("terminal.integrated.defaultLocation = %v; want \"editor\" so the reed attach terminal opens in the main area", got)
	}
	watcherExclude, ok := settings["files.watcherExclude"].(map[string]any)
	if !ok {
		t.Fatalf("files.watcherExclude not found or not a map in settings.json")
	}
	if excludeLym := watcherExclude["**/_lym/**"]; excludeLym != true {
		t.Fatalf("files.watcherExclude[**/_lym/**] = %v; want true", excludeLym)
	}

	tasks := readTasksFile(t, dir).Tasks
	if len(tasks) != 1 {
		t.Fatalf("got %d tasks, want exactly 1: %v", len(tasks), tasks)
	}
	task := tasks[0]
	if task["label"] != "reed attach" {
		t.Errorf("label = %v; want reed attach", task["label"])
	}
	if task["command"] != lymPath {
		t.Errorf("command = %v; want %q", task["command"], lymPath)
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

// TestWriteConfigFallsBackToBareName pins that an empty lym path degrades to the bare binary name in the generated task.
func TestWriteConfigFallsBackToBareName(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	if err := WriteConfig(dir, ".", testLook, ""); err != nil {
		t.Fatalf("WriteConfig failed: %v", err)
	}

	tasks := readTasksFile(t, dir).Tasks
	if len(tasks) != 1 {
		t.Fatalf("got %d tasks, want exactly 1: %v", len(tasks), tasks)
	}
	if tasks[0]["command"] != "lym" {
		t.Errorf("command = %v; want lym", tasks[0]["command"])
	}
}

// TestWriteConfigRewritesOwnSettingsKeysAndOverwritesTasks pins that a rerun sets lym's title and colour keys in an existing settings.json, keeping its other keys and colour customizations, and replaces an existing tasks.json.
//
//testtiming:keep pins the kept settings keys and the overwritten tasks.json, which the gitignore test does not assert
func TestWriteConfigRewritesOwnSettingsKeysAndOverwritesTasks(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	vscodePath := filepath.Join(dir, ".vscode")
	if err := os.MkdirAll(vscodePath, 0o755); err != nil {
		t.Fatalf("failed to create .vscode: %v", err)
	}
	settingsPath := filepath.Join(vscodePath, "settings.json")
	existing := "{\n  // operator note\n  \"custom\": \"value\",\n  \"window.title\": \"old\",\n  \"workbench.colorCustomizations\": {\"titleBar.activeBackground\": \"#2d7d46\", \"statusBar.background\": \"#123456\"},\n}"
	if err := os.WriteFile(settingsPath, []byte(existing), 0o644); err != nil {
		t.Fatalf("failed to write original settings.json: %v", err)
	}
	if err := os.WriteFile(filepath.Join(vscodePath, "tasks.json"), []byte(`{"version":"999.0.0"}`), 0o644); err != nil {
		t.Fatalf("failed to write original tasks.json: %v", err)
	}

	if err := WriteConfig(dir, ".", testLook, "/opt/lym/bin/lym"); err != nil {
		t.Fatalf("WriteConfig failed: %v", err)
	}

	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("failed to read settings.json: %v", err)
	}
	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("settings.json is not valid JSON: %v\n%s", err, data)
	}
	if settings["custom"] != "value" {
		t.Errorf("custom = %v; want the operator's value kept", settings["custom"])
	}
	customizations, _ := settings["workbench.colorCustomizations"].(map[string]any)
	if customizations["statusBar.background"] != "#123456" {
		t.Errorf("statusBar.background = %v; want the operator's value kept", customizations["statusBar.background"])
	}
	delete(customizations, "statusBar.background")
	assertLook(t, settings)
	if _, ok := settings["files.watcherExclude"]; ok {
		t.Errorf("files.watcherExclude added to an existing settings.json; want only lym's title and colour keys set")
	}
	tasks := readTasksFile(t, dir)
	if tasks.Version != "2.0.0" {
		t.Errorf("tasks.json was not overwritten: version = %v", tasks.Version)
	}
	if len(tasks.Tasks) != 1 || tasks.Tasks[0]["label"] != "reed attach" {
		t.Errorf("tasks = %v; want the single reed attach task", tasks.Tasks)
	}
}

// TestWriteConfigLeavesUndecodableSettingsAlone pins that a settings.json that is not a JSON object keeps its bytes while tasks.json is still written.
func TestWriteConfigLeavesUndecodableSettingsAlone(t *testing.T) {
	t.Parallel()
	for _, content := range []string{"{not json", "[1, 2]", "null"} {
		dir := t.TempDir()
		settingsPath := filepath.Join(dir, ".vscode", "settings.json")
		if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(settingsPath, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}

		if err := WriteConfig(dir, ".", testLook, "/opt/lym/bin/lym"); err != nil {
			t.Fatalf("WriteConfig(%q settings) failed: %v", content, err)
		}

		if got, _ := os.ReadFile(settingsPath); string(got) != content {
			t.Errorf("settings.json = %q; want %q untouched", got, content)
		}
		if tasks := readTasksFile(t, dir); len(tasks.Tasks) != 1 {
			t.Errorf("tasks = %v; want the single reed attach task", tasks.Tasks)
		}
	}
}

// TestWriteConfigLeavesGitignoreAlone pins that WriteConfig neither creates a .gitignore nor modifies an existing one.
//
//testtiming:keep pins that no .gitignore is created or modified, which its covering tests do not assert
func TestWriteConfigLeavesGitignoreAlone(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	if err := WriteConfig(dir, ".", testLook, "/opt/lym/bin/lym"); err != nil {
		t.Fatalf("WriteConfig failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".gitignore")); !os.IsNotExist(err) {
		t.Errorf(".gitignore should not exist, stat err = %v", err)
	}

	existing := []byte("build/\n")
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), existing, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := WriteConfig(dir, ".", testLook, "/opt/lym/bin/lym"); err != nil {
		t.Fatalf("WriteConfig failed: %v", err)
	}
	got, _ := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if string(got) != string(existing) {
		t.Errorf(".gitignore modified: %q", got)
	}
}
