// commitstatus_integration_test.go drives the commit-status seam each wiring entry point installs against a real hub pair and its hub board, to pin the board entry a finished run leaves per recipe.

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

// TestWiredCommitStatus_FinishedRunEntry drives the seam each of wireLightweight and wire installs through a run's last transition and asserts the board entry it leaves:
// a recipe with no Finalize row makes the run-held task a Sketch with no status, unless another entry depends on it, which leaves the task draft;
// one with a Finalize row leaves the task with the status of the last running transition.
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
