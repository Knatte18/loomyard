package wiki_test

import (
	"testing"

	"github.com/Knatte18/mhgo/internal/wiki"
)

func TestNewTask(t *testing.T) {
	t.Run("creates task with correct defaults when only slug provided", func(t *testing.T) {
		fields := map[string]any{
			"slug": "my-task",
		}
		task, err := wiki.NewTask(fields, 1)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if task.Slug != "my-task" {
			t.Errorf("expected slug 'my-task', got %q", task.Slug)
		}
		if len(task.DependsOn) != 0 {
			t.Errorf("expected empty DependsOn, got %v", task.DependsOn)
		}
		if task.Isolated != false {
			t.Errorf("expected Isolated=false, got %v", task.Isolated)
		}
		if task.Deferred != false {
			t.Errorf("expected Deferred=false, got %v", task.Deferred)
		}
		if task.Status != nil {
			t.Errorf("expected Status=nil, got %v", task.Status)
		}
	})

	t.Run("ID is set to the provided nextID", func(t *testing.T) {
		fields := map[string]any{
			"slug": "test-task",
		}
		task, err := wiki.NewTask(fields, 42)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if task.ID != 42 {
			t.Errorf("expected ID=42, got %d", task.ID)
		}
	})

	t.Run("missing slug returns error", func(t *testing.T) {
		fields := map[string]any{}
		_, err := wiki.NewTask(fields, 1)
		if err == nil {
			t.Fatalf("expected error for missing slug, got nil")
		}
	})

	t.Run("group key present returns error", func(t *testing.T) {
		fields := map[string]any{
			"slug":  "test-task",
			"group": "some-group",
		}
		_, err := wiki.NewTask(fields, 1)
		if err == nil {
			t.Fatalf("expected error for group key, got nil")
		}
		expectedMsg := "group key is not allowed"
		if errStr := err.Error(); len(errStr) < len(expectedMsg) || errStr[:len(expectedMsg)] != expectedMsg {
			t.Errorf("expected error containing %q, got %q", expectedMsg, errStr)
		}
	})

	t.Run("explicit DependsOn provided in fields is stored correctly", func(t *testing.T) {
		fields := map[string]any{
			"slug":       "test-task",
			"depends_on": []string{"task-a", "task-b"},
		}
		task, err := wiki.NewTask(fields, 1)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(task.DependsOn) != 2 {
			t.Errorf("expected 2 dependencies, got %d", len(task.DependsOn))
		}
		if task.DependsOn[0] != "task-a" || task.DependsOn[1] != "task-b" {
			t.Errorf("expected [task-a task-b], got %v", task.DependsOn)
		}
	})
}

func TestApplyPatch(t *testing.T) {
	t.Run("overlays title onto existing task, other fields unchanged", func(t *testing.T) {
		existing := wiki.Task{
			ID:       1,
			Slug:     "test",
			Title:    "Old Title",
			Brief:    "Original brief",
			DependsOn: []string{"a"},
		}
		patch := map[string]any{
			"title": "New Title",
		}
		result, err := wiki.ApplyPatch(existing, patch)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Title != "New Title" {
			t.Errorf("expected Title='New Title', got %q", result.Title)
		}
		if result.Brief != "Original brief" {
			t.Errorf("expected Brief='Original brief', got %q", result.Brief)
		}
		if len(result.DependsOn) != 1 || result.DependsOn[0] != "a" {
			t.Errorf("expected DependsOn=[a], got %v", result.DependsOn)
		}
	})

	t.Run("DependsOn is updated when provided", func(t *testing.T) {
		existing := wiki.Task{
			ID:        1,
			Slug:      "test",
			DependsOn: []string{"a"},
		}
		patch := map[string]any{
			"depends_on": []string{"x", "y", "z"},
		}
		result, err := wiki.ApplyPatch(existing, patch)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result.DependsOn) != 3 {
			t.Errorf("expected 3 dependencies, got %d", len(result.DependsOn))
		}
		if result.DependsOn[0] != "x" || result.DependsOn[1] != "y" || result.DependsOn[2] != "z" {
			t.Errorf("expected [x y z], got %v", result.DependsOn)
		}
	})

	t.Run("group key returns error", func(t *testing.T) {
		existing := wiki.Task{
			ID:   1,
			Slug: "test",
		}
		patch := map[string]any{
			"group": "some-group",
		}
		_, err := wiki.ApplyPatch(existing, patch)
		if err == nil {
			t.Fatalf("expected error for group key, got nil")
		}
		expectedMsg := "group key is not allowed"
		if errStr := err.Error(); len(errStr) < len(expectedMsg) || errStr[:len(expectedMsg)] != expectedMsg {
			t.Errorf("expected error containing %q, got %q", expectedMsg, errStr)
		}
	})

	t.Run("existing Status is preserved when not in patch", func(t *testing.T) {
		statusVal := "in-progress"
		existing := wiki.Task{
			ID:     1,
			Slug:   "test",
			Status: &statusVal,
		}
		patch := map[string]any{
			"title": "Updated",
		}
		result, err := wiki.ApplyPatch(existing, patch)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Status == nil {
			t.Errorf("expected Status to be preserved, got nil")
		}
		if result.Status != nil && *result.Status != "in-progress" {
			t.Errorf("expected Status='in-progress', got %q", *result.Status)
		}
	})

	t.Run("Status can be cleared by patching with status: nil", func(t *testing.T) {
		statusVal := "in-progress"
		existing := wiki.Task{
			ID:     1,
			Slug:   "test",
			Status: &statusVal,
		}
		patch := map[string]any{
			"status": nil,
		}
		result, err := wiki.ApplyPatch(existing, patch)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Status != nil {
			t.Errorf("expected Status=nil, got %v", result.Status)
		}
	})
}
