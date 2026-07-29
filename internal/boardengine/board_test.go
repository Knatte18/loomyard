// board_test.go — unit tests for the Board facade (board.go).
//
// Upsert / remove / rerender against a temp board with git skipped.

package boardengine_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/boardengine"
)

// TestUpsertTask tests the facade persistence wiring: creating a task writes
// both tasks.json and Home.md. Drop: store-layer assertion "update preserves
// fields" (owned by store_test.go:TestUpsertTaskPreservesFields).
func TestUpsertTask(t *testing.T) {
	boardPath := t.TempDir()
	cfg := boardengine.Config{Path: boardPath, Readme: "Home.md", DesignPrefix: "proposal-", SkipGit: true}
	w := boardengine.New(cfg)

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

// TestUpsertTaskUnconfiguredOutputsFailsBeforeWriting locks in writeOp's
// fail-fast guard: a Board built without output filenames (the --board-path
// shape) must reject a write before touching disk, never save tasks.json and
// then fail the render on an empty filename.
func TestUpsertTaskUnconfiguredOutputsFailsBeforeWriting(t *testing.T) {
	boardPath := t.TempDir()
	cfg := boardengine.Config{Path: boardPath, SkipGit: true}
	w := boardengine.New(cfg)

	_, err := w.UpsertTask(map[string]any{"slug": "test-task", "title": "Test Task"})
	if err == nil {
		t.Fatalf("UpsertTask without configured outputs should error")
	}

	// The half-applied failure mode this guards against saved tasks.json first;
	// the guard must fire before any disk mutation.
	if _, statErr := os.Stat(filepath.Join(boardPath, "tasks.json")); !os.IsNotExist(statErr) {
		t.Fatalf("tasks.json must not be created on a rejected write; stat err = %v", statErr)
	}
}

func TestRerender(t *testing.T) {
	boardPath := t.TempDir()
	cfg := boardengine.Config{Path: boardPath, Readme: "Home.md", DesignPrefix: "proposal-", SkipGit: true}
	w := boardengine.New(cfg)

	// (d) Writes all output files without error on empty store
	err := w.Rerender()
	if err != nil {
		t.Fatalf("Rerender failed: %v", err)
	}

	// Check that Home.md exists
	homePath := filepath.Join(boardPath, "Home.md")

	if _, err := os.Stat(homePath); err != nil {
		t.Fatalf("Home.md not created: %v", err)
	}
}

func TestHealthCheckPasses(t *testing.T) {
	boardPath := t.TempDir()
	cfg := boardengine.Config{Path: boardPath, Readme: "Home.md", DesignPrefix: "proposal-", SkipGit: true}
	w := boardengine.New(cfg)

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
	cfg := boardengine.Config{Path: boardPath, Readme: "Home.md", DesignPrefix: "proposal-"}
	w := boardengine.New(cfg)

	// HealthCheck should fail when board directory does not exist
	err := w.HealthCheck()
	if err == nil {
		t.Fatalf("HealthCheck should fail when board directory is absent")
	}
}

func TestHealthCheckFailsNoTasksFile(t *testing.T) {
	boardPath := t.TempDir()
	cfg := boardengine.Config{Path: boardPath, Readme: "Home.md", DesignPrefix: "proposal-"}
	w := boardengine.New(cfg)

	// HealthCheck should fail when tasks.json does not exist
	err := w.HealthCheck()
	if err == nil {
		t.Fatalf("HealthCheck should fail when tasks.json is absent")
	}
}

func TestHealthCheckPassesCorruptFile(t *testing.T) {
	boardPath := t.TempDir()
	cfg := boardengine.Config{Path: boardPath, Readme: "Home.md", DesignPrefix: "proposal-"}
	w := boardengine.New(cfg)

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

// TestGlobalSlugUniqueness locks in the cross-store slug check wired into
// UpsertTask/UpsertNote: tasks.json and notes.json share one global slug
// namespace, so introducing a slug in one store that already exists in the
// other must fail, in both directions, while an in-place update of a slug
// already present in its own store must succeed regardless of what the other
// store holds.
func TestGlobalSlugUniqueness(t *testing.T) {
	t.Run("task then note with the same slug", func(t *testing.T) {
		boardPath := t.TempDir()
		cfg := boardengine.Config{Path: boardPath, Readme: "Home.md", DesignPrefix: "proposal-", SkipGit: true}
		w := boardengine.New(cfg)

		if _, err := w.UpsertTask(map[string]any{"slug": "shared", "title": "T"}); err != nil {
			t.Fatalf("UpsertTask failed: %v", err)
		}
		if _, err := w.UpsertNote(map[string]any{"slug": "shared", "title": "N"}); err == nil {
			t.Fatalf("expected error upserting a note with a slug already used by a task")
		} else if !strings.Contains(err.Error(), "already exists") {
			t.Errorf("expected error containing 'already exists', got: %v", err)
		}
	})

	t.Run("note then task with the same slug", func(t *testing.T) {
		boardPath := t.TempDir()
		cfg := boardengine.Config{Path: boardPath, Readme: "Home.md", DesignPrefix: "proposal-", SkipGit: true}
		w := boardengine.New(cfg)

		if _, err := w.UpsertNote(map[string]any{"slug": "shared", "title": "N"}); err != nil {
			t.Fatalf("UpsertNote failed: %v", err)
		}
		if _, err := w.UpsertTask(map[string]any{"slug": "shared", "title": "T"}); err == nil {
			t.Fatalf("expected error upserting a task with a slug already used by a note")
		} else if !strings.Contains(err.Error(), "already exists") {
			t.Errorf("expected error containing 'already exists', got: %v", err)
		}
	})

	t.Run("in-place update of an existing same-store slug succeeds regardless of the other store", func(t *testing.T) {
		boardPath := t.TempDir()
		cfg := boardengine.Config{Path: boardPath, Readme: "Home.md", DesignPrefix: "proposal-", SkipGit: true}
		w := boardengine.New(cfg)

		// Seed the other store so it is non-empty at update time.
		if _, err := w.UpsertNote(map[string]any{"slug": "note-slug", "title": "N"}); err != nil {
			t.Fatalf("UpsertNote failed: %v", err)
		}
		if _, err := w.UpsertTask(map[string]any{"slug": "task-slug", "title": "T1"}); err != nil {
			t.Fatalf("initial UpsertTask failed: %v", err)
		}

		task, err := w.UpsertTask(map[string]any{"slug": "task-slug", "title": "T2"})
		if err != nil {
			t.Fatalf("in-place update of an existing slug should succeed, got: %v", err)
		}
		if task.Title != "T2" {
			t.Errorf("expected updated title 'T2', got %q", task.Title)
		}
	})
}

// TestPromoteNoteDanglingDependencyFails covers PromoteNote's validation-failure
// path: promoting a note whose depends_on names a second, not-yet-promoted
// note-only slug must error with a dangling-dependency message and leave both
// stores' on-disk files byte-identical to their pre-call state — the failure
// happens inside tasksStore.UpsertTask, before either store's Save is reached.
func TestPromoteNoteDanglingDependencyFails(t *testing.T) {
	boardPath := t.TempDir()
	cfg := boardengine.Config{Path: boardPath, Readme: "Home.md", DesignPrefix: "proposal-", SkipGit: true}
	w := boardengine.New(cfg)

	// Seed tasks.json onto disk (empty) so the byte-identical comparison below
	// has a real pre-call file to compare against, mirroring a board that has
	// already been initialized.
	if err := w.Rerender(); err != nil {
		t.Fatalf("Rerender failed: %v", err)
	}

	if _, err := w.UpsertNote(map[string]any{"slug": "note-b", "title": "B"}); err != nil {
		t.Fatalf("UpsertNote note-b failed: %v", err)
	}
	if _, err := w.UpsertNote(map[string]any{"slug": "note-a", "title": "A", "depends_on": []string{"note-b"}}); err != nil {
		t.Fatalf("UpsertNote note-a failed: %v", err)
	}

	tasksPath := filepath.Join(boardPath, "tasks.json")
	notesPath := filepath.Join(boardPath, "notes.json")
	tasksBefore, err := os.ReadFile(tasksPath)
	if err != nil {
		t.Fatalf("read tasks.json before PromoteNote: %v", err)
	}
	notesBefore, err := os.ReadFile(notesPath)
	if err != nil {
		t.Fatalf("read notes.json before PromoteNote: %v", err)
	}

	if _, err := w.PromoteNote("note-a"); err == nil {
		t.Fatalf("expected PromoteNote to fail on a dangling dependency")
	} else if !strings.Contains(err.Error(), "dangling dependency") {
		t.Errorf("expected error containing 'dangling dependency', got: %v", err)
	}

	tasksAfter, err := os.ReadFile(tasksPath)
	if err != nil {
		t.Fatalf("read tasks.json after PromoteNote: %v", err)
	}
	notesAfter, err := os.ReadFile(notesPath)
	if err != nil {
		t.Fatalf("read notes.json after PromoteNote: %v", err)
	}
	if !bytes.Equal(tasksBefore, tasksAfter) {
		t.Errorf("tasks.json changed on a failed PromoteNote\nbefore: %s\nafter:  %s", tasksBefore, tasksAfter)
	}
	if !bytes.Equal(notesBefore, notesAfter) {
		t.Errorf("notes.json changed on a failed PromoteNote\nbefore: %s\nafter:  %s", notesBefore, notesAfter)
	}
}

// TestPromoteNoteCrashBetweenSavesConvergesOnRetry covers PromoteNote's
// documented crash-safety story: a crash between the tasksStore.Save and
// notesStore.Save leaves the entry in BOTH files; a retry must converge
// idempotently, with no duplicate id in tasks.json and the note actually gone
// from notes.json. The crash midpoint is simulated by writing directly to
// tasks.json via a raw Store (not Board.UpsertTask, which the cross-store
// slug check would reject once the slug exists in notesStore) — mirroring
// PromoteNote's own documented bypass pattern.
func TestPromoteNoteCrashBetweenSavesConvergesOnRetry(t *testing.T) {
	boardPath := t.TempDir()
	cfg := boardengine.Config{Path: boardPath, Readme: "Home.md", DesignPrefix: "proposal-", SkipGit: true}
	w := boardengine.New(cfg)

	fields := map[string]any{"slug": "promo-note", "title": "Promo Note", "brief": "note brief"}
	if _, err := w.UpsertNote(fields); err != nil {
		t.Fatalf("UpsertNote failed: %v", err)
	}

	// Simulate the post-first-save/pre-second-save crash state: place the
	// slug directly into tasks.json while it is still present in notes.json.
	tasksStore := boardengine.NewStore(filepath.Join(boardPath, "tasks.json"))
	if err := tasksStore.Load(); err != nil {
		t.Fatalf("load tasks store: %v", err)
	}
	if _, err := tasksStore.UpsertTask(fields); err != nil {
		t.Fatalf("seed tasks store: %v", err)
	}
	if err := tasksStore.Save(); err != nil {
		t.Fatalf("save tasks store: %v", err)
	}

	result, err := w.PromoteNote("promo-note")
	if err != nil {
		t.Fatalf("PromoteNote retry should converge idempotently, got: %v", err)
	}
	if result.Slug != "promo-note" {
		t.Errorf("expected returned task slug 'promo-note', got %q", result.Slug)
	}

	if _, found, err := w.GetNote("promo-note"); err != nil {
		t.Fatalf("GetNote: %v", err)
	} else if found {
		t.Errorf("note should be gone from notes.json after PromoteNote converges")
	}

	tasks, err := w.ListTasksFull()
	if err != nil {
		t.Fatalf("ListTasksFull: %v", err)
	}
	count := 0
	for _, task := range tasks {
		if task.Slug == "promo-note" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected exactly one tasks.json entry for promo-note (no duplicate id), got %d", count)
	}
}
