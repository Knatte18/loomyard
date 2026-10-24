//go:build tier0

// taskguard_test.go — the refusal of a write that creates or promotes a task from a run agent's session (taskguard.go), driven through the Board facade.

package boardengine_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/boardengine"
)

// TestTaskCreationGuard pins which writes a run agent's session may make, and that the operator, the hub orch and an orch thread may create a task.
// A refused row leaves board.json byte-identical.
func TestTaskCreationGuard(t *testing.T) {
	t.Parallel()

	const runAgent = "ly:some-task:webster"
	existingTask := map[string]any{"slug": "existing", "title": "Existing", "kind": "task", "labels": []string{"bug"}}
	existingNote := map[string]any{"slug": "idea", "title": "Idea", "kind": "note", "labels": []string{"bug"}}
	newTask := map[string]any{"slug": "follow-up", "title": "Follow-up", "kind": "task", "labels": []string{"bug"}}

	tests := []struct {
		name       string
		strandName string
		act        func(b *boardengine.Board) error
		// refused names the slug the refusal must name; empty means the write passes.
		refused string
	}{
		{name: "upsert a new task", strandName: runAgent, refused: "follow-up", act: func(b *boardengine.Board) error {
			_, err := b.UpsertTask(newTask)
			return err
		}},
		{name: "upsert a note to kind task", strandName: runAgent, refused: "idea", act: func(b *boardengine.Board) error {
			_, err := b.UpsertTask(map[string]any{"slug": "idea", "kind": "task"})
			return err
		}},
		{name: "promote a note", strandName: runAgent, refused: "idea", act: func(b *boardengine.Board) error {
			_, err := b.Promote("idea")
			return err
		}},
		{name: "batch carrying a new task", strandName: runAgent, refused: "follow-up", act: func(b *boardengine.Board) error {
			_, err := b.UpsertTasksBatch([]map[string]any{{"slug": "existing", "body": "new"}, newTask})
			return err
		}},
		{name: "merge upserting a new task", strandName: runAgent, refused: "follow-up", act: func(b *boardengine.Board) error {
			_, _, err := b.MergeTasks([]string{"existing"}, newTask, nil)
			return err
		}},

		{name: "a run agent files a note", strandName: runAgent, act: func(b *boardengine.Board) error {
			_, err := b.UpsertTask(map[string]any{"slug": "finding", "title": "Finding", "kind": "note", "labels": []string{"bug"}})
			return err
		}},
		{name: "a run agent edits an existing task", strandName: runAgent, act: func(b *boardengine.Board) error {
			_, err := b.UpsertTask(map[string]any{"slug": "existing", "body": "new"})
			return err
		}},
		{name: "a run agent renames a task", strandName: runAgent, act: func(b *boardengine.Board) error {
			_, err := b.Rename("existing", "renamed")
			return err
		}},
		{name: "the operator creates a task", act: func(b *boardengine.Board) error {
			_, err := b.UpsertTask(newTask)
			return err
		}},
		{name: "the hub orch promotes a note", strandName: "ly:orch", act: func(b *boardengine.Board) error {
			_, err := b.Promote("idea")
			return err
		}},
		{name: "an orch thread creates a task", strandName: "ly:some-task:orch", act: func(b *boardengine.Board) error {
			_, err := b.UpsertTask(newTask)
			return err
		}},
		{name: "a numbered orch thread creates a task", strandName: "ly:some-task:orch-2", act: func(b *boardengine.Board) error {
			_, err := b.UpsertTask(newTask)
			return err
		}},
		{name: "a slug-free non-orch session creates a task", strandName: "ly:webster", act: func(b *boardengine.Board) error {
			_, err := b.UpsertTask(newTask)
			return err
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			boardPath := t.TempDir()
			config := boardengine.Config{Path: boardPath, Readme: "Home.md", DesignPrefix: "proposal-", Types: testTypes, Labels: testLabels, SkipGit: true}
			seeder := boardengine.New(config)
			for _, seed := range []map[string]any{existingTask, existingNote} {
				if _, err := seeder.UpsertTask(seed); err != nil {
					t.Fatalf("seed %v: %v", seed["slug"], err)
				}
			}
			before, err := os.ReadFile(filepath.Join(boardPath, "board.json"))
			if err != nil {
				t.Fatalf("read board.json: %v", err)
			}
			config.StrandName = tt.strandName

			err = tt.act(boardengine.New(config))

			if tt.refused == "" {
				if err != nil {
					t.Fatalf("write refused: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("write passed; want it refused")
			}
			for _, want := range []string{tt.strandName, `"` + tt.refused + `"`, `"kind":"note"`} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("message %q lacks %q", err.Error(), want)
				}
			}
			after, err := os.ReadFile(filepath.Join(boardPath, "board.json"))
			if err != nil {
				t.Fatalf("read board.json: %v", err)
			}
			if string(after) != string(before) {
				t.Error("board.json changed on a refused write")
			}
		})
	}
}
