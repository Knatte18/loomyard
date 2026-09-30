//go:build integration

// destroyremote_integration_test.go pins the remote-branch gate against a real hub, since each case needs a real git spawn: the unset-declaration and naming-predicate refusals are hermetic and covered in destroy_test.go instead, per the Test Tier Purity Invariant.
//
// The first two cases drive checkRemoteBranchRequest through CheckRemoteBranchRequestForTest (export_test.go) and pin refusals the real call sites cannot reach.
// The rest are executor-level cases driven through DeleteArchivedWeftBranchForTest: a pair's weft branch, still checked out at its weft worktree, is deleted from origin under a lease, and every refusal or stale lease leaves origin untouched.
//
// Package fabricengine_test: building either case needs a real hub via hubforge.NewHub, but an
// internal fabricengine test file cannot import internal/hubforge without closing an import cycle
// (hubforge -> fabriccli -> fabricengine). Shares the single TestMain in testmain_test.go.

package fabricengine_test

import (
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/gitkit"
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

// weftBareTip returns the SHA of branch on the weft bare, or "" when absent.
func weftBareTip(t *testing.T, weftBare, branch string) string {
	t.Helper()

	out, err := gitexec.Run([]string{"rev-parse", "--verify", "--quiet", "refs/heads/" + branch}, weftBare)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

// archivedDeleteFixture builds a hub with a pushed pair and returns the hub, its weft repo root, the pair's weft branch and the branch's tip on origin.
func archivedDeleteFixture(t *testing.T, slug string) (*hubforge.Hub, string, string, string) {
	t.Helper()

	h := hubforge.NewHub(t, ".")
	hubforge.AddPair(t, h, slug)
	weftRoot, err := fabricengine.WeftRepoRoot(h.Location)
	if err != nil {
		t.Fatalf("WeftRepoRoot: %v", err)
	}
	branch := fabricengine.WeftBranchName(slug)
	tip := weftBareTip(t, h.WeftBare, branch)
	if tip == "" {
		t.Fatalf("weft branch %s is not on origin after Add", branch)
	}
	return h, weftRoot, branch, tip
}

// TestDeleteArchivedWeftBranch_CheckedOutBranchDeleted proves the pair's own weft branch, checked out at its weft worktree, is deleted from origin under a valid tag and a lease at the tip,
// and it records one entry.
func TestDeleteArchivedWeftBranch_CheckedOutBranchDeleted(t *testing.T) {
	t.Parallel()

	const slug = "archdel"
	h, weftRoot, branch, tip := archivedDeleteFixture(t, slug)

	rec, err := fabricengine.DeleteArchivedWeftBranchForTest(h.Location, weftRoot, slug, branch, "archive/"+slug+"/"+tip[:12], tip)
	if err != nil {
		t.Fatalf("DeleteArchivedWeftBranchForTest error = %v; want nil", err)
	}
	if got := weftBareTip(t, h.WeftBare, branch); got != "" {
		t.Errorf("origin still has %s at %s; want it deleted", branch, got)
	}
	entries := rec.Entries()
	if len(entries) != 1 || entries[0].Kind != fabricengine.KindRemoteBranchDeleted {
		t.Errorf("record = %+v; want exactly one %s entry", entries, fabricengine.KindRemoteBranchDeleted)
	}
}

// TestDeleteArchivedWeftBranch_PrimaryRefused proves the primary weft branch is refused on ownership.
func TestDeleteArchivedWeftBranch_PrimaryRefused(t *testing.T) {
	t.Parallel()

	h, weftRoot, _, _ := archivedDeleteFixture(t, "archprimary")
	primary := fabricengine.WeftBranchName("main")
	tip := weftBareTip(t, h.WeftBare, primary)

	_, err := fabricengine.DeleteArchivedWeftBranchForTest(h.Location, weftRoot, "main", primary, "archive/main/x", tip)
	if !RefusedByGate(err, fabricengine.CheckOwnership) {
		t.Fatalf("error = %v; want a CheckOwnership refusal", err)
	}
	if got := weftBareTip(t, h.WeftBare, primary); got != tip {
		t.Errorf("origin primary = %q; want unchanged %q", got, tip)
	}
}

// TestDeleteArchivedWeftBranch_EmptyArchiveTagRefused proves an empty archive tag is refused on dirtiness.
func TestDeleteArchivedWeftBranch_EmptyArchiveTagRefused(t *testing.T) {
	t.Parallel()

	const slug = "archnotag"
	h, weftRoot, branch, tip := archivedDeleteFixture(t, slug)

	_, err := fabricengine.DeleteArchivedWeftBranchForTest(h.Location, weftRoot, slug, branch, "", tip)
	if !RefusedByGate(err, fabricengine.CheckDirtiness) {
		t.Fatalf("error = %v; want a CheckDirtiness refusal", err)
	}
	if got := weftBareTip(t, h.WeftBare, branch); got != tip {
		t.Errorf("origin %s = %q; want unchanged %q", branch, got, tip)
	}
}

// TestDeleteArchivedWeftBranch_EmptyLeaseRefused proves a valid tag with no lease SHA is refused on dirtiness.
func TestDeleteArchivedWeftBranch_EmptyLeaseRefused(t *testing.T) {
	t.Parallel()

	const slug = "archnolease"
	h, weftRoot, branch, tip := archivedDeleteFixture(t, slug)

	_, err := fabricengine.DeleteArchivedWeftBranchForTest(h.Location, weftRoot, slug, branch, "archive/"+slug+"/"+tip[:12], "")
	if !RefusedByGate(err, fabricengine.CheckDirtiness) {
		t.Fatalf("error = %v; want a CheckDirtiness refusal", err)
	}
	if got := weftBareTip(t, h.WeftBare, branch); got != tip {
		t.Errorf("origin %s = %q; want unchanged %q", branch, got, tip)
	}
}

// TestDeleteArchivedWeftBranch_StaleLeaseFailsAndKeepsTip proves a lease behind an advanced origin fails without a gate refusal, records nothing, and leaves the advanced tip.
func TestDeleteArchivedWeftBranch_StaleLeaseFailsAndKeepsTip(t *testing.T) {
	t.Parallel()

	const slug = "archstale"
	h, weftRoot, branch, staleTip := archivedDeleteFixture(t, slug)

	other := t.TempDir()
	gitkit.MustRun(t, other, "git", "clone", "--branch", branch, h.WeftBare, "clone")
	clone := other + "/clone"
	gitkit.MustRun(t, clone, "git", "-c", "user.name=t", "-c", "user.email=t@t", "commit", "--allow-empty", "-m", "advance")
	gitkit.MustRun(t, clone, "git", "push", "origin", branch)
	advanced := weftBareTip(t, h.WeftBare, branch)
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
	if got := weftBareTip(t, h.WeftBare, branch); got != advanced {
		t.Errorf("origin %s = %q; want the advanced tip %q", branch, got, advanced)
	}
}
