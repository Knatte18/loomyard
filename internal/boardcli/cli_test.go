// cli_test.go — tests for the board CLI (cli.go).
//
// Drives RunCLI in-process and asserts the JSON + exit-code contract for each
// subcommand: JSON envelope shape (ok=true/false), exit codes (0 for success,
// 1 for error), and each verb's distinctive field (task, tasks[], Home.md written).

package boardcli_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/boardcli"
	"github.com/Knatte18/loomyard/internal/paths"
)

// seedCwd creates a temp directory with _lyx/config/board.yaml seeded with all template keys,
// changes to that directory, and returns the cwd path. The caller must restore
// the original cwd after the test (or use t.Chdir).
func seedCwd(t *testing.T) string {
	t.Helper()

	cwd := t.TempDir()
	lyxDir := filepath.Join(cwd, paths.LyxDirName)
	if err := os.MkdirAll(lyxDir, 0o755); err != nil {
		t.Fatalf("failed to create _lyx: %v", err)
	}

	configDir := paths.ConfigDir(cwd)
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("failed to create _lyx/config: %v", err)
	}

	// Write board config with all template keys
	configPath := paths.ConfigFile(cwd, "board")
	configContent := `path: board
home: Home.md
sidebar: _Sidebar.md
proposal_prefix: proposal-
`
	if err := os.WriteFile(configPath, []byte(configContent), 0o644); err != nil {
		t.Fatalf("failed to write board.yaml: %v", err)
	}

	t.Chdir(cwd)
	return cwd
}

// runCLI invokes boardcli.RunCLI in-process and returns the exit code plus the JSON
// written to out. Caller must have called seedCwd and t.Chdir to set up the cwd.
// BOARD_SKIP_GIT must be set by the caller.
func runCLI(t *testing.T, args ...string) (exitCode int, stdout string) {
	t.Helper()

	var buf bytes.Buffer
	code := boardcli.RunCLI(&buf, args)
	return code, buf.String()
}

// TestCLIContract tests the JSON envelope shape and exit code behavior for each
// happy-path verb: upsert, list, get, set-status, rerender. Each case asserts
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
			payload:        `{"slug":"foo"}`,
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
			verb:           "set-status",
			payload:        `{"slug":"foo","status":"active"}`,
			wantExitCode:   0,
			wantOK:         true,
			wantFieldExist: "ok",
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
			payload:      `{"slug":"nonexistent"}`,
			wantExitCode: 0,
			wantOK:       true,
			assertResult: func(t *testing.T, result map[string]any) {
				// A valid-but-absent target returns ok=true with task:null (not an error).
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
			payload:      `{"slug":"nonexistent"}`,
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

// TestCLINoArg asserts that invoking board with no subcommand exits 0 and
// lists available subcommands in the output. Under cobra, the no-arg parent
// command prints usage/help and exits cleanly — no config resolution is attempted.
func TestCLINoArg(t *testing.T) {
	t.Setenv("BOARD_SKIP_GIT", "1")

	// No-arg board does not require a seeded cwd because PersistentPreRunE is
	// not invoked when no subcommand is given — cobra handles the listing path.
	cwd := t.TempDir()
	t.Chdir(cwd)

	exitCode, stdout := runCLI(t)

	if exitCode != 0 {
		t.Errorf("RunCLI() exit = %d; want 0\nstdout: %s", exitCode, stdout)
	}

	// cobra's usage output lists subcommand names; verify at least one known
	// subcommand name appears so we know a real listing was printed.
	if !strings.Contains(stdout, "upsert") {
		t.Errorf("no-arg output does not list subcommands; stdout: %s", stdout)
	}
}

// TestCLIUnknownSubcommand asserts that an unknown subcommand exits 1 and emits
// a JSON error envelope with ok=false. GroupRunE handles the unknown-subcommand
// path, so the output is always a machine-parseable JSON envelope.
func TestCLIUnknownSubcommand(t *testing.T) {
	t.Setenv("BOARD_SKIP_GIT", "1")

	// GroupRunE fires before PersistentPreRunE reaches layout resolution, so a
	// plain temp dir (no board config, no git repo) is sufficient.
	cwd := t.TempDir()
	t.Chdir(cwd)

	exitCode, stdout := runCLI(t, "no-such-subcommand")

	if exitCode != 1 {
		t.Errorf("RunCLI(unknown) exit = %d; want 1\nstdout: %s", exitCode, stdout)
	}

	// GroupRunE wraps the error in a JSON envelope; parse and assert ok=false.
	var env map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &env); err != nil {
		t.Fatalf("RunCLI(unknown) output is not valid JSON: %v; stdout: %q", err, stdout)
	}
	if ok, _ := env["ok"].(bool); ok {
		t.Errorf("RunCLI(unknown) ok = true; want false")
	}
	// The error text contains "unknown" (GroupRunE produces "unknown subcommand").
	if errMsg, _ := env["error"].(string); !strings.Contains(errMsg, "unknown") {
		t.Errorf("RunCLI(unknown) error = %q; want \"unknown\" substring", errMsg)
	}
}

// TestCLIStrictPayloadShapes verifies the strict key/shape validation added in Card 5
// for set-deps, upsert-batch, and merge (top-level and inner set_status object).
func TestCLIStrictPayloadShapes(t *testing.T) {
	t.Setenv("BOARD_SKIP_GIT", "1")

	tests := []struct {
		name         string
		setup        func(*testing.T)
		verb         string
		payload      string
		wantExitCode int
		wantOK       bool
		wantError    string
		assertResult func(*testing.T, map[string]any)
	}{
		// set-deps: unknown key errors
		{
			name: "set_deps_unknown_key_errors",
			setup: func(t *testing.T) {
				seedCwd(t)
				runCLI(t, "upsert", `{"slug":"task-a","title":"A"}`)
			},
			verb:         "set-deps",
			payload:      `{"slug":"task-a","depends":["task-b"]}`,
			wantExitCode: 1,
			wantOK:       false,
			wantError:    "unknown field",
		},
		// set-deps: absent depends_on key errors
		{
			name: "set_deps_absent_depends_on_errors",
			setup: func(t *testing.T) {
				seedCwd(t)
				runCLI(t, "upsert", `{"slug":"task-a","title":"A"}`)
			},
			verb:         "set-deps",
			payload:      `{"slug":"task-a"}`,
			wantExitCode: 1,
			wantOK:       false,
			wantError:    "missing required field: depends_on",
		},
		// set-deps: explicit [] clears the list
		{
			name: "set_deps_empty_array_clears",
			setup: func(t *testing.T) {
				seedCwd(t)
				runCLI(t, "upsert", `{"slug":"task-a","title":"A"}`)
				runCLI(t, "upsert", `{"slug":"task-b","title":"B"}`)
				runCLI(t, "set-deps", `{"slug":"task-b","depends_on":["task-a"]}`)
			},
			verb:         "set-deps",
			payload:      `{"slug":"task-b","depends_on":[]}`,
			wantExitCode: 0,
			wantOK:       true,
			assertResult: func(t *testing.T, _ map[string]any) {
				_, out := runCLI(t, "get", `{"slug":"task-b"}`)
				var r map[string]any
				if err := json.Unmarshal([]byte(out), &r); err != nil {
					t.Fatalf("parse: %v", err)
				}
				task, _ := r["task"].(map[string]any)
				deps, _ := task["depends_on"].([]any)
				if len(deps) != 0 {
					t.Errorf("expected empty depends_on after clear, got %v", deps)
				}
			},
		},
		// upsert-batch: typo'd wrapper key errors
		{
			name: "upsert_batch_typo_wrapper_errors",
			setup: func(t *testing.T) {
				seedCwd(t)
			},
			verb:         "upsert-batch",
			payload:      `{"taks":[{"slug":"task-a","title":"A"}]}`,
			wantExitCode: 1,
			wantOK:       false,
			wantError:    "unknown field",
		},
		// upsert-batch: absent tasks key errors
		{
			name: "upsert_batch_absent_tasks_errors",
			setup: func(t *testing.T) {
				seedCwd(t)
			},
			verb:         "upsert-batch",
			payload:      `{}`,
			wantExitCode: 1,
			wantOK:       false,
			wantError:    "missing required field: tasks",
		},
		// upsert-batch: empty tasks array errors
		{
			name: "upsert_batch_empty_tasks_errors",
			setup: func(t *testing.T) {
				seedCwd(t)
			},
			verb:         "upsert-batch",
			payload:      `{"tasks":[]}`,
			wantExitCode: 1,
			wantOK:       false,
			wantError:    "tasks array must not be empty",
		},
		// merge: stale top-level set_phase errors
		{
			name: "merge_stale_set_phase_errors",
			setup: func(t *testing.T) {
				seedCwd(t)
			},
			verb:         "merge",
			payload:      `{"upsert":{"slug":"task-a","title":"A"},"set_phase":["task-a","done"]}`,
			wantExitCode: 1,
			wantOK:       false,
			wantError:    "unknown field",
		},
		// merge: inner set_status with unknown key (phase) errors
		{
			name: "merge_set_status_unknown_inner_key_errors",
			setup: func(t *testing.T) {
				seedCwd(t)
			},
			verb:         "merge",
			payload:      `{"upsert":{"slug":"task-a","title":"A"},"set_status":{"slug":"task-a","phase":"done"}}`,
			wantExitCode: 1,
			wantOK:       false,
			wantError:    "unknown field",
		},
		// merge: inner set_status missing status key errors
		{
			name: "merge_set_status_missing_status_key_errors",
			setup: func(t *testing.T) {
				seedCwd(t)
			},
			verb:         "merge",
			payload:      `{"upsert":{"slug":"task-a","title":"A"},"set_status":{"slug":"task-a"}}`,
			wantExitCode: 1,
			wantOK:       false,
			wantError:    "missing required field: status",
		},
		// merge: happy path with set_status succeeds
		{
			name: "merge_with_set_status_succeeds",
			setup: func(t *testing.T) {
				seedCwd(t)
				runCLI(t, "upsert", `{"slug":"old-task","title":"Old"}`)
			},
			verb:         "merge",
			payload:      `{"remove_slugs":["old-task"],"upsert":{"slug":"new-task","title":"New"},"set_status":{"slug":"new-task","status":"active"}}`,
			wantExitCode: 0,
			wantOK:       true,
			assertResult: func(t *testing.T, _ map[string]any) {
				_, out := runCLI(t, "get", `{"slug":"new-task"}`)
				var r map[string]any
				if err := json.Unmarshal([]byte(out), &r); err != nil {
					t.Fatalf("parse: %v", err)
				}
				task, _ := r["task"].(map[string]any)
				if task == nil {
					t.Fatalf("new-task not found")
				}
				if task["status"] != "active" {
					t.Errorf("expected status=active, got %v", task["status"])
				}
			},
		},
		// merge: set_status targeting non-existent slug errors (atomic rollback via writeOp)
		{
			name: "merge_set_status_missing_target_errors",
			setup: func(t *testing.T) {
				seedCwd(t)
			},
			verb:         "merge",
			payload:      `{"upsert":{"slug":"new-task","title":"New"},"set_status":{"slug":"ghost","status":"done"}}`,
			wantExitCode: 1,
			wantOK:       false,
			wantError:    "task not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setup(t)

			exitCode, stdout := runCLI(t, tt.verb, tt.payload)

			if exitCode != tt.wantExitCode {
				t.Fatalf("exit = %d; want %d; stdout: %s", exitCode, tt.wantExitCode, stdout)
			}

			var result map[string]any
			if err := json.Unmarshal([]byte(stdout), &result); err != nil {
				t.Fatalf("parse: %v; stdout: %s", err, stdout)
			}

			if ok, _ := result["ok"].(bool); ok != tt.wantOK {
				t.Fatalf("ok = %v; want %v; stdout: %s", ok, tt.wantOK, stdout)
			}

			if tt.wantError != "" {
				if errMsg, _ := result["error"].(string); !strings.Contains(errMsg, tt.wantError) {
					t.Fatalf("error = %q; want substring %q", errMsg, tt.wantError)
				}
			}

			if tt.assertResult != nil {
				tt.assertResult(t, result)
			}
		})
	}
}

// TestCLILookupContract covers the slug-or-id lookup contract on get, set-status,
// and remove: both key forms succeed; id=0 resolves the first-created task; neither
// key and both keys error; unknown keys (e.g. old id_or_slug) error.
func TestCLILookupContract(t *testing.T) {
	t.Setenv("BOARD_SKIP_GIT", "1")

	tests := []struct {
		name         string
		setup        func(*testing.T) // seeds cwd + board state
		verb         string
		payload      string
		wantExitCode int
		wantOK       bool
		wantError    string // substring; empty means any error message is fine
		assertResult func(*testing.T, map[string]any)
	}{
		{
			name: "get_by_slug",
			setup: func(t *testing.T) {
				seedCwd(t)
				runCLI(t, "upsert", `{"slug":"task-a","title":"A"}`)
			},
			verb:         "get",
			payload:      `{"slug":"task-a"}`,
			wantExitCode: 0,
			wantOK:       true,
			assertResult: func(t *testing.T, result map[string]any) {
				task, ok := result["task"].(map[string]any)
				if !ok {
					t.Fatalf("expected task object, got %v", result["task"])
				}
				if task["slug"] != "task-a" {
					t.Errorf("get_by_slug: got slug %v; want task-a", task["slug"])
				}
			},
		},
		{
			name: "get_by_id",
			setup: func(t *testing.T) {
				seedCwd(t)
				// The first upserted task gets id=0.
				runCLI(t, "upsert", `{"slug":"task-a","title":"A"}`)
			},
			verb:         "get",
			payload:      `{"id":0}`,
			wantExitCode: 0,
			wantOK:       true,
			assertResult: func(t *testing.T, result map[string]any) {
				// id:0 must resolve the first-created task; verifies the int-vs-float64
				// JSON-number decode boundary (JSON decodes 0 as float64(0)).
				task, ok := result["task"].(map[string]any)
				if !ok {
					t.Fatalf("get_by_id: expected task object for id=0, got %v", result["task"])
				}
				if task["slug"] != "task-a" {
					t.Errorf("get_by_id: got slug %v; want task-a", task["slug"])
				}
			},
		},
		{
			name: "get_neither_key_errors",
			setup: func(t *testing.T) {
				seedCwd(t)
			},
			verb:         "get",
			payload:      `{}`,
			wantExitCode: 1,
			wantOK:       false,
			wantError:    "one of slug or id is required",
		},
		{
			name: "get_both_keys_errors",
			setup: func(t *testing.T) {
				seedCwd(t)
			},
			verb:         "get",
			payload:      `{"slug":"x","id":1}`,
			wantExitCode: 1,
			wantOK:       false,
			wantError:    "only one of slug or id may be given",
		},
		{
			name: "get_unknown_key_errors",
			setup: func(t *testing.T) {
				seedCwd(t)
			},
			verb:         "get",
			payload:      `{"id_or_slug":"x"}`,
			wantExitCode: 1,
			wantOK:       false,
			wantError:    "unknown field",
		},
		{
			name: "remove_by_slug",
			setup: func(t *testing.T) {
				seedCwd(t)
				runCLI(t, "upsert", `{"slug":"task-a","title":"A"}`)
			},
			verb:         "remove",
			payload:      `{"slug":"task-a"}`,
			wantExitCode: 0,
			wantOK:       true,
		},
		{
			name: "remove_by_id",
			setup: func(t *testing.T) {
				seedCwd(t)
				runCLI(t, "upsert", `{"slug":"task-a","title":"A"}`)
			},
			verb:         "remove",
			payload:      `{"id":0}`,
			wantExitCode: 0,
			wantOK:       true,
		},
		{
			name: "set_status_by_slug",
			setup: func(t *testing.T) {
				seedCwd(t)
				runCLI(t, "upsert", `{"slug":"task-a","title":"A"}`)
			},
			verb:         "set-status",
			payload:      `{"slug":"task-a","status":"active"}`,
			wantExitCode: 0,
			wantOK:       true,
		},
		{
			name: "set_status_by_id",
			setup: func(t *testing.T) {
				seedCwd(t)
				runCLI(t, "upsert", `{"slug":"task-a","title":"A"}`)
			},
			verb:         "set-status",
			payload:      `{"id":0,"status":"active"}`,
			wantExitCode: 0,
			wantOK:       true,
		},
		// Card 3: set-status requires the status key and errors on missing target.
		{
			name: "set_status_absent_status_key_errors",
			setup: func(t *testing.T) {
				seedCwd(t)
				runCLI(t, "upsert", `{"slug":"task-a","title":"A"}`)
			},
			verb:         "set-status",
			payload:      `{"slug":"task-a"}`,
			wantExitCode: 1,
			wantOK:       false,
			wantError:    "missing required field: status",
		},
		{
			name: "set_status_null_status_clears",
			setup: func(t *testing.T) {
				seedCwd(t)
				runCLI(t, "upsert", `{"slug":"task-a","title":"A"}`)
				runCLI(t, "set-status", `{"slug":"task-a","status":"active"}`)
			},
			verb:         "set-status",
			payload:      `{"slug":"task-a","status":null}`,
			wantExitCode: 0,
			wantOK:       true,
			assertResult: func(t *testing.T, result map[string]any) {
				// Verify status was actually cleared by doing a get.
				_, getOut := runCLI(t, "get", `{"slug":"task-a"}`)
				var getResult map[string]any
				if err := json.Unmarshal([]byte(getOut), &getResult); err != nil {
					t.Fatalf("failed to parse get output: %v", err)
				}
				task, _ := getResult["task"].(map[string]any)
				if task == nil {
					t.Fatalf("task not found after status clear")
				}
				// status field should be absent (omitempty) or null in the JSON.
				if _, hasStatus := task["status"]; hasStatus {
					t.Errorf("expected status to be absent after null clear, got %v", task["status"])
				}
			},
		},
		{
			name: "set_status_missing_target_errors",
			setup: func(t *testing.T) {
				seedCwd(t)
			},
			verb:         "set-status",
			payload:      `{"slug":"nonexistent","status":"active"}`,
			wantExitCode: 1,
			wantOK:       false,
			wantError:    "task not found",
		},
		// Stray old key "phase" in set-status payload must error; the strict
		// resolveLookup allows only {slug, id, status} and rejects all others.
		{
			name: "set_status_stray_phase_errors",
			setup: func(t *testing.T) {
				seedCwd(t)
				runCLI(t, "upsert", `{"slug":"x","title":"X"}`)
			},
			verb:         "set-status",
			payload:      `{"slug":"x","phase":"done","status":"active"}`,
			wantExitCode: 1,
			wantOK:       false,
			wantError:    "unknown field",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setup(t)

			exitCode, stdout := runCLI(t, tt.verb, tt.payload)

			if exitCode != tt.wantExitCode {
				t.Fatalf("exit = %d; want %d; stdout: %s", exitCode, tt.wantExitCode, stdout)
			}

			var result map[string]any
			if err := json.Unmarshal([]byte(stdout), &result); err != nil {
				t.Fatalf("failed to parse output: %v; stdout: %s", err, stdout)
			}

			if ok, _ := result["ok"].(bool); ok != tt.wantOK {
				t.Fatalf("ok = %v; want %v; stdout: %s", ok, tt.wantOK, stdout)
			}

			if tt.wantError != "" {
				if errMsg, _ := result["error"].(string); !strings.Contains(errMsg, tt.wantError) {
					t.Fatalf("error = %q; want substring %q", errMsg, tt.wantError)
				}
			}

			if tt.assertResult != nil {
				tt.assertResult(t, result)
			}
		})
	}
}
