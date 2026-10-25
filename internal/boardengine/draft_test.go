//go:build tier0

// draft_test.go — the draft status every new task takes at the write critical section, and the writes that release it.

package boardengine_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/boardengine"
)

func TestDraftMigration(t *testing.T) {
	t.Parallel()

	statusOf := func(t *testing.T, w *boardengine.Board, slug string) *string {
		t.Helper()
		got, found, err := w.GetTask(slug)
		if err != nil || !found {
			t.Fatalf("GetTask(%q) = %+v, %v, %v; want the entry", slug, got, found, err)
		}
		return got.Status
	}

	t.Run("a board without the key marks its open tasks with no status on the next write", func(t *testing.T) {
		t.Parallel()
		w, boardPath := newLegacyBoard(t, nil)
		seedBoardJSON(t, boardPath,
			`{"id":0,"slug":"nilled","title":"Nilled","kind":"task","labels":["bug"]}`,
			`{"id":1,"slug":"blank","title":"Blank","kind":"task","labels":["bug"],"status":""}`,
			`{"id":2,"slug":"finished","title":"Finished","kind":"task","labels":["bug"],"status":"done"}`,
			`{"id":3,"slug":"held","title":"Held","kind":"task","labels":["bug"],"status":"running · X"}`,
		)

		if err := w.Rerender(); err != nil {
			t.Fatalf("Rerender() = %v; want nil", err)
		}

		for slug, want := range map[string]string{"nilled": boardengine.StatusDraft, "blank": boardengine.StatusDraft, "finished": "done", "held": "running · X"} {
			if got := statusOf(t, w, slug); got == nil || *got != want {
				t.Errorf("status of %q = %v; want %q", slug, got, want)
			}
		}
		raw, err := os.ReadFile(filepath.Join(boardPath, "board.json"))
		if err != nil || !strings.Contains(string(raw), `"draft_status": true`) {
			t.Errorf("board.json = %s, %v; want it to hold draft_status", raw, err)
		}
	})

	t.Run("a null-status release that is the first write to a board without the key stays released", func(t *testing.T) {
		t.Parallel()
		w, boardPath := newLegacyBoard(t, nil)
		seedBoardJSON(t, boardPath, `{"id":0,"slug":"nilled","title":"Nilled","kind":"task","labels":["bug"]}`)

		if err := w.SetStatus("nilled", nil); err != nil {
			t.Fatalf("SetStatus() = %v; want nil", err)
		}

		if got := statusOf(t, w, "nilled"); got != nil {
			t.Errorf("status of nilled = %q; want nil", *got)
		}
	})

	t.Run("a board holding the key never migrates again", func(t *testing.T) {
		t.Parallel()
		w, boardPath := newLegacyBoard(t, nil)
		content := `{"version":1,"draft_status":true,"entries":[{"id":0,"slug":"released","title":"Released","kind":"task","labels":["bug"]}]}`
		if err := os.WriteFile(filepath.Join(boardPath, "board.json"), []byte(content), 0o644); err != nil {
			t.Fatalf("WriteFile() = %v; want nil", err)
		}

		if err := w.Rerender(); err != nil {
			t.Fatalf("Rerender() = %v; want nil", err)
		}

		if got := statusOf(t, w, "released"); got != nil {
			t.Errorf("status of released = %q; want nil", *got)
		}
	})
}

func TestDraftStatus(t *testing.T) {
	t.Parallel()

	empty := ""
	running := "running · X"
	seedTask := func(slug string, fields map[string]any) func(*testing.T, *boardengine.Board) {
		return func(t *testing.T, w *boardengine.Board) {
			t.Helper()
			fields["slug"] = slug
			if _, err := w.UpsertTask(fields); err != nil {
				t.Fatalf("seed UpsertTask(%q) = %v; want nil", slug, err)
			}
		}
	}
	taskFields := func() map[string]any { return map[string]any{"kind": "task", "labels": bugLabels} }

	tests := []struct {
		name      string
		setup     func(*testing.T, *boardengine.Board)
		act       func(*boardengine.Board) error
		slug      string
		wantDraft bool
	}{
		{
			name: "an upserted new task reads draft",
			act: func(w *boardengine.Board) error {
				_, err := w.UpsertTask(map[string]any{"slug": "t", "kind": "task", "labels": bugLabels})
				return err
			},
			slug: "t", wantDraft: true,
		},
		{
			name: "a batch create reads draft",
			act: func(w *boardengine.Board) error {
				_, err := w.UpsertTasksBatch([]map[string]any{{"slug": "t", "kind": "task", "labels": bugLabels}})
				return err
			},
			slug: "t", wantDraft: true,
		},
		{
			name: "a merge create reads draft",
			act: func(w *boardengine.Board) error {
				_, _, err := w.MergeTasks(nil, map[string]any{"slug": "t", "kind": "task", "labels": bugLabels}, nil)
				return err
			},
			slug: "t", wantDraft: true,
		},
		{
			name:  "a note upserted to a task with a null status reads draft",
			setup: seedTask("t", map[string]any{"labels": bugLabels}),
			act: func(w *boardengine.Board) error {
				_, err := w.UpsertTask(map[string]any{"slug": "t", "kind": "task", "status": nil})
				return err
			},
			slug: "t", wantDraft: true,
		},
		{
			name:  "a project with no members upserted to a task reads draft",
			setup: seedTask("t", map[string]any{"kind": "project", "labels": bugLabels}),
			act: func(w *boardengine.Board) error {
				_, err := w.UpsertTask(map[string]any{"slug": "t", "kind": "task"})
				return err
			},
			slug: "t", wantDraft: true,
		},
		{
			name:  "a promoted note reads draft",
			setup: seedTask("t", map[string]any{"labels": bugLabels}),
			act: func(w *boardengine.Board) error {
				_, err := w.Promote("t")
				return err
			},
			slug: "t", wantDraft: true,
		},
		{
			name: "an empty status written to a task holding a run status reads draft",
			setup: seedTask("t", map[string]any{
				"kind": "task", "labels": bugLabels, "status": running,
			}),
			act:  func(w *boardengine.Board) error { return w.SetStatus("t", &empty) },
			slug: "t", wantDraft: true,
		},
		{
			name:  "a null status set on a draft task releases it",
			setup: seedTask("t", taskFields()),
			act:   func(w *boardengine.Board) error { return w.SetStatus("t", nil) },
			slug:  "t",
		},
		{
			name:  "an upsert with a null status releases a task",
			setup: seedTask("t", taskFields()),
			act: func(w *boardengine.Board) error {
				_, err := w.UpsertTask(map[string]any{"slug": "t", "status": nil})
				return err
			},
			slug: "t",
		},
		{
			name:  "a merge whose set_status sets null releases a task",
			setup: seedTask("t", taskFields()),
			act: func(w *boardengine.Board) error {
				_, _, err := w.MergeTasks(nil, map[string]any{"slug": "t", "kind": "task"}, &boardengine.MergeStatusUpdate{Selector: "t"})
				return err
			},
			slug: "t",
		},
		{
			name: "a later edit of a released task that leaves its status alone keeps it released",
			setup: func(t *testing.T, w *boardengine.Board) {
				t.Helper()
				seedTask("t", taskFields())(t, w)
				if err := w.SetStatus("t", nil); err != nil {
					t.Fatalf("release SetStatus() = %v; want nil", err)
				}
			},
			act: func(w *boardengine.Board) error {
				_, err := w.UpsertTask(map[string]any{"slug": "t", "brief": "changed"})
				return err
			},
			slug: "t",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			w, _ := newLegacyBoard(t, nil)
			if tt.setup != nil {
				tt.setup(t, w)
			}
			if err := tt.act(w); err != nil {
				t.Fatalf("write = %v; want nil", err)
			}
			got, found, err := w.GetTask(tt.slug)
			if err != nil || !found {
				t.Fatalf("GetTask(%q) = %+v, %v, %v; want the entry", tt.slug, got, found, err)
			}
			if tt.wantDraft {
				if got.Status == nil || *got.Status != boardengine.StatusDraft {
					t.Errorf("status = %v; want %q", got.Status, boardengine.StatusDraft)
				}
			} else if got.Status != nil {
				t.Errorf("status = %q; want nil", *got.Status)
			}
		})
	}
}
