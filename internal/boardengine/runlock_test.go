// runlock_test.go — the run-status form and the lock on a run-held entry's scope (runlock.go), driven through the Board facade.

package boardengine_test

import (
	"errors"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/boardengine"
)

func strPtr(s string) *string { return &s }

// TestRunLock pins the run-status form and which writes a run-held entry refuses.
// A refused row leaves board.json byte-identical.
func TestRunLock(t *testing.T) {
	t.Parallel()

	// lockedBase is the held entry; rows override fields of it.
	lockedBase := map[string]any{
		"slug": "locked", "title": "Held", "kind": "task", "labels": []string{"bug"},
		"brief": "brief", "body": "body", "status": "running · Webster",
	}
	other := map[string]any{"slug": "other", "title": "Other", "kind": "task", "labels": []string{"bug"}}
	done := map[string]any{"slug": "fin", "title": "Fin", "kind": "task", "labels": []string{"bug"}, "status": "done"}

	rows := []struct {
		name    string
		locked  map[string]any
		extra   []map[string]any
		act     func(b *boardengine.Board) error
		refused bool
	}{
		{name: "upsert body", refused: true, act: upsertLocked(map[string]any{"body": "new"})},
		{name: "upsert brief", refused: true, act: upsertLocked(map[string]any{"brief": "new"})},
		{name: "upsert labels", refused: true, act: upsertLocked(map[string]any{"labels": []string{"enhancement"}})},
		{name: "upsert issues", refused: true, act: upsertLocked(map[string]any{"issues": []int{7}})},
		{name: "upsert title", refused: true, act: upsertLocked(map[string]any{"title": "new"})},
		{name: "upsert kind", refused: true, act: upsertLocked(map[string]any{"kind": "note"})},
		{name: "upsert recipe", refused: true, act: upsertLocked(map[string]any{"recipe": "other-recipe"})},
		{name: "upsert priority", refused: true, act: upsertLocked(map[string]any{"priority": "high"})},
		{name: "upsert depends_on", refused: true, extra: []map[string]any{other}, act: upsertLocked(map[string]any{"depends_on": []string{"other"}})},
		{name: "upsert status and body", refused: true, act: upsertLocked(map[string]any{"status": "done", "body": "new"})},
		{name: "status blocked", refused: true, locked: map[string]any{"status": "blocked · X"}, act: upsertLocked(map[string]any{"body": "new"})},
		{name: "status paused", refused: true, locked: map[string]any{"status": "paused · X"}, act: upsertLocked(map[string]any{"body": "new"})},
		{name: "status failed", refused: true, locked: map[string]any{"status": "failed · X"}, act: upsertLocked(map[string]any{"body": "new"})},
		{name: "status awaiting", refused: true, locked: map[string]any{"status": "awaiting · X"}, act: upsertLocked(map[string]any{"body": "new"})},
		{name: "set deps", refused: true, extra: []map[string]any{other}, act: func(b *boardengine.Board) error { return b.SetDeps("locked", []string{"other"}) }},
		{name: "promote a locked note", refused: true, locked: map[string]any{"kind": "note"}, act: func(b *boardengine.Board) error {
			_, err := b.Promote("locked")
			return err
		}},
		{name: "remove", refused: true, act: func(b *boardengine.Board) error { return b.RemoveTask("locked") }},
		{name: "merge removing it", refused: true, extra: []map[string]any{other}, act: func(b *boardengine.Board) error {
			_, err := b.MergeTasks([]string{"locked"}, map[string]any{"slug": "other", "body": "merged"}, nil)
			return err
		}},
		{name: "merge upserting a change into it", refused: true, extra: []map[string]any{other}, act: func(b *boardengine.Board) error {
			_, err := b.MergeTasks([]string{"other"}, map[string]any{"slug": "locked", "body": "merged"}, nil)
			return err
		}},
		{name: "merge changing it and clearing its status", refused: true, extra: []map[string]any{other}, act: func(b *boardengine.Board) error {
			_, err := b.MergeTasks([]string{"other"}, map[string]any{"slug": "locked", "body": "merged"}, &boardengine.MergeStatusUpdate{Selector: "locked"})
			return err
		}},
		{name: "batch with one changing item", refused: true, extra: []map[string]any{other}, act: func(b *boardengine.Board) error {
			return b.UpsertTasksBatch([]map[string]any{{"slug": "other", "body": "batched"}, {"slug": "locked", "body": "new"}})
		}},
		{name: "batch clearing the status then changing the body", refused: true, act: func(b *boardengine.Board) error {
			return b.UpsertTasksBatch([]map[string]any{{"slug": "locked", "status": ""}, {"slug": "locked", "body": "new"}})
		}},

		{name: "set-status to another value", act: func(b *boardengine.Board) error { return b.SetStatus("locked", strPtr("paused · X")) }},
		{name: "set-status to nil", act: func(b *boardengine.Board) error { return b.SetStatus("locked", nil) }},
		{name: "merge whose set_status targets it", extra: []map[string]any{other}, act: func(b *boardengine.Board) error {
			_, err := b.MergeTasks(nil, map[string]any{"slug": "other", "body": "merged"}, &boardengine.MergeStatusUpdate{Selector: "locked", Status: strPtr("done")})
			return err
		}},
		{name: "identical upsert", act: upsertLocked(map[string]any{"body": "body", "labels": []string{"bug"}, "issues": []int{}, "depends_on": []string{}})},
		{name: "status-only upsert", act: upsertLocked(map[string]any{"status": "done"})},
		{name: "normal priority on an entry without one", act: upsertLocked(map[string]any{"priority": "normal"})},
		{name: "batch whose only item on it is status-only", extra: []map[string]any{other}, act: func(b *boardengine.Board) error {
			return b.UpsertTasksBatch([]map[string]any{{"slug": "other", "body": "batched"}, {"slug": "locked", "status": "done"}})
		}},
		{name: "prune dropping a done dependency", locked: map[string]any{"depends_on": []string{"fin"}}, extra: []map[string]any{done}, act: func(b *boardengine.Board) error {
			removed, err := b.Prune()
			if err == nil && (len(removed) != 1 || removed[0] != "fin") {
				t.Errorf("prune removed %v, want [fin]", removed)
			}
			return err
		}},
		{name: "write to another entry", extra: []map[string]any{other}, act: func(b *boardengine.Board) error {
			_, err := b.UpsertTask(map[string]any{"slug": "other", "body": "new"})
			return err
		}},
		{name: "add a dependency on the locked entry", extra: []map[string]any{other}, act: func(b *boardengine.Board) error { return b.SetDeps("other", []string{"locked"}) }},
		{name: "body change on an entry whose status is not a run status", locked: map[string]any{"status": "active"}, act: upsertLocked(map[string]any{"body": "new"})},
	}

	for _, tc := range rows {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			boardPath := t.TempDir()
			b := boardengine.New(boardengine.Config{Path: boardPath, Readme: "Home.md", DesignPrefix: "proposal-", Types: testTypes, Labels: testLabels, SkipGit: true})

			fields := maps.Clone(lockedBase)
			maps.Copy(fields, tc.locked)
			// Seeded dependencies go first, as an entry may only depend on one already stored.
			for _, seed := range append(append([]map[string]any{}, tc.extra...), fields) {
				if _, err := b.UpsertTask(seed); err != nil {
					t.Fatalf("seed %v: %v", seed["slug"], err)
				}
			}
			before, err := os.ReadFile(filepath.Join(boardPath, "board.json"))
			if err != nil {
				t.Fatalf("read board.json: %v", err)
			}

			err = tc.act(b)

			after, readErr := os.ReadFile(filepath.Join(boardPath, "board.json"))
			if readErr != nil {
				t.Fatalf("read board.json: %v", readErr)
			}
			if !tc.refused {
				if err != nil {
					t.Fatalf("write refused: %v", err)
				}
				return
			}
			if !errors.Is(err, boardengine.ErrRunLocked) {
				t.Fatalf("err = %v, want ErrRunLocked", err)
			}
			status, _ := fields["status"].(string)
			for _, want := range []string{`"locked"`, status, "lyx batten status locked", "prime worktree", `"kind":"note"`} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("message %q lacks %q", err.Error(), want)
				}
			}
			if string(after) != string(before) {
				t.Error("board.json changed on a refused write")
			}
		})
	}

	t.Run("run status form", func(t *testing.T) {
		t.Parallel()
		if got := boardengine.RunStatus("running", "Webster"); got != "running · Webster" {
			t.Errorf("RunStatus = %q", got)
		}
		cases := []struct {
			status *string
			want   bool
		}{
			{nil, false},
			{strPtr(""), false},
			{strPtr("done"), false},
			{strPtr("abandoned"), false},
			{strPtr("active"), false},
			{strPtr("running · Webster"), true},
			{strPtr("blocked · X"), true},
			{strPtr("paused · X"), true},
			{strPtr("failed · X"), true},
			{strPtr("awaiting · X"), true},
		}
		for _, tc := range cases {
			if got := boardengine.IsRunStatus(tc.status); got != tc.want {
				t.Errorf("IsRunStatus(%v) = %v, want %v", tc.status, got, tc.want)
			}
		}
	})
}

// upsertLocked returns an action upserting patch onto the entry "locked".
func upsertLocked(patch map[string]any) func(b *boardengine.Board) error {
	return func(b *boardengine.Board) error {
		fields := map[string]any{"slug": "locked"}
		maps.Copy(fields, patch)
		_, err := b.UpsertTask(fields)
		return err
	}
}
