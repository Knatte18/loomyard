// originreads_integration_test.go covers the reads of a run's state from origin's fetched branches over hubforge hubs.
//
// Package fabricengine_test; shares the single TestMain in testmain_test.go.

package fabricengine_test

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestOriginRunReads pins the three reads from origin's fetched branches:
// the parent-side merge, a child run record with no pair on this machine, and a task branch, none making a network call.
func TestOriginRunReads(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	t.Run("ParentHoldsTaskWorkReadsOriginsFetchedTipNotTheLocalBranch", func(t *testing.T) {
		t.Parallel()
		const slug = "or-parent"
		h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
		hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{})
		warpBranchCommit(t, h.Location, slug, "work.txt")
		prime := h.PrimeWorktree()
		gitkit.MustRun(t, prime, "git", "merge", "--no-edit", slug)
		pair, err := lymcwd.NamedWorktree(h.Path, slug)
		if err != nil {
			t.Fatalf("NamedWorktree(%q): %v", slug, err)
		}

		localHolds, err := fabricengine.ParentHoldsTaskWork(h.Location, pair, slug)
		if err != nil || !localHolds {
			t.Fatalf("ParentHoldsTaskWork() = %v, %v; want true, nil", localHolds, err)
		}
		originHolds, err := fabricengine.OriginParentHoldsTaskWork(h.Location, pair, slug)
		if err != nil || originHolds {
			t.Errorf("OriginParentHoldsTaskWork() before the parent push = %v, %v; want false, nil", originHolds, err)
		}

		gitkit.MustRun(t, prime, "git", "push", "origin", "HEAD")
		originHolds, err = fabricengine.OriginParentHoldsTaskWork(h.Location, pair, slug)
		if err != nil || !originHolds {
			t.Errorf("OriginParentHoldsTaskWork() after the parent push = %v, %v; want true, nil", originHolds, err)
		}
	})

	t.Run("RunStatusIsReadFromTheRecordsBranchWithNoPairOnThisMachine", func(t *testing.T) {
		t.Parallel()
		const slug = "or-status"
		h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
		hubforge.AddPair(t, h, slug)
		weft := h.PairRecordsSibling(slug)
		gitkit.CommitFile(t, weft, "_lym/shed/"+slug+"/status.json", `{"state":"paused","future_key":1}`, "child run status")
		gitkit.MustRun(t, weft, "git", "push", "origin", gitkit.CurrentBranch(t, weft))
		if _, err := h.Topology.Remove(h.Location, slug, false, false); err != nil {
			t.Fatalf("Remove(%q): %v", slug, err)
		}

		status, found, err := fabricengine.OriginRunStatus(h.Location, slug)
		if err != nil || !found || status.State != shedengine.StatePaused {
			t.Errorf("OriginRunStatus(%q) = state %q, found %v, %v; want paused, true, nil", slug, status.State, found, err)
		}
		if _, found, err := fabricengine.OriginRunStatus(h.Location, "or-never-pushed"); err != nil || found {
			t.Errorf("OriginRunStatus(never pushed) = found %v, %v; want false, nil", found, err)
		}
	})

	t.Run("FetchedTaskBranchReadsTheTrackingRefWithNoNetworkCall", func(t *testing.T) {
		t.Parallel()
		const slug = "or-branch"
		h := pushedPair(t, slug)
		prime := h.Location

		if onOrigin, err := fabricengine.FetchedTaskBranchOnOrigin(prime, slug); err != nil || !onOrigin {
			t.Errorf("FetchedTaskBranchOnOrigin(pushed) = %v, %v; want true, nil", onOrigin, err)
		}
		if onOrigin, err := fabricengine.FetchedTaskBranchOnOrigin(prime, "or-never-pushed"); err != nil || onOrigin {
			t.Errorf("FetchedTaskBranchOnOrigin(never pushed) = %v, %v; want false, nil", onOrigin, err)
		}

		gitkit.MustRun(t, h.PrimeWorktree(), "git", "remote", "set-url", "origin", t.TempDir()+"/no-such-origin")
		if onOrigin, err := fabricengine.FetchedTaskBranchOnOrigin(prime, slug); err != nil || !onOrigin {
			t.Errorf("FetchedTaskBranchOnOrigin(origin unreachable) = %v, %v; want true, nil", onOrigin, err)
		}
	})
}
