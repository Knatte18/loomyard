package wiki_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/mhgo/internal/wiki"
)

func TestUpsertTask(t *testing.T) {
	t.Setenv("WIKI_SKIP_GIT", "1")

	wikiPath := t.TempDir()
	w := wiki.New(wikiPath)

	// (a) Creates task, tasks.json written, Home.md written
	task, err := w.UpsertTask(map[string]interface{}{
		"slug":  "test-task",
		"title": "Test Task",
	})
	if err != nil {
		t.Fatalf("UpsertTask failed: %v", err)
	}

	if task.Slug != "test-task" || task.Title != "Test Task" {
		t.Fatalf("Task not created correctly: %+v", task)
	}

	// Check tasks.json exists
	tasksPath := filepath.Join(wikiPath, "tasks.json")
	if _, err := os.Stat(tasksPath); err != nil {
		t.Fatalf("tasks.json not created: %v", err)
	}

	// Check Home.md exists
	homePath := filepath.Join(wikiPath, "Home.md")
	if _, err := os.Stat(homePath); err != nil {
		t.Fatalf("Home.md not created: %v", err)
	}

	// (b) Update preserves other fields
	task2, err := w.UpsertTask(map[string]interface{}{
		"slug":  "test-task",
		"title": "Updated Title",
		"brief": "Brief description",
	})
	if err != nil {
		t.Fatalf("UpsertTask update failed: %v", err)
	}

	if task2.Title != "Updated Title" || task2.Brief != "Brief description" {
		t.Fatalf("Update failed: %+v", task2)
	}

	// ID should be preserved
	if task2.ID != task.ID {
		t.Fatalf("ID changed during update: %d vs %d", task.ID, task2.ID)
	}
}

func TestRemoveTask(t *testing.T) {
	t.Setenv("WIKI_SKIP_GIT", "1")

	wikiPath := t.TempDir()
	w := wiki.New(wikiPath)

	// (c) Error for missing slug
	err := w.RemoveTask("nonexistent")
	if err == nil {
		t.Fatalf("RemoveTask should error for missing task")
	}
}

func TestRerender(t *testing.T) {
	t.Setenv("WIKI_SKIP_GIT", "1")

	wikiPath := t.TempDir()
	w := wiki.New(wikiPath)

	// (d) Writes all output files without error on empty store
	err := w.Rerender()
	if err != nil {
		t.Fatalf("Rerender failed: %v", err)
	}

	// Check that Home.md and _Sidebar.md exist
	homePath := filepath.Join(wikiPath, "Home.md")
	sidebarPath := filepath.Join(wikiPath, "_Sidebar.md")

	if _, err := os.Stat(homePath); err != nil {
		t.Fatalf("Home.md not created: %v", err)
	}

	if _, err := os.Stat(sidebarPath); err != nil {
		t.Fatalf("_Sidebar.md not created: %v", err)
	}
}
