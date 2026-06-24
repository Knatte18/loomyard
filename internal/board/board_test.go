// board_test.go — unit tests for the Board facade (board.go).
//
// Upsert / remove / rerender against a temp board with git skipped.

package board_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/board"
)

// TestUpsertTask tests the facade persistence wiring: creating a task writes
// both tasks.json and Home.md. Drop: store-layer assertion "update preserves
// fields" (owned by store_test.go:TestUpsertTaskPreservesFields).
func TestUpsertTask(t *testing.T) {
	boardPath := t.TempDir()
	cfg := board.Config{Path: boardPath, Home: "Home.md", Sidebar: "_Sidebar.md", ProposalPrefix: "proposal-", SkipGit: true}
	w := board.New(cfg)

	// Creates task, tasks.json written, Home.md written
	task, err := w.UpsertTask(map[string]any{
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
	tasksPath := filepath.Join(boardPath, "tasks.json")
	if _, err := os.Stat(tasksPath); err != nil {
		t.Fatalf("tasks.json not created: %v", err)
	}

	// Check Home.md exists
	homePath := filepath.Join(boardPath, "Home.md")
	if _, err := os.Stat(homePath); err != nil {
		t.Fatalf("Home.md not created: %v", err)
	}
}

func TestRerender(t *testing.T) {
	boardPath := t.TempDir()
	cfg := board.Config{Path: boardPath, Home: "Home.md", Sidebar: "_Sidebar.md", ProposalPrefix: "proposal-", SkipGit: true}
	w := board.New(cfg)

	// (d) Writes all output files without error on empty store
	err := w.Rerender()
	if err != nil {
		t.Fatalf("Rerender failed: %v", err)
	}

	// Check that Home.md and _Sidebar.md exist
	homePath := filepath.Join(boardPath, "Home.md")
	sidebarPath := filepath.Join(boardPath, "_Sidebar.md")

	if _, err := os.Stat(homePath); err != nil {
		t.Fatalf("Home.md not created: %v", err)
	}

	if _, err := os.Stat(sidebarPath); err != nil {
		t.Fatalf("_Sidebar.md not created: %v", err)
	}
}

func TestHealthCheckPasses(t *testing.T) {
	boardPath := t.TempDir()
	cfg := board.Config{Path: boardPath, Home: "Home.md", Sidebar: "_Sidebar.md", ProposalPrefix: "proposal-", SkipGit: true}
	w := board.New(cfg)

	// Create a task to initialize the board directory and tasks.json
	_, err := w.UpsertTask(map[string]any{
		"slug":  "test-task",
		"title": "Test Task",
	})
	if err != nil {
		t.Fatalf("UpsertTask failed: %v", err)
	}

	// HealthCheck should pass for a healthy board
	err = w.HealthCheck()
	if err != nil {
		t.Fatalf("HealthCheck failed for healthy board: %v", err)
	}
}

func TestHealthCheckFailsNoBoardDir(t *testing.T) {
	boardPath := filepath.Join(t.TempDir(), "nonexistent")
	cfg := board.Config{Path: boardPath, Home: "Home.md", Sidebar: "_Sidebar.md", ProposalPrefix: "proposal-"}
	w := board.New(cfg)

	// HealthCheck should fail when board directory does not exist
	err := w.HealthCheck()
	if err == nil {
		t.Fatalf("HealthCheck should fail when board directory is absent")
	}
}

func TestHealthCheckFailsNoTasksFile(t *testing.T) {
	boardPath := t.TempDir()
	cfg := board.Config{Path: boardPath, Home: "Home.md", Sidebar: "_Sidebar.md", ProposalPrefix: "proposal-"}
	w := board.New(cfg)

	// HealthCheck should fail when tasks.json does not exist
	err := w.HealthCheck()
	if err == nil {
		t.Fatalf("HealthCheck should fail when tasks.json is absent")
	}
}

func TestHealthCheckPassesCorruptFile(t *testing.T) {
	boardPath := t.TempDir()
	cfg := board.Config{Path: boardPath, Home: "Home.md", Sidebar: "_Sidebar.md", ProposalPrefix: "proposal-"}
	w := board.New(cfg)

	// Create a corrupt but readable tasks.json
	tasksPath := filepath.Join(boardPath, "tasks.json")
	err := os.WriteFile(tasksPath, []byte("{invalid json"), 0o644)
	if err != nil {
		t.Fatalf("Failed to write corrupt tasks.json: %v", err)
	}

	// HealthCheck should pass even if JSON is corrupt, as long as it's readable
	err = w.HealthCheck()
	if err != nil {
		t.Fatalf("HealthCheck failed for corrupt but readable tasks.json: %v", err)
	}
}
