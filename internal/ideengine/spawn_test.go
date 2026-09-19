// spawn_test.go covers the end-to-end spawn flow with a stubbed code launcher.

package ideengine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

func TestSpawn(t *testing.T) {
	tests := []struct {
		name         string
		relpath      string
		checkClobber bool
	}{
		{
			name:         "TestSpawnGeneratesConfig",
			relpath:      ".",
			checkClobber: false,
		},
		{
			name:         "TestSpawnCallsCodeLauncher",
			relpath:      "subdir",
			checkClobber: false,
		},
		{
			name:         "TestSpawnDoesNotClobber",
			relpath:      ".",
			checkClobber: true,
		},
	}

	originalLauncher := CodeLauncher
	defer func() { CodeLauncher = originalLauncher }()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			container := tmpDir
			mainWorktreePath := filepath.Join(container, "main")
			childWorktreePath := filepath.Join(container, "child")

			for _, p := range []string{mainWorktreePath, childWorktreePath} {
				if err := os.MkdirAll(p, 0o755); err != nil {
					t.Fatalf("failed to create dir: %v", err)
				}
			}

			layout := &lyxcwd.Location{HubPath: container, AnchorRel: tt.relpath}

			var launchedDir string
			CodeLauncher = func(dir string) error {
				launchedDir = dir
				return nil
			}

			err := Spawn(layout, "child")
			if err != nil {
				t.Fatalf("Spawn failed: %v", err)
			}

			settingsPath := filepath.Join(childWorktreePath, tt.relpath, ".vscode", "settings.json")
			if _, err := os.Stat(settingsPath); err != nil {
				t.Fatalf("settings.json not created: %v", err)
			}

			tasksPath := filepath.Join(childWorktreePath, tt.relpath, ".vscode", "tasks.json")
			if _, err := os.Stat(tasksPath); err != nil {
				t.Fatalf("tasks.json not created: %v", err)
			}

			expectedDir := filepath.Join(childWorktreePath, tt.relpath)
			if launchedDir != expectedDir {
				t.Errorf("CodeLauncher called with %q; want %q", launchedDir, expectedDir)
			}

			assertSpawnedTasksStampTheReedChain(t, tasksPath)

			if tt.checkClobber {
				originalSettings, err := os.ReadFile(settingsPath)
				if err != nil {
					t.Fatalf("failed to read settings.json after first Spawn: %v", err)
				}

				if err := Spawn(layout, "child"); err != nil {
					t.Fatalf("second Spawn failed: %v", err)
				}

				newSettings, err := os.ReadFile(settingsPath)
				if err != nil {
					t.Fatalf("failed to read settings.json after second Spawn: %v", err)
				}

				if string(originalSettings) != string(newSettings) {
					t.Errorf("settings.json was modified by second Spawn")
				}
			}
		})
	}
}

// assertSpawnedTasksStampTheReedChain parses the tasks.json Spawn generated and asserts it
// carries the reed launch chain convention internal/vscode/config.go writes: the entry task
// still triggers on folderOpen, the "reed add claude" step's args carry --if-absent, --name
// claude and --focus, and every step task's command is an absolute path — proof that Spawn
// resolved and passed the lyx path rather than leaving WriteConfig to fall back to the bare
// name. Under go test the resolved executable is the test binary, so the assertion pins
// absoluteness rather than any particular filename: reading os.Executable() for its value is
// deliberate and a different act from re-execing it, which CONSTRAINTS.md bars under go test.
func assertSpawnedTasksStampTheReedChain(t *testing.T, tasksPath string) {
	t.Helper()

	data, err := os.ReadFile(tasksPath)
	if err != nil {
		t.Fatalf("failed to read tasks.json: %v", err)
	}
	var tasksFile map[string]any
	if err := json.Unmarshal(data, &tasksFile); err != nil {
		t.Fatalf("tasks.json is not valid JSON: %v", err)
	}
	tasksList, ok := tasksFile["tasks"].([]any)
	if !ok {
		t.Fatalf("tasks.json missing tasks array")
	}

	var entryTask map[string]any
	var addTask map[string]any
	for _, raw := range tasksList {
		task, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("task is not a map: %v", raw)
		}

		if command, ok := task["command"].(string); ok {
			if !filepath.IsAbs(command) {
				t.Errorf("task %v command = %q; want an absolute path", task["label"], command)
			}
		}

		switch task["label"] {
		case "Start Claude":
			entryTask = task
		case "reed add claude":
			addTask = task
		}
	}

	if entryTask == nil {
		t.Fatalf("missing entry task labelled 'Start Claude'")
	}
	runOptions, ok := entryTask["runOptions"].(map[string]any)
	if !ok {
		t.Fatalf("entry task missing runOptions")
	}
	if runOn, ok := runOptions["runOn"].(string); !ok || runOn != "folderOpen" {
		t.Errorf("entry task runOptions.runOn = %v; want 'folderOpen'", runOptions["runOn"])
	}

	if addTask == nil {
		t.Fatalf("missing step task labelled 'reed add claude'")
	}
	argsRaw, ok := addTask["args"].([]any)
	if !ok {
		t.Fatalf("'reed add claude' args is not an array: %v", addTask["args"])
	}
	var args []string
	for _, a := range argsRaw {
		s, ok := a.(string)
		if !ok {
			t.Fatalf("'reed add claude' has non-string arg: %v", a)
		}
		args = append(args, s)
	}
	for _, want := range []string{"--if-absent", "--name", "claude", "--focus"} {
		found := false
		for _, a := range args {
			if a == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("'reed add claude' args = %v; missing %q", args, want)
		}
	}
}
