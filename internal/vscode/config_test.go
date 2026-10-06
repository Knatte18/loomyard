// config_test.go covers config generation and its non-clobbering behavior when .vscode files
// already exist.

package vscode

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// TestWriteVSCodeConfigCreatesFilesWhenAbsent pins the full generated interactive config: the
// settings.json keys and the four-task chain with its entry task, commands, args and presentation.
//
//testtiming:keep pins the whole interactive settings.json and four-task chain shape, which no covering test asserts
func TestWriteVSCodeConfigCreatesFilesWhenAbsent(t *testing.T) {
	tmpDir := t.TempDir()
	worktreeDir := tmpDir
	relpath := "."
	slug := "test-slug"
	color := "#2d7d46"
	lyxPath := "/opt/lyx/bin/lyx"
	claudePath := "/usr/local/bin/claude"

	err := WriteConfig(worktreeDir, relpath, slug, color, lyxPath, claudePath, TaskChainInteractive)
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
	if got := settings["terminal.integrated.defaultLocation"]; got != "editor" {
		t.Errorf("terminal.integrated.defaultLocation = %v; want \"editor\" so the reed attach terminal opens in the main area", got)
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
		"reed", "add", "--if-absent", "--unless-name", "orch", "--cmd", claudePath, "--name", "claude", "--focus",
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

// taskLabels returns the sorted labels of tasks.
func taskLabels(tasks []map[string]any) []string {
	labels := make([]string, 0, len(tasks))
	for _, task := range tasks {
		label, _ := task["label"].(string)
		labels = append(labels, label)
	}
	slices.Sort(labels)
	return labels
}

// TestWriteConfigFallsBackToBareNames pins that an empty lyx or claude path degrades to the bare
// binary name in the generated tasks, for both task chains.
func TestWriteConfigFallsBackToBareNames(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		chain         TaskChain
		lyxPath       string
		claudePath    string
		wantCommand   string
		wantClaudeCmd string
		wantTaskCount int
	}{
		{"interactive empty lyx path", TaskChainInteractive, "", "/usr/local/bin/claude", "lyx", "", 0},
		{"interactive empty claude path", TaskChainInteractive, "/opt/lyx/bin/lyx", "", "/opt/lyx/bin/lyx", "claude", 0},
		{"attach-only empty lyx path", TaskChainAttachOnly, "", "", "lyx", "", 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()

			if err := WriteConfig(dir, ".", "test-slug", "#2d7d46", tt.lyxPath, tt.claudePath, tt.chain); err != nil {
				t.Fatalf("WriteConfig failed: %v", err)
			}

			tasks := readTasksFile(t, dir).Tasks
			if tt.wantTaskCount != 0 && len(tasks) != tt.wantTaskCount {
				t.Fatalf("got %d tasks, want %d: %v", len(tasks), tt.wantTaskCount, tasks)
			}
			var addTask map[string]any
			for _, task := range tasks {
				switch task["label"] {
				case "Start Claude":
					continue
				case "reed add claude":
					addTask = task
				}
				if task["command"] != tt.wantCommand {
					t.Errorf("task %v command = %v; want %q", task["label"], task["command"], tt.wantCommand)
				}
			}
			if tt.wantClaudeCmd == "" {
				return
			}
			if addTask == nil {
				t.Fatalf("missing 'reed add claude' task")
			}
			args, _ := addTask["args"].([]any)
			cmdIndex := slices.Index(args, any("--cmd"))
			if cmdIndex < 0 || cmdIndex+1 >= len(args) {
				t.Fatalf("--cmd flag not found in args %v", args)
			}
			if args[cmdIndex+1] != tt.wantClaudeCmd {
				t.Errorf("--cmd arg = %v; want %q", args[cmdIndex+1], tt.wantClaudeCmd)
			}
		})
	}
}

// TestWriteConfigKeepsSettingsAndOverwritesTasks pins that a rerun leaves an existing settings.json
// untouched and replaces an existing tasks.json with the requested chain.
//
//testtiming:keep pins the untouched settings.json and the overwritten tasks.json per chain, which the gitignore test does not assert
func TestWriteConfigKeepsSettingsAndOverwritesTasks(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		chain      TaskChain
		wantLabels []string
	}{
		{"interactive", TaskChainInteractive, []string{"Start Claude", "reed add claude", "reed attach", "reed up"}},
		{"attach-only", TaskChainAttachOnly, []string{"reed attach"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
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

			if err := WriteConfig(dir, ".", "slug", "#2d7d46", "/opt/lyx/bin/lyx", "/usr/local/bin/claude", tt.chain); err != nil {
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
				t.Errorf("tasks.json was not overwritten with the current chain: version = %v", tasks.Version)
			}
			if labels := taskLabels(tasks.Tasks); !slices.Equal(labels, tt.wantLabels) {
				t.Errorf("task labels = %v; want %v", labels, tt.wantLabels)
			}
		})
	}
}

// TestWriteConfigLeavesGitignoreAlone pins that WriteConfig neither creates a .gitignore nor
// modifies an existing one, for both task chains.
//
//testtiming:keep pins that no .gitignore is created or modified, which its covering tests do not assert
func TestWriteConfigLeavesGitignoreAlone(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		chain      TaskChain
		claudePath string
	}{
		{"interactive", TaskChainInteractive, "/usr/local/bin/claude"},
		{"attach-only", TaskChainAttachOnly, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()

			if err := WriteConfig(dir, ".", "slug", "#2d7d46", "/opt/lyx/bin/lyx", tt.claudePath, tt.chain); err != nil {
				t.Fatalf("WriteConfig failed: %v", err)
			}
			if _, err := os.Stat(filepath.Join(dir, ".gitignore")); !os.IsNotExist(err) {
				t.Errorf(".gitignore should not exist, stat err = %v", err)
			}

			existing := []byte("build/\n")
			if err := os.WriteFile(filepath.Join(dir, ".gitignore"), existing, 0o644); err != nil {
				t.Fatal(err)
			}
			if err := WriteConfig(dir, ".", "slug", "#2d7d46", "/opt/lyx/bin/lyx", tt.claudePath, tt.chain); err != nil {
				t.Fatalf("WriteConfig failed: %v", err)
			}
			got, _ := os.ReadFile(filepath.Join(dir, ".gitignore"))
			if string(got) != string(existing) {
				t.Errorf(".gitignore modified: %q", got)
			}
		})
	}
}

// TestWriteConfigAttachOnlyWritesSingleFolderOpenTask pins the attach-only chain's single task:
// its label, command, args, folderOpen trigger and absence of dependsOn.
//
//testtiming:keep pins the attach-only task's label, args, runOn and missing dependsOn, which the bare-name fallback test does not assert
func TestWriteConfigAttachOnlyWritesSingleFolderOpenTask(t *testing.T) {
	dir := t.TempDir()

	if err := WriteConfig(dir, ".", "slug", "#2d7d46", "/opt/lyx/bin/lyx", "", TaskChainAttachOnly); err != nil {
		t.Fatalf("WriteConfig failed: %v", err)
	}

	tasks := readTasksFile(t, dir).Tasks
	if len(tasks) != 1 {
		t.Fatalf("got %d tasks, want exactly 1", len(tasks))
	}
	task := tasks[0]
	if task["label"] != "reed attach" {
		t.Errorf("label = %v, want reed attach", task["label"])
	}
	if task["command"] != "/opt/lyx/bin/lyx" {
		t.Errorf("command = %v, want /opt/lyx/bin/lyx", task["command"])
	}
	args, _ := task["args"].([]any)
	if len(args) != 2 || args[0] != "reed" || args[1] != "attach" {
		t.Errorf("args = %v, want [reed attach]", task["args"])
	}
	runOptions, _ := task["runOptions"].(map[string]any)
	if runOptions["runOn"] != "folderOpen" {
		t.Errorf("runOptions.runOn = %v, want folderOpen", runOptions["runOn"])
	}
	if _, ok := task["dependsOn"]; ok {
		t.Errorf("attach-only task must not carry dependsOn")
	}
}
