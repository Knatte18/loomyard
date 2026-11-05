// commitstatus_integration_test.go drives the commit-status seam each wiring entry point installs against a real hub pair and its hub board, to pin the finished status it writes per recipe.

package loomcli

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestWiredCommitStatus_WritesFinishedStatus drives the seam each of wireLightweight and wire installs through a run's last transition and asserts the board status it leaves:
// a recipe with no Finalize row marks the entry discussed, and one with a Finalize row leaves the status of the last running transition.
// It is serial because it sets the board skip variable, which is process-global.
func TestWiredCommitStatus_WritesFinishedStatus(t *testing.T) {
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
		want   string
	}{
		{"FinalizeLessRunMarksTheEntryDiscussed", "motifrun", shedrun.RecipeMotif, boardengine.StatusDiscussed},
		{"FinalizingRunLeavesTheLastRunningStatus", "tapestryrun", shedrun.RecipeTapestry, running},
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
				if task.Status == nil || *task.Status != tt.want {
					t.Errorf("board status = %v; want %q", task.Status, tt.want)
				}
			})
		}
	}
}
