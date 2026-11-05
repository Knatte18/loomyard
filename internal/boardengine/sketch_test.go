// sketch_test.go pins the Sketch end of a Discussion-only run: MakeSketch's one write on a run-held task, the entries it leaves alone or refuses, and the migration of a task an older binary left discussed.

package boardengine_test

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// newSketchBoard returns a board in a fresh directory whose vocabulary carries the sketch type label LoadConfig adds, the facade tests' types and the plain label board.
func newSketchBoard(t *testing.T) *boardengine.Board {
	t.Helper()
	types := append([]boardengine.Label{{Name: boardengine.SketchType}}, testTypes...)
	return boardengine.New(boardengine.Config{Path: t.TempDir(), Readme: "Home.md", DesignPrefix: "proposal-", Types: types, Labels: []boardengine.Label{{Name: "board"}}, SkipGit: true})
}

// mustUpsertEntry upserts fields onto b, failing t on a refusal.
func mustUpsertEntry(t *testing.T, b *boardengine.Board, fields map[string]any) {
	t.Helper()
	if _, err := b.UpsertTask(fields); err != nil {
		t.Fatalf("UpsertTask(%v) error = %v; want nil", fields, err)
	}
}

// mustGetEntry returns the entry slug of b, failing t when it is absent.
func mustGetEntry(t *testing.T, b *boardengine.Board, slug string) boardengine.Task {
	t.Helper()
	task, found, err := b.GetTask(slug)
	if err != nil || !found {
		t.Fatalf("GetTask(%q) = %v, %v; want the entry", slug, found, err)
	}
	return task
}

// TestMakeSketch asserts MakeSketch turns a run-held task into a Sketch in one write, leaves an absent or done entry alone and refuses an entry another depends on.
func TestMakeSketch(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	t.Run("HeldTaskBecomesASketch", func(t *testing.T) {
		t.Parallel()
		b := newSketchBoard(t)
		mustUpsertEntry(t, b, map[string]any{"slug": "base", "title": "Base", "kind": "task", "labels": bugLabels})
		mustUpsertEntry(t, b, map[string]any{"slug": "grouping", "title": "Grouping", "kind": "project", "labels": bugLabels})
		mustUpsertEntry(t, b, map[string]any{"slug": "motif", "title": "Motif", "kind": "task", "labels": []string{"bug", "board"}, "recipe": "motif", "depends_on": []string{"base"}, "run_settings": map[string]any{"loom": map[string]any{"code_max_bounces": 1}},
			"isolated": true, "profile": "light", "pull_request": "on", "notify_on_land": true,
			"brief": "short", "body": "The body.", "issues": []int{3}})
		mustUpsertEntry(t, b, map[string]any{"slug": "motif", "project": "grouping"})
		if err := b.SetRunStatus("motif", boardengine.RunStatus("running", "Discussion-Review")); err != nil {
			t.Fatalf("SetRunStatus() error = %v; want nil", err)
		}
		if err := b.StoreDecisionRecord("motif", []byte("record\n")); err != nil {
			t.Fatalf("StoreDecisionRecord() error = %v; want nil", err)
		}

		created := mustGetEntry(t, b, "motif").Created

		if err := b.MakeSketch("motif"); err != nil {
			t.Fatalf("MakeSketch() error = %v; want nil", err)
		}

		got := mustGetEntry(t, b, "motif")
		if !got.Created.Equal(created) || got.Revised.Before(created) {
			t.Errorf("entry created, revised = %v, %v; want created %v kept and revised not before it", got.Created, got.Revised, created)
		}
		if got.Kind != boardengine.KindNote || !slices.Equal(got.Labels, []string{boardengine.SketchType, "board"}) || got.Handle != "S1" {
			t.Errorf("entry kind, labels, handle = %q, %v, %q; want note, [sketch board], S1", got.Kind, got.Labels, got.Handle)
		}
		if got.Status != nil || got.Recipe != "" || len(got.DependsOn) != 0 || got.RunSettings != nil {
			t.Errorf("entry status, recipe, depends_on, run_settings = %v, %q, %v, %v; want none of them", got.Status, got.Recipe, got.DependsOn, got.RunSettings)
		}
		if !got.Discussed || got.Title != "Motif" {
			t.Errorf("entry discussed, title = %t, %q; want true, Motif", got.Discussed, got.Title)
		}
		if got.Isolated || got.Profile != "" || got.PullRequest != "" || got.NotifyOnLand {
			t.Errorf("entry isolated, profile, pull_request, notify_on_land = %t, %q, %q, %t; want all cleared", got.Isolated, got.Profile, got.PullRequest, got.NotifyOnLand)
		}
		if got.Project != "grouping" || got.Brief != "short" || got.Body != "The body." || !slices.Equal(got.Issues, []int{3}) {
			t.Errorf("entry project, brief, body, issues = %q, %q, %q, %v; want grouping, short, The body., [3]", got.Project, got.Brief, got.Body, got.Issues)
		}
	})

	t.Run("AbsentAndDoneEntriesStay", func(t *testing.T) {
		t.Parallel()
		b := newSketchBoard(t)
		mustUpsertEntry(t, b, map[string]any{"slug": "landed", "title": "Landed", "kind": "task", "labels": bugLabels})
		done := "done"
		if err := b.SetStatus("landed", &done); err != nil {
			t.Fatalf("SetStatus() error = %v; want nil", err)
		}
		mustUpsertEntry(t, b, map[string]any{"slug": "sketched", "title": "Sketched", "kind": "note", "labels": []string{boardengine.SketchType}})
		for _, slug := range []string{"absent", "landed", "sketched"} {
			if err := b.MakeSketch(slug); err != nil {
				t.Errorf("MakeSketch(%q) error = %v; want nil", slug, err)
			}
		}
		if got := mustGetEntry(t, b, "sketched"); !slices.Equal(got.Labels, []string{boardengine.SketchType}) || got.Kind != boardengine.KindNote {
			t.Errorf("sketched entry kind, labels = %q, %v; want note, [sketch]", got.Kind, got.Labels)
		}
		if got := mustGetEntry(t, b, "landed"); got.Kind != boardengine.KindTask {
			t.Errorf("done entry kind = %q; want task", got.Kind)
		}
	})

	t.Run("DependedOnEntryIsRefused", func(t *testing.T) {
		t.Parallel()
		b := newSketchBoard(t)
		mustUpsertEntry(t, b, map[string]any{"slug": "motif", "title": "Motif", "kind": "task", "labels": bugLabels})
		mustUpsertEntry(t, b, map[string]any{"slug": "after", "title": "After", "kind": "task", "labels": bugLabels, "depends_on": []string{"motif"}})

		err := b.MakeSketch("motif")
		if !errors.Is(err, boardengine.ErrSketchRefused) || !strings.Contains(err.Error(), "after") || !strings.Contains(err.Error(), "set-deps") {
			t.Errorf("MakeSketch() error = %v; want an ErrSketchRefused refusal naming the dependent and set-deps", err)
		}
		if got := mustGetEntry(t, b, "motif"); got.Kind != boardengine.KindTask {
			t.Errorf("refused entry kind = %q; want task", got.Kind)
		}
	})
}

// TestRunLock_SketchExemptionIsTheWholeNetChange asserts a run-held task passes the run lock only when it becomes a status-less Sketch with no other scope change.
func TestRunLock_SketchExemptionIsTheWholeNetChange(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	running := boardengine.RunStatus("running", "Discussion-Review")
	vocab := boardengine.Vocabulary{Types: []string{boardengine.SketchType, "bug"}}
	before := boardengine.Task{Slug: "held", Title: "Held", Body: "Body", Kind: boardengine.KindTask, Labels: []string{"bug", "board"}, Recipe: "motif", Status: &running}
	sketch := func(edit func(*boardengine.Task)) boardengine.Task {
		now := boardengine.Task{Slug: "held", Title: "Held", Body: "Body", Kind: boardengine.KindNote, Labels: []string{boardengine.SketchType, "board"}}
		edit(&now)
		return now
	}
	tests := []struct {
		name    string
		after   boardengine.Task
		wantErr bool
	}{
		{"PlainSketchPasses", sketch(func(*boardengine.Task) {}), false},
		{"SketchWithATitleChangeIsRefused", sketch(func(task *boardengine.Task) { task.Title = "Renamed" }), true},
		{"SketchWithABodyChangeIsRefused", sketch(func(task *boardengine.Task) { task.Body = "Rewritten" }), true},
		{"SketchThatKeepsItsRunStatusIsRefused", sketch(func(task *boardengine.Task) { task.Status = &running }), true},
		{"NoteWithoutTheSketchLabelIsRefused", sketch(func(task *boardengine.Task) { task.Labels = []string{"board"} }), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := boardengine.CheckRunLocks([]boardengine.Task{before}, []boardengine.Task{tt.after}, vocab)
			if tt.wantErr != errors.Is(err, boardengine.ErrRunLocked) || (!tt.wantErr && err != nil) {
				t.Errorf("CheckRunLocks error = %v; want a refusal matching ErrRunLocked = %t", err, tt.wantErr)
			}
		})
	}
}

// TestDiscussedTaskBecomesASketchOnTheNextWrite asserts a task left in status discussed becomes a Sketch at the next write of any entry, unless another entry depends on it.
func TestDiscussedTaskBecomesASketchOnTheNextWrite(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	b := newSketchBoard(t)
	mustUpsertEntry(t, b, map[string]any{"slug": "talked", "title": "Talked", "kind": "task", "labels": []string{"bug", "board"}})
	mustUpsertEntry(t, b, map[string]any{"slug": "needed", "title": "Needed", "kind": "task", "labels": bugLabels})
	mustUpsertEntry(t, b, map[string]any{"slug": "needs", "title": "Needs", "kind": "task", "labels": bugLabels, "depends_on": []string{"needed"}})
	discussed := boardengine.LegacyStatusDiscussed
	for _, slug := range []string{"talked", "needed"} {
		if err := b.SetStatus(slug, &discussed); err != nil {
			t.Fatalf("SetStatus(%q) error = %v; want nil", slug, err)
		}
	}

	if err := b.Rerender(); err != nil {
		t.Fatalf("Rerender() error = %v; want nil", err)
	}

	got := mustGetEntry(t, b, "talked")
	if got.Kind != boardengine.KindNote || !slices.Equal(got.Labels, []string{boardengine.SketchType, "board"}) || got.Status != nil {
		t.Errorf("entry kind, labels, status = %q, %v, %v; want note, [sketch board], none", got.Kind, got.Labels, got.Status)
	}
	if kept := mustGetEntry(t, b, "needed"); kept.Kind != boardengine.KindTask || kept.Status == nil || *kept.Status != discussed {
		t.Errorf("depended-on entry kind, status = %q, %v; want task, discussed", kept.Kind, kept.Status)
	}
}

// newPromotionBoard returns a board whose vocabulary carries the sketch type label, and the board's directory.
func newPromotionBoard(t *testing.T) (*boardengine.Board, string) {
	t.Helper()
	path := t.TempDir()
	types := append([]boardengine.Label{{Name: boardengine.SketchType}}, testTypes...)
	return boardengine.New(boardengine.Config{Path: path, Readme: "Home.md", DesignPrefix: "proposal-", Types: types, Labels: []boardengine.Label{{Name: "board"}}, SkipGit: true}), path
}

// TestDiscussedPromotion asserts a Sketch, or a discussed note a note write stripped of its label, becomes a task only through a promotion with recipe raddle, which releases it in the same write,
// and that a note whose record file is missing or that was never discussed promotes as any note does.
func TestDiscussedPromotion(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	sketch := func(t *testing.T, b *boardengine.Board, _ string) {
		t.Helper()
		mustUpsertEntry(t, b, map[string]any{"slug": "idea", "title": "Idea", "kind": "note", "labels": []string{boardengine.SketchType}})
		if err := b.StoreDecisionRecord("idea", []byte("record\n")); err != nil {
			t.Fatalf("StoreDecisionRecord() error = %v; want nil", err)
		}
	}
	stripped := func(t *testing.T, b *boardengine.Board, boardPath string) {
		t.Helper()
		sketch(t, b, boardPath)
		mustUpsertEntry(t, b, map[string]any{"slug": "idea", "labels": bugLabels})
	}
	promoteTo := func(recipe string) func(b *boardengine.Board) error {
		return func(b *boardengine.Board) error {
			_, err := b.PromoteTo("idea", recipe)
			return err
		}
	}
	wayForward := "lym board promote idea --recipe raddle"

	tests := []struct {
		name  string
		setup func(t *testing.T, b *boardengine.Board, boardPath string)
		act   func(b *boardengine.Board) error
		// wantErr lists substrings of the refusal; nil means the write is admitted.
		wantErr       []string
		wantKind      string
		wantRecipe    string
		wantDraft     bool
		wantSketch    bool
		wantDiscussed bool
		wantRecord    bool
	}{
		{
			name: "RaddlePromoteOfASketch", setup: sketch, act: promoteTo(shedrun.RecipeRaddle),
			wantKind: boardengine.KindTask, wantRecipe: shedrun.RecipeRaddle, wantSketch: true, wantDiscussed: true, wantRecord: true,
		},
		{
			name: "RaddlePromoteOfAStrippedNote", setup: stripped, act: promoteTo(shedrun.RecipeRaddle),
			wantKind: boardengine.KindTask, wantRecipe: shedrun.RecipeRaddle, wantSketch: true, wantDiscussed: true, wantRecord: true,
		},
		{
			name: "PlainPromoteOfASketchIsRefusedByTheLabelRule", setup: sketch, act: promoteTo(""),
			wantErr:  []string{wayForward},
			wantKind: boardengine.KindNote, wantSketch: true, wantDiscussed: true, wantRecord: true,
		},
		{
			name: "PlainPromoteOfAStrippedNoteIsRefused", setup: stripped, act: promoteTo(""),
			wantErr:  []string{"discussed note with a stored decision record", wayForward},
			wantKind: boardengine.KindNote, wantDiscussed: true, wantRecord: true,
		},
		{
			name: "DarnPromoteOfAStrippedNoteIsRefused", setup: stripped, act: promoteTo("darn"),
			wantErr:  []string{"discussed note with a stored decision record", wayForward},
			wantKind: boardengine.KindNote, wantDiscussed: true, wantRecord: true,
		},
		{
			name: "UpsertToADarnTaskOfASketchIsRefusedByTheLabelRule", setup: sketch,
			act: func(b *boardengine.Board) error {
				_, err := b.UpsertTask(map[string]any{"slug": "idea", "kind": "task", "recipe": "darn"})
				return err
			},
			wantErr:  []string{wayForward},
			wantKind: boardengine.KindNote, wantSketch: true, wantDiscussed: true, wantRecord: true,
		},
		{
			name: "NoteWithAMissingRecordFileIsNotJudged",
			setup: func(t *testing.T, b *boardengine.Board, boardPath string) {
				t.Helper()
				stripped(t, b, boardPath)
				if err := os.Remove(filepath.Join(boardPath, boardengine.DecisionFileName("idea"))); err != nil {
					t.Fatalf("remove record file: %v", err)
				}
			},
			act:      promoteTo("darn"),
			wantKind: boardengine.KindTask, wantRecipe: "darn", wantDraft: true, wantDiscussed: true,
		},
		{
			name: "DarnPromoteOfAnUndiscussedNote",
			setup: func(t *testing.T, b *boardengine.Board, _ string) {
				t.Helper()
				mustUpsertEntry(t, b, map[string]any{"slug": "idea", "title": "Idea", "kind": "note", "labels": bugLabels})
			},
			act:      promoteTo("darn"),
			wantKind: boardengine.KindTask, wantRecipe: "darn", wantDraft: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			b, boardPath := newPromotionBoard(t)
			tt.setup(t, b, boardPath)

			err := tt.act(b)
			if tt.wantErr == nil && err != nil {
				t.Fatalf("write error = %v; want nil", err)
			}
			if tt.wantErr != nil && err == nil {
				t.Fatalf("write error = nil; want a refusal containing %v", tt.wantErr)
			}
			for _, want := range tt.wantErr {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("write error = %q; want it to contain %q", err, want)
				}
			}

			got := mustGetEntry(t, b, "idea")
			if got.Kind != tt.wantKind || got.Recipe != tt.wantRecipe {
				t.Errorf("entry kind, recipe = %q, %q; want %q, %q", got.Kind, got.Recipe, tt.wantKind, tt.wantRecipe)
			}
			if draft := got.Status != nil && *got.Status == boardengine.StatusDraft; draft != tt.wantDraft || (!tt.wantDraft && got.Status != nil) {
				t.Errorf("entry status = %v; want draft = %t, otherwise none", got.Status, tt.wantDraft)
			}
			if sketched := slices.Contains(got.Labels, boardengine.SketchType); sketched != tt.wantSketch {
				t.Errorf("entry labels = %v; want the sketch label = %t", got.Labels, tt.wantSketch)
			}
			if got.Discussed != tt.wantDiscussed {
				t.Errorf("entry discussed = %t; want %t", got.Discussed, tt.wantDiscussed)
			}
			if _, held, err := b.DecisionRecord("idea"); err != nil || held != tt.wantRecord {
				t.Errorf("DecisionRecord() held, error = %t, %v; want held = %t", held, err, tt.wantRecord)
			}
			if boardengine.IsSketch(got.Kind, got.Labels) != (tt.wantKind == boardengine.KindNote && tt.wantSketch) {
				t.Errorf("IsSketch(%q, %v) = %t; want it true only for the Sketch note left as it was", got.Kind, got.Labels, !(tt.wantKind == boardengine.KindNote && tt.wantSketch))
			}
		})
	}
}
