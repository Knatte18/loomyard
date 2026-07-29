// task_test.go — unit tests for Task construction (task.go).
//
// NewTask defaults and type validation; ApplyPatch field overlay.

package boardengine_test

import (
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/boardengine"
)

func TestNewTask(t *testing.T) {
	t.Run("creates task with correct defaults when only slug provided", func(t *testing.T) {
		fields := map[string]any{
			"slug": "my-task",
		}
		task, err := boardengine.NewTask(fields, 1)
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
		task, err := boardengine.NewTask(fields, 42)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if task.ID != 42 {
			t.Errorf("expected ID=42, got %d", task.ID)
		}
	})

	t.Run("missing slug returns error", func(t *testing.T) {
		fields := map[string]any{}
		_, err := boardengine.NewTask(fields, 1)
		if err == nil {
			t.Fatalf("expected error for missing slug, got nil")
		}
	})

	t.Run("explicit DependsOn provided in fields is stored correctly", func(t *testing.T) {
		fields := map[string]any{
			"slug":       "test-task",
			"depends_on": []string{"task-a", "task-b"},
		}
		task, err := boardengine.NewTask(fields, 1)
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

	t.Run("slug exceeding maxSlugLength is rejected", func(t *testing.T) {
		fields := map[string]any{
			"slug": strings.Repeat("a", 33),
		}
		_, err := boardengine.NewTask(fields, 1)
		if err == nil {
			t.Fatalf("expected error for 33-character slug, got nil")
		}
		if !strings.Contains(err.Error(), "exceeds max length") {
			t.Errorf("expected error containing 'exceeds max length', got: %v", err)
		}
	})

	t.Run("slug at maxSlugLength is accepted", func(t *testing.T) {
		fields := map[string]any{
			"slug": strings.Repeat("a", 32),
		}
		task, err := boardengine.NewTask(fields, 1)
		if err != nil {
			t.Fatalf("unexpected error for 32-character slug: %v", err)
		}
		if task.Slug != strings.Repeat("a", 32) {
			t.Errorf("expected slug to round-trip, got %q", task.Slug)
		}
	})

	t.Run("short_name field round-trips onto Task.ShortName", func(t *testing.T) {
		fields := map[string]any{
			"slug":       "my-task",
			"short_name": "mt",
		}
		task, err := boardengine.NewTask(fields, 1)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if task.ShortName != "mt" {
			t.Errorf("expected ShortName='mt', got %q", task.ShortName)
		}
	})
}

func TestShortNameOrSlug(t *testing.T) {
	t.Run("falls back to Slug when ShortName is empty", func(t *testing.T) {
		task := boardengine.Task{Slug: "x"}
		if got := task.ShortNameOrSlug(); got != "x" {
			t.Errorf("expected 'x', got %q", got)
		}
	})

	t.Run("prefers ShortName when set", func(t *testing.T) {
		task := boardengine.Task{Slug: "x", ShortName: "y"}
		if got := task.ShortNameOrSlug(); got != "y" {
			t.Errorf("expected 'y', got %q", got)
		}
	})
}

func TestApplyPatch(t *testing.T) {
	t.Run("overlays title onto existing task, other fields unchanged", func(t *testing.T) {
		existing := boardengine.Task{
			ID:        1,
			Slug:      "test",
			Title:     "Old Title",
			Brief:     "Original brief",
			DependsOn: []string{"a"},
		}
		patch := map[string]any{
			"title": "New Title",
		}
		result, err := boardengine.ApplyPatch(existing, patch)
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
		existing := boardengine.Task{
			ID:        1,
			Slug:      "test",
			DependsOn: []string{"a"},
		}
		patch := map[string]any{
			"depends_on": []string{"x", "y", "z"},
		}
		result, err := boardengine.ApplyPatch(existing, patch)
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

	t.Run("existing Status is preserved when not in patch", func(t *testing.T) {
		statusVal := "in-progress"
		existing := boardengine.Task{
			ID:     1,
			Slug:   "test",
			Status: &statusVal,
		}
		patch := map[string]any{
			"title": "Updated",
		}
		result, err := boardengine.ApplyPatch(existing, patch)
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
		existing := boardengine.Task{
			ID:     1,
			Slug:   "test",
			Status: &statusVal,
		}
		patch := map[string]any{
			"status": nil,
		}
		result, err := boardengine.ApplyPatch(existing, patch)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Status != nil {
			t.Errorf("expected Status=nil, got %v", result.Status)
		}
	})

	t.Run("patching slug beyond maxSlugLength is rejected", func(t *testing.T) {
		existing := boardengine.Task{ID: 1, Slug: "test"}
		patch := map[string]any{
			"slug": strings.Repeat("a", 33),
		}
		_, err := boardengine.ApplyPatch(existing, patch)
		if err == nil {
			t.Fatalf("expected error for 33-character slug patch, got nil")
		}
		if !strings.Contains(err.Error(), "exceeds max length") {
			t.Errorf("expected error containing 'exceeds max length', got: %v", err)
		}
	})

	t.Run("patching slug at maxSlugLength is accepted", func(t *testing.T) {
		existing := boardengine.Task{ID: 1, Slug: "test"}
		patch := map[string]any{
			"slug": strings.Repeat("a", 32),
		}
		result, err := boardengine.ApplyPatch(existing, patch)
		if err != nil {
			t.Fatalf("unexpected error for 32-character slug patch: %v", err)
		}
		if result.Slug != strings.Repeat("a", 32) {
			t.Errorf("expected slug to round-trip, got %q", result.Slug)
		}
	})
}
