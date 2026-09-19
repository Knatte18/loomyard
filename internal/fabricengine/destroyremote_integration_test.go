//go:build integration

// destroyremote_integration_test.go pins the remote-branch executor's own request shape via two
// direct-call cases that need a real hub and therefore a real git spawn: an unset ownership/dirtiness
// declaration and the naming-predicate refusal are hermetic and covered in destroy_test.go instead,
// per the Test Tier Purity Invariant.
//
// Each case drives checkRemoteBranchRequest through CheckRemoteBranchRequestForTest
// (export_test.go), never through deleteRemoteBranch itself — no test here asserts a ref was
// actually deleted on a remote. Both are refusals the real call sites cannot reach: those run only
// after the same branch's local `git branch -D` already succeeded, at which point the branch is
// already gone locally, so its own dirtiness probe never sees it.
//
// Package fabricengine_test: building either case needs a real hub via hubforge.NewHub, but an
// internal fabricengine test file cannot import internal/hubforge without closing an import cycle
// (hubforge -> fabriccli -> fabricengine). Shares the single TestMain in testmain_test.go.

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
// worktree is refused with CheckOwnership (not CheckDirtiness), reaching listWeftBranches from
// inside resolveManagedBranch — a real git spawn only an integration-tier test can exercise.
// resolveManagedBranch's own checked-out-at-a-worktree check runs before checkBranchDirtiness's
// duplicate check is ever reached, so the latter is unreachable dead code on an
// ownedManagedBranch-typed request.
//
// This is also a refusal the real call sites cannot reach: they run only after the same branch's
// local `git branch -D` already succeeded, at which point the branch is already gone locally and so
// is no longer checked out anywhere.
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
