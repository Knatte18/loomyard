//go:build integration

// checkout_index_refresh_test.go proves Checkout refreshes the pair's
// per-worktree correspondence index for the newly-current branch. Pre-fix,
// entries recorded on the previous branch survived the switch and kept passing
// the SHAExists staleness check (their commits still exist on the other
// branch's refs), so WeftSHAForWarpSHA served weft SHAs the current branch's
// trailer history — the documented sole source of truth — would never produce,
// and RevertWithWeft against such an answer would graft the current branches
// onto the other branch's history.
//
// Package fabricengine_test to reuse buildDiffPair/currentBranchOf from
// lifecycle_differential_test.go; shares the TestMain in testmain_test.go.

package fabricengine_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lyxtest"
)

// TestCheckout_RefreshesCorrespondenceIndex records a correspondence on the
// primary pair's original branch, coordinated-switches to a pre-existing
// divergent branch pair whose weft history predates that record, and asserts
// the lookup now misses (ErrNoCorrespondence) instead of serving the other
// branch's weft SHA.
func TestCheckout_RefreshesCorrespondenceIndex(t *testing.T) {
	t.Parallel()

	dp := buildDiffPair(t, "")
	l := dp.FabricFixture.Layout
	top := dp.Fabric

	// Healthy junction so Checkout's wiring step succeeds.
	slug := filepath.Base(l.WorktreeRoot)
	if err := fabricengine.WireJunctions(l, slug); err != nil {
		t.Fatalf("setup WireJunctions: %v", err)
	}

	// Create the divergent target pair NOW — before the sync commit below — so
	// the target weft branch's history predates (and never contains) the
	// correspondence-carrying commit recorded on the original branch.
	const targetBranch = "index-refresh-target"
	lyxtest.MustRun(t, l.WorktreeRoot, "git", "branch", targetBranch)
	lyxtest.MustRun(t, l.WeftRepoRoot(), "git", "branch", fabricengine.WeftBranchName(targetBranch))

	f, err := fabricengine.New(l.WorktreeRoot, l.WeftWorktree())
	if err != nil {
		t.Fatalf("fabricengine.New: %v", err)
	}

	// Record one correspondence on the original branch via a real scoped commit.
	if err := os.WriteFile(filepath.Join(l.WeftWorktree(), "_lyx", "config.yaml"), []byte("index refresh probe"), 0o644); err != nil {
		t.Fatalf("write weft config: %v", err)
	}
	warpSHA, err := f.Warp.CurrentSHA()
	if err != nil {
		t.Fatalf("warp CurrentSHA: %v", err)
	}
	weftSHA, committed, err := f.CommitWeft([]string{"_lyx"}, fabricengine.DefaultCommitMessage, fabricengine.SyncOptions{})
	if err != nil || !committed {
		t.Fatalf("CommitWeft = (%q, %v, %v); want a real commit", weftSHA, committed, err)
	}

	// Sanity: the lookup answers while the recording branch is still current.
	if got, err := f.WeftSHAForWarpSHA(warpSHA); err != nil || got != weftSHA {
		t.Fatalf("pre-switch WeftSHAForWarpSHA(%s) = (%q, %v); want (%q, nil)", warpSHA, got, err, weftSHA)
	}

	// Coordinated switch to the divergent pair.
	if _, err := top.Checkout(l, targetBranch); err != nil {
		t.Fatalf("Checkout(%q): %v", targetBranch, err)
	}
	if got := currentBranchOf(t, l.WeftWorktree()); got != fabricengine.WeftBranchName(targetBranch) {
		t.Fatalf("weft branch after Checkout = %q; want %q", got, fabricengine.WeftBranchName(targetBranch))
	}

	// The refreshed index reflects only the now-current branch's trailer
	// history, which predates the recorded correspondence — the lookup must
	// miss instead of serving the other branch's weft SHA.
	got, err := f.WeftSHAForWarpSHA(warpSHA)
	if err == nil {
		t.Fatalf("post-switch WeftSHAForWarpSHA(%s) = (%q, nil); want ErrNoCorrespondence — a cross-branch answer served from the stale cache", warpSHA, got)
	}
	if !errors.Is(err, fabricengine.ErrNoCorrespondence) {
		t.Errorf("post-switch WeftSHAForWarpSHA(%s) error = %v; want ErrNoCorrespondence", warpSHA, err)
	}
}
