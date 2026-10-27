//go:build tier1

// checkout_anchor_integration_test.go proves Checkout refuses, before it switches anything, a target weft branch anchored elsewhere than the worktree.
// It refuses a worktree whose own anchor marker is empty too, and forks a target with no weft branch yet instead of refusing it.
// The target is made by re-anchoring a second pair with hubforge.ReanchorPair and then parking that pair, so its branches are free to check out.
//
// Package fabricengine_test; shares the single TestMain in testmain_test.go.

package fabricengine_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/lymdirs"
)

// newCheckoutHub builds a "." hub whose prime warp tracks sub/ before any pair is added, so sub is a tracked directory in every pair's warp.
func newCheckoutHub(t *testing.T) *hubforge.Hub {
	t.Helper()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	gitkit.CommitFile(t, h.PrimeWorktree(), "sub/.keep", "", "track sub")
	return h
}

// parkedPairAt adds the pair slug, moves it to anchor, and detaches both its worktrees so its branches are free to check out elsewhere.
func parkedPairAt(t *testing.T, h *hubforge.Hub, slug, anchor string) {
	t.Helper()

	hubforge.AddPair(t, h, slug)
	hubforge.ReanchorPair(t, h, slug, anchor)
	gitkit.Git(t, h.PairCodeWorktree(slug), "switch", "--detach")
	gitkit.Git(t, h.PairRecordsSibling(slug), "switch", "--detach")
}

// requireCheckoutLeftAlone fails the test unless the prime still sits on its branches with its junctions wired at its anchor and the checkout recorded nothing.
func requireCheckoutLeftAlone(t *testing.T, h *hubforge.Hub, res fabricengine.CheckoutResult, warpBranch, weftBranch string) {
	t.Helper()

	if got := gitkit.CurrentBranch(t, h.PrimeWorktree()); got != warpBranch {
		t.Errorf("warp branch = %q; want %q, nothing switched", got, warpBranch)
	}
	if got := gitkit.CurrentBranch(t, h.PrimeRecords()); got != weftBranch {
		t.Errorf("weft branch = %q; want %q, nothing switched", got, weftBranch)
	}
	if _, err := os.Lstat(filepath.Join(h.PrimeWorktree(), lymdirs.LymDirName)); err != nil {
		t.Errorf("the prime's junction went missing: %v", err)
	}
	if res.Mutated().Len() != 0 {
		t.Errorf("Checkout() mutations = %v; want none", res.Mutated().Entries())
	}
}

// TestCheckout_RefusesAnotherAnchor covers each way a checkout is refused for an anchor, and the fork that is not.
func TestCheckout_RefusesAnotherAnchor(t *testing.T) {
	t.Parallel()

	t.Run("A target re-anchored at a directory the warp tracks names the anchor change and its cost", func(t *testing.T) {
		t.Parallel()

		h := newCheckoutHub(t)
		parkedPairAt(t, h, "mover", "sub")
		warpBranch, weftBranch := gitkit.CurrentBranch(t, h.PrimeWorktree()), gitkit.CurrentBranch(t, h.PrimeRecords())

		res, err := h.Topology.Checkout(h.Location, "mover")

		if !errors.Is(err, fabricengine.ErrAnchorChange) {
			t.Fatalf("Checkout() error = %v; want it to wrap ErrAnchorChange", err)
		}
		for _, want := range []string{"lym fabric anchor --set sub", "shared by every machine that checks it out", "run the checkout again"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("Checkout() error = %q; want it to contain %q", err, want)
			}
		}
		if strings.Contains(err.Error(), "update this warp checkout") {
			t.Errorf("Checkout() error = %q; want no warp update, since sub is tracked here", err)
		}
		requireCheckoutLeftAlone(t, h, res, warpBranch, weftBranch)
	})

	t.Run("A target re-anchored at a directory only its warp branch tracks names the warp update first", func(t *testing.T) {
		t.Parallel()

		h := newCheckoutHub(t)
		parkedPairAt(t, h, "mover", "only-there")
		warpBranch, weftBranch := gitkit.CurrentBranch(t, h.PrimeWorktree()), gitkit.CurrentBranch(t, h.PrimeRecords())

		res, err := h.Topology.Checkout(h.Location, "mover")

		if !errors.Is(err, fabricengine.ErrAnchorChange) {
			t.Fatalf("Checkout() error = %v; want it to wrap ErrAnchorChange", err)
		}
		update, anchorChange := strings.Index(err.Error(), "first update this warp checkout"), strings.Index(err.Error(), "lym fabric anchor --set only-there")
		if update < 0 || anchorChange < 0 || update > anchorChange {
			t.Errorf("Checkout() error = %q; want the warp update named before `lym fabric anchor --set only-there`", err)
		}
		requireCheckoutLeftAlone(t, h, res, warpBranch, weftBranch)
	})

	t.Run("A target pushed from a second clone and never fetched is refused with no local weft branch created", func(t *testing.T) {
		t.Parallel()

		h := newCheckoutHub(t)
		second := hubforge.CloneSecond(t, h)
		hubforge.AddPair(t, second, "remote-mover")
		hubforge.ReanchorPair(t, second, "remote-mover", "sub")
		if _, err := fabricengine.CoalescePushBothAt(second.PairCodeWorktree("remote-mover"), second.PairRecordsSibling("remote-mover"), fabricengine.SyncOptions{}); err != nil {
			t.Fatalf("push the re-anchored pair from the second clone: %v", err)
		}
		warpBranch, weftBranch := gitkit.CurrentBranch(t, h.PrimeWorktree()), gitkit.CurrentBranch(t, h.PrimeRecords())

		res, err := h.Topology.Checkout(h.Location, "remote-mover")

		if !errors.Is(err, fabricengine.ErrAnchorChange) || !strings.Contains(err.Error(), "lym fabric anchor --set sub") {
			t.Fatalf("Checkout() error = %v; want an ErrAnchorChange refusal naming `lym fabric anchor --set sub`", err)
		}
		if gitkit.BranchExists(t, mustRecordsRepoRoot(t, h.Location), fabricengine.RecordsBranchName("remote-mover")) {
			t.Errorf("a local weft branch for the refused target exists; want none")
		}
		requireCheckoutLeftAlone(t, h, res, warpBranch, weftBranch)
	})

	t.Run("A checkout to a new branch forks its weft branch and succeeds", func(t *testing.T) {
		t.Parallel()

		h := newCheckoutHub(t)
		gitkit.Git(t, h.PrimeWorktree(), "branch", "fresh")

		if _, err := h.Topology.Checkout(h.Location, "fresh"); err != nil {
			t.Fatalf("Checkout() error = %v; want nil, a forked branch inherits the marker", err)
		}

		if got := gitkit.CurrentBranch(t, h.PrimeRecords()); got != fabricengine.RecordsBranchName("fresh") {
			t.Errorf("weft branch = %q; want the forked %q", got, fabricengine.RecordsBranchName("fresh"))
		}
	})

	t.Run("An empty own marker refuses naming the marker file", func(t *testing.T) {
		t.Parallel()

		h := newCheckoutHub(t)
		gitkit.Git(t, h.PrimeWorktree(), "branch", "fresh")
		warpBranch, weftBranch := gitkit.CurrentBranch(t, h.PrimeWorktree()), gitkit.CurrentBranch(t, h.PrimeRecords())
		writeFile(t, filepath.Join(h.PrimeRecords(), lymcwd.AnchorFileName), "\n")

		res, err := h.Topology.Checkout(h.Location, "fresh")

		if !errors.Is(err, fabricengine.ErrAnchorChange) || !strings.Contains(err.Error(), lymcwd.AnchorFileName) {
			t.Fatalf("Checkout() error = %v; want an ErrAnchorChange refusal naming %s", err, lymcwd.AnchorFileName)
		}
		requireCheckoutLeftAlone(t, h, res, warpBranch, weftBranch)
	})

	t.Run("A move record on the worktree's weft refuses a checkout to a branch at the same anchor", func(t *testing.T) {
		t.Parallel()

		h := newCheckoutHub(t)
		gitkit.Git(t, h.PrimeWorktree(), "branch", "fresh")
		warpBranch, weftBranch := gitkit.CurrentBranch(t, h.PrimeWorktree()), gitkit.CurrentBranch(t, h.PrimeRecords())
		writeMoveRecord(t, h.PrimeRecords(), fabricengine.MoveRecord{A: ".", B: "sub", Kind: fabricengine.MoveKindMove, PID: 4242})

		res, err := h.Topology.Checkout(h.Location, "fresh")

		if !errors.Is(err, fabricengine.ErrAnchorChange) || !strings.Contains(err.Error(), "lym fabric anchor") {
			t.Fatalf("Checkout() error = %v; want an ErrAnchorChange refusal naming `lym fabric anchor`", err)
		}
		requireCheckoutLeftAlone(t, h, res, warpBranch, weftBranch)
	})
}
