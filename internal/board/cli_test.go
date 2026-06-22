// cli_test.go — tests for the board CLI (cli.go).
//
// Drives RunCLI in-process and asserts the JSON + exit-code contract for each
// subcommand: JSON envelope shape (ok=true/false), exit codes (0 for success,
// 1 for error), and each verb's distinctive field (task, tasks[], Home.md written).

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

// TestCLIContract tests the JSON envelope shape and exit code behavior for each
// happy-path verb: upsert, list, get, set-phase, rerender. Each case asserts
// exit 0 + ok=true + the verb's distinctive field.
//
// Folds: TestCLIUpsertTask, TestCLIListTasks, TestCLIGetTask, TestCLISetPhase,
// TestCLIRerender (as subtests preserving original names)
func TestCLIContract(t *testing.T) {
	t.Setenv("BOARD_SKIP_GIT", "1")

	tests := []struct {
		name              string
		setup             func(*testing.T) string // returns cwd
		verb              string
		payload           string
		wantExitCode      int
		wantOK            bool
		wantFieldExist    string                                   // field that must exist in result
		assertFieldExists func(*testing.T, map[string]any, string) // custom assertion
	}{
		{
			name: "TestCLIUpsertTask",
			setup: func(t *testing.T) string {
				return seedCwd(t)
			},
			verb:           "upsert",
			payload:        `{"slug":"foo","title":"Foo task"}`,
			wantExitCode:   0,
			wantOK:         true,
			wantFieldExist: "task",
		},
		{
			name: "TestCLIListTasks",
			setup: func(t *testing.T) string {
				cwd := seedCwd(t)
				// First upsert a task
				runCLI(t, "upsert", `{"slug":"foo","title":"Foo task"}`)
				return cwd
			},
			verb:           "list",
			payload:        "",
			wantExitCode:   0,
			wantOK:         true,
			wantFieldExist: "tasks",
			assertFieldExists: func(t *testing.T, result map[string]any, _ string) {
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
			},
		},
		{
			name: "TestCLIGetTask",
			setup: func(t *testing.T) string {
				cwd := seedCwd(t)
				// First upsert a task
				runCLI(t, "upsert", `{"slug":"foo","title":"Foo task"}`)
				return cwd
			},
			verb:           "get",
			payload:        `{"id_or_slug":"foo"}`,
			wantExitCode:   0,
			wantOK:         true,
			wantFieldExist: "task",
		},
		{
			name: "TestCLISetPhase",
			setup: func(t *testing.T) string {
				cwd := seedCwd(t)
				// First upsert a task
				runCLI(t, "upsert", `{"slug":"foo","title":"Foo task"}`)
				return cwd
			},
			verb:           "set-phase",
			payload:        `{"id_or_slug":"foo","phase":"active"}`,
			wantExitCode:   0,
			wantOK:         true,
			wantFieldExist: "task",
		},
		{
			name: "TestCLIRerender",
			setup: func(t *testing.T) string {
				return seedCwd(t)
			},
			verb:           "rerender",
			payload:        "",
			wantExitCode:   0,
			wantOK:         true,
			wantFieldExist: "ok",
			assertFieldExists: func(t *testing.T, result map[string]any, cwd string) {
				// Check Home.md exists in <cwd>/board/
				homePath := filepath.Join(cwd, "board", "Home.md")
				if _, err := os.Stat(homePath); err != nil {
					t.Fatalf("Home.md not created: %v", err)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cwd := tt.setup(t)

			var args []string
			args = append(args, tt.verb)
			if tt.payload != "" {
				args = append(args, tt.payload)
			}
			exitCode, stdout := runCLI(t, args...)

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

// TestCLIErrorAndEdgeCases tests error paths and edge cases: null task for
// nonexistent get, error for nonexistent remove, and not-initialized error.
//
// Folds: TestCLIGetNonexistentTask (null task case),
// TestCLIRemoveNonexistentTask (exit 1 + error), TestCLINotInitialized
func TestCLIErrorAndEdgeCases(t *testing.T) {
	t.Setenv("BOARD_SKIP_GIT", "1")

	tests := []struct {
		name         string
		setup        func(*testing.T) string
		verb         string
		payload      string
		wantExitCode int
		wantOK       bool
		wantError    string // if non-empty, error must contain this substring
		assertResult func(*testing.T, map[string]any)
	}{
		{
			name: "TestCLIGetNonexistentTask",
			setup: func(t *testing.T) string {
				return seedCwd(t)
			},
			verb:         "get",
			payload:      `{"id_or_slug":"nonexistent"}`,
			wantExitCode: 0,
			wantOK:       true,
			assertResult: func(t *testing.T, result map[string]any) {
				// null task is ok=true, but task field is nil
				if task, exists := result["task"]; !exists || task != nil {
					t.Fatalf("expected null task, got %v", result)
				}
			},
		},
		{
			name: "TestCLIRemoveNonexistentTask",
			setup: func(t *testing.T) string {
				return seedCwd(t)
			},
			verb:         "remove",
			payload:      `{"id_or_slug":"nonexistent"}`,
			wantExitCode: 1,
			wantOK:       false,
			wantError:    "", // any error is fine
		},
		{
			name: "TestCLINotInitialized",
			setup: func(t *testing.T) string {
				// Do NOT call seedCwd: cwd has no _lyx/
				cwd := t.TempDir()
				t.Chdir(cwd)
				return cwd
			},
			verb:         "list",
			payload:      "",
			wantExitCode: 1,
			wantOK:       false,
			wantError:    "not initialized",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setup(t)

			var args []string
			args = append(args, tt.verb)
			if tt.payload != "" {
				args = append(args, tt.payload)
			}
			exitCode, stdout := runCLI(t, args...)

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

			if tt.wantError != "" {
				if errMsg, exists := result["error"].(string); !exists {
					t.Fatalf("expected error message, got %v", result)
				} else if !strings.Contains(errMsg, tt.wantError) {
					t.Fatalf("expected error to contain %q, got %q", tt.wantError, errMsg)
				}
			}

			if tt.assertResult != nil {
				tt.assertResult(t, result)
			}
		})
	}
}
