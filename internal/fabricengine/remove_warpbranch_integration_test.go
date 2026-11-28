// remove_warpbranch_integration_test.go covers Remove's local warp-branch deletion.
// A pair's task branch goes when the destructive gate proves its work is pushed or landed on the parent.
// The parent is the one recorded in the pair's origin record.
// Otherwise the branch is kept with a reason.
//
// Every hub is built through hubforge.NewHub with an empty branch_prefix, so the pair's warp branch is the bare slug.
//
// Package fabricengine_test; shares the single TestMain in testmain_test.go.

package fabricengine_test

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// warpBranchCommit commits a new file in the pair's warp worktree.
func warpBranchCommit(t *testing.T, l *lymcwd.Location, slug, name string) {
	t.Helper()
	dir := fabricengine.WorktreePath(l, slug)
	gitkit.CommitFile(t, dir, name, name, name)
}

//testtiming:keep a local task branch carrying unlanded commits surviving Remove, with and without force; coverage of its blocks by other tests does not show an assertion of this
func TestRemove_UnlandedWarpBranchIsKept(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	for _, force := range []bool{false, true} {
		name := "no-force"
		if force {
			name = "force"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			slug := "wb-unlanded-" + name
			h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
			l := h.Location
			topology := h.Topology
			if _, err := topology.Add(l, slug, fabricengine.AddOptions{}); err != nil {
				t.Fatalf("setup Add(%q): %v", slug, err)
			}
			warpBranchCommit(t, l, slug, "work.txt")

			res, err := topology.Remove(l, slug, force, false)
			if err != nil {
				t.Fatalf("Remove(%q, force=%v) error = %v; a kept branch is not a failure", slug, force, err)
			}
			if res.CodeBranchDeleted {
				t.Errorf("CodeBranchDeleted = true; want the unlanded branch kept")
			}
			if res.CodeBranchKeptReason == "" {
				t.Errorf("CodeBranchKeptReason is empty; want the gate's reason")
			}
			if !gitkit.BranchExists(t, l.WorktreePath(), slug) {
				t.Errorf("warp branch %q was deleted despite unlanded work", slug)
			}
		})
	}
}
