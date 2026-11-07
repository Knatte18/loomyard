// destroyremote_integration_test.go pins the remote-branch gate and archived weft branch deletion against a real hub.
// The refusals that need no git spawn are in destroy_test.go.
// It is in package fabricengine_test because hubforge imports fabriccli, which imports fabricengine.

package fabricengine_test

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestDeleteRemoteBranchGate_PrimaryWeftBranchRefused proves the primary weft branch is refused with `CheckOwnership`.
// The check reaches `primaryWeftBranch`, which is a real git spawn that only an integration-tier test can exercise.
// The real call sites cannot reach this refusal.
// They never name the primary weft branch, since `Cleanup` never enumerates it as an orphan.
//
//testtiming:keep the remote-branch gate refusing the primary weft branch on ownership, a refusal the real call sites cannot reach; coverage of its blocks by other tests does not show an assertion of this
func TestDeleteRemoteBranchGate_PrimaryWeftBranchRefused(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	weftRoot, err := fabricengine.RecordsRepoRoot(h.Location)
	if err != nil {
		t.Fatalf("RecordsRepoRoot: %v", err)
	}
	primary := fabricengine.RecordsBranchName("main")

	err = fabricengine.CheckRemoteBranchRequestForTest(h.Location, weftRoot, "origin", primary)
	if !RefusedByGate(err, fabricengine.CheckOwnership) {
		t.Fatalf("CheckRemoteBranchRequestForTest(%q) = %v; want a CheckOwnership refusal", primary, err)
	}
}

// TestDeleteRemoteBranchGate_CheckedOutBranchRefused proves a weft branch still checked out at a worktree is refused.
// The refusal is `CheckOwnership`, not `CheckDirtiness`.
// The check reaches `listWeftBranches` from inside `resolveManagedBranch`.
// That is a real git spawn, so only an integration-tier test can exercise it.
// `resolveManagedBranch` checks for a worktree checkout before `checkBranchDirtiness` runs its own duplicate check,
// so that duplicate check is unreachable for an `ownedManagedBranch`-typed request.
//
// The real call sites cannot reach this refusal.
// They run only after the branch's local `git branch -D` has succeeded.
// By then the branch is gone locally and checked out nowhere.
//
//testtiming:keep the remote-branch gate refusing a checked-out weft branch on ownership rather than dirtiness, a refusal the real call sites cannot reach; coverage of its blocks by other tests does not show an assertion of this
func TestDeleteRemoteBranchGate_CheckedOutBranchRefused(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	const slug = "remoteslug"
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	hubforge.AddPair(t, h, slug)

	weftRoot, err := fabricengine.RecordsRepoRoot(h.Location)
	if err != nil {
		t.Fatalf("RecordsRepoRoot: %v", err)
	}
	checkedOut := fabricengine.RecordsBranchName(slug)

	err = fabricengine.CheckRemoteBranchRequestForTest(h.Location, weftRoot, "origin", checkedOut)
	if !RefusedByGate(err, fabricengine.CheckOwnership) {
		t.Fatalf("CheckRemoteBranchRequestForTest(%q) = %v; want a CheckOwnership refusal", checkedOut, err)
	}
}

// archivedDeleteFixture builds a hub with a pushed pair.
// It returns the hub, the weft repo root, the pair's weft branch, and the branch's tip on origin.
func archivedDeleteFixture(t *testing.T, slug string) (*hubforge.Hub, string, string, string) {
	t.Helper()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	hubforge.AddPair(t, h, slug)
	weftRoot, err := fabricengine.RecordsRepoRoot(h.Location)
	if err != nil {
		t.Fatalf("RecordsRepoRoot: %v", err)
	}
	branch := fabricengine.RecordsBranchName(slug)
	tip := gitkit.RevParse(t, h.RecordsBare, branch)
	if tip == "" {
		t.Fatalf("weft branch %s is not on origin after Add", branch)
	}
	return h, weftRoot, branch, tip
}

// TestDeleteArchivedWeftBranch_CheckedOutBranchDeleted proves the pair's own weft branch is deleted from origin.
// The branch is still checked out at its weft worktree,
// and the deletion runs under a valid tag and a lease at the tip.
// It records one entry.
//
//testtiming:keep the pair's own checked-out weft branch being deleted from origin under a valid tag and a tip lease with one recorded entry; coverage of its blocks by other tests does not show an assertion of this
func TestDeleteArchivedWeftBranch_CheckedOutBranchDeleted(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	const slug = "archdel"
	h, weftRoot, branch, tip := archivedDeleteFixture(t, slug)

	rec, err := fabricengine.DeleteArchivedWeftBranchForTest(h.Location, weftRoot, slug, branch, "archive/"+slug+"/"+tip[:12], tip)
	if err != nil {
		t.Fatalf("DeleteArchivedWeftBranchForTest error = %v; want nil", err)
	}
	if gitkit.BranchExists(t, h.RecordsBare, branch) {
		t.Errorf("origin still has %s at %s; want it deleted", branch, gitkit.RevParse(t, h.RecordsBare, branch))
	}
	entries := rec.Entries()
	if len(entries) != 1 || entries[0].Kind != fabricengine.KindRemoteBranchDeleted {
		t.Errorf("record = %+v; want exactly one %s entry", entries, fabricengine.KindRemoteBranchDeleted)
	}
}

// TestDeleteArchivedWeftBranch_PrimaryRefused proves the primary weft branch is refused on ownership.
func TestDeleteArchivedWeftBranch_PrimaryRefused(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h, weftRoot, _, _ := archivedDeleteFixture(t, "archprimary")
	primary := fabricengine.RecordsBranchName("main")
	tip := gitkit.RevParse(t, weftRoot, primary)
	onOriginBefore := gitkit.BranchExists(t, h.RecordsBare, primary)

	_, err := fabricengine.DeleteArchivedWeftBranchForTest(h.Location, weftRoot, "main", primary, "archive/main/x", tip)
	if !RefusedByGate(err, fabricengine.CheckOwnership) {
		t.Fatalf("error = %v; want a CheckOwnership refusal", err)
	}
	if onOrigin := gitkit.BranchExists(t, h.RecordsBare, primary); onOrigin != onOriginBefore {
		t.Errorf("origin primary present = %v; want unchanged %v", onOrigin, onOriginBefore)
	}
}

// TestDeleteArchivedWeftBranch_EmptyArchiveTagRefused proves an empty archive tag is refused on dirtiness.
func TestDeleteArchivedWeftBranch_EmptyArchiveTagRefused(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	const slug = "archnotag"
	h, weftRoot, branch, tip := archivedDeleteFixture(t, slug)

	_, err := fabricengine.DeleteArchivedWeftBranchForTest(h.Location, weftRoot, slug, branch, "", tip)
	if !RefusedByGate(err, fabricengine.CheckDirtiness) {
		t.Fatalf("error = %v; want a CheckDirtiness refusal", err)
	}
	if got := gitkit.RevParse(t, h.RecordsBare, branch); got != tip {
		t.Errorf("origin %s = %q; want unchanged %q", branch, got, tip)
	}
}

// TestDeleteArchivedWeftBranch_EmptyLeaseRefused proves a valid tag with no lease SHA is refused on dirtiness.
func TestDeleteArchivedWeftBranch_EmptyLeaseRefused(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	const slug = "archnolease"
	h, weftRoot, branch, tip := archivedDeleteFixture(t, slug)

	_, err := fabricengine.DeleteArchivedWeftBranchForTest(h.Location, weftRoot, slug, branch, "archive/"+slug+"/"+tip[:12], "")
	if !RefusedByGate(err, fabricengine.CheckDirtiness) {
		t.Fatalf("error = %v; want a CheckDirtiness refusal", err)
	}
	if got := gitkit.RevParse(t, h.RecordsBare, branch); got != tip {
		t.Errorf("origin %s = %q; want unchanged %q", branch, got, tip)
	}
}

// TestDeleteArchivedWeftBranch_StaleLeaseFailsAndKeepsTip proves a lease behind an advanced origin fails.
// The call fails without a gate refusal, records nothing, and leaves the advanced tip.
//
//testtiming:keep a lease behind an advanced origin failing without a gate refusal, recording nothing and leaving the advanced tip; coverage of its blocks by other tests does not show an assertion of this
func TestDeleteArchivedWeftBranch_StaleLeaseFailsAndKeepsTip(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	const slug = "archstale"
	h, weftRoot, branch, staleTip := archivedDeleteFixture(t, slug)

	other := t.TempDir()
	gitkit.MustRun(t, other, "git", "clone", "--branch", branch, h.RecordsBare, "clone")
	clone := other + "/clone"
	gitkit.MustRun(t, clone, "git", "-c", "user.name=t", "-c", "user.email=t@t", "commit", "--allow-empty", "-m", "advance")
	gitkit.MustRun(t, clone, "git", "push", "origin", branch)
	advanced := gitkit.RevParse(t, h.RecordsBare, branch)
	if advanced == staleTip {
		t.Fatal("origin did not advance")
	}

	rec, err := fabricengine.DeleteArchivedWeftBranchForTest(h.Location, weftRoot, slug, branch, "archive/"+slug+"/"+staleTip[:12], staleTip)
	if err == nil {
		t.Fatal("error = nil; want a stale-lease failure")
	}
	if _, refused := fabricengine.RefusalOf(err); refused {
		t.Fatalf("error = %v; want a non-refusal error", err)
	}
	if rec.Len() != 0 {
		t.Errorf("record = %+v; want empty", rec.Entries())
	}
	if got := gitkit.RevParse(t, h.RecordsBare, branch); got != advanced {
		t.Errorf("origin %s = %q; want the advanced tip %q", branch, got, advanced)
	}
}
