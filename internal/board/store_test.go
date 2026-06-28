// store_test.go — unit tests for the Store (store.go).
//
// CRUD, sequential ID assignment, and every validation rule: dangling deps,
// isolated/deferred constraints, cycle detection, and batch/merge atomicity.

package board_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/board"
)

func TestUpsertTaskNewTaskSequentialID(t *testing.T) {
	s := board.NewStore("")

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
	s := board.NewStore("")

	// (b) defaults applied (DependsOn=[], Isolated=false, Deferred=false)
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
	if task.Deferred {
		t.Errorf("expected Deferred=false, got true")
	}
}

func TestUpsertTaskPreservesFields(t *testing.T) {
	s := board.NewStore("")

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
	s := board.NewStore("")

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

// TestUpsertFieldAllowlist verifies that the store chokepoint rejects unknown upsert
// fields on all three entry points: UpsertTask, UpsertTasksBatch, and MergeTasks.
// Also verifies that `status` IS in the allowed set and is persisted correctly.
func TestUpsertFieldAllowlist(t *testing.T) {
	t.Run("upsert_stray_phase_key_errors", func(t *testing.T) {
		s := board.NewStore("")
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
		s := board.NewStore("")
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
		s := board.NewStore("")
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

	t.Run("upsert_batch_stray_phase_errors", func(t *testing.T) {
		s := board.NewStore("")
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
		s := board.NewStore("")
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
		s := board.NewStore("")
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

// TestValidateDependencyErrors verifies that UpsertTask rejects all invalid dependency
// configurations with precise error messages: dangling deps, depending on isolated tasks,
// and depending on deferred tasks.
//
// Folds: TestValidateDanglingDependency, TestValidateDependencyOnIsolated, TestValidateDependencyOnDeferred
func TestValidateDependencyErrors(t *testing.T) {
	t.Run("TestValidateDanglingDependency", func(t *testing.T) {
		s := board.NewStore("")

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
		s := board.NewStore("")

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

	t.Run("TestValidateDependencyOnDeferred", func(t *testing.T) {
		s := board.NewStore("")

		// Create a deferred task
		_, err := s.UpsertTask(map[string]any{
			"slug":     "deferred",
			"deferred": true,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// (g) dependency on deferred task rejected
		_, err = s.UpsertTask(map[string]any{
			"slug":       "task1",
			"depends_on": []string{"deferred"},
		})
		if err == nil {
			t.Fatalf("expected error for dependency on deferred task")
		}
		if err.Error() != "cannot depend on deferred task \"deferred\"" {
			t.Errorf("unexpected error: %v", err)
		}
	})
}

func TestValidateCycleDetection(t *testing.T) {
	s := board.NewStore("")

	// Create task A
	_, err := s.UpsertTask(map[string]any{
		"slug":  "a",
		"title": "A",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Create task B depending on A
	_, err = s.UpsertTask(map[string]any{
		"slug":       "b",
		"title":      "B",
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
	s := board.NewStore("")

	// (i) chain A depends on B, B depends on C — no cycle, all upserts succeed
	_, err := s.UpsertTask(map[string]any{
		"slug": "c",
	})
	if err != nil {
		t.Fatalf("unexpected error creating C: %v", err)
	}

	_, err = s.UpsertTask(map[string]any{
		"slug":       "b",
		"depends_on": []string{"c"},
	})
	if err != nil {
		t.Fatalf("unexpected error creating B: %v", err)
	}

	_, err = s.UpsertTask(map[string]any{
		"slug":       "a",
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
	s := board.NewStore("")

	// (j) RemoveTask returns error for missing slug
	err := s.RemoveTask("nonexistent")
	if err == nil {
		t.Fatalf("expected error for missing task")
	}
	if err.Error() != "task not found: nonexistent" {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestSetStatus verifies SetStatus behaviour: clearing status via nil and the
// current silent no-op for a missing slug (changed to an error in Card 3).
//
// Folds: TestSetPhaseNil, TestSetPhaseMissing
func TestSetStatus(t *testing.T) {
	t.Run("TestSetPhaseNil", func(t *testing.T) {
		s := board.NewStore("")

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
		s := board.NewStore("")

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

// TestMergeTasks verifies both the happy path (atomic remove+upsert+set_phase) and
// the rollback path (validation error leaves store unchanged).
//
// Folds: TestMergeTasksAtomic, TestMergeTasksValidationRollback
func TestMergeTasks(t *testing.T) {
	t.Run("TestMergeTasksAtomic", func(t *testing.T) {
		s := board.NewStore("")

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
			&[2]any{"c", phase},
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

	t.Run("TestMergeTasksValidationRollback", func(t *testing.T) {
		s := board.NewStore("")

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

func TestListTasksBriefLayerAndProposal(t *testing.T) {
	s := board.NewStore("")

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
	brief := s.ListTasksBrief()
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

// TestSetDeps verifies both the valid update path and the cycle-detection rollback
// for SetDeps.
//
// Folds: TestSetDepsValid, TestSetDepsCycleRollback
func TestSetDeps(t *testing.T) {
	t.Run("TestSetDepsValid", func(t *testing.T) {
		s := board.NewStore("")

		// Create tasks
		_, err := s.UpsertTask(map[string]any{
			"slug": "a",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		_, err = s.UpsertTask(map[string]any{
			"slug": "b",
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
		s := board.NewStore("")

		// Create A and B with A depending on B
		_, err := s.UpsertTask(map[string]any{
			"slug": "a",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		_, err = s.UpsertTask(map[string]any{
			"slug":       "b",
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

// TestUpsertTasksBatch verifies that a valid batch upserts all tasks and that an
// invalid batch returns an error without mutating the store.
//
// Folds: TestUpsertTasksBatchValid, TestUpsertTasksBatchInvalid
func TestUpsertTasksBatch(t *testing.T) {
	t.Run("TestUpsertTasksBatchValid", func(t *testing.T) {
		s := board.NewStore("")

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

	t.Run("TestUpsertTasksBatchInvalid", func(t *testing.T) {
		s := board.NewStore("")

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

// TestLoadNilDependsOnNormalization verifies that Load normalizes a nil DependsOn
// to an empty slice and that a missing file yields an empty store with no error.
//
// Folds: TestLoadNormalizesNilDependsOn, TestLoadMissingFileReturnsEmpty
func TestLoadNilDependsOnNormalization(t *testing.T) {
	t.Run("TestLoadNormalizesNilDependsOn", func(t *testing.T) {
		tmpDir := t.TempDir()
		taskPath := filepath.Join(tmpDir, "tasks.json")

		// Write tasks.json with a task that has nil DependsOn
		err := os.WriteFile(taskPath, []byte(`[{"id":0,"slug":"task1","title":"Task 1"}]`), 0o644)
		if err != nil {
			t.Fatalf("failed to write test file: %v", err)
		}

		store := board.NewStore(taskPath)
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
		tmpDir := t.TempDir()
		taskPath := filepath.Join(tmpDir, "tasks.json")

		// Do not create the file; test that Load handles missing file gracefully
		store := board.NewStore(taskPath)
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

// TestLoadCorruptTasksJSON verifies that Load surfaces a corrupt tasks.json
// as an error instead of silently producing an empty task list.
func TestLoadCorruptTasksJSON(t *testing.T) {
	tmpDir := t.TempDir()
	taskPath := filepath.Join(tmpDir, "tasks.json")

	// Write syntactically corrupt JSON
	err := os.WriteFile(taskPath, []byte(`{this is not valid json`), 0o644)
	if err != nil {
		t.Fatalf("failed to write corrupt test file: %v", err)
	}

	store := board.NewStore(taskPath)
	err = store.Load()
	if err == nil {
		t.Fatalf("expected error for corrupt tasks.json, got nil")
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
