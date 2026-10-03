package battencli

import (
	"slices"
	"testing"

	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/testkit/envkit"
)

// intentionallyNil maps each shedrecipe.Env field path wire leaves nil to the reason.
// envkit.NilSeams already skips the seams whose nil is a documented default, so only wire's own gaps are listed.
// batten wires only its own producers, so every loom seam of shedrecipe.Env stays nil here.
var intentionallyNil = map[string]string{
	"Shuttle":                  "loom-only seam, batten drives no agent itself",
	"Burler":                   "loom-only seam, batten runs no review round",
	"WebsterRun":               "loom-only seam, batten runs no webster",
	"WebsterDeps.Starter":      "loom-only seam, batten runs no webster",
	"WebsterDeps.Reed":         "loom-only seam, batten runs no webster",
	"WebsterDeps.Engine":       "loom-only seam, batten runs no webster",
	"WebsterDeps.RefMatcher":   "loom-only seam, batten runs no webster",
	"CommitWebster":            "loom-only seam, batten runs no webster",
	"Landing.PushBranch":       "loom-only seam, batten lands no branch",
	"Landing.OpenFabric":       "loom-only seam, batten lands no branch",
	"Landing.OpenParentFabric": "loom-only seam, batten lands no branch",
	"Landing.TaskHead":         "loom-only seam, batten lands no branch",
	"Landing.Shuttle":          "loom-only seam, batten lands no branch",
	"DiscussionSpec":           "loom-only seam, batten has no discussion segment",
	"CommitDiscussion":         "loom-only seam, batten has no discussion segment",
	"DescribeSpec":             "loom-only seam, batten has no describe segment",
	"CommitDescription":        "loom-only seam, batten has no describe segment",
	"PlanSpec":                 "loom-only seam, batten has no plan segment",
	"CommitPlan":               "loom-only seam, batten has no plan segment",
	"ApprovePlan":              "loom-only seam, batten has no plan segment",
	"SkipPlanReview":           "loom-only seam, batten has no plan segment",
	"ReflectFriction":          "loom-only seam, batten has no reflect segment",
	"ReworkSpec":               "loom-only seam, batten has no rework segment",
	"Rework.ReadCommitted":     "loom-only seam, batten has no rework segment",
	"Rework.ReadRejection":     "loom-only seam, batten has no rework segment",
	"Rework.ClearRejection":    "loom-only seam, batten has no rework segment",
	"Rework.ArchiveWebster":    "loom-only seam, batten has no rework segment",
	"Rework.Commit":            "loom-only seam, batten has no rework segment",
}

func TestWire_EverySeamFilled(t *testing.T) {
	t.Parallel()

	location := &lyxcwd.Location{
		RepoName:     "example",
		HubPath:      t.TempDir(),
		WorktreeName: "hub-repo",
		AnchorRel:    ".",
	}

	c := &battenCLI{}
	if err := c.wire(location, "a-slug-with-no-worktree-anywhere"); err != nil {
		t.Fatalf("wire() error = %v; want nil", err)
	}

	nils := envkit.NilSeams(c.env)
	for _, path := range nils {
		if _, ok := intentionallyNil[path]; !ok {
			t.Errorf("c.env.%s is nil after wire(); fill it or list it in intentionallyNil with a reason", path)
		}
	}
	for path := range intentionallyNil {
		if !slices.Contains(nils, path) {
			t.Errorf("intentionallyNil lists c.env.%s but wire() now fills it; remove the entry", path)
		}
	}
}
