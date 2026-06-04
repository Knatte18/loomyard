package wiki_test

import (
	"testing"

	"github.com/Knatte18/mhgo/internal/wiki"
)

func TestComputeLayers(t *testing.T) {
	t.Run("single task with no dependencies is assigned layer A", func(t *testing.T) {
		tasks := []wiki.Task{
			{ID: 1, Slug: "task-a", Title: "Task A"},
		}
		layers, err := wiki.ComputeLayers(tasks)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if layers["task-a"] != "A" {
			t.Errorf("expected layer A, got %q", layers["task-a"])
		}
	})

	t.Run("A depends on B means B has lower layer than A", func(t *testing.T) {
		tasks := []wiki.Task{
			{ID: 1, Slug: "task-a", Title: "Task A", DependsOn: []string{"task-b"}},
			{ID: 2, Slug: "task-b", Title: "Task B"},
		}
		layers, err := wiki.ComputeLayers(tasks)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if layers["task-a"] != "B" {
			t.Errorf("expected layer B for task-a, got %q", layers["task-a"])
		}
		if layers["task-b"] != "A" {
			t.Errorf("expected layer A for task-b, got %q", layers["task-b"])
		}
	})

	t.Run("done task is excluded from depth calculation and marked __done__", func(t *testing.T) {
		done := "done"
		tasks := []wiki.Task{
			{ID: 1, Slug: "task-a", Title: "Task A", DependsOn: []string{"task-done"}},
			{ID: 2, Slug: "task-done", Title: "Task Done", Status: &done},
		}
		layers, err := wiki.ComputeLayers(tasks)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if layers["task-a"] != "A" {
			t.Errorf("expected layer A for task-a (done dep excluded), got %q", layers["task-a"])
		}
		if layers["task-done"] != "__done__" {
			t.Errorf("expected __done__ for task-done, got %q", layers["task-done"])
		}
	})

	t.Run("deferred task is marked __deferred__", func(t *testing.T) {
		tasks := []wiki.Task{
			{ID: 1, Slug: "task-deferred", Title: "Task Deferred", Deferred: true},
		}
		layers, err := wiki.ComputeLayers(tasks)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if layers["task-deferred"] != "__deferred__" {
			t.Errorf("expected __deferred__, got %q", layers["task-deferred"])
		}
	})

	t.Run("isolated task is marked Z", func(t *testing.T) {
		tasks := []wiki.Task{
			{ID: 1, Slug: "task-isolated", Title: "Task Isolated", Isolated: true},
		}
		layers, err := wiki.ComputeLayers(tasks)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if layers["task-isolated"] != "Z" {
			t.Errorf("expected Z, got %q", layers["task-isolated"])
		}
	})

	t.Run("chain of 3 tasks assigns correct depths", func(t *testing.T) {
		// C (root, no deps) -> depth 0 (A)
		// B (depends on C) -> depth 1 (B)
		// A (depends on B) -> depth 2 (C)
		tasks := []wiki.Task{
			{ID: 1, Slug: "task-a", Title: "Task A", DependsOn: []string{"task-b"}},
			{ID: 2, Slug: "task-b", Title: "Task B", DependsOn: []string{"task-c"}},
			{ID: 3, Slug: "task-c", Title: "Task C"},
		}
		layers, err := wiki.ComputeLayers(tasks)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if layers["task-c"] != "A" {
			t.Errorf("expected layer A for task-c, got %q", layers["task-c"])
		}
		if layers["task-b"] != "B" {
			t.Errorf("expected layer B for task-b, got %q", layers["task-b"])
		}
		if layers["task-a"] != "C" {
			t.Errorf("expected layer C for task-a, got %q", layers["task-a"])
		}
	})

	t.Run("depth >= 25 returns error", func(t *testing.T) {
		// Create a chain of 26 tasks to exceed depth 24
		tasks := make([]wiki.Task, 26)
		for i := 0; i < 26; i++ {
			slug := "task-" + string(rune('a'+i))
			tasks[i] = wiki.Task{
				ID:    i + 1,
				Slug:  slug,
				Title: "Task " + string(rune('A'+i)),
			}
			if i > 0 {
				tasks[i].DependsOn = []string{"task-" + string(rune('a' + i - 1))}
			}
		}
		_, err := wiki.ComputeLayers(tasks)
		if err == nil {
			t.Fatalf("expected error for depth >= 25, got nil")
		}
		if err.Error() != "layer depth exceeds A..Y cap" {
			t.Errorf("expected 'layer depth exceeds A..Y cap', got %q", err.Error())
		}
	})
}

func TestRenderOrder(t *testing.T) {
	t.Run("buckets appear in correct order: letter, Z, deferred, done", func(t *testing.T) {
		done := "done"
		tasks := []wiki.Task{
			{ID: 1, Slug: "task-a", Title: "Task A"},
			{ID: 2, Slug: "task-b", Title: "Task B", Deferred: true},
			{ID: 3, Slug: "task-c", Title: "Task C", Isolated: true},
			{ID: 4, Slug: "task-d", Title: "Task D", Status: &done},
		}
		ordered, err := wiki.RenderOrder(tasks)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Expected order: A (letter), C (Z/isolated), B (deferred), D (done)
		expectedSlugs := []string{"task-a", "task-c", "task-b", "task-d"}
		expectedLayers := []string{"A", "Z", "__deferred__", "__done__"}

		if len(ordered) != len(expectedSlugs) {
			t.Fatalf("expected %d tasks, got %d", len(expectedSlugs), len(ordered))
		}

		for i, expected := range expectedSlugs {
			if ordered[i].Slug != expected {
				t.Errorf("at position %d, expected slug %q, got %q", i, expected, ordered[i].Slug)
			}
			if ordered[i].Layer != expectedLayers[i] {
				t.Errorf("at position %d, expected layer %q, got %q", i, expectedLayers[i], ordered[i].Layer)
			}
		}
	})

	t.Run("tasks within same bucket are sorted by ID", func(t *testing.T) {
		tasks := []wiki.Task{
			{ID: 3, Slug: "task-c", Title: "Task C"},
			{ID: 1, Slug: "task-a", Title: "Task A"},
			{ID: 2, Slug: "task-b", Title: "Task B"},
		}
		ordered, err := wiki.RenderOrder(tasks)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// All in layer A, should be sorted by ID
		if ordered[0].ID != 1 || ordered[1].ID != 2 || ordered[2].ID != 3 {
			t.Errorf("expected IDs 1,2,3, got %d,%d,%d", ordered[0].ID, ordered[1].ID, ordered[2].ID)
		}
	})

	t.Run("multiple tasks in letter buckets are sorted alphabetically within layer", func(t *testing.T) {
		tasks := []wiki.Task{
			{ID: 2, Slug: "task-b", Title: "Task B", DependsOn: []string{"task-a"}},
			{ID: 1, Slug: "task-a", Title: "Task A"},
		}
		ordered, err := wiki.RenderOrder(tasks)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// task-a should be layer A (ID 1), task-b should be layer B (ID 2)
		// Both in letter buckets, so A comes before B
		if len(ordered) != 2 {
			t.Fatalf("expected 2 tasks, got %d", len(ordered))
		}
		if ordered[0].Layer != "A" {
			t.Errorf("expected first task in layer A, got %q", ordered[0].Layer)
		}
		if ordered[1].Layer != "B" {
			t.Errorf("expected second task in layer B, got %q", ordered[1].Layer)
		}
	})
}

func TestExtendedTitle(t *testing.T) {
	t.Run("letter bucket appends layer to title", func(t *testing.T) {
		task := wiki.Task{ID: 1, Slug: "test", Title: "My Task"}
		result := wiki.ExtendedTitle(task, "A")
		expected := "My Task [A]"
		if result != expected {
			t.Errorf("expected %q, got %q", expected, result)
		}
	})

	t.Run("Z bucket appends layer to title", func(t *testing.T) {
		task := wiki.Task{ID: 1, Slug: "test", Title: "My Task"}
		result := wiki.ExtendedTitle(task, "Z")
		expected := "My Task [Z]"
		if result != expected {
			t.Errorf("expected %q, got %q", expected, result)
		}
	})

	t.Run("done bucket returns plain title", func(t *testing.T) {
		task := wiki.Task{ID: 1, Slug: "test", Title: "My Task"}
		result := wiki.ExtendedTitle(task, "__done__")
		expected := "My Task"
		if result != expected {
			t.Errorf("expected %q, got %q", expected, result)
		}
	})

	t.Run("deferred bucket returns plain title", func(t *testing.T) {
		task := wiki.Task{ID: 1, Slug: "test", Title: "My Task"}
		result := wiki.ExtendedTitle(task, "__deferred__")
		expected := "My Task"
		if result != expected {
			t.Errorf("expected %q, got %q", expected, result)
		}
	})

	t.Run("all letter buckets A-Y are annotated", func(t *testing.T) {
		task := wiki.Task{ID: 1, Slug: "test", Title: "Task"}
		for i := 0; i < 25; i++ {
			layer := string(rune('A' + i))
			result := wiki.ExtendedTitle(task, layer)
			expected := "Task [" + layer + "]"
			if result != expected {
				t.Errorf("layer %s: expected %q, got %q", layer, expected, result)
			}
		}
	})
}
