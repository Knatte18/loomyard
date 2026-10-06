//go:build integration

// cli_test.go — tests for the board CLI (cli.go).
//
// Drives RunCLI in-process and asserts the JSON + exit-code contract for each subcommand: JSON envelope shape (ok=true/false), exit codes (0 for success, 1 for error), and each verb's distinctive field (task, tasks[], Home.md written).
//
// Board data dir strategy: TestCLI seeds one git repo (git init) as its cwd so that PersistentPreRunE can call lyxcwd.Resolve without error.
// The board data dir is then Hub/_board where Hub = filepath.Dir(cwd).
// This is the production code path; no --board-path injection is used for operational tests.
//
// seedCwd spawns "git init" and every RunCLI call spawns "git rev-parse" via lyxcwd.Resolve, so this file is integration-tagged per the Test Tier Purity Invariant; spawn-free CLI tests live in cli_unit_test.go.

package boardcli_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
)

// defaultHubBoardConfig is the hub board.yaml every step starts from.
const defaultHubBoardConfig = "readme: Home.md\ndesign_prefix: proposal-\n"

// seedCwd creates a temp directory with the default hub board.yaml, initialises a git repo there (so lyxcwd.Resolve succeeds), changes to that directory, and returns the cwd path.
// The board data dir is Hub/_board where Hub = filepath.Dir(cwd); callers can compute it as fabricengine.BoardDir(filepath.Dir(cwd)).
func seedCwd(t *testing.T) string {
	t.Helper()

	cwd := t.TempDir()

	// Initialise a git repo so PersistentPreRunE can call lyxcwd.Resolve without error.
	if out, err := exec.Command("git", "-C", cwd, "init").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}

	seedHubBoardConfig(t, filepath.Dir(cwd), defaultHubBoardConfig)

	t.Chdir(cwd)
	return cwd
}

// seedHubBoardConfig writes content as the board.yaml in the hub's board dir, the one file every board reader loads.
func seedHubBoardConfig(t *testing.T, hub, content string) {
	t.Helper()
	boardDir := fabricengine.BoardDir(hub)
	if err := os.MkdirAll(configengine.ConfigDir(boardDir), 0o755); err != nil {
		t.Fatalf("failed to create hub board config dir: %v", err)
	}
	if err := os.WriteFile(configengine.ConfigFile(boardDir, "board"), []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write hub board.yaml: %v", err)
	}
}

// cliFixture is the one seeded repository the steps of TestCLI share.
type cliFixture struct {
	cwd string
}

// reset empties the fixture's board, restores the default hub board.yaml, drops any worktree copy of the board config, and returns the cwd.
func (f *cliFixture) reset(t *testing.T) string {
	t.Helper()
	hub := filepath.Dir(f.cwd)
	if err := os.RemoveAll(fabricengine.BoardDir(hub)); err != nil {
		t.Fatalf("remove hub board dir: %v", err)
	}
	if err := os.RemoveAll(configengine.ConfigDir(f.cwd)); err != nil {
		t.Fatalf("remove worktree board config: %v", err)
	}
	seedHubBoardConfig(t, hub, defaultHubBoardConfig)
	return f.cwd
}

// TestCLI runs every board CLI contract against one seeded repository, in the order below.
// Each step starts from an empty board with the default board.yaml, so a step relies on no earlier step's state;
// the fixed order only lets a failing step stop the run.
// It calls no t.Parallel, and neither does a step, because t.Setenv, t.Chdir and the os.Stdin swap of pipeStdin are process-global state.
func TestCLI(t *testing.T) {
	t.Setenv("BOARD_SKIP_GIT", "1")
	fixture := &cliFixture{cwd: seedCwd(t)}

	steps := []struct {
		name string
		run  func(t *testing.T, f *cliFixture)
	}{
		{"Contract", stepContract},
		{"ErrorAndEdgeCases", stepErrorAndEdgeCases},
		{"StrictPayloadShapes", stepStrictPayloadShapes},
		{"LookupContract", stepLookupContract},
		{"LoadsHubBoardConfig", stepLoadsHubBoardConfig},
		{"BoardPathResolution", stepBoardPathResolution},
		{"Promote", stepPromote},
		{"Promote_Refusals", stepPromote_Refusals},
		{"Prune", stepPrune},
		{"Find", stepFind},
		{"ListAndFindLabelFilter", stepListAndFindLabelFilter},
		{"Find_NoArgumentRefused", stepFind_NoArgumentRefused},
		{"ListAndFindText", stepListAndFindText},
		{"RetireLegacy", stepRetireLegacy},
		{"GetAndRemoveByID", stepGetAndRemoveByID},
		{"UpsertBodyFile", stepUpsertBodyFile},
		{"MergeBodyFile", stepMergeBodyFile},
		{"GetBody", stepGetBody},
		{"GetBodyRoundTrip", stepGetBodyRoundTrip},
		{"LabelsMapShapedPrintsFileOrderWithDescriptions", stepLabelsMapShapedPrintsFileOrderWithDescriptions},
		{"LabelsListShapedPrintsNamesWithEmptyDescriptions", stepLabelsListShapedPrintsNamesWithEmptyDescriptions},
	}
	for _, step := range steps {
		if !t.Run(step.name, func(t *testing.T) {
			fixture.reset(t)
			step.run(t, fixture)
		}) {
			return
		}
	}
}

// stepContract tests the JSON envelope shape and exit code behavior for each happy-path verb:
// upsert, list, get, set-status, rerender.
// Each case asserts exit 0 + ok=true + the verb's distinctive field.
//
// Folds: TestCLIUpsertTask, TestCLIListTasks, TestCLIGetTask, TestCLISetPhase, TestCLIRerender (as subtests preserving original names)
func stepContract(t *testing.T, f *cliFixture) {

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
				return f.reset(t)
			},
			verb:           "upsert",
			payload:        `{"slug":"foo","title":"Foo task","labels":["bug"]}`,
			wantExitCode:   0,
			wantOK:         true,
			wantFieldExist: "task",
		},
		{
			name: "TestCLIListTasks",
			setup: func(t *testing.T) string {
				cwd := f.reset(t)
				// First upsert a task
				runCLI(t, "upsert", `{"slug":"foo","title":"Foo task","labels":["bug"]}`)
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
				cwd := f.reset(t)
				// First upsert a task
				runCLI(t, "upsert", `{"slug":"foo","title":"Foo task","labels":["bug"]}`)
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
				cwd := f.reset(t)
				// First upsert a task
				runCLI(t, "upsert", `{"slug":"foo","title":"Foo task","labels":["bug"]}`)
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
				return f.reset(t)
			},
			verb:           "rerender",
			payload:        "",
			wantExitCode:   0,
			wantOK:         true,
			wantFieldExist: "ok",
			assertFieldExists: func(t *testing.T, result map[string]any, cwd string) {
				// The fixture initialised a git repo at cwd; Hub = filepath.Dir(cwd);
				// lyxcwd.Resolve derives Hub from the git root, so board renders at Hub/_board.
				homePath := filepath.Join(fabricengine.BoardDir(filepath.Dir(cwd)), "Home.md")
				if _, err := os.Stat(homePath); err != nil {
					t.Fatalf("Home.md not created at %q: %v", homePath, err)
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

// stepErrorAndEdgeCases tests error paths and edge cases: null task for nonexistent get, error for nonexistent remove.
// The not-initialized refusal is covered by the LoadsHubBoardConfig step.
//
// Folds: TestCLIGetNonexistentTask (null task case), TestCLIRemoveNonexistentTask (exit 1 + error)
func stepErrorAndEdgeCases(t *testing.T, f *cliFixture) {

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
				return f.reset(t)
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
				return f.reset(t)
			},
			verb:         "remove",
			payload:      `{"slug":"nonexistent"}`,
			wantExitCode: 1,
			wantOK:       false,
			wantError:    "", // any error is fine
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

// stepStrictPayloadShapes verifies the strict key/shape validation added in Card 5 for set-deps, upsert-batch, and merge (top-level and inner set_status object).
func stepStrictPayloadShapes(t *testing.T, f *cliFixture) {

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
				f.reset(t)
				runCLI(t, "upsert", `{"slug":"task-a","title":"A","labels":["bug"]}`)
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
				f.reset(t)
				runCLI(t, "upsert", `{"slug":"task-a","title":"A","labels":["bug"]}`)
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
				f.reset(t)
				runCLI(t, "upsert", `{"slug":"task-a","title":"A","labels":["bug"]}`)
				runCLI(t, "upsert", `{"slug":"task-b","title":"B","labels":["bug"]}`)
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
				f.reset(t)
			},
			verb:         "upsert-batch",
			payload:      `{"taks":[{"slug":"task-a","title":"A","labels":["bug"]}]}`,
			wantExitCode: 1,
			wantOK:       false,
			wantError:    "unknown field",
		},
		// upsert-batch: absent tasks key errors
		{
			name: "upsert_batch_absent_tasks_errors",
			setup: func(t *testing.T) {
				f.reset(t)
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
				f.reset(t)
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
				f.reset(t)
			},
			verb:         "merge",
			payload:      `{"upsert":{"slug":"task-a","title":"A","labels":["bug"]},"set_phase":["task-a","done"]}`,
			wantExitCode: 1,
			wantOK:       false,
			wantError:    "unknown field",
		},
		// merge: inner set_status with unknown key (phase) errors
		{
			name: "merge_set_status_unknown_inner_key_errors",
			setup: func(t *testing.T) {
				f.reset(t)
			},
			verb:         "merge",
			payload:      `{"upsert":{"slug":"task-a","title":"A","labels":["bug"]},"set_status":{"slug":"task-a","phase":"done"}}`,
			wantExitCode: 1,
			wantOK:       false,
			wantError:    "unknown field",
		},
		// merge: inner set_status missing status key errors
		{
			name: "merge_set_status_missing_status_key_errors",
			setup: func(t *testing.T) {
				f.reset(t)
			},
			verb:         "merge",
			payload:      `{"upsert":{"slug":"task-a","title":"A","labels":["bug"]},"set_status":{"slug":"task-a"}}`,
			wantExitCode: 1,
			wantOK:       false,
			wantError:    "missing required field: status",
		},
		// merge: happy path with set_status succeeds
		{
			name: "merge_with_set_status_succeeds",
			setup: func(t *testing.T) {
				f.reset(t)
				runCLI(t, "upsert", `{"slug":"old-task","title":"Old","labels":["bug"]}`)
			},
			verb:         "merge",
			payload:      `{"remove_slugs":["old-task"],"upsert":{"slug":"new-task","title":"New","labels":["bug"]},"set_status":{"slug":"new-task","status":"active"}}`,
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
		// merge: a non-string upsert slug must produce an envelope error, not a
		// store-level interface-conversion panic (which would crash the CLI).
		{
			name: "merge_numeric_upsert_slug_errors",
			setup: func(t *testing.T) {
				f.reset(t)
			},
			verb:         "merge",
			payload:      `{"upsert":{"slug":123,"title":"num"}}`,
			wantExitCode: 1,
			wantOK:       false,
			wantError:    "slug must be a non-empty string",
		},
		// merge: set_status targeting non-existent slug errors (atomic rollback via writeOp)
		{
			name: "merge_set_status_missing_target_errors",
			setup: func(t *testing.T) {
				f.reset(t)
			},
			verb:         "merge",
			payload:      `{"upsert":{"slug":"new-task","title":"New","labels":["bug"]},"set_status":{"slug":"ghost","status":"done"}}`,
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

// stepLookupContract covers the slug-or-id lookup contract on get, set-status, and remove: both key forms succeed;
// id=0 resolves the first-created task;
// neither key and both keys error;
// unknown keys (e.g.
// old id_or_slug) error.
func stepLookupContract(t *testing.T, f *cliFixture) {

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
				f.reset(t)
				runCLI(t, "upsert", `{"slug":"task-a","title":"A","labels":["bug"]}`)
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
				f.reset(t)
				// The first upserted task gets id=0.
				runCLI(t, "upsert", `{"slug":"task-a","title":"A","labels":["bug"]}`)
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
				f.reset(t)
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
				f.reset(t)
			},
			verb:         "get",
			payload:      `{"slug":"x","id":1}`,
			wantExitCode: 1,
			wantOK:       false,
			wantError:    "only one of slug or id may be given",
		},
		{
			name: "get_fractional_id_errors",
			setup: func(t *testing.T) {
				f.reset(t)
				runCLI(t, "upsert", `{"slug":"task-a","title":"A","labels":["bug"]}`)
				runCLI(t, "upsert", `{"slug":"task-b","title":"B","labels":["bug"]}`)
			},
			verb: "get",
			// 1.5 must error, not silently truncate to task id 1 (task-b).
			payload:      `{"id":1.5}`,
			wantExitCode: 1,
			wantOK:       false,
			wantError:    "id must be an integer",
		},
		{
			name: "get_unknown_key_errors",
			setup: func(t *testing.T) {
				f.reset(t)
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
				f.reset(t)
				runCLI(t, "upsert", `{"slug":"task-a","title":"A","labels":["bug"]}`)
			},
			verb:         "remove",
			payload:      `{"slug":"task-a"}`,
			wantExitCode: 0,
			wantOK:       true,
		},
		{
			name: "remove_by_id",
			setup: func(t *testing.T) {
				f.reset(t)
				runCLI(t, "upsert", `{"slug":"task-a","title":"A","labels":["bug"]}`)
			},
			verb:         "remove",
			payload:      `{"id":0}`,
			wantExitCode: 0,
			wantOK:       true,
		},
		{
			name: "set_status_by_slug",
			setup: func(t *testing.T) {
				f.reset(t)
				runCLI(t, "upsert", `{"slug":"task-a","title":"A","labels":["bug"]}`)
			},
			verb:         "set-status",
			payload:      `{"slug":"task-a","status":"active"}`,
			wantExitCode: 0,
			wantOK:       true,
		},
		{
			name: "set_status_by_id",
			setup: func(t *testing.T) {
				f.reset(t)
				runCLI(t, "upsert", `{"slug":"task-a","title":"A","labels":["bug"]}`)
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
				f.reset(t)
				runCLI(t, "upsert", `{"slug":"task-a","title":"A","labels":["bug"]}`)
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
				f.reset(t)
				runCLI(t, "upsert", `{"slug":"task-a","title":"A","labels":["bug"]}`)
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
				f.reset(t)
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
				f.reset(t)
				runCLI(t, "upsert", `{"slug":"x","title":"X","labels":["bug"]}`)
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

// stepLoadsHubBoardConfig verifies the board config is the hub's board.yaml, whatever the worktree's own copy holds:
// a hub-declared type and label are accepted and rendered, and without the hub file the verb refuses naming `lyx fabric reconcile`.
func stepLoadsHubBoardConfig(t *testing.T, f *cliFixture) {
	tests := []struct {
		name         string
		hubConfig    string
		wantExitCode int
		wantError    string
		wantReadme   string
	}{
		{
			name:         "hub vocabulary applies although the worktree copy lacks it",
			hubConfig:    "readme: Home.md\ndesign_prefix: proposal-\ntypes:\n  feature: A new capability\nlabels:\n  area: An area\n",
			wantExitCode: 0,
			wantReadme:   "### Features",
		},
		{
			name:         "worktree copy alone does not satisfy the reader",
			wantExitCode: 1,
			wantError:    "lyx fabric reconcile",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f.reset(t)
			hub := filepath.Dir(f.cwd)
			if err := os.MkdirAll(configengine.ConfigDir(f.cwd), 0o755); err != nil {
				t.Fatalf("mkdir worktree config: %v", err)
			}
			worktreeConfig := "readme: Home.md\ndesign_prefix: proposal-\n"
			if err := os.WriteFile(configengine.ConfigFile(f.cwd, "board"), []byte(worktreeConfig), 0o644); err != nil {
				t.Fatalf("write worktree board.yaml: %v", err)
			}
			if tt.hubConfig != "" {
				seedHubBoardConfig(t, hub, tt.hubConfig)
			} else if err := os.Remove(configengine.ConfigFile(fabricengine.BoardDir(hub), "board")); err != nil {
				t.Fatalf("remove hub board.yaml: %v", err)
			}

			exitCode, stdout := runCLI(t, "upsert", `{"slug":"n","title":"N","kind":"note","labels":["feature","area"]}`)
			if exitCode != tt.wantExitCode {
				t.Fatalf("upsert exit %d, want %d; stdout: %s", exitCode, tt.wantExitCode, stdout)
			}
			if tt.wantError != "" && !strings.Contains(stdout, tt.wantError) {
				t.Fatalf("stdout %q does not name %q", stdout, tt.wantError)
			}
			if tt.wantReadme != "" {
				readme, err := os.ReadFile(filepath.Join(fabricengine.BoardDir(hub), "Home.md"))
				if err != nil {
					t.Fatalf("read rendered README: %v", err)
				}
				if !strings.Contains(string(readme), tt.wantReadme) {
					t.Fatalf("README lacks %q:\n%s", tt.wantReadme, readme)
				}
			}
		})
	}
}

// stepBoardPathResolution verifies the two board data dir resolution paths in PersistentPreRunE:
// without --board-path the CLI uses fabricengine.BoardDir(hub) derived from lyxcwd.Resolve;
// with --board-path the supplied path takes precedence.
// The fixture's real git repo lets lyxcwd.Resolve succeed.
func stepBoardPathResolution(t *testing.T, f *cliFixture) {
	// The fixture's repository is the worktree and its parent is the hub:
	// lyxcwd.Resolve(worktree) derives Hub = topDir, so fabricengine.BoardDir(Hub) = filepath.Join(topDir, "_board").
	topDir := filepath.Dir(f.cwd)

	expectedBoardDir := fabricengine.BoardDir(topDir)

	t.Run("no_board_path_resolves_via_paths", func(t *testing.T) {
		// PersistentPreRunE calls lyxcwd.Resolve and derives cfg.Path = fabricengine.BoardDir(topDir).
		// Upsert writes board.json inside that derived board dir.
		exitCode, stdout := runCLI(t, "upsert", `{"slug":"path-test","title":"Path Test","labels":["bug"]}`)
		if exitCode != 0 {
			t.Fatalf("upsert exit %d; stdout: %s", exitCode, stdout)
		}
		storeFile := filepath.Join(expectedBoardDir, "board.json")
		if _, err := os.Stat(storeFile); err != nil {
			t.Errorf("board not at fabricengine.BoardDir(hub) %q: %v", expectedBoardDir, err)
		}
	})

	t.Run("board_path_flag_overrides_resolution", func(t *testing.T) {
		// --board-path bypasses lyxcwd.Resolve and uses the supplied path directly.
		// list is a read-only operation that works under --board-path (no render step),
		// so we verify redirection by comparing task counts: absOverride is a fresh
		// empty dir (0 tasks) while expectedBoardDir already has the "path-test" task
		// from the first subtest (1 task).
		absOverride := t.TempDir()

		// list from the override dir returns 0 tasks (empty board).
		exitOverride, outOverride := runCLI(t, "--board-path", absOverride, "list")
		if exitOverride != 0 {
			t.Fatalf("list --board-path exit %d; stdout: %s", exitOverride, outOverride)
		}
		var overrideResult map[string]any
		if err := json.Unmarshal([]byte(outOverride), &overrideResult); err != nil {
			t.Fatalf("parse override list: %v", err)
		}
		overrideTasks, _ := overrideResult["tasks"].([]any)
		if len(overrideTasks) != 0 {
			t.Errorf("--board-path override: got %d task(s) from fresh dir %q; want 0",
				len(overrideTasks), absOverride)
		}

		// list via the default (git-resolved) path returns the "path-test" task.
		exitDefault, outDefault := runCLI(t, "list")
		if exitDefault != 0 {
			t.Fatalf("list (default path) exit %d; stdout: %s", exitDefault, outDefault)
		}
		var defaultResult map[string]any
		if err := json.Unmarshal([]byte(outDefault), &defaultResult); err != nil {
			t.Fatalf("parse default list: %v", err)
		}
		defaultTasks, _ := defaultResult["tasks"].([]any)
		if len(defaultTasks) == 0 {
			t.Errorf("default board path %q: got 0 tasks; want >= 1 (path-test task from first subtest)",
				expectedBoardDir)
		}
	})
}
