//go:build integration

// notes_test.go — tests for the "notes" subcommand group (cli.go).
//
// Mirrors TestCLIContract's table-driven shape from cli_test.go, but drives
// the notes verbs, which share the one board.json store with the task verbs:
// a notes upsert must land in board.json and create no legacy file, and
// notes list returns every entry. seedCwd and runCLI are defined in cli_test.go and
// cli_unit_test.go respectively, same package, directly callable.

package boardcli_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
)

// TestCLINotesAliasReachesTopLevelEntries asserts that the notes group is an alias onto the same
// store: an entry created by the top-level upsert is reachable and removable through notes.
func TestCLINotesAliasReachesTopLevelEntries(t *testing.T) {
	t.Setenv("BOARD_SKIP_GIT", "1")
	seedCwd(t)

	if exitCode, stdout := runCLI(t, "upsert", `{"slug":"shared","title":"Shared"}`); exitCode != 0 {
		t.Fatalf("upsert: exit %d; stdout: %s", exitCode, stdout)
	}

	exitCode, stdout := runCLI(t, "notes", "get", `{"slug":"shared"}`)
	if exitCode != 0 {
		t.Fatalf("notes get: exit %d; stdout: %s", exitCode, stdout)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("parse notes get output: %v; stdout: %s", err, stdout)
	}
	if task, ok := got["task"].(map[string]any); !ok || task["slug"] != "shared" {
		t.Fatalf("notes get did not return the top-level entry: %v", got)
	}

	if exitCode, stdout := runCLI(t, "notes", "remove", `{"slug":"shared"}`); exitCode != 0 {
		t.Fatalf("notes remove: exit %d; stdout: %s", exitCode, stdout)
	}
	_, stdout = runCLI(t, "get", `{"slug":"shared"}`)
	var after map[string]any
	if err := json.Unmarshal([]byte(stdout), &after); err != nil {
		t.Fatalf("parse get output: %v; stdout: %s", err, stdout)
	}
	if task, exists := after["task"]; !exists || task != nil {
		t.Fatalf("expected the entry removed through notes to be gone from the top level, got %v", after)
	}
}

// TestCLINotesContract tests the JSON envelope shape and exit code behavior for each happy-path
// notes verb: upsert, list, get, set-status, remove.
// Each case asserts exit 0 + ok=true + the verb's distinctive field, plus a notes-specific
// assertion distinguishing it from the task-verb behavior.
func TestCLINotesContract(t *testing.T) {
	t.Setenv("BOARD_SKIP_GIT", "1")

	tests := []struct {
		name              string
		setup             func(*testing.T) string // returns cwd
		args              []string
		wantExitCode      int
		wantOK            bool
		wantFieldExist    string
		assertFieldExists func(*testing.T, map[string]any, string) // (t, result, cwd)
	}{
		{
			name: "notes upsert",
			setup: func(t *testing.T) string {
				return seedCwd(t)
			},
			args:           []string{"notes", "upsert", `{"slug":"foo-note","title":"Foo Note"}`},
			wantExitCode:   0,
			wantOK:         true,
			wantFieldExist: "task",
			assertFieldExists: func(t *testing.T, result map[string]any, cwd string) {
				// The write must land in board.json, with no legacy file created in the board dir.
				boardDir := fabricengine.BoardDir(filepath.Dir(cwd))
				storePath := filepath.Join(boardDir, "board.json")
				if _, err := os.Stat(storePath); err != nil {
					t.Fatalf("board.json not created at %q: %v", storePath, err)
				}
				for _, legacy := range []string{"tasks.json", "notes.json"} {
					if _, err := os.Stat(filepath.Join(boardDir, legacy)); err == nil {
						t.Fatalf("%s unexpectedly created by notes upsert", legacy)
					}
				}
			},
		},
		{
			name: "notes list",
			setup: func(t *testing.T) string {
				cwd := seedCwd(t)
				runCLI(t, "notes", "upsert", `{"slug":"foo-note","title":"Foo Note"}`)
				runCLI(t, "upsert", `{"slug":"foo-task","title":"Foo Task"}`)
				return cwd
			},
			args:           []string{"notes", "list"},
			wantExitCode:   0,
			wantOK:         true,
			wantFieldExist: "tasks", // envelope key stays "tasks" (ListTasksBrief reuses BriefTask)
			assertFieldExists: func(t *testing.T, result map[string]any, _ string) {
				tasks, ok := result["tasks"].([]any)
				if !ok || len(tasks) != 2 {
					t.Fatalf("expected the note and the task in the tasks array, got %v", result)
				}
			},
		},
		{
			name: "notes get",
			setup: func(t *testing.T) string {
				cwd := seedCwd(t)
				runCLI(t, "notes", "upsert", `{"slug":"foo-note","title":"Foo Note"}`)
				return cwd
			},
			args:           []string{"notes", "get", `{"slug":"foo-note"}`},
			wantExitCode:   0,
			wantOK:         true,
			wantFieldExist: "task",
		},
		{
			name: "notes set-status",
			setup: func(t *testing.T) string {
				cwd := seedCwd(t)
				runCLI(t, "notes", "upsert", `{"slug":"foo-note","title":"Foo Note"}`)
				return cwd
			},
			args:           []string{"notes", "set-status", `{"slug":"foo-note","status":"active"}`},
			wantExitCode:   0,
			wantOK:         true,
			wantFieldExist: "ok",
		},
		{
			name: "notes remove",
			setup: func(t *testing.T) string {
				cwd := seedCwd(t)
				runCLI(t, "notes", "upsert", `{"slug":"foo-note","title":"Foo Note"}`)
				return cwd
			},
			args:           []string{"notes", "remove", `{"slug":"foo-note"}`},
			wantExitCode:   0,
			wantOK:         true,
			wantFieldExist: "ok",
			assertFieldExists: func(t *testing.T, result map[string]any, cwd string) {
				exitCode, stdout := runCLI(t, "notes", "get", `{"slug":"foo-note"}`)
				if exitCode != 0 {
					t.Fatalf("notes get after remove: exit %d; stdout: %s", exitCode, stdout)
				}
				var getResult map[string]any
				if err := json.Unmarshal([]byte(stdout), &getResult); err != nil {
					t.Fatalf("failed to parse notes get output: %v; stdout: %s", err, stdout)
				}
				if task, exists := getResult["task"]; !exists || task != nil {
					t.Fatalf("expected removed note to be gone, got %v", getResult)
				}

				// A notes remove must not create a legacy file.
				boardDir := fabricengine.BoardDir(filepath.Dir(cwd))
				for _, legacy := range []string{"tasks.json", "notes.json"} {
					if _, err := os.Stat(filepath.Join(boardDir, legacy)); err == nil {
						t.Fatalf("%s unexpectedly present after notes remove", legacy)
					}
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cwd := tt.setup(t)

			exitCode, stdout := runCLI(t, tt.args...)

			if exitCode != tt.wantExitCode {
				t.Fatalf("expected exit %d, got %d; stdout: %s", tt.wantExitCode, exitCode, stdout)
			}

			var result map[string]any
			if err := json.Unmarshal([]byte(stdout), &result); err != nil {
				t.Fatalf("failed to parse output: %v; stdout: %s", err, stdout)
			}

			if ok, exists := result["ok"].(bool); !exists || ok != tt.wantOK {
				t.Fatalf("expected ok=%v, got %v", tt.wantOK, result)
			}

			if _, exists := result[tt.wantFieldExist]; !exists {
				t.Fatalf("expected %s in result, got %v", tt.wantFieldExist, result)
			}

			if tt.assertFieldExists != nil {
				tt.assertFieldExists(t, result, cwd)
			}
		})
	}
}
