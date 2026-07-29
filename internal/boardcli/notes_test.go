//go:build integration

// notes_test.go — tests for the "notes" subcommand group (cli.go).
//
// Mirrors TestCLIContract's table-driven shape from cli_test.go, but drives
// the notes verbs (targeting notes.json instead of tasks.json) and adds the
// distinctive cross-store assertions: a notes upsert must write notes.json
// (not tasks.json) in the board dir, and a notes remove must leave
// tasks.json untouched. seedCwd and runCLI are defined in cli_test.go and
// cli_unit_test.go respectively, same package, directly callable.

package boardcli_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/hubgeometry"
)

// TestCLINotesContract tests the JSON envelope shape and exit code behavior
// for each happy-path notes verb: upsert, list, get, set-status, remove.
// Each case asserts exit 0 + ok=true + the verb's distinctive field, plus a
// notes-specific assertion distinguishing it from the task-verb behavior.
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
				// The write must land in notes.json, not tasks.json, in the board dir.
				boardDir := hubgeometry.BoardDir(filepath.Dir(cwd))
				notesPath := filepath.Join(boardDir, "notes.json")
				if _, err := os.Stat(notesPath); err != nil {
					t.Fatalf("notes.json not created at %q: %v", notesPath, err)
				}
				tasksPath := filepath.Join(boardDir, "tasks.json")
				if _, err := os.Stat(tasksPath); err == nil {
					t.Fatalf("tasks.json unexpectedly created by notes upsert at %q", tasksPath)
				}
			},
		},
		{
			name: "notes list",
			setup: func(t *testing.T) string {
				cwd := seedCwd(t)
				runCLI(t, "notes", "upsert", `{"slug":"foo-note","title":"Foo Note"}`)
				return cwd
			},
			args:           []string{"notes", "list"},
			wantExitCode:   0,
			wantOK:         true,
			wantFieldExist: "tasks", // envelope key stays "tasks" even for notes output (ListNotesBrief reuses BriefTask)
			assertFieldExists: func(t *testing.T, result map[string]any, _ string) {
				tasks, ok := result["tasks"].([]any)
				if !ok || len(tasks) == 0 {
					t.Fatalf("expected non-empty tasks array, got %v", result)
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

				// tasks.json must be untouched by a notes-scoped remove.
				boardDir := hubgeometry.BoardDir(filepath.Dir(cwd))
				tasksPath := filepath.Join(boardDir, "tasks.json")
				if _, err := os.Stat(tasksPath); err == nil {
					t.Fatalf("tasks.json unexpectedly present after notes remove at %q", tasksPath)
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
