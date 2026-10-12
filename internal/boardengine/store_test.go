// store_test.go — unit tests for the Store (store.go).
//
// CRUD, sequential ID assignment, and every validation rule: dangling deps, isolated/kind/label constraints, cycle detection, and batch/merge atomicity.

package boardengine_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/boardengine"
)

func TestUpsertTaskNewTaskSequentialID(t *testing.T) {
	s := boardengine.NewStore("")

	// (a) new task gets sequential ID starting at 0
	task1, err := s.UpsertTask(map[string]any{
		"slug":  "task1",
		"title": "Task 1",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if task1.ID != 0 {
		t.Errorf("expected ID 0, got %d", task1.ID)
	}

	task2, err := s.UpsertTask(map[string]any{
		"slug":  "task2",
		"title": "Task 2",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if task2.ID != 1 {
		t.Errorf("expected ID 1, got %d", task2.ID)
	}
}

func TestUpsertTaskDefaults(t *testing.T) {
	s := boardengine.NewStore("")

	// (b) defaults applied (DependsOn=[], Isolated=false, Kind=note, Labels=[])
	task, err := s.UpsertTask(map[string]any{
		"slug": "task1",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(task.DependsOn) != 0 {
		t.Errorf("expected empty DependsOn, got %v", task.DependsOn)
	}
	if task.Isolated {
		t.Errorf("expected Isolated=false, got true")
	}
	if task.Kind != boardengine.KindNote {
		t.Errorf("expected Kind=note, got %q", task.Kind)
	}
	if task.Labels == nil || len(task.Labels) != 0 {
		t.Errorf("expected empty non-nil Labels, got %#v", task.Labels)
	}

	if err := s.UpsertTasksBatch([]map[string]any{{"slug": "task2"}}); err != nil {
		t.Fatalf("batch: %v", err)
	}
	batched, _ := s.GetTask("task2")
	if batched.Kind != boardengine.KindNote || len(batched.Labels) != 0 {
		t.Errorf("batch defaults: got kind=%q labels=%v", batched.Kind, batched.Labels)
	}
}

func TestUpsertKindAndRetiredKeys(t *testing.T) {
	s := boardengine.NewStore("")

	_, err := s.UpsertTask(map[string]any{"slug": "a", "kind": "epic"})
	if err == nil || !stringContains(err.Error(), "kind") {
		t.Errorf("expected kind error, got %v", err)
	}
	if err := s.UpsertTasksBatch([]map[string]any{{"slug": "a", "kind": "epic"}}); err == nil {
		t.Errorf("expected batch kind error")
	}

	for key, wantHint := range map[string]string{"tier": `"kind"`, "type": `"labels"`, "deferred": `"kind": "note"`} {
		_, err = s.UpsertTask(map[string]any{"slug": "a", key: 1})
		if err == nil || !stringContains(err.Error(), "unknown field") || !stringContains(err.Error(), wantHint) {
			t.Errorf("key %q: expected unknown-field refusal naming %s, got %v", key, wantHint, err)
		}
	}

	if _, err := s.UpsertTask(map[string]any{"slug": "b", "kind": "task", "labels": []string{"bug"}}); err != nil {
		t.Fatalf("valid kind and labels refused: %v", err)
	}
	if _, err := s.UpsertTask(map[string]any{"slug": "b", "kind": "epic"}); err == nil {
		t.Errorf("patch to kind epic should be refused")
	}
}

func TestKindDependencyRules(t *testing.T) {
	// seed builds a task "dep" and a note "idea", with no vocabulary so labels are not checked.
	seed := func(t *testing.T) *boardengine.Store {
		t.Helper()
		s := boardengine.NewStore("")
		if _, err := s.UpsertTask(map[string]any{"slug": "dep", "kind": "task"}); err != nil {
			t.Fatal(err)
		}
		if _, err := s.UpsertTask(map[string]any{"slug": "idea", "kind": "note"}); err != nil {
			t.Fatal(err)
		}
		return s
	}

	t.Run("a note with depends_on refused", func(t *testing.T) {
		s := seed(t)
		_, err := s.UpsertTask(map[string]any{"slug": "n", "kind": "note", "depends_on": []string{"dep"}})
		if err == nil || !stringContains(err.Error(), "set-deps") || !stringContains(err.Error(), "promote") {
			t.Errorf("expected refusal naming set-deps and promote, got %v", err)
		}
		if _, err := s.UpsertTask(map[string]any{"slug": "x", "kind": "task", "depends_on": []string{"dep"}}); err != nil {
			t.Fatal(err)
		}
		if _, err := s.UpsertTask(map[string]any{"slug": "x", "kind": "note"}); err == nil {
			t.Errorf("demoting a task that has depends_on should be refused")
		}
	})

	t.Run("depending on an open note refused", func(t *testing.T) {
		s := seed(t)
		_, err := s.UpsertTask(map[string]any{"slug": "x", "kind": "task", "depends_on": []string{"idea"}})
		if err == nil || !stringContains(err.Error(), `"idea"`) || !stringContains(err.Error(), "promote") || !stringContains(err.Error(), "set-deps") {
			t.Errorf("expected refusal naming idea, promote and set-deps, got %v", err)
		}
	})

	t.Run("depending on a done note refused", func(t *testing.T) {
		s := seed(t)
		if err := s.SetStatus("idea", stringPtr("done")); err != nil {
			t.Fatal(err)
		}
		_, err := s.UpsertTask(map[string]any{"slug": "x", "kind": "task", "depends_on": []string{"idea"}})
		if err == nil || !stringContains(err.Error(), `"idea"`) {
			t.Errorf("expected refusal naming idea, got %v", err)
		}
	})

	t.Run("demoting a task another entry depends on refused", func(t *testing.T) {
		s := seed(t)
		if _, err := s.UpsertTask(map[string]any{"slug": "x", "kind": "task", "depends_on": []string{"dep"}}); err != nil {
			t.Fatal(err)
		}
		_, err := s.UpsertTask(map[string]any{"slug": "dep", "kind": "note"})
		if err == nil || !stringContains(err.Error(), "x") || !stringContains(err.Error(), "set-deps") {
			t.Errorf("expected refusal naming the dependent x and set-deps, got %v", err)
		}
	})

	t.Run("set-deps on a note refused", func(t *testing.T) {
		s := seed(t)
		if err := s.SetDeps("idea", []string{"dep"}); err == nil {
			t.Errorf("a note must not take depends_on")
		}
	})
}

func TestUpsertTaskPreservesFields(t *testing.T) {
	s := boardengine.NewStore("")

	// Create a task
	_, err := s.UpsertTask(map[string]any{
		"slug":  "task1",
		"title": "Original",
		"brief": "Original brief",
		"body":  "Original body",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// (c) update preserves unmentioned fields
	task, err := s.UpsertTask(map[string]any{
		"slug":  "task1",
		"title": "Updated",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if task.Title != "Updated" {
		t.Errorf("expected title Updated, got %s", task.Title)
	}
	if task.Brief != "Original brief" {
		t.Errorf("expected brief Original brief, got %s", task.Brief)
	}
	if task.Body != "Original body" {
		t.Errorf("expected body Original body, got %s", task.Body)
	}
}

func TestUpsertTaskGroupKeyError(t *testing.T) {
	s := boardengine.NewStore("")

	// (d) `group` key is rejected by the store allowlist (coverage relocated from task_test.go)
	_, err := s.UpsertTask(map[string]any{
		"slug":  "task1",
		"group": "something",
	})
	if err == nil {
		t.Fatalf("expected error for group key")
	}
	if err.Error() != `unknown field: "group"` {
		t.Errorf("unexpected error message: %v", err)
	}
}

// TestUpsertFieldAllowlist verifies that the store chokepoint rejects unknown upsert fields on all
// three entry points: UpsertTask, UpsertTasksBatch, and MergeTasks.
// Also verifies that `status` IS in the allowed set and is persisted correctly.
func TestUpsertFieldAllowlist(t *testing.T) {
	t.Run("upsert_stray_phase_key_errors", func(t *testing.T) {
		s := boardengine.NewStore("")
		_, err := s.UpsertTask(map[string]any{
			"slug":  "task1",
			"phase": "active",
		})
		if err == nil {
			t.Fatalf("expected error for stray phase key")
		}
		// The "phase" key gets a friendly hint toward the renamed "status" field.
		wantSubstr := `unknown field: "phase"`
		if !stringContains(err.Error(), wantSubstr) {
			t.Errorf("expected error containing %q, got %v", wantSubstr, err)
		}
		if !stringContains(err.Error(), "status") {
			t.Errorf("expected hint mentioning 'status', got %v", err)
		}
	})

	t.Run("upsert_typo_key_errors", func(t *testing.T) {
		s := boardengine.NewStore("")
		_, err := s.UpsertTask(map[string]any{
			"slug":  "task1",
			"titel": "A",
		})
		if err == nil {
			t.Fatalf("expected error for typo key 'titel'")
		}
		if !stringContains(err.Error(), `"titel"`) {
			t.Errorf("expected error to name the offending key, got %v", err)
		}
	})

	t.Run("upsert_status_field_allowed_and_persisted", func(t *testing.T) {
		s := boardengine.NewStore("")
		task, err := s.UpsertTask(map[string]any{
			"slug":   "task1",
			"status": "active",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if task.Status == nil || *task.Status != "active" {
			t.Errorf("expected status=active, got %v", task.Status)
		}
		// Verify the value is persisted in the store.
		retrieved, found := s.GetTask("task1")
		if !found {
			t.Fatalf("task not found after upsert")
		}
		if retrieved.Status == nil || *retrieved.Status != "active" {
			t.Errorf("expected stored status=active, got %v", retrieved.Status)
		}
	})

	t.Run("upsert_short_name_field_allowed_and_persisted", func(t *testing.T) {
		s := boardengine.NewStore("")
		task, err := s.UpsertTask(map[string]any{
			"slug":       "task1",
			"short_name": "t1",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if task.ShortName != "t1" {
			t.Errorf("expected short_name=t1, got %v", task.ShortName)
		}
		// Verify the value is persisted in the store.
		retrieved, found := s.GetTask("task1")
		if !found {
			t.Fatalf("task not found after upsert")
		}
		if retrieved.ShortName != "t1" {
			t.Errorf("expected stored short_name=t1, got %v", retrieved.ShortName)
		}
	})

	t.Run("upsert_run_settings_field_allowed_and_persisted", func(t *testing.T) {
		s := boardengine.NewStore("")
		task, err := s.UpsertTask(map[string]any{
			"slug":         "task1",
			"run_settings": map[string]any{"loom": map[string]any{"review_max_bounces": 2}},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		retrieved, found := s.GetTask("task1")
		if !found {
			t.Fatalf("task not found after upsert")
		}
		loom, _ := retrieved.RunSettings["loom"].(map[string]any)
		if loom["review_max_bounces"] != float64(2) || task.RunSettings["loom"] == nil {
			t.Errorf("expected stored run_settings loom.review_max_bounces=2, got %v", retrieved.RunSettings)
		}
	})

	t.Run("upsert_recipe_field_allowed_and_persisted", func(t *testing.T) {
		s := boardengine.NewStore("")
		task, err := s.UpsertTask(map[string]any{
			"slug":   "task1",
			"recipe": "batten",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if task.Recipe != "batten" {
			t.Errorf("expected recipe=batten, got %v", task.Recipe)
		}
		// Verify the value is persisted in the store.
		retrieved, found := s.GetTask("task1")
		if !found {
			t.Fatalf("task not found after upsert")
		}
		if retrieved.Recipe != "batten" {
			t.Errorf("expected stored recipe=batten, got %v", retrieved.Recipe)
		}
	})

	t.Run("upsert_batch_stray_phase_errors", func(t *testing.T) {
		s := boardengine.NewStore("")
		err := s.UpsertTasksBatch([]map[string]any{
			{"slug": "task1", "phase": "done"},
		})
		if err == nil {
			t.Fatalf("expected error for batch with stray phase key")
		}
		if !stringContains(err.Error(), `"phase"`) {
			t.Errorf("expected error naming 'phase', got %v", err)
		}
	})

	t.Run("merge_upsert_stray_phase_errors", func(t *testing.T) {
		s := boardengine.NewStore("")
		_, err := s.MergeTasks(
			nil,
			map[string]any{"slug": "task1", "phase": "done"},
			nil,
		)
		if err == nil {
			t.Fatalf("expected error for merge-upsert with stray phase key")
		}
		if !stringContains(err.Error(), `"phase"`) {
			t.Errorf("expected error naming 'phase', got %v", err)
		}
	})

	t.Run("group_still_errors_via_allowlist", func(t *testing.T) {
		s := boardengine.NewStore("")
		err := s.UpsertTasksBatch([]map[string]any{
			{"slug": "task1", "group": "G"},
		})
		if err == nil {
			t.Fatalf("expected error for group key via allowlist")
		}
		if !stringContains(err.Error(), `"group"`) {
			t.Errorf("expected error naming 'group', got %v", err)
		}
	})
}

// TestValidateDependencyGraph verifies that UpsertTask rejects a dangling dependency, a dependency on an isolated task and a cycle with precise error messages, and accepts a chain without a cycle (the kind rules have their own test).
func TestValidateDependencyGraph(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		// seed holds the upserts that must succeed before the checked one.
		seed   []map[string]any
		upsert map[string]any
		// wantErr is the exact error message; wantErrContains is a substring of it; both empty means the upsert succeeds.
		wantErr         string
		wantErrContains string
		wantTasks       int
	}{
		{
			name:    "a dangling dependency is rejected",
			upsert:  map[string]any{"slug": "task1", "depends_on": []string{"nonexistent"}},
			wantErr: `dangling dependency: "nonexistent" does not exist`,
		},
		{
			name:    "a dependency on an isolated task is rejected",
			seed:    []map[string]any{{"slug": "isolated", "isolated": true}},
			upsert:  map[string]any{"slug": "task1", "depends_on": []string{"isolated"}},
			wantErr: `cannot depend on isolated task "isolated"`,
		},
		{
			name: "a cycle is detected",
			seed: []map[string]any{
				{"slug": "a", "title": "A", "kind": "task"},
				{"slug": "b", "title": "B", "kind": "task", "depends_on": []string{"a"}},
			},
			upsert:          map[string]any{"slug": "a", "depends_on": []string{"b"}},
			wantErrContains: "cycle detected",
		},
		{
			name: "a chain without a cycle is accepted",
			seed: []map[string]any{
				{"slug": "c", "kind": "task"},
				{"slug": "b", "kind": "task", "depends_on": []string{"c"}},
			},
			upsert:    map[string]any{"slug": "a", "kind": "task", "depends_on": []string{"b"}},
			wantTasks: 3,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s := boardengine.NewStore("")
			for _, fields := range tt.seed {
				if _, err := s.UpsertTask(fields); err != nil {
					t.Fatalf("seed %v: %v", fields, err)
				}
			}

			_, err := s.UpsertTask(tt.upsert)

			switch {
			case tt.wantErr != "":
				if err == nil || err.Error() != tt.wantErr {
					t.Fatalf("error = %v; want %q", err, tt.wantErr)
				}
			case tt.wantErrContains != "":
				if err == nil || !strings.Contains(err.Error(), tt.wantErrContains) {
					t.Fatalf("error = %v; want one containing %q", err, tt.wantErrContains)
				}
			default:
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if got := len(s.Tasks()); got != tt.wantTasks {
					t.Errorf("expected %d tasks, got %d", tt.wantTasks, got)
				}
			}
		})
	}
}

func TestRemoveTaskMissing(t *testing.T) {
	s := boardengine.NewStore("")

	// (j) RemoveTask returns error for missing slug
	err := s.RemoveTask("nonexistent")
	if err == nil {
		t.Fatalf("expected error for missing task")
	}
	if err.Error() != "task not found: nonexistent" {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestSetStatus verifies SetStatus behaviour: clearing status via nil and the current silent no-op
// for a missing slug (changed to an error in Card 3).
//
// Folds: TestSetPhaseNil, TestSetPhaseMissing
func TestSetStatus(t *testing.T) {
	t.Run("TestSetPhaseNil", func(t *testing.T) {
		s := boardengine.NewStore("")

		task, err := s.UpsertTask(map[string]any{
			"slug": "task1",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		status := "in progress"
		task.Status = &status
		// Manually update via upsert so the store has the initial status set.
		s.UpsertTask(map[string]any{
			"slug":   "task1",
			"status": status,
		})

		// nil status clears the stored status field.
		err = s.SetStatus("task1", nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		retrieved, _ := s.GetTask("task1")
		if retrieved.Status != nil {
			t.Errorf("expected nil status, got %v", retrieved.Status)
		}
	})

	t.Run("TestSetPhaseMissing", func(t *testing.T) {
		s := boardengine.NewStore("")

		// Missing target now returns "task not found" instead of the former silent no-op.
		err := s.SetStatus("nonexistent", nil)
		if err == nil {
			t.Fatalf("expected error for missing task, got nil")
		}
		if err.Error() != "task not found: nonexistent" {
			t.Errorf("unexpected error message: %v", err)
		}
	})
}

// TestMergeTasks verifies both the happy path (atomic remove+upsert+set_phase) and the rollback
// path (validation error leaves store unchanged).
//
// Folds: TestMergeTasksAtomic, TestMergeTasksValidationRollback
func TestMergeTasks(t *testing.T) {
	t.Run("TestMergeTasksAtomic", func(t *testing.T) {
		s := boardengine.NewStore("")

		// Create initial tasks
		_, err := s.UpsertTask(map[string]any{
			"slug":  "a",
			"title": "A",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		_, err = s.UpsertTask(map[string]any{
			"slug":  "b",
			"title": "B",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// (l) remove + upsert + set_status all execute atomically
		phase := "done"
		result, err := s.MergeTasks(
			[]string{"a"},
			map[string]any{
				"slug":  "c",
				"title": "C",
			},
			&boardengine.MergeStatusUpdate{Selector: "c", Status: &phase},
		)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Verify removed
		_, found := s.GetTask("a")
		if found {
			t.Errorf("expected task a to be removed")
		}

		// Verify upserted
		if result.Slug != "c" {
			t.Errorf("expected upserted task to be c, got %s", result.Slug)
		}

		// Verify phase set
		retrieved, _ := s.GetTask("c")
		if retrieved.Status == nil || *retrieved.Status != "done" {
			t.Errorf("expected status done, got %v", retrieved.Status)
		}

		// Verify b still exists
		_, found = s.GetTask("b")
		if !found {
			t.Errorf("expected task b to still exist")
		}
	})

	t.Run("TestMergeTasksNonStringSlugErrors", func(t *testing.T) {
		s := boardengine.NewStore("")

		// A JSON payload can carry any type as slug; a number must produce an
		// envelope-able error, not an interface-conversion panic.
		_, err := s.MergeTasks(nil, map[string]any{"slug": float64(123), "title": "num"}, nil)
		if err == nil {
			t.Fatalf("MergeTasks with numeric slug should error")
		}
		if got, want := err.Error(), "slug must be a non-empty string"; got != want {
			t.Errorf("MergeTasks numeric slug error = %q; want %q", got, want)
		}
	})

	t.Run("TestMergeTasksValidationRollback", func(t *testing.T) {
		s := boardengine.NewStore("")

		// Create tasks
		_, err := s.UpsertTask(map[string]any{
			"slug":     "a",
			"isolated": true,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		_, err = s.UpsertTask(map[string]any{
			"slug":  "b",
			"title": "B",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// (m) validation error on upsert rolls back — nothing is mutated
		before := len(s.Tasks())
		_, err = s.MergeTasks(
			[]string{"b"},
			map[string]any{
				"slug":       "c",
				"depends_on": []string{"nonexistent"},
			},
			nil,
		)
		if err == nil {
			t.Fatalf("expected validation error")
		}

		// Verify nothing changed
		after := len(s.Tasks())
		if before != after {
			t.Errorf("expected store to be unchanged, but length changed from %d to %d", before, after)
		}

		// Verify b still exists
		_, found := s.GetTask("b")
		if !found {
			t.Errorf("expected task b to still exist after rollback")
		}
	})
}

// TestMergeTasksSetStatusRollback verifies that a merge whose set_status targets a non-existent
// slug returns an error and leaves the store unchanged.
// The remove and upsert steps are applied in-memory but writeOp discards them when mutate errors —
// confirmed by loading a fresh store from disk and asserting the task list is identical.
func TestMergeTasksSetStatusRollback(t *testing.T) {
	taskPath := t.TempDir()

	// Create an initial store with one task.
	s := boardengine.NewStore(taskPath)
	if err := s.Load(); err != nil {
		t.Fatalf("load: %v", err)
	}
	if _, err := s.UpsertTask(map[string]any{"slug": "existing", "title": "Existing"}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if err := s.Save(); err != nil {
		t.Fatalf("save: %v", err)
	}

	// Reload to simulate a fresh writeOp cycle.
	s2 := boardengine.NewStore(taskPath)
	if err := s2.Load(); err != nil {
		t.Fatalf("load s2: %v", err)
	}

	// MergeTasks with a set_status that targets a non-existent slug. The remove +
	// upsert steps execute in-memory, but set_status errors so MergeTasks returns an
	// error. The caller (writeOp) must not call Save() on error.
	status := "active"
	_, err := s2.MergeTasks(
		nil,
		map[string]any{"slug": "new-task", "title": "New"},
		&boardengine.MergeStatusUpdate{Selector: "ghost", Status: &status},
	)
	if err == nil {
		t.Fatalf("expected error when set_status targets non-existent slug")
	}
	if !stringContains(err.Error(), "task not found") {
		t.Errorf("expected 'task not found' error, got %v", err)
	}

	// Load a fresh store from disk (as writeOp would after not calling Save).
	// The on-disk task list must be identical to before the failed merge.
	s3 := boardengine.NewStore(taskPath)
	if err := s3.Load(); err != nil {
		t.Fatalf("load s3: %v", err)
	}
	tasks := s3.Tasks()
	if len(tasks) != 1 || tasks[0].Slug != "existing" {
		t.Errorf("expected on-disk store to be unchanged (1 task 'existing'), got %v", tasks)
	}
}

func TestListTasksBriefLayerAndProposal(t *testing.T) {
	s := boardengine.NewStore("")

	// Create tasks
	_, err := s.UpsertTask(map[string]any{
		"slug": "task1",
		"body": "Some body content",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = s.UpsertTask(map[string]any{
		"slug": "task2",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// (n) ListTasksBrief returns Layer and HasProposal computed correctly
	brief := s.ListTasksBrief(nil)
	if len(brief) != 2 {
		t.Errorf("expected 2 brief tasks, got %d", len(brief))
	}

	task1 := brief[0]
	if !task1.HasProposal {
		t.Errorf("expected HasProposal=true for task1 (has body)")
	}

	task2 := brief[1]
	if task2.HasProposal {
		t.Errorf("expected HasProposal=false for task2 (no body)")
	}

	// Both should have a layer assigned (even if empty or a letter)
	if task1.Layer == "" || task2.Layer == "" {
		t.Logf("task1 layer: %s, task2 layer: %s", task1.Layer, task2.Layer)
	}
}

// TestListAndFindReadmeOrder verifies ListTasksBrief and Find return entries in README order:
// open tasks first, then open notes, done last, with priority ordering within a group, and that each listed entry carries its priority.
func TestListAndFindReadmeOrder(t *testing.T) {
	s := boardengine.NewStore("")
	for _, f := range []map[string]any{
		{"slug": "x-low", "kind": "task", "priority": "low"},
		{"slug": "x-note", "kind": "note"},
		{"slug": "x-task", "kind": "task"},
		{"slug": "x-note2", "kind": "note", "priority": "high"},
		{"slug": "x-done", "kind": "task"},
	} {
		if _, err := s.UpsertTask(f); err != nil {
			t.Fatalf("UpsertTask %v: %v", f, err)
		}
	}
	if err := s.SetStatus("x-done", stringPtr("done")); err != nil {
		t.Fatalf("SetStatus: %v", err)
	}

	want := "x-task,x-low:low,x-note2:high,x-note,x-done"
	slugs := func(bs []boardengine.BriefTask) string {
		var out []string
		for _, b := range bs {
			if b.Priority != "" {
				out = append(out, b.Slug+":"+b.Priority)
				continue
			}
			out = append(out, b.Slug)
		}
		return strings.Join(out, ",")
	}
	if got := slugs(s.ListTasksBrief(nil)); got != want {
		t.Errorf("ListTasksBrief order = %s, want %s", got, want)
	}
	if got := slugs(s.Find("x-", nil)); got != want {
		t.Errorf("Find order = %s, want %s", got, want)
	}
}

// TestListAndFindLabelFilter verifies the label filter keeps only entries carrying every named label, in README order.
func TestListAndFindLabelFilter(t *testing.T) {
	s := boardengine.NewStore("")
	for _, f := range []map[string]any{
		{"slug": "x-a", "kind": "note", "labels": []string{"a"}},
		{"slug": "x-ab", "kind": "task", "labels": []string{"a", "b"}},
		{"slug": "x-b", "kind": "note", "labels": []string{"b"}},
	} {
		if _, err := s.UpsertTask(f); err != nil {
			t.Fatalf("UpsertTask %v: %v", f, err)
		}
	}
	slugs := func(bs []boardengine.BriefTask) string {
		var out []string
		for _, b := range bs {
			out = append(out, b.Slug)
		}
		return strings.Join(out, ",")
	}

	if got := slugs(s.ListTasksBrief([]string{"a"})); got != "x-ab,x-a" {
		t.Errorf("list --label a = %s, want x-ab,x-a", got)
	}
	if got := slugs(s.ListTasksBrief([]string{"a", "b"})); got != "x-ab" {
		t.Errorf("list --label a --label b = %s, want x-ab", got)
	}
	if got := slugs(s.Find("x-", []string{"b"})); got != "x-ab,x-b" {
		t.Errorf("find --label b = %s, want x-ab,x-b", got)
	}
	if got := slugs(s.Find("x-", []string{"a", "b"})); got != "x-ab" {
		t.Errorf("find --label a --label b = %s, want x-ab", got)
	}
}

// TestSetDeps verifies both the valid update path and the cycle-detection rollback for SetDeps.
//
// Folds: TestSetDepsValid, TestSetDepsCycleRollback
func TestSetDeps(t *testing.T) {
	t.Run("TestSetDepsValid", func(t *testing.T) {
		s := boardengine.NewStore("")

		// Create tasks
		_, err := s.UpsertTask(map[string]any{
			"slug": "a",
			"kind": "task",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		_, err = s.UpsertTask(map[string]any{
			"slug": "b",
			"kind": "task",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// (o) SetDeps: valid update succeeds
		err = s.SetDeps("b", []string{"a"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		retrieved, _ := s.GetTask("b")
		if len(retrieved.DependsOn) != 1 || retrieved.DependsOn[0] != "a" {
			t.Errorf("expected DependsOn [a], got %v", retrieved.DependsOn)
		}
	})

	t.Run("TestSetDepsCycleRollback", func(t *testing.T) {
		s := boardengine.NewStore("")

		// Create A and B with A depending on B
		_, err := s.UpsertTask(map[string]any{
			"slug": "a",
			"kind": "task",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		_, err = s.UpsertTask(map[string]any{
			"slug":       "b",
			"kind":       "task",
			"depends_on": []string{"a"},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// (p) setting deps that create a cycle returns error and leaves store unchanged
		originalB, _ := s.GetTask("b")
		err = s.SetDeps("a", []string{"b"})
		if err == nil {
			t.Fatalf("expected error for cycle detection")
		}

		// Verify a is unchanged
		retrievedA, _ := s.GetTask("a")
		if len(retrievedA.DependsOn) != 0 {
			t.Errorf("expected a to have no deps, got %v", retrievedA.DependsOn)
		}

		// Verify b is unchanged
		retrievedB, _ := s.GetTask("b")
		if !sliceEqualStrings(retrievedB.DependsOn, originalB.DependsOn) {
			t.Errorf("expected b deps to be unchanged, got %v", retrievedB.DependsOn)
		}
	})
}

// TestUpsertTasksBatch verifies that a valid batch upserts all tasks and that an invalid batch
// returns an error without mutating the store.
//
// Folds: TestUpsertTasksBatchValid, TestUpsertTasksBatchInvalid
func TestUpsertTasksBatch(t *testing.T) {
	t.Run("TestUpsertTasksBatchValid", func(t *testing.T) {
		s := boardengine.NewStore("")

		// (q) valid batch of two tasks both upserted
		err := s.UpsertTasksBatch([]map[string]any{
			{
				"slug":  "task1",
				"title": "Task 1",
			},
			{
				"slug":  "task2",
				"title": "Task 2",
			},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		tasks := s.Tasks()
		if len(tasks) != 2 {
			t.Errorf("expected 2 tasks, got %d", len(tasks))
		}

		if tasks[0].Slug != "task1" || tasks[1].Slug != "task2" {
			t.Errorf("expected slugs task1 and task2, got %s and %s", tasks[0].Slug, tasks[1].Slug)
		}
	})

	t.Run("TestUpsertTasksBatchForwardReference", func(t *testing.T) {
		s := boardengine.NewStore("")

		// A batch is validated against its full projected snapshot, so an
		// element may depend on one introduced later in the same batch.
		err := s.UpsertTasksBatch([]map[string]any{
			{"slug": "task-a", "title": "A", "kind": "task", "depends_on": []string{"task-b"}},
			{"slug": "task-b", "title": "B", "kind": "task"},
		})
		if err != nil {
			t.Fatalf("forward-referencing batch should succeed: %v", err)
		}

		taskA, found := s.GetTask("task-a")
		if !found || len(taskA.DependsOn) != 1 || taskA.DependsOn[0] != "task-b" {
			t.Errorf("task-a after batch = %+v; want depends_on [task-b]", taskA)
		}
		if _, found := s.GetTask("task-b"); !found {
			t.Errorf("task-b missing after batch")
		}
	})

	t.Run("TestUpsertTasksBatchFailureLeavesStoreUntouched", func(t *testing.T) {
		s := boardengine.NewStore("")
		if _, err := s.UpsertTask(map[string]any{"slug": "existing", "title": "Old title"}); err != nil {
			t.Fatalf("seed upsert: %v", err)
		}

		// The first element patches an existing task; the second fails
		// validation. The projection must not have written through to the live
		// store, so the patched title must be rolled back too — not just the
		// task count.
		err := s.UpsertTasksBatch([]map[string]any{
			{"slug": "existing", "title": "Patched title"},
			{"slug": "broken", "depends_on": []string{"nonexistent"}},
		})
		if err == nil {
			t.Fatalf("expected error for invalid batch")
		}

		existing, _ := s.GetTask("existing")
		if existing.Title != "Old title" {
			t.Errorf("existing.Title after failed batch = %q; want %q (in-memory rollback)", existing.Title, "Old title")
		}
	})

	t.Run("TestUpsertTasksBatchInvalid", func(t *testing.T) {
		s := boardengine.NewStore("")

		// Create initial state
		_, err := s.UpsertTask(map[string]any{
			"slug": "existing",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// (r) batch with one invalid task returns error and neither task is mutated
		beforeCount := len(s.Tasks())
		err = s.UpsertTasksBatch([]map[string]any{
			{
				"slug":       "task1",
				"depends_on": []string{"nonexistent"},
			},
			{
				"slug":  "task2",
				"title": "Task 2",
			},
		})
		if err == nil {
			t.Fatalf("expected error for invalid batch")
		}

		// Verify nothing was mutated
		afterCount := len(s.Tasks())
		if beforeCount != afterCount {
			t.Errorf("expected store unchanged, but count changed from %d to %d", beforeCount, afterCount)
		}
	})
}

// keptValue decodes the saved board.json raw and returns the compact value of key on the entry slug, or on the top level when slug is empty, and whether it is present.
func keptValue(t *testing.T, raw []byte, slug, key string) (string, bool) {
	t.Helper()
	members := map[string]json.RawMessage{}
	if err := json.Unmarshal(raw, &members); err != nil {
		t.Fatalf("decode board.json: %v", err)
	}
	if slug != "" {
		var entries []map[string]json.RawMessage
		if err := json.Unmarshal(members["entries"], &entries); err != nil {
			t.Fatalf("decode entries: %v", err)
		}
		members = nil
		for _, entry := range entries {
			var entrySlug string
			if err := json.Unmarshal(entry["slug"], &entrySlug); err != nil {
				t.Fatalf("decode slug: %v", err)
			}
			if entrySlug == slug {
				members = entry
				break
			}
		}
	}
	value, present := members[key]
	if !present {
		return "", false
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, value); err != nil {
		t.Fatalf("compact %s: %v", key, err)
	}
	return compact.String(), true
}

// keptBoardBody is the board.json the kept-key rows of TestLoad share: a top-level key, a task a with a big integer, a nested value, strings holding an escaped quote and closing brackets, and a key spelled with an escape, listed unsorted, a task b, a note n and a done task fin.
const keptBoardBody = `{"version":1,"entries":[` +
	`{"id":0,"slug":"a","title":"A","kind":"task","labels":["bug"],"zz_scalar":9007199254740993,"zz_nested":{"a":[1,{"b":"c"}]},` +
	`"zz_quote":"q\"}],x","zz_nested_quote":{"s":["q\"}],x",{"t":"\\}],"}]},"zz\u005fesc":1},` +
	`{"id":1,"slug":"b","title":"B","kind":"task","labels":["bug"],"zz_b":true},` +
	`{"id":2,"slug":"n","title":"N","kind":"note","labels":["bug"],"zz_n":"x"},` +
	`{"id":3,"slug":"fin","title":"Fin","kind":"task","labels":["bug"],"status":"done"}],` +
	`"zz_top":{"k":[1,2]}}`

// keptA lists the kept keys of task a and the top level in keptBoardBody, as slug, key and compact value.
var keptA = [][3]string{
	{"a", "zz_scalar", "9007199254740993"},
	{"a", "zz_nested", `{"a":[1,{"b":"c"}]}`},
	{"a", "zz_quote", `"q\"}],x"`},
	{"a", "zz_nested_quote", `{"s":["q\"}],x",{"t":"\\}],"}]}`},
	{"a", "zz_esc", "1"},
	{"", "zz_top", `{"k":[1,2]}`},
}

// TestLoad asserts Store.Load's handling of board.json: a missing file yields an empty store, nil depends_on and labels normalize to empty slices, the version-1 and old tier-and-type shapes load and a Save round-trips them, and an unknown version or corrupt file is an error.
// It also pins the kept keys: an entry key or top-level key this binary does not know survives every write path with its exact digits, after the known fields in key order, while the retired tier and type are still dropped.
func TestLoad(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		// body is the board.json content; empty means no file.
		body string
		// wantErr is a substring the Load error must contain; empty means Load succeeds.
		wantErr string
		// check inspects the loaded entries; nil skips it.
		check func(t *testing.T, tasks []boardengine.Task)
		// act mutates the loaded store after check and before the save.
		act func(t *testing.T, store *boardengine.Store)
		// save writes the store back, as do act and wantKeys; saveContains and saveAbsent are the whitespace-free substrings the saved file must and must not contain.
		save                     bool
		saveContains, saveAbsent []string
		// saveOrder lists whitespace-free substrings that must appear in this order in the saved file.
		saveOrder []string
		// wantKeys lists slug, key and compact value read back from the saved file; an empty value means the key must be absent.
		wantKeys [][3]string
	}{
		{
			name: "a missing file yields an empty store",
			check: func(t *testing.T, tasks []boardengine.Task) {
				if len(tasks) != 0 {
					t.Errorf("expected empty task list for missing file, got %d tasks", len(tasks))
				}
			},
		},
		{
			name: "a nil depends_on becomes an empty slice",
			body: `{"version":1,"entries":[{"id":0,"slug":"task1","title":"Task 1","tier":3,"type":"feature"}]}`,
			check: func(t *testing.T, tasks []boardengine.Task) {
				if len(tasks) != 1 || tasks[0].DependsOn == nil || len(tasks[0].DependsOn) != 0 {
					t.Errorf("want one entry with an empty non-nil DependsOn, got %+v", tasks)
				}
			},
		},
		{
			name: "a kind with no labels key becomes an empty labels list",
			body: `{"version":1,"entries":[{"id":0,"slug":"a","title":"A","kind":"note"}]}`,
			check: func(t *testing.T, tasks []boardengine.Task) {
				if len(tasks) != 1 || tasks[0].Labels == nil || len(tasks[0].Labels) != 0 {
					t.Errorf("want one entry with an empty non-nil labels list, got %+v", tasks)
				}
			},
		},
		{
			name: "the version-1 shape loads and Save keeps legacy_done",
			body: `{"version":1,"entries":[{"id":4,"slug":"a","title":"A","kind":"task","labels":["bug"],"depends_on":[]}],"legacy_done":["old"]}`,
			check: func(t *testing.T, tasks []boardengine.Task) {
				if len(tasks) != 1 || tasks[0].Slug != "a" || tasks[0].Kind != boardengine.KindTask || len(tasks[0].Labels) != 1 || tasks[0].Labels[0] != "bug" {
					t.Errorf("loaded %+v; want entry a as a task labelled bug", tasks)
				}
			},
			save:         true,
			saveContains: []string{`"legacy_done"`},
		},
		{
			name: "the old tier-and-type shape converts on Load and Save writes kind and labels",
			body: `{"version":1,"entries":[{"id":0,"slug":"a","title":"A","tier":1,"type":"bug","brief":"[infra] fix it","depends_on":[],"zz_scalar":9007199254740993}]}`,
			check: func(t *testing.T, tasks []boardengine.Task) {
				if len(tasks) != 1 || tasks[0].Kind != boardengine.KindTask || strings.Join(tasks[0].Labels, ",") != "bug,infra" || tasks[0].Brief != "fix it" {
					t.Errorf("loaded %+v; want a task labelled bug, infra with the stripped brief", tasks)
				}
			},
			save:         true,
			saveContains: []string{`"kind":"task"`, `"labels"`},
			saveAbsent:   []string{`"tier"`, `"type"`},
			wantKeys:     [][3]string{{"a", "zz_scalar", "9007199254740993"}},
		},
		{
			name: "set-status keeps kept keys after the known fields in key order",
			body: keptBoardBody,
			act: func(t *testing.T, store *boardengine.Store) {
				status := "active"
				if err := store.SetStatus("a", &status); err != nil {
					t.Fatalf("SetStatus: %v", err)
				}
			},
			saveOrder: []string{`"status":`, `"zz_nested"`, `"zz_scalar"`},
			wantKeys:  keptA,
		},
		{
			name: "an upsert keeps kept keys",
			body: keptBoardBody,
			act: func(t *testing.T, store *boardengine.Store) {
				if _, err := store.UpsertTask(map[string]any{"slug": "a", "title": "A2"}); err != nil {
					t.Fatalf("UpsertTask: %v", err)
				}
			},
			wantKeys: keptA,
		},
		{
			name: "a batch upsert keeps kept keys",
			body: keptBoardBody,
			act: func(t *testing.T, store *boardengine.Store) {
				if err := store.UpsertTasksBatch([]map[string]any{{"slug": "a", "title": "A2"}, {"slug": "b", "title": "B2"}}); err != nil {
					t.Fatalf("UpsertTasksBatch: %v", err)
				}
			},
			wantKeys: append([][3]string{{"b", "zz_b", "true"}}, keptA...),
		},
		{
			name: "promote keeps kept keys",
			body: keptBoardBody,
			act: func(t *testing.T, store *boardengine.Store) {
				if _, _, err := store.Promote("n"); err != nil {
					t.Fatalf("Promote: %v", err)
				}
			},
			wantKeys: append([][3]string{{"n", "zz_n", `"x"`}}, keptA...),
		},
		{
			name: "prune keeps kept keys",
			body: keptBoardBody,
			act: func(t *testing.T, store *boardengine.Store) {
				store.Prune()
			},
			wantKeys: keptA,
		},
		{
			name: "set-deps keeps kept keys",
			body: keptBoardBody,
			act: func(t *testing.T, store *boardengine.Store) {
				if err := store.SetDeps("a", []string{"b"}); err != nil {
					t.Fatalf("SetDeps: %v", err)
				}
			},
			wantKeys: keptA,
		},
		{
			name: "a merge keeps the upserted entry's keys and drops the removed entry's",
			body: keptBoardBody,
			act: func(t *testing.T, store *boardengine.Store) {
				if _, err := store.MergeTasks([]string{"b"}, map[string]any{"slug": "a", "title": "A2"}, nil); err != nil {
					t.Fatalf("MergeTasks: %v", err)
				}
			},
			wantKeys: append([][3]string{{"a", "zz_b", ""}, {"", "zz_b", ""}}, keptA...),
		},
		{
			name: "a remove drops the removed entry's keys",
			body: keptBoardBody,
			act: func(t *testing.T, store *boardengine.Store) {
				if err := store.RemoveTask("b"); err != nil {
					t.Fatalf("RemoveTask: %v", err)
				}
			},
			wantKeys: append([][3]string{{"a", "zz_b", ""}, {"", "zz_b", ""}}, keptA...),
		},
		{
			name: "a top-level key survives the first entry added to an empty store",
			body: `{"version":1,"entries":[],"zz_top":"x"}`,
			act: func(t *testing.T, store *boardengine.Store) {
				if _, err := store.UpsertTask(map[string]any{"slug": "first"}); err != nil {
					t.Fatalf("UpsertTask: %v", err)
				}
			},
			wantKeys: [][3]string{{"", "zz_top", `"x"`}},
		},
		{
			name: "a case-variant of a known key fills the field and is not also kept",
			body: `{"version":1,"entries":[{"id":0,"slug":"a","Title":"A","kind":"note"}]}`,
			check: func(t *testing.T, tasks []boardengine.Task) {
				if len(tasks) != 1 || tasks[0].Title != "A" {
					t.Errorf("loaded %+v; want title A", tasks)
				}
			},
			save:         true,
			saveContains: []string{`"title":"A"`},
			saveAbsent:   []string{`"Title"`},
		},
		{
			name:    "an unknown version is refused naming it",
			body:    `{"version":2,"entries":[]}`,
			wantErr: "version 2",
		},
		{
			name:    "a corrupt file is an error, not an empty store",
			body:    `{this is not valid json`,
			wantErr: "load store",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			boardDir := t.TempDir()
			if tt.body != "" {
				if err := os.WriteFile(filepath.Join(boardDir, "board.json"), []byte(tt.body), 0o644); err != nil {
					t.Fatalf("write board.json: %v", err)
				}
			}

			store := boardengine.NewStore(boardDir)
			err := store.Load()
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("Load error = %v; want one containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if tt.check != nil {
				tt.check(t, store.Tasks())
			}
			if tt.act != nil {
				tt.act(t, store)
			}

			if !tt.save && tt.act == nil && tt.wantKeys == nil {
				return
			}
			if err := store.Save(); err != nil {
				t.Fatalf("Save: %v", err)
			}
			raw, err := os.ReadFile(filepath.Join(boardDir, "board.json"))
			if err != nil {
				t.Fatalf("read board.json: %v", err)
			}
			compact := strings.Join(strings.Fields(string(raw)), "")
			for _, want := range tt.saveContains {
				if !strings.Contains(compact, want) {
					t.Errorf("Save output lacks %s: %s", want, raw)
				}
			}
			for _, absent := range tt.saveAbsent {
				if strings.Contains(compact, absent) {
					t.Errorf("Save output must not contain %s: %s", absent, raw)
				}
			}
			position := 0
			for _, want := range tt.saveOrder {
				index := strings.Index(compact[position:], want)
				if index < 0 {
					t.Errorf("Save output lacks %s after offset %d: %s", want, position, compact)
					break
				}
				position += index + len(want)
			}
			for _, key := range tt.wantKeys {
				slug, name, want := key[0], key[1], key[2]
				got, present := keptValue(t, raw, slug, name)
				if want == "" {
					if present {
						t.Errorf("entry %q key %s = %s; want it absent", slug, name, got)
					}
					continue
				}
				if !present || got != want {
					t.Errorf("entry %q key %s = %q (present %v); want %q", slug, name, got, present, want)
				}
			}
		})
	}
}

func sliceEqualStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func stringContains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestPromote(t *testing.T) {
	seed := func(t *testing.T) *boardengine.Store {
		t.Helper()
		s := boardengine.NewStore("")
		for _, f := range []map[string]any{
			{"slug": "a", "kind": "note"},
			{"slug": "c", "kind": "task"},
		} {
			if _, err := s.UpsertTask(f); err != nil {
				t.Fatal(err)
			}
		}
		return s
	}

	t.Run("a note becomes a task", func(t *testing.T) {
		s := seed(t)
		got, changed, err := s.Promote("a")
		if err != nil || !changed || got.Kind != boardengine.KindTask {
			t.Fatalf("got kind %q changed %v, err %v", got.Kind, changed, err)
		}
		stored, _ := s.GetTask("a")
		if stored.Kind != boardengine.KindTask {
			t.Errorf("store not updated: kind %q", stored.Kind)
		}
	})

	t.Run("an id selects the entry", func(t *testing.T) {
		s := seed(t)
		note, _ := s.GetTask("a")
		got, changed, err := s.Promote(note.ID)
		if err != nil || !changed || got.Slug != "a" {
			t.Fatalf("got %+v changed %v, err %v", got, changed, err)
		}
	})

	t.Run("a task is returned unchanged", func(t *testing.T) {
		s := seed(t)
		got, changed, err := s.Promote("c")
		if err != nil || changed || got.Slug != "c" || got.Kind != boardengine.KindTask {
			t.Fatalf("got %+v changed %v, err %v; want c unchanged", got, changed, err)
		}
	})

	t.Run("missing entry refused", func(t *testing.T) {
		s := seed(t)
		if _, _, err := s.Promote("nope"); err == nil || !stringContains(err.Error(), "not found") {
			t.Errorf("expected not found, got %v", err)
		}
	})
}

func TestPrune(t *testing.T) {
	s := boardengine.NewStore("")
	if _, err := s.UpsertTask(map[string]any{"slug": "d1", "kind": "task"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpsertTask(map[string]any{"slug": "live", "kind": "task", "depends_on": []string{"d1"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpsertTask(map[string]any{"slug": "d2"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpsertTask(map[string]any{"slug": "ab"}); err != nil {
		t.Fatal(err)
	}
	done, abandoned := "done", "abandoned"
	if err := s.SetStatus("d1", &done); err != nil {
		t.Fatal(err)
	}
	if err := s.SetStatus("d2", &done); err != nil {
		t.Fatal(err)
	}
	if err := s.SetStatus("ab", &abandoned); err != nil {
		t.Fatal(err)
	}

	removed := s.Prune()
	if !sliceEqualStrings(removed, []string{"d1", "d2"}) {
		t.Errorf("removed = %v", removed)
	}
	if _, ok := s.GetTask("d1"); ok {
		t.Errorf("d1 should be gone")
	}
	if _, ok := s.GetTask("ab"); !ok {
		t.Errorf("abandoned entry should survive")
	}
	live, _ := s.GetTask("live")
	if len(live.DependsOn) != 0 {
		t.Errorf("depends_on not stripped: %v", live.DependsOn)
	}
	if again := s.Prune(); len(again) != 0 {
		t.Errorf("second prune removed %v", again)
	}
}

func TestFind(t *testing.T) {
	s := boardengine.NewStore("")
	for _, f := range []map[string]any{
		{"slug": "alpha-slug", "title": "One"},
		{"slug": "b", "title": "Needle Title"},
		{"slug": "c", "title": "Three", "brief": "has a NEEDLE here"},
		{"slug": "d", "title": "Four", "body": "deep needle body"},
		{"slug": "e", "title": "Five"},
	} {
		if _, err := s.UpsertTask(f); err != nil {
			t.Fatal(err)
		}
	}
	done := "done"
	if err := s.SetStatus("d", &done); err != nil {
		t.Fatal(err)
	}

	slugs := func(bs []boardengine.BriefTask) []string {
		out := []string{}
		for _, b := range bs {
			out = append(out, b.Slug)
		}
		return out
	}

	if got := slugs(s.Find("NEEDLE", nil)); !sliceEqualStrings(got, []string{"b", "c", "d"}) {
		t.Errorf("title/brief/body match (done included) = %v", got)
	}
	if got := slugs(s.Find("ALPHA-SLUG", nil)); !sliceEqualStrings(got, []string{"alpha-slug"}) {
		t.Errorf("slug match = %v", got)
	}
	got := s.Find("zzz", nil)
	if got == nil || len(got) != 0 {
		t.Errorf("no match should be an empty slice, got %v", got)
	}
}

// TestUpsertIssues verifies issues defaults to empty, is replaced as a whole and refuses duplicates and non-positive numbers.
func TestUpsertIssues(t *testing.T) {
	s := boardengine.NewStore("")
	task, err := s.UpsertTask(map[string]any{"slug": "a"})
	if err != nil {
		t.Fatalf("UpsertTask: %v", err)
	}
	if task.Issues == nil || len(task.Issues) != 0 {
		t.Errorf("Issues = %#v, want an empty list", task.Issues)
	}

	if task, err = s.UpsertTask(map[string]any{"slug": "a", "issues": []int{4, 5}}); err != nil || len(task.Issues) != 2 {
		t.Fatalf("set issues: %v, %v", task.Issues, err)
	}
	if task, err = s.UpsertTask(map[string]any{"slug": "a", "issues": []int{6}}); err != nil || len(task.Issues) != 1 || task.Issues[0] != 6 {
		t.Errorf("issues must be replaced as a whole: %v, %v", task.Issues, err)
	}

	for name, issues := range map[string][]int{"duplicate": {2, 2}, "zero": {0}, "negative": {-3}} {
		if _, err := s.UpsertTask(map[string]any{"slug": "a", "issues": issues}); err == nil {
			t.Errorf("%s: want a refusal", name)
		}
	}
	if got := s.Tasks()[0].Issues; len(got) != 1 || got[0] != 6 {
		t.Errorf("a refused upsert changed issues to %v", got)
	}
}

// TestMergeTasksIssueCarry verifies that a merge records the removed entries' issues on the upserted entry.
func TestMergeTasksIssueCarry(t *testing.T) {
	seed := func(t *testing.T) *boardengine.Store {
		t.Helper()
		s := boardengine.NewStore("")
		for _, fields := range []map[string]any{
			{"slug": "a", "title": "A", "issues": []int{3}},
			{"slug": "b", "title": "B", "issues": []int{3, 5}},
		} {
			if _, err := s.UpsertTask(fields); err != nil {
				t.Fatalf("seed: %v", err)
			}
		}
		return s
	}

	t.Run("carries removed issues after the existing ones", func(t *testing.T) {
		s := seed(t)
		if _, err := s.UpsertTask(map[string]any{"slug": "c", "title": "C", "issues": []int{1}}); err != nil {
			t.Fatalf("seed: %v", err)
		}
		// The payload does not name issues,
		// so the existing entry's own list leads.
		got, err := s.MergeTasks([]string{"a", "b"}, map[string]any{"slug": "c"}, nil)
		if err != nil {
			t.Fatalf("merge: %v", err)
		}
		if want := []int{1, 3, 5}; !slices.Equal(got.Issues, want) {
			t.Errorf("returned issues = %v; want %v", got.Issues, want)
		}
		saved, _ := s.GetTask("c")
		if want := []int{1, 3, 5}; !slices.Equal(saved.Issues, want) {
			t.Errorf("saved issues = %v; want %v", saved.Issues, want)
		}
	})

	t.Run("a payload that names issues keeps its own numbers first", func(t *testing.T) {
		s := seed(t)
		got, err := s.MergeTasks([]string{"a", "b"}, map[string]any{"slug": "c", "title": "C", "issues": []int{9, 3}}, nil)
		if err != nil {
			t.Fatalf("merge: %v", err)
		}
		if want := []int{9, 3, 5}; !slices.Equal(got.Issues, want) {
			t.Errorf("issues = %v; want %v", got.Issues, want)
		}
	})

	t.Run("a remove slug that matches nothing adds nothing", func(t *testing.T) {
		s := seed(t)
		got, err := s.MergeTasks([]string{"ghost"}, map[string]any{"slug": "c", "title": "C", "issues": []int{1}}, nil)
		if err != nil {
			t.Fatalf("merge: %v", err)
		}
		if want := []int{1}; !slices.Equal(got.Issues, want) {
			t.Errorf("issues = %v; want %v", got.Issues, want)
		}
	})

	t.Run("a set-status failure leaves the store unchanged", func(t *testing.T) {
		dir := t.TempDir()
		s := boardengine.NewStore(dir)
		if err := s.Load(); err != nil {
			t.Fatalf("load: %v", err)
		}
		if _, err := s.UpsertTask(map[string]any{"slug": "b", "title": "B", "issues": []int{3, 5}}); err != nil {
			t.Fatalf("seed: %v", err)
		}
		if err := s.Save(); err != nil {
			t.Fatalf("save: %v", err)
		}

		s2 := boardengine.NewStore(dir)
		if err := s2.Load(); err != nil {
			t.Fatalf("load s2: %v", err)
		}
		status := "active"
		_, err := s2.MergeTasks(
			[]string{"b"},
			map[string]any{"slug": "c", "title": "C"},
			&boardengine.MergeStatusUpdate{Selector: "ghost", Status: &status},
		)
		if err == nil {
			t.Fatalf("expected error when set_status targets a missing slug")
		}

		s3 := boardengine.NewStore(dir)
		if err := s3.Load(); err != nil {
			t.Fatalf("load s3: %v", err)
		}
		b, found := s3.GetTask("b")
		if !found || !slices.Equal(b.Issues, []int{3, 5}) {
			t.Errorf("on-disk b = %v (found %v); want issues [3 5]", b.Issues, found)
		}
		if _, found := s3.GetTask("c"); found {
			t.Errorf("c was saved despite the failed merge")
		}
	})
}
