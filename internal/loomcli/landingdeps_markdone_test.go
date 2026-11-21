// landingdeps_markdone_test.go drives landingDeps' MarkTaskDone closure against a hub directory.
// It holds only the board config, so the config load, the hub-board path and the status write run for real.
// No git and no hub fixture are involved.
// Every test sets the board skip variables, which are process-global, so none calls t.Parallel.

package loomcli

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/landingshed"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/boardkit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// markDoneFixtureDir builds a hub directory whose board directory carries the board config.
// It returns landingDeps' MarkTaskDone closure for the pair named slug.
// It also returns the hub-board handle and the slug.
func markDoneFixtureDir(t *testing.T) (markDone func() error, board *boardengine.Board, slug string) {
	t.Helper()
	t.Setenv("BOARD_SKIP_GIT", "1")
	t.Setenv("BOARD_SKIP_PUSH", "1")

	slug = "markdone"
	location := &lymcwd.Location{HubPath: boardkit.HubWithBoardConfig(t), WorktreeName: slug, AnchorRel: "."}

	deps := landingDeps(location, shedrun.RecipeTapestry, websterengine.Geometry{}, "task", "main",
		true, modelspec.Registry{}, &shuttleengine.Runner{}, landingshed.Config{}, "", nil, nil, nil, planVerifySource(location), parentLockDeps(location, shedrun.SelfRunID, "", ""))

	board, err := boardengine.OpenHub(location.HubPath)
	if err != nil {
		t.Fatalf("OpenHub error = %v; want nil", err)
	}
	return deps.MarkTaskDone, board, slug
}

// TestLandingDeps_MarkTaskDone_SetsStatusDone asserts MarkTaskDone leaves the task done through the pushed write.
// Under the skip variables the write commits and pushes nothing, so the board file alone holds the status.
func TestLandingDeps_MarkTaskDone_SetsStatusDone(t *testing.T) {
	tiergate.Require(t, 0)
	markDone, board, slug := markDoneFixtureDir(t)

	if _, err := board.UpsertTask(map[string]any{"slug": slug, "title": "Mark done", "kind": "task", "labels": []string{"bug"}}); err != nil {
		t.Fatalf("UpsertTask error = %v; want nil", err)
	}
	if err := markDone(); err != nil {
		t.Fatalf("MarkTaskDone() error = %v; want nil", err)
	}

	task, found, err := board.GetTask(slug)
	if err != nil || !found {
		t.Fatalf("GetTask(%q) = found %v, err %v; want found", slug, found, err)
	}
	if task.Status == nil || *task.Status != "done" {
		t.Errorf("task status = %v; want done", task.Status)
	}
}

func TestLandingDeps_MarkTaskDone_UnknownSlugIsError(t *testing.T) {
	tiergate.Require(t, 0)
	markDone, _, _ := markDoneFixtureDir(t)

	if err := markDone(); err == nil {
		t.Error("MarkTaskDone() error = nil for a slug with no board task; want an error")
	}
}
