//go:build integration

// destroyremote_integration_test.go pins the remote-branch executor's own request shape via two
// direct-call cases that need a real hub and therefore a real git spawn: an unset ownership/dirtiness
// declaration and the naming-predicate refusal are hermetic and covered in destroy_test.go instead,
// per the Test Tier Purity Invariant.
//
// These are direct-call tests: each drives checkRemoteBranchRequest through
// CheckRemoteBranchRequestForTest (export_test.go), never through deleteRemoteBranch itself — no
// test in this file asserts that a remote ref was actually deleted, since that is batch 3's
// behaviour-test surface over a real hub with a real remote. What these pin is the executor's own
// request shape: that it cannot be reached with an undeclared predicate, and that a future call site
// arriving without a preceding local deletion is still gated.
//
// Neither case is an end-to-end guarantee, and must not be read as one: the remote executor's two
// real call sites (batch 3) run only after the local `git branch -D` for the same branch already
// succeeded, so by the time either real call site's dirtiness probe would read the branch, it is
// already gone from the local repo, and the probe therefore cannot refuse there. Both cases below
// exercise a refusal the real call sites cannot reach.
//
// Package fabricengine_test, not package fabricengine: building either case needs a real hub, which
// the hubforge Fabric-Fixture Invariant requires be built through hubforge.NewHub, but an internal
// (unsuffixed package fabricengine) test file cannot import internal/hubforge at all — hubforge
// imports fabriccli, which imports fabricengine, closing an import cycle for Go's internal test
// augmentation. Shares the single TestMain in testmain_test.go.

package fabricengine_test

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
)

// TestDeleteRemoteBranchGate_PrimaryWeftBranchRefused proves the repo's primary weft branch is
// refused with CheckOwnership, reaching primaryWeftBranch — a real git spawn only an integration-tier
// test can exercise. This is a refusal the real call sites cannot reach: neither real call site ever
// names the primary weft branch, since Cleanup itself never enumerates it as an orphan.
func TestDeleteRemoteBranchGate_PrimaryWeftBranchRefused(t *testing.T) {
	t.Parallel()

	h := hubforge.NewHub(t, ".")
	weftRoot, err := fabricengine.WeftRepoRoot(h.Location)
	if err != nil {
		t.Fatalf("WeftRepoRoot: %v", err)
	}
	primary := fabricengine.WeftBranchName("main")

	err = fabricengine.CheckRemoteBranchRequestForTest(h.Location, weftRoot, "origin", primary, "")
	if !RefusedByGate(err, fabricengine.CheckOwnership) {
		t.Fatalf("CheckRemoteBranchRequestForTest(%q) = %v; want a CheckOwnership refusal", primary, err)
	}
}

// TestDeleteRemoteBranchGate_CheckedOutBranchRefused proves a weft branch still checked out at a
// worktree is refused with CheckOwnership, reaching listWeftBranches from inside resolveManagedBranch
// — a real git spawn only an integration-tier test can exercise.
//
// This is CheckOwnership, not CheckDirtiness: resolveManagedBranch's own checked-out-at-a-worktree
// test (reused unchanged from the local branchRequest gate) runs before checkBranchDirtiness's own
// duplicate test is ever reached, so the latter is unreachable dead code on every
// ownedManagedBranch-typed request — true of the existing local branchRequest gate today as well, not
// something this batch introduces. The assertion this case pins is still "reaching
// listWeftBranches" — both the ownership and dirtiness steps call it — only the reported Check value
// differs from what an earlier draft of this test assumed.
//
// This is also a refusal the real call sites cannot reach: the remote executor's real call sites run
// only after the same branch's local `git branch -D` already succeeded, at which point the branch has
// already been deleted from the weft repo (and so is no longer checked out anywhere, nor even
// enumerable) by the time the remote deletion runs.
func TestDeleteRemoteBranchGate_CheckedOutBranchRefused(t *testing.T) {
	t.Parallel()

	const slug = "remoteslug"
	h := hubforge.NewHub(t, ".")
	hubforge.AddPair(t, h, slug)

	weftRoot, err := fabricengine.WeftRepoRoot(h.Location)
	if err != nil {
		t.Fatalf("WeftRepoRoot: %v", err)
	}
	checkedOut := fabricengine.WeftBranchName(slug)

	err = fabricengine.CheckRemoteBranchRequestForTest(h.Location, weftRoot, "origin", checkedOut, "")
	if !RefusedByGate(err, fabricengine.CheckOwnership) {
		t.Fatalf("CheckRemoteBranchRequestForTest(%q) = %v; want a CheckOwnership refusal", checkedOut, err)
	}
}
