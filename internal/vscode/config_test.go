// config_test.go covers config generation and its non-clobbering behavior when .vscode files
// already exist.

package vscode

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteVSCodeConfigCreatesFilesWhenAbsent(t *testing.T) {
	tmpDir := t.TempDir()
	worktreeDir := tmpDir
	relpath := "."
	slug := "test-slug"
	color := "#2d7d46"
	lyxPath := "/opt/lyx/bin/lyx"
	claudePath := "/usr/local/bin/claude"

	err := WriteConfig(worktreeDir, relpath, slug, color, lyxPath, claudePath)
	if err != nil {
		t.Fatalf("WriteConfig failed: %v", err)
	}

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

	if _, ok := settings["workbench.colorCustomizations"]; !ok {
		t.Fatalf("missing workbench.colorCustomizations in settings.json")
	}
	if _, ok := settings["window.title"]; !ok {
		t.Fatalf("missing window.title in settings.json")
	}

	watcherExclude, ok := settings["files.watcherExclude"].(map[string]any)
	if !ok {
		t.Fatalf("files.watcherExclude not found or not a map in settings.json")
	}
	if excludeLyx, ok := watcherExclude["**/_lyx/**"]; !ok {
		t.Fatalf("**/_lyx/** key missing from files.watcherExclude")
	} else if excludeLyx != true {
		t.Fatalf("**/_lyx/** value is not true, got %v", excludeLyx)
	}

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

	tasksList, ok := tasks["tasks"].([]any)
	if !ok {
		t.Fatalf("tasks.json missing tasks array")
	}

	if len(tasksList) != 4 {
		t.Fatalf("expected 4 tasks (3 steps + entry), got %d", len(tasksList))
	}

	byLabel := make(map[string]map[string]any, len(tasksList))
	for _, raw := range tasksList {
		task, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("task is not a map: %v", raw)
		}
		label, ok := task["label"].(string)
		if !ok {
			t.Fatalf("task missing string label: %v", task)
		}
		byLabel[label] = task
	}

	entryTask, ok := byLabel["Start Claude"]
	if !ok {
		t.Fatalf("missing entry task labelled 'Start Claude'")
	}

	// The entry task carries exactly four keys and holds no type, command, or presentation
	// of its own.
	wantEntryKeys := map[string]bool{"label": true, "dependsOn": true, "dependsOrder": true, "runOptions": true}
	if len(entryTask) != len(wantEntryKeys) {
		t.Fatalf("entry task has %d keys, want exactly %v", len(entryTask), wantEntryKeys)
	}
	for key := range entryTask {
		if !wantEntryKeys[key] {
			t.Fatalf("entry task has unexpected key %q", key)
		}
	}

	if order, ok := entryTask["dependsOrder"].(string); !ok || order != "sequence" {
		t.Fatalf("expected entry task dependsOrder 'sequence', got %v", entryTask["dependsOrder"])
	}

	dependsOnRaw, ok := entryTask["dependsOn"].([]any)
	if !ok {
		t.Fatalf("entry task dependsOn is not an array: %v", entryTask["dependsOn"])
	}
	var dependsOn []string
	for _, d := range dependsOnRaw {
		s, ok := d.(string)
		if !ok {
			t.Fatalf("dependsOn entry is not a string: %v", d)
		}
		dependsOn = append(dependsOn, s)
	}
	wantOrder := []string{"reed up", "reed add claude", "reed attach"}
	if len(dependsOn) != len(wantOrder) {
		t.Fatalf("entry task dependsOn = %v; want %v", dependsOn, wantOrder)
	}
	for i, want := range wantOrder {
		if dependsOn[i] != want {
			t.Fatalf("entry task dependsOn[%d] = %q; want %q", i, dependsOn[i], want)
		}
	}

	if runOptions, ok := entryTask["runOptions"].(map[string]any); !ok {
		t.Fatalf("missing runOptions in entry task")
	} else {
		if runOn, ok := runOptions["runOn"].(string); !ok || runOn != "folderOpen" {
			t.Fatalf("expected runOn 'folderOpen', got %v", runOptions["runOn"])
		}
	}

	upTask, ok := byLabel["reed up"]
	if !ok {
		t.Fatalf("missing step task labelled 'reed up'")
	}
	assertStepTaskCommand(t, "reed up", upTask, lyxPath)
	assertStepTaskArgs(t, "reed up", upTask, []string{"reed", "up"})
	assertPresentation(t, "reed up", upTask, "silent", "shared", false)

	addTask, ok := byLabel["reed add claude"]
	if !ok {
		t.Fatalf("missing step task labelled 'reed add claude'")
	}
	assertStepTaskCommand(t, "reed add claude", addTask, lyxPath)
	assertStepTaskArgs(t, "reed add claude", addTask, []string{
		"reed", "add", "--if-absent", "--cmd", claudePath, "--name", "claude", "--focus",
	})
	assertPresentation(t, "reed add claude", addTask, "silent", "shared", false)

	attachTask, ok := byLabel["reed attach"]
	if !ok {
		t.Fatalf("missing step task labelled 'reed attach'")
	}
	assertStepTaskCommand(t, "reed attach", attachTask, lyxPath)
	assertStepTaskArgs(t, "reed attach", attachTask, []string{"reed", "attach"})
	assertPresentation(t, "reed attach", attachTask, "always", "new", true)
}

// assertStepTaskCommand asserts that a step task's command matches the given resolved path.
func assertStepTaskCommand(t *testing.T, label string, task map[string]any, wantCommand string) {
	t.Helper()
	command, ok := task["command"].(string)
	if !ok || command != wantCommand {
		t.Errorf("task %q command = %v; want %q", label, task["command"], wantCommand)
	}
}

// assertStepTaskArgs asserts that a step task's args array matches wantArgs exactly.
func assertStepTaskArgs(t *testing.T, label string, task map[string]any, wantArgs []string) {
	t.Helper()
	argsRaw, ok := task["args"].([]any)
	if !ok {
		t.Fatalf("task %q args is not an array: %v", label, task["args"])
	}
	var args []string
	for _, a := range argsRaw {
		s, ok := a.(string)
		if !ok {
			t.Fatalf("task %q has non-string arg: %v", label, a)
		}
		args = append(args, s)
	}
	if len(args) != len(wantArgs) {
		t.Fatalf("task %q args = %v; want %v", label, args, wantArgs)
	}
	for i, want := range wantArgs {
		if args[i] != want {
			t.Errorf("task %q args[%d] = %q; want %q", label, i, args[i], want)
		}
	}
}

// assertPresentation asserts a step task's presentation block matches the given reveal, panel,
// and focus values, with echo always expected true.
func assertPresentation(t *testing.T, label string, task map[string]any, wantReveal, wantPanel string, wantFocus bool) {
	t.Helper()
	presentation, ok := task["presentation"].(map[string]any)
	if !ok {
		t.Fatalf("task %q missing presentation", label)
	}
	if reveal, ok := presentation["reveal"].(string); !ok || reveal != wantReveal {
		t.Errorf("task %q presentation.reveal = %v; want %q", label, presentation["reveal"], wantReveal)
	}
	if panel, ok := presentation["panel"].(string); !ok || panel != wantPanel {
		t.Errorf("task %q presentation.panel = %v; want %q", label, presentation["panel"], wantPanel)
	}
	if echo, ok := presentation["echo"].(bool); !ok || !echo {
		t.Errorf("task %q presentation.echo = %v; want true", label, presentation["echo"])
	}
	if focus, ok := presentation["focus"].(bool); !ok || focus != wantFocus {
		t.Errorf("task %q presentation.focus = %v; want %v", label, presentation["focus"], wantFocus)
	}
}

func TestWriteVSCodeConfigFallsBackToBareLyxName(t *testing.T) {
	tmpDir := t.TempDir()

	if err := WriteConfig(tmpDir, ".", "test-slug", "#2d7d46", "", "/usr/local/bin/claude"); err != nil {
		t.Fatalf("WriteConfig failed: %v", err)
	}

	tasks := readTasksList(t, filepath.Join(tmpDir, ".vscode", "tasks.json"))
	for _, raw := range tasks {
		task := raw.(map[string]any)
		if task["label"] == "Start Claude" {
			continue
		}
		if command, ok := task["command"].(string); !ok || command != "lyx" {
			t.Errorf("task %v command = %v; want bare name 'lyx'", task["label"], task["command"])
		}
	}
}

func TestWriteVSCodeConfigFallsBackToBareClaudeName(t *testing.T) {
	tmpDir := t.TempDir()

	if err := WriteConfig(tmpDir, ".", "test-slug", "#2d7d46", "/opt/lyx/bin/lyx", ""); err != nil {
		t.Fatalf("WriteConfig failed: %v", err)
	}

	tasks := readTasksList(t, filepath.Join(tmpDir, ".vscode", "tasks.json"))
	var addTask map[string]any
	for _, raw := range tasks {
		task := raw.(map[string]any)
		if task["label"] == "reed add claude" {
			addTask = task
		}
	}
	if addTask == nil {
		t.Fatalf("missing 'reed add claude' task")
	}
	argsRaw := addTask["args"].([]any)
	var args []string
	for _, a := range argsRaw {
		args = append(args, a.(string))
	}
	found := false
	for i, a := range args {
		if a == "--cmd" && i+1 < len(args) {
			if args[i+1] != "claude" {
				t.Errorf("--cmd arg = %q; want bare name 'claude'", args[i+1])
			}
			found = true
		}
	}
	if !found {
		t.Fatalf("--cmd flag not found in args %v", args)
	}
}

// readTasksList reads and parses a generated tasks.json, returning its "tasks" array. It fails
// the test on any I/O or JSON error, or if the file does not contain valid JSON.
func readTasksList(t *testing.T, tasksPath string) []any {
	t.Helper()
	data, err := os.ReadFile(tasksPath)
	if err != nil {
		t.Fatalf("failed to read tasks.json: %v", err)
	}
	var tasks map[string]any
	if err := json.Unmarshal(data, &tasks); err != nil {
		t.Fatalf("tasks.json is not valid JSON: %v", err)
	}
	tasksList, ok := tasks["tasks"].([]any)
	if !ok {
		t.Fatalf("tasks.json missing tasks array")
	}
	return tasksList
}

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

	originalSettings := map[string]any{"custom": "value"}
	originalData, _ := json.Marshal(originalSettings)
	settingsPath := filepath.Join(vscodePath, "settings.json")
	if err := os.WriteFile(settingsPath, originalData, 0o644); err != nil {
		t.Fatalf("failed to write original settings.json: %v", err)
	}

	originalTasks := map[string]any{"version": "999.0.0"}
	originalTasksData, _ := json.Marshal(originalTasks)
	tasksPath := filepath.Join(vscodePath, "tasks.json")
	if err := os.WriteFile(tasksPath, originalTasksData, 0o644); err != nil {
		t.Fatalf("failed to write original tasks.json: %v", err)
	}

	// Call WriteConfig
	err := WriteConfig(worktreeDir, relpath, slug, color, "/opt/lyx/bin/lyx", "/usr/local/bin/claude")
	if err != nil {
		t.Fatalf("WriteConfig failed: %v", err)
	}

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

func TestWriteVSCodeConfigRegistersInGitignore(t *testing.T) {
	tmpDir := t.TempDir()
	worktreeDir := tmpDir
	relpath := "."
	slug := "test-slug"
	color := "#2d7d46"

	err := WriteConfig(worktreeDir, relpath, slug, color, "/opt/lyx/bin/lyx", "/usr/local/bin/claude")
	if err != nil {
		t.Fatalf("WriteConfig failed: %v", err)
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
