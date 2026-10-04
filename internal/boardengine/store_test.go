// store_test.go — unit tests for the Store (store.go).
//
// CRUD, sequential ID assignment, and every validation rule: dangling deps, isolated/kind/label constraints, cycle detection, and batch/merge atomicity.

package boardengine_test

import (
	"os"
	"path/filepath"
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

// TestValidateDependencyErrors verifies that UpsertTask rejects all invalid dependency configurations with precise error messages: dangling deps and depending on isolated tasks (the kind rules have their own test).
//
// Folds: TestValidateDanglingDependency, TestValidateDependencyOnIsolated
func TestValidateDependencyErrors(t *testing.T) {
	t.Run("TestValidateDanglingDependency", func(t *testing.T) {
		s := boardengine.NewStore("")

		// (e) dangling dependency rejected
		_, err := s.UpsertTask(map[string]any{
			"slug":       "task1",
			"depends_on": []string{"nonexistent"},
		})
		if err == nil {
			t.Fatalf("expected error for dangling dependency")
		}
		if err.Error() != "dangling dependency: \"nonexistent\" does not exist" {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("TestValidateDependencyOnIsolated", func(t *testing.T) {
		s := boardengine.NewStore("")

		// Create an isolated task
		_, err := s.UpsertTask(map[string]any{
			"slug":     "isolated",
			"isolated": true,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// (f) dependency on isolated task rejected
		_, err = s.UpsertTask(map[string]any{
			"slug":       "task1",
			"depends_on": []string{"isolated"},
		})
		if err == nil {
			t.Fatalf("expected error for dependency on isolated task")
		}
		if err.Error() != "cannot depend on isolated task \"isolated\"" {
			t.Errorf("unexpected error: %v", err)
		}
	})
}

func TestValidateCycleDetection(t *testing.T) {
	s := boardengine.NewStore("")

	// Create task A
	_, err := s.UpsertTask(map[string]any{
		"slug":  "a",
		"title": "A",
		"kind":  "task",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Create task B depending on A
	_, err = s.UpsertTask(map[string]any{
		"slug":       "b",
		"title":      "B",
		"kind":       "task",
		"depends_on": []string{"a"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// (h) cycle A→B, B→A detected and rejected with "cycle detected" in error message
	_, err = s.UpsertTask(map[string]any{
		"slug":       "a",
		"depends_on": []string{"b"},
	})
	if err == nil {
		t.Fatalf("expected error for cycle detection")
	}
	errMsg := err.Error()
	if !stringContains(errMsg, "cycle detected") {
		t.Errorf("expected 'cycle detected' in error, got: %v", err)
	}
}

func TestValidateNoCycleLongChain(t *testing.T) {
	s := boardengine.NewStore("")

	// (i) chain A depends on B, B depends on C — no cycle, all upserts succeed
	_, err := s.UpsertTask(map[string]any{
		"slug": "c",
		"kind": "task",
	})
	if err != nil {
		t.Fatalf("unexpected error creating C: %v", err)
	}

	_, err = s.UpsertTask(map[string]any{
		"slug":       "b",
		"kind":       "task",
		"depends_on": []string{"c"},
	})
	if err != nil {
		t.Fatalf("unexpected error creating B: %v", err)
	}

	_, err = s.UpsertTask(map[string]any{
		"slug":       "a",
		"kind":       "task",
		"depends_on": []string{"b"},
	})
	if err != nil {
		t.Fatalf("unexpected error creating A: %v", err)
	}

	tasks := s.Tasks()
	if len(tasks) != 3 {
		t.Errorf("expected 3 tasks, got %d", len(tasks))
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
// open tasks first, then open notes, done last.
func TestListAndFindReadmeOrder(t *testing.T) {
	s := boardengine.NewStore("")
	for _, f := range []map[string]any{
		{"slug": "x-note", "kind": "note"},
		{"slug": "x-task", "kind": "task"},
		{"slug": "x-note2", "kind": "note"},
		{"slug": "x-done", "kind": "task"},
	} {
		if _, err := s.UpsertTask(f); err != nil {
			t.Fatalf("UpsertTask %v: %v", f, err)
		}
	}
	if err := s.SetStatus("x-done", stringPtr("done")); err != nil {
		t.Fatalf("SetStatus: %v", err)
	}

	want := "x-task,x-note,x-note2,x-done"
	slugs := func(bs []boardengine.BriefTask) string {
		var out []string
		for _, b := range bs {
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

// TestLoadNilDependsOnNormalization verifies that Load normalizes a nil DependsOn to an empty slice
// and that a missing file yields an empty store with no error.
//
// Folds: TestLoadNormalizesNilDependsOn, TestLoadMissingFileReturnsEmpty
func TestLoadNilDependsOnNormalization(t *testing.T) {
	t.Run("TestLoadNormalizesNilDependsOn", func(t *testing.T) {
		boardDir := t.TempDir()

		// Write board.json with an entry that has nil DependsOn
		err := os.WriteFile(filepath.Join(boardDir, "board.json"), []byte(`{"version":1,"entries":[{"id":0,"slug":"task1","title":"Task 1","tier":3,"type":"feature"}]}`), 0o644)
		if err != nil {
			t.Fatalf("failed to write test file: %v", err)
		}

		store := boardengine.NewStore(boardDir)
		err = store.Load()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		tasks := store.Tasks()
		if len(tasks) != 1 {
			t.Fatalf("expected 1 task, got %d", len(tasks))
		}

		// Verify DependsOn is normalized to an empty slice, not nil
		if tasks[0].DependsOn == nil {
			t.Errorf("expected empty slice for DependsOn, got nil")
		}
		if len(tasks[0].DependsOn) != 0 {
			t.Errorf("expected empty DependsOn, got %v", tasks[0].DependsOn)
		}
	})

	t.Run("TestLoadMissingFileReturnsEmpty", func(t *testing.T) {
		// Do not create the file; test that Load handles missing file gracefully
		store := boardengine.NewStore(t.TempDir())
		err := store.Load()
		if err != nil {
			t.Fatalf("expected no error for missing file, got %v", err)
		}

		tasks := store.Tasks()
		if len(tasks) != 0 {
			t.Errorf("expected empty task list for missing file, got %d tasks", len(tasks))
		}
	})
}

// TestLoadFromBoardJSON verifies that Load reads the version-1 shape and a Save round-trips it.
func TestLoadFromBoardJSON(t *testing.T) {
	boardDir := t.TempDir()
	body := `{"version":1,"entries":[{"id":4,"slug":"a","title":"A","kind":"task","labels":["bug"],"depends_on":[]}],"legacy_done":["old"]}`
	if err := os.WriteFile(filepath.Join(boardDir, "board.json"), []byte(body), 0o644); err != nil {
		t.Fatalf("write board.json: %v", err)
	}

	store := boardengine.NewStore(boardDir)
	if err := store.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	tasks := store.Tasks()
	if len(tasks) != 1 || tasks[0].Slug != "a" || tasks[0].Kind != boardengine.KindTask || len(tasks[0].Labels) != 1 || tasks[0].Labels[0] != "bug" {
		t.Fatalf("loaded %+v; want entry a as a task labelled bug", tasks)
	}

	if err := store.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(boardDir, "board.json"))
	if err != nil {
		t.Fatalf("read board.json: %v", err)
	}
	if !stringContains(string(raw), `"legacy_done"`) {
		t.Errorf("Save dropped legacy_done: %s", raw)
	}
}

// TestLoadOldShapeRoundTrip verifies an old-shape board.json converts on Load and persists with kind and labels and without tier or type after Save.
func TestLoadOldShapeRoundTrip(t *testing.T) {
	boardDir := t.TempDir()
	body := `{"version":1,"entries":[{"id":0,"slug":"a","title":"A","tier":1,"type":"bug","brief":"[infra] fix it","depends_on":[]}]}`
	if err := os.WriteFile(filepath.Join(boardDir, "board.json"), []byte(body), 0o644); err != nil {
		t.Fatalf("write board.json: %v", err)
	}

	store := boardengine.NewStore(boardDir)
	if err := store.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	tasks := store.Tasks()
	if len(tasks) != 1 || tasks[0].Kind != boardengine.KindTask || strings.Join(tasks[0].Labels, ",") != "bug,infra" || tasks[0].Brief != "fix it" {
		t.Fatalf("loaded %+v; want a task labelled bug, infra with the stripped brief", tasks)
	}

	if err := store.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(boardDir, "board.json"))
	if err != nil {
		t.Fatalf("read board.json: %v", err)
	}
	if !stringContains(string(raw), `"kind": "task"`) && !stringContains(string(raw), `"kind":"task"`) {
		t.Errorf("Save did not write kind: %s", raw)
	}
	if !stringContains(string(raw), `"labels"`) || stringContains(string(raw), `"tier"`) || stringContains(string(raw), `"type"`) {
		t.Errorf("Save must write labels and no tier or type: %s", raw)
	}
}

// TestLoadUnknownVersionRefused verifies a board.json version other than 1 refuses the load, naming the version.
func TestLoadUnknownVersionRefused(t *testing.T) {
	boardDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(boardDir, "board.json"), []byte(`{"version":2,"entries":[]}`), 0o644); err != nil {
		t.Fatalf("write board.json: %v", err)
	}

	err := boardengine.NewStore(boardDir).Load()
	if err == nil {
		t.Fatalf("expected an error for version 2")
	}
	if !stringContains(err.Error(), "version 2") {
		t.Errorf("error %q does not name version 2", err)
	}
}

// TestLoadCorruptBoardJSON verifies that Load surfaces a corrupt board.json as an error instead of
// silently producing an empty task list.
func TestLoadCorruptBoardJSON(t *testing.T) {
	boardDir := t.TempDir()

	// Write syntactically corrupt JSON
	err := os.WriteFile(filepath.Join(boardDir, "board.json"), []byte(`{this is not valid json`), 0o644)
	if err != nil {
		t.Fatalf("failed to write corrupt test file: %v", err)
	}

	store := boardengine.NewStore(boardDir)
	err = store.Load()
	if err == nil {
		t.Fatalf("expected error for corrupt board.json, got nil")
	}

	// Verify the error message indicates a load error
	errMsg := err.Error()
	if !stringContains(errMsg, "load store") {
		t.Errorf("expected 'load store' in error, got: %v", err)
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
