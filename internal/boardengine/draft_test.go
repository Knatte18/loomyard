// draft_test.go — the draft status every new task takes at the write critical section, and the writes that release it.

package boardengine_test

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

//testtiming:keep pins that a new task reads draft through each write path and which writes release the draft status, which no covering test asserts
func TestDraftStatus(t *testing.T) {
	tiergate.Require(t, 0)
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
			w, _ := newTestBoard(t)
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
