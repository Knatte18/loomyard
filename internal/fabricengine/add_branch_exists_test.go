// add_branch_exists_test.go proves that Add's branch-already-exists rejection names a way forward.
// Remove leaves the warp branch behind, since it may carry unmerged work;
// so the everyday remove-then-re-add cycle hits this rejection.
// A bare "already exists" would leave the operator stuck without out-of-band git knowledge.
//
// Package fabricengine_test; shares the TestMain in testmain_test.go.

package fabricengine_test

import (
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestAdd_ExistingBranchErrorNamesRemedy creates the warp branch a slug would claim and calls Add with that slug.
// It asserts the rejection names all three remedies: checkout onto it, delete the leftover, or adopt it with --adopt-local.
func TestAdd_ExistingBranchErrorNamesRemedy(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	l := h.Location
	topology := h.Topology

	const slug = "leftover-pair"
	gitkit.MustRun(t, l.WorktreePath(), "git", "branch", slug)

	_, err := topology.Add(l, slug, fabricengine.AddOptions{SkipGit: true})
	if err == nil {
		t.Fatalf("Add(%q) error = nil; want branch-already-exists rejection", slug)
	}
	for _, want := range []string{"already exists", "lym fabric checkout " + slug, "git branch -D " + slug, "lym fabric add <slug> --adopt-local"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("Add(%q) error = %q; want substring %q", slug, err.Error(), want)
		}
	}
}
