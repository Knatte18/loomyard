//go:build tier1

// parentrecordsanchor_integration_test.go covers ParentRecordsAnchor over a hubforge hub:
// the anchor of a task pair against the anchor of the parent records the reset would copy, read from the parent's records branch alone.
//
// Package fabricengine_test; shares the single TestMain in testmain_test.go.

package fabricengine_test

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
)

func TestParentRecordsAnchor(t *testing.T) {
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	const beam = "beam"
	hubforge.AddPairWith(t, h, beam, fabricengine.AddOptions{SkipPush: true})
	hubforge.AddPairWith(t, h, "before", fabricengine.AddOptions{SkipPush: true, Parent: beam})
	hubforge.ReanchorPair(t, h, beam, "sub")
	hubforge.AddPairWith(t, h, "after", fabricengine.AddOptions{SkipPush: true, Parent: beam})

	requireAnchors := func(t *testing.T, slug, wantOwn, wantParent string) {
		t.Helper()
		own, parent, err := fabricengine.ParentRecordsAnchor(h.Location, slug)
		if err != nil {
			t.Fatalf("ParentRecordsAnchor(%q) error = %v; want nil", slug, err)
		}
		if own != wantOwn || parent != wantParent {
			t.Errorf("ParentRecordsAnchor(%q) = (%q, %q); want (%q, %q)", slug, own, parent, wantOwn, wantParent)
		}
	}

	t.Run("A task forked before its beam was re-anchored sits at another anchor than its parent's records", func(t *testing.T) {
		requireAnchors(t, "before", ".", "sub")
	})

	t.Run("A task forked after the re-anchor sits at its parent's anchor", func(t *testing.T) {
		requireAnchors(t, "after", "sub", "sub")
	})

	t.Run("The parent's records branch alone answers once its worktrees are gone", func(t *testing.T) {
		gitkit.Git(t, h.PrimeRecords(), "worktree", "remove", "--force", h.PairRecordsSibling(beam))
		gitkit.Git(t, h.PrimeWorktree(), "worktree", "remove", "--force", h.PairCodeWorktree(beam))
		requireAnchors(t, "before", ".", "sub")
	})

	t.Run("A pair that is no task of a parent is refused with the way forward", func(t *testing.T) {
		if _, _, err := fabricengine.ParentRecordsAnchor(h.Location, "absent"); err == nil {
			t.Error("ParentRecordsAnchor(absent) error = nil; want the missing origin record refused")
		}
	})
}
