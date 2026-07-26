//go:build integration

// checkout_rollback_test.go proves Checkout's all-or-nothing contract holds even
// when junction wiring (step 5) fails AFTER the weft branch was already switched
// (step 4). Pre-fix, the rollback reverted only the host, stranding the weft on
// the new branch — a half-switched pair, the exact state Checkout promises never
// to produce (a live review round reproduced this by making the host _lyx a real
// directory so seedLyxJunction refuses). Checkout now rolls back BOTH sides.
//
// Package fabricengine_test to reuse newFabricFixture/currentBranchOf/
// branchExistsAt from reconcile_stale_registration_test.go; shares the single
// TestMain in testmain_test.go.

package fabricengine_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lyxtest"
)

// TestCheckout_JunctionFailureRollsBackBothSides wires a healthy primary pair,
// then corrupts the host _lyx into a real (non-junction) directory so the
// junction-wiring step of Checkout fails after the weft has already switched. It
// asserts Checkout errors AND leaves both the host and the weft on their original
// branches — never a half-switched pair.
func TestCheckout_JunctionFailureRollsBackBothSides(t *testing.T) {
	t.Parallel()

	fixture := newFabricFixture(t)
	l := fixture.Layout
	top := fabricengine.NewTopology(fabricengine.Config{})

	const targetBranch = "checkout-rollback-target"

	// Establish the healthy primary junction and the target branch on both sides
	// so Checkout's steps 3 (host switch) and 4 (weft switch) can succeed and the
	// failure is isolated to step 5 (junction wiring).
	// Checkout derives the slug the same way (filepath.Base of the worktree root).
	slug := filepath.Base(l.WorktreeRoot)
	if err := fabricengine.WireJunctions(l, slug); err != nil {
		t.Fatalf("setup WireJunctions: %v", err)
	}
	lyxtest.MustRun(t, l.WorktreeRoot, "git", "branch", targetBranch)
	lyxtest.MustRun(t, l.WeftRepoRoot(), "git", "branch", fabricengine.WeftBranchName(targetBranch))

	originalHostBranch := currentBranchOf(t, l.WorktreeRoot)
	originalWeftBranch := currentBranchOf(t, l.WeftWorktree())

	// Corrupt the host _lyx into a real directory: WireJunctions -> seedLyxJunction
	// refuses a real (non-link) _lyx, so Checkout's step 5 fails after step 4 has
	// already moved the weft.
	hostLyx := l.HostLyxLinkHere()
	if err := os.Remove(hostLyx); err != nil {
		t.Fatalf("remove host junction to corrupt it: %v", err)
	}
	if err := os.MkdirAll(hostLyx, 0o755); err != nil {
		t.Fatalf("create real _lyx dir: %v", err)
	}

	res, err := top.Checkout(l, targetBranch)
	if err == nil {
		t.Fatalf("Checkout(%q) error = nil; want a junction-wiring failure (res=%+v)", targetBranch, res)
	}

	// The all-or-nothing contract: both sides restored to their originals, never
	// a half-switched pair (host rolled back but weft stranded on the new branch).
	if got := currentBranchOf(t, l.WorktreeRoot); got != originalHostBranch {
		t.Errorf("host branch after failed Checkout = %q; want %q (original)", got, originalHostBranch)
	}
	if got := currentBranchOf(t, l.WeftWorktree()); got != originalWeftBranch {
		t.Errorf("weft branch after failed Checkout = %q; want %q (original) — half-switched pair", got, originalWeftBranch)
	}

	// The target weft branch pre-existed this Checkout (adopted, not forked), so
	// the rollback must NOT have deleted it.
	if !branchExistsAt(t, l.WeftRepoRoot(), fabricengine.WeftBranchName(targetBranch)) {
		t.Errorf("pre-existing weft branch %q deleted by rollback; want it untouched", fabricengine.WeftBranchName(targetBranch))
	}
}

// TestCheckout_JunctionFailureDeletesForkedWeftBranch is the fork-path sibling
// of the rollback test above: the target has no weft branch yet, so Checkout's
// step 4 forks one; the junction-wiring failure at step 5 must then roll both
// sides back AND delete the branch this very Checkout forked — otherwise every
// rolled-back fork-checkout strands an orphan weft branch that cleanup later
// flags, created by fabric's own failed operation.
func TestCheckout_JunctionFailureDeletesForkedWeftBranch(t *testing.T) {
	t.Parallel()

	fixture := newFabricFixture(t)
	l := fixture.Layout
	top := fabricengine.NewTopology(fabricengine.Config{})

	const targetBranch = "checkout-rollback-forked"

	slug := filepath.Base(l.WorktreeRoot)
	if err := fabricengine.WireJunctions(l, slug); err != nil {
		t.Fatalf("setup WireJunctions: %v", err)
	}
	// Only the host branch exists: the weft side must be forked by Checkout.
	lyxtest.MustRun(t, l.WorktreeRoot, "git", "branch", targetBranch)

	originalHostBranch := currentBranchOf(t, l.WorktreeRoot)
	originalWeftBranch := currentBranchOf(t, l.WeftWorktree())

	// Corrupt the host _lyx into a real directory so step 5 fails after the fork.
	hostLyx := l.HostLyxLinkHere()
	if err := os.Remove(hostLyx); err != nil {
		t.Fatalf("remove host junction to corrupt it: %v", err)
	}
	if err := os.MkdirAll(hostLyx, 0o755); err != nil {
		t.Fatalf("create real _lyx dir: %v", err)
	}

	res, err := top.Checkout(l, targetBranch)
	if err == nil {
		t.Fatalf("Checkout(%q) error = nil; want a junction-wiring failure (res=%+v)", targetBranch, res)
	}

	if got := currentBranchOf(t, l.WorktreeRoot); got != originalHostBranch {
		t.Errorf("host branch after failed Checkout = %q; want %q (original)", got, originalHostBranch)
	}
	if got := currentBranchOf(t, l.WeftWorktree()); got != originalWeftBranch {
		t.Errorf("weft branch after failed Checkout = %q; want %q (original) — half-switched pair", got, originalWeftBranch)
	}

	// The branch step 4 forked must be gone: the rolled-back Checkout tears down
	// exactly what it created.
	forked := fabricengine.WeftBranchName(targetBranch)
	if branchExistsAt(t, l.WeftRepoRoot(), forked) {
		t.Errorf("forked weft branch %q survived the rollback; want it deleted (orphan branch stranded by fabric's own failed operation)", forked)
	}
}
