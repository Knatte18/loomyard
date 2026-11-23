// commitstatus_integration_test.go drives the commit-status seam that each wiring entry point installs.
// It runs against a real hub pair and its hub board, and pins the board entry a finished run leaves per recipe.

package loomcli

import (
	"slices"
	"testing"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestWiredCommitStatus_FinishedRunEntry drives the seam that wireLightweight and wire each install.
// The test takes a run through its last transition and asserts the board entry that transition leaves.
// A recipe with no Finalize row leaves the run-held task a Sketch with no status.
// It does so whether or not another entry depends on it.
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
		// dependedOn makes another entry depend on the run's entry.
		dependedOn bool
	}{
		{"FinalizeLessRunMakesTheEntryASketch", "motifrun", shedrun.RecipeMotif, boardengine.KindNote, []string{boardengine.SketchType}, "", false},
		{"FinalizingRunLeavesTheLastRunningStatus", "tapestryrun", shedrun.RecipeTapestry, boardengine.KindTask, []string{"bug"}, running, false},
		{"DependedOnEntryStillBecomesASketch", "dependedrun", shedrun.RecipeMotif, boardengine.KindNote, []string{boardengine.SketchType}, "", true},
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

// TestCommitStatusDeps_SkipsTheBoardStatusAfterAFilingMark drives the board status write of commitStatusDeps.
// It runs against a real hub board.
// A recipe with a Raddle-File row writes nothing once the run's filing mark stands in the prime.
// A recipe with no such row writes whether or not a mark stands.
// It is serial because it sets the board skip variable, which is process-global.
func TestCommitStatusDeps_SkipsTheBoardStatusAfterAFilingMark(t *testing.T) {
	tiergate.Require(t, 1)
	t.Setenv("BOARD_SKIP_GIT", "1")
	hub := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})

	before := boardengine.RunStatus("running", "Raddle-Write")
	written := boardengine.RunStatus("running", "Raddle-File")
	tests := []struct {
		name       string
		slug       string
		recipe     string
		marked     bool
		wantStatus string
	}{
		{"ARaddleFileRecipeWithoutTheMarkWrites", "arrasopen", shedrun.RecipeArras, false, written},
		{"ARaddleFileRecipeWithTheMarkWritesNothing", "arrasmarked", shedrun.RecipeArras, true, before},
		{"ARecipeWithNoRaddleFileRowWritesDespiteTheMark", "tapestrymarked", shedrun.RecipeTapestry, true, written},
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
			if _, err := board.UpsertTask(map[string]any{"slug": tt.slug, "title": "Run", "kind": "task", "labels": []string{"bug"}}); err != nil {
				t.Fatalf("UpsertTask() error = %v; want nil", err)
			}
			if err := board.SetRunStatus(tt.slug, before); err != nil {
				t.Fatalf("SetRunStatus() error = %v; want nil", err)
			}
			if tt.marked {
				prime, err := primeOfHub(location)
				if err != nil {
					t.Fatalf("primeOfHub() error = %v; want nil", err)
				}
				if err := shedrun.WriteFilingMark(prime, tt.slug); err != nil {
					t.Fatalf("WriteFilingMark() error = %v; want nil", err)
				}
			}

			c := &loomCLI{recipe: tt.recipe, runID: shedrun.SelfRunID}
			if err := c.commitStatusDeps(location).SetBoardStatus(written); err != nil {
				t.Fatalf("SetBoardStatus() error = %v; want nil", err)
			}

			task, found, err := board.GetTask(tt.slug)
			if err != nil || !found || task.Status == nil || *task.Status != tt.wantStatus {
				t.Errorf("board entry = %+v, found %t, error %v; want status %q", task, found, err, tt.wantStatus)
			}
		})
	}
}
