// commitstatus_integration_test.go drives the commit-status seam that each wiring entry point installs.
// It runs against a real hub pair and its hub board, and pins the board entry a finished run leaves per recipe.

package loomcli

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestWiredCommitStatus_FinishedRunEntry drives the seam that wireLightweight and wire each install.
// The test takes a run through its last transition and asserts the board entry that transition leaves.
// A recipe with no Finalize row leaves the run-held task a Sketch with no status;
// the task stays draft instead when another entry depends on it.
// A recipe with a Finalize row leaves the task with the status of the last running transition.
// It is serial because it sets the board skip variable, which is process-global.
func TestWiredCommitStatus_FinishedRunEntry(t *testing.T) {
	tiergate.Require(t, 1)
	t.Setenv("BOARD_SKIP_GIT", "1")
	hub := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})

	running := boardengine.RunStatus("running", "Discussion-Bouncer")
	wirings := []struct {
		name string
		// slugSuffix keeps the pairs of the two entry points apart on the shared hub.
		slugSuffix string
		wire       func(c *loomCLI, location *lymcwd.Location) error
	}{
		{"WireLightweight", "light", func(c *loomCLI, location *lymcwd.Location) error {
			return c.wireLightweight(location, location.AnchorPath())
		}},
		{"Wire", "full", func(c *loomCLI, location *lymcwd.Location) error {
			return c.wire(location, location.AnchorPath())
		}},
	}
	tests := []struct {
		name   string
		slug   string
		recipe string
		// wantKind and wantLabels are the entry's kind and labels after the finish, and wantStatus its status, "" for none.
		wantKind   string
		wantLabels []string
		wantStatus string
		// dependedOn makes another entry depend on the run's entry, so the board refuses to make it a Sketch.
		dependedOn bool
	}{
		{"FinalizeLessRunMakesTheEntryASketch", "motifrun", shedrun.RecipeMotif, boardengine.KindNote, []string{boardengine.SketchType}, "", false},
		{"FinalizingRunLeavesTheLastRunningStatus", "tapestryrun", shedrun.RecipeTapestry, boardengine.KindTask, []string{"bug"}, running, false},
		{"RefusedSketchLeavesTheEntryDraft", "dependedrun", shedrun.RecipeMotif, boardengine.KindTask, []string{"bug"}, boardengine.StatusDraft, true},
	}
	for _, w := range wirings {
		for _, tt := range tests {
			t.Run(w.name+"_"+tt.name, func(t *testing.T) {
				slug := tt.slug + w.slugSuffix
				hubforge.AddPair(t, hub, slug)
				location, err := lymcwd.ResolveWorktree(hub.PairCodeWorktree(slug))
				if err != nil {
					t.Fatalf("ResolveWorktree() error = %v; want nil", err)
				}
				board, err := boardengine.OpenHub(location.HubPath)
				if err != nil {
					t.Fatalf("OpenHub() error = %v; want nil", err)
				}
				if _, err := board.UpsertTask(map[string]any{"slug": slug, "title": "Finish", "kind": "task", "labels": []string{"bug"}}); err != nil {
					t.Fatalf("UpsertTask() error = %v; want nil", err)
				}
				if tt.dependedOn {
					if _, err := board.UpsertTask(map[string]any{"slug": slug + "-after", "title": "After", "kind": "task", "labels": []string{"bug"}, "depends_on": []string{slug}}); err != nil {
						t.Fatalf("UpsertTask() error = %v; want nil", err)
					}
				}
				if err := board.SetRunStatus(slug, running); err != nil {
					t.Fatalf("SetRunStatus() error = %v; want nil", err)
				}

				c := &loomCLI{recipe: tt.recipe, runID: shedrun.SelfRunID}
				if err := w.wire(c, location); err != nil {
					t.Fatalf("%s() error = %v; want nil", w.name, err)
				}
				// The board write precedes the seam's commit, so the commit's own outcome does not bear on what this test asserts.
				_ = c.shedPaths.CommitStatus("Discussion-Bouncer", "done")

				task, found, err := board.GetTask(slug)
				if err != nil || !found {
					t.Fatalf("GetTask(%q) = %v, %v; want the entry", slug, found, err)
				}
				if task.Kind != tt.wantKind || !slices.Equal(task.Labels, tt.wantLabels) {
					t.Errorf("board entry kind, labels = %q, %v; want %q, %v", task.Kind, task.Labels, tt.wantKind, tt.wantLabels)
				}
				status := ""
				if task.Status != nil {
					status = *task.Status
				}
				if status != tt.wantStatus {
					t.Errorf("board status = %q; want %q", status, tt.wantStatus)
				}
			})
		}
	}
}

// TestSketchFinish_TellsTheParentOnlyOfARefusedSketch drives a finished motif run's Finish against a real hub board.
// The finish posts one notice when the board refuses the Sketch, naming the dependent entry and both ways forward.
// It posts none when the board takes the Sketch.
// A notice that cannot be sent leaves the finish succeeding after the one attempt.
// It is serial because it sets the board skip variable, which is process-global.
func TestSketchFinish_TellsTheParentOnlyOfARefusedSketch(t *testing.T) {
	tiergate.Require(t, 1)
	t.Setenv("BOARD_SKIP_GIT", "1")
	hub := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})

	tests := []struct {
		name       string
		slug       string
		dependedOn bool
		wantNotice bool
		// notifyErr is what the notify seam returns after recording its line.
		notifyErr error
	}{
		{"ASketchTheBoardTakesPostsNothing", "takenrun", false, false, nil},
		{"ARefusedSketchPostsOneNotice", "refusedrun", true, true, nil},
		{"ANoticeThatCannotBeSentStillFinishesTheRun", "unsentrun", true, true, errors.New("creel is down")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hubforge.AddPair(t, hub, tt.slug)
			location, err := lymcwd.ResolveWorktree(hub.PairCodeWorktree(tt.slug))
			if err != nil {
				t.Fatalf("ResolveWorktree() error = %v; want nil", err)
			}
			board, err := boardengine.OpenHub(location.HubPath)
			if err != nil {
				t.Fatalf("OpenHub() error = %v; want nil", err)
			}
			if _, err := board.UpsertTask(map[string]any{"slug": tt.slug, "title": "Finish", "kind": "task", "labels": []string{"bug"}}); err != nil {
				t.Fatalf("UpsertTask() error = %v; want nil", err)
			}
			dependent := tt.slug + "-after"
			if tt.dependedOn {
				if _, err := board.UpsertTask(map[string]any{"slug": dependent, "title": "After", "kind": "task", "labels": []string{"bug"}, "depends_on": []string{tt.slug}}); err != nil {
					t.Fatalf("UpsertTask() error = %v; want nil", err)
				}
			}

			var notices []string
			finish := sketchFinish(location, shedrun.SelfRunID, func(line string) error {
				notices = append(notices, line)
				return tt.notifyErr
			})
			if err := finish(); err != nil {
				t.Fatalf("Finish() error = %v; want nil", err)
			}

			if !tt.wantNotice {
				if len(notices) != 0 {
					t.Errorf("notices = %q; want none", notices)
				}
				return
			}
			if len(notices) != 1 {
				t.Fatalf("notices = %q; want one", notices)
			}
			for _, want := range []string{"loom " + tt.slug + ":", dependent, "lym board set-deps", `lym board upsert '{"slug":"` + tt.slug + `","kind":"note","labels":["sketch"],"status":null`, `"recipe":"raddle"`, "lym board set-status"} {
				if !strings.Contains(notices[0], want) {
					t.Errorf("notice = %q; want it to name %q", notices[0], want)
				}
			}
		})
	}
}
