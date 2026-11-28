// cleanupremotewarp_integration_test.go covers Topology.CleanupRemoteWarp against hubforge bare origins.
// A dry run classifies every branch on the warp origin.
// Apply deletes only the candidate, and a tip that moves after it was observed keeps its branch.
//
// Every hub is built through hubforge.CopyHub with an empty branch_prefix.
// A branch is fabric-managed when the weft origin holds its weft branch.
// One test sets a prefix afterwards.
// Package fabricengine_test; shares the single TestMain in testmain_test.go.

package fabricengine_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// markManaged gives branch a weft branch on the weft origin, which is what makes it fabric-managed.
func markManaged(t *testing.T, h *hubforge.Hub, branch string) {
	t.Helper()

	gitkit.MustRun(t, mustRecordsRepoRoot(t, h.Location), "git", "push", "origin", "HEAD:refs/heads/"+fabricengine.RecordsBranchName(branch))
}

// pushLandedLeftover pushes the prime's HEAD to origin as branch,
// so all its work is already on the default branch.
func pushLandedLeftover(t *testing.T, h *hubforge.Hub, branch string) {
	t.Helper()

	gitkit.MustRun(t, h.Location.WorktreePath(), "git", "push", "origin", "HEAD:refs/heads/"+branch)
}

// pushUnlandedLeftover pushes a commit reachable from no other ref to origin as branch.
// It uses force, so it can also move an existing tip.
func pushUnlandedLeftover(t *testing.T, h *hubforge.Hub, branch, file string) {
	t.Helper()

	prime := h.Location.WorktreePath()
	home := gitkit.CurrentBranch(t, prime)
	const scratch = "scratch-unlanded"
	gitkit.Git(t, prime, "checkout", "-b", scratch)
	gitkit.CommitFile(t, prime, file, file, file)
	gitkit.MustRun(t, prime, "git", "push", "--force", "origin", scratch+":refs/heads/"+branch)
	gitkit.Git(t, prime, "checkout", home)
	gitkit.Git(t, prime, "branch", "-D", scratch)
}

// remoteWarpEntries indexes res.Entries by branch.
func remoteWarpEntries(res fabricengine.RemoteWarpCleanupResult) map[string]fabricengine.RemoteWarpBranchEntry {
	byBranch := map[string]fabricengine.RemoteWarpBranchEntry{}
	for _, e := range res.Entries {
		byBranch[e.Branch] = e
	}
	return byBranch
}

// sweepFixture builds a hub whose warp origin holds one of each branch kind the sweep classifies.
func sweepFixture(t *testing.T) *hubforge.Hub {
	t.Helper()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	hubforge.AddPairWith(t, h, "live", fabricengine.AddOptions{})
	gitkit.MustRun(t, fabricengine.WorktreePath(h.Location, "live"), "git", "push", "origin", "live")

	pushLandedLeftover(t, h, "gone-landed")
	markManaged(t, h, "gone-landed")

	pushLandedLeftover(t, h, "unmanaged")

	pushLandedLeftover(t, h, "has-pr")
	markManaged(t, h, "has-pr")

	pushUnlandedLeftover(t, h, "gone-unlanded", "unlanded.txt")
	markManaged(t, h, "gone-unlanded")
	return h
}

//testtiming:keep a dry run recording no mutations and leaving every classified branch on origin; coverage of its blocks by other tests does not show an assertion of this
func TestCleanupRemoteWarp_DryRunClassifiesEveryBranch(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := sweepFixture(t)
	defaultBranch := gitkit.CurrentBranch(t, h.Location.WorktreePath())

	res, err := h.Topology.CleanupRemoteWarp(h.Location, false, map[string]bool{"has-pr": true})
	if err != nil {
		t.Fatalf("CleanupRemoteWarp error = %v", err)
	}
	got := remoteWarpEntries(res)

	if e := got["gone-landed"]; !e.Candidate || e.Reason != "" || e.Deleted {
		t.Errorf("gone-landed = %+v; want a candidate with no reason, not deleted", e)
	}
	wantReasons := map[string]string{
		"unmanaged":     "not fabric-managed",
		"live":          "checked out",
		"has-pr":        "open pull request",
		"gone-unlanded": "commit(s)",
		defaultBranch:   "default branch",
	}
	for branch, fragment := range wantReasons {
		e, ok := got[branch]
		if !ok {
			t.Errorf("no entry for %q; entries = %+v", branch, res.Entries)
			continue
		}
		if e.Candidate || e.Deleted || !strings.Contains(e.Reason, fragment) {
			t.Errorf("%s = %+v; want a non-candidate whose reason contains %q", branch, e, fragment)
		}
	}
	if res.Mutations.Len() != 0 {
		t.Errorf("dry run recorded mutations: %+v", res.Mutations.Entries())
	}
	for branch := range wantReasons {
		if !originHasBranch(t, h.CodeBare, branch) {
			t.Errorf("dry run removed %q from origin", branch)
		}
	}
}

func TestCleanupRemoteWarp_ApplyDeletesOnlyTheCandidate(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	t.Run("DeletesOnlyTheCandidate", func(t *testing.T) {
		t.Parallel()

		h := sweepFixture(t)

		res, err := h.Topology.CleanupRemoteWarp(h.Location, true, map[string]bool{"has-pr": true})
		if err != nil {
			t.Fatalf("CleanupRemoteWarp error = %v", err)
		}
		got := remoteWarpEntries(res)

		if e := got["gone-landed"]; !e.Deleted || e.Error != "" {
			t.Errorf("gone-landed = %+v; want deleted", e)
		}
		if originHasBranch(t, h.CodeBare, "gone-landed") {
			t.Errorf("gone-landed still on origin")
		}
		for _, kept := range []string{"unmanaged", "live", "has-pr", "gone-unlanded"} {
			if !originHasBranch(t, h.CodeBare, kept) {
				t.Errorf("%q was deleted from origin", kept)
			}
		}
		deletions := 0
		for _, m := range res.Mutations.Entries() {
			if m.Kind == fabricengine.KindRemoteBranchDeleted {
				deletions++
				if m.Target != "gone-landed" {
					t.Errorf("recorded deletion of %q; want only gone-landed", m.Target)
				}
			}
		}
		if deletions != 1 {
			t.Errorf("recorded %d %s mutations; want exactly 1", deletions, fabricengine.KindRemoteBranchDeleted)
		}
	})

	t.Run("KeepsTheCandidateWhoseTeardownLockIsHeld", func(t *testing.T) {
		t.Parallel()

		h := sweepFixture(t)
		holdTeardownLock(t, h, "gone-landed")

		res, err := h.Topology.CleanupRemoteWarp(h.Location, true, map[string]bool{"has-pr": true})
		if err != nil {
			t.Fatalf("CleanupRemoteWarp error = %v", err)
		}

		e := remoteWarpEntries(res)["gone-landed"]
		if e.Deleted || e.Error != "" || !strings.Contains(e.Reason, "teardown lock") {
			t.Errorf("gone-landed = %+v; want kept with a reason naming the teardown lock and no Error", e)
		}
		if !originHasBranch(t, h.CodeBare, "gone-landed") {
			t.Errorf("gone-landed was deleted from origin despite the held lock")
		}
	})

	t.Run("RecordsTheLockErrorWhenTheLockDirectoryCannotBeMade", func(t *testing.T) {
		t.Parallel()

		h := sweepFixture(t)
		scratch := fabricengine.HubScratchDir(h.Path)
		teardown := filepath.Join(scratch, "teardown")
		if err := os.RemoveAll(teardown); err != nil {
			t.Fatalf("remove %s: %v", teardown, err)
		}
		if err := os.MkdirAll(scratch, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", scratch, err)
		}
		if err := os.WriteFile(teardown, []byte("not a directory"), 0o644); err != nil {
			t.Fatalf("plant a file at the teardown lock directory: %v", err)
		}

		res, err := h.Topology.CleanupRemoteWarp(h.Location, true, map[string]bool{"has-pr": true})
		if err != nil {
			t.Fatalf("CleanupRemoteWarp error = %v; want the lock error on the entry only", err)
		}

		e := remoteWarpEntries(res)["gone-landed"]
		if e.Deleted || e.Error == "" || e.Reason != "" {
			t.Errorf("gone-landed = %+v; want Error set, no Reason and not deleted", e)
		}
		if !originHasBranch(t, h.CodeBare, "gone-landed") {
			t.Errorf("gone-landed was deleted from origin despite the lock error")
		}
	})

	t.Run("KeepsTheCandidateAWorktreeNowHolds", func(t *testing.T) {
		t.Parallel()

		h := sweepFixture(t)

		res, err := fabricengine.CleanupRemoteWarpWithHookForTest(h.Topology, h.Location, true, map[string]bool{"has-pr": true}, func() {
			gitkit.Git(t, h.PrimeWorktree(), "worktree", "add", "-b", "gone-landed", filepath.Join(t.TempDir(), "live"))
		})
		if err != nil {
			t.Fatalf("CleanupRemoteWarp error = %v", err)
		}

		e := remoteWarpEntries(res)["gone-landed"]
		if e.Deleted || e.Error != "" || !strings.Contains(e.Reason, "pair is live") {
			t.Errorf("gone-landed = %+v; want kept with a reason naming the live pair and no Error", e)
		}
		if !originHasBranch(t, h.CodeBare, "gone-landed") {
			t.Errorf("gone-landed was deleted from origin despite the checked-out worktree")
		}
	})
}

// With branch_prefix set, a branch lacking the prefix is read as the bare slug of a pair made earlier.
// Weft evidence makes it managed and deletable; without evidence it is kept as not fabric-managed.
//
//testtiming:keep the managed bare branch deleted on origin and the unmanaged one left on origin under branch_prefix; coverage of its blocks by other tests does not show an assertion of this
func TestCleanupRemoteWarp_BranchPrefixSetOverBareBranches(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := sweepFixture(t)
	top := topologyWithPrefix(t, h.Location, laterBranchPrefix)

	res, err := top.CleanupRemoteWarp(h.Location, true, map[string]bool{"has-pr": true})
	if err != nil {
		t.Fatalf("CleanupRemoteWarp error = %v", err)
	}
	got := remoteWarpEntries(res)

	if e := got["gone-landed"]; !e.Candidate || !e.Deleted || e.Error != "" {
		t.Errorf("gone-landed = %+v; want a managed candidate, deleted", e)
	}
	if originHasBranch(t, h.CodeBare, "gone-landed") {
		t.Errorf("gone-landed still on origin")
	}
	if e := got["unmanaged"]; e.Candidate || e.Deleted || !strings.Contains(e.Reason, "not fabric-managed") {
		t.Errorf("unmanaged = %+v; want a non-candidate whose reason says not fabric-managed", e)
	}
	if !originHasBranch(t, h.CodeBare, "unmanaged") {
		t.Errorf("unmanaged was deleted from origin")
	}
}

// With branch_prefix set, the sweep takes the teardown lock of a bare branch's own slug.
// It keeps the branch while that lock is held.
//
//testtiming:keep the bare branch left on origin while the teardown lock of its own slug is held; coverage of its blocks by other tests does not show an assertion of this
func TestCleanupRemoteWarp_BranchPrefixSetTakesTheBareSlugsLock(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := sweepFixture(t)
	holdTeardownLock(t, h, "gone-landed")
	top := topologyWithPrefix(t, h.Location, laterBranchPrefix)

	res, err := top.CleanupRemoteWarp(h.Location, true, map[string]bool{"has-pr": true})
	if err != nil {
		t.Fatalf("CleanupRemoteWarp error = %v", err)
	}

	e := remoteWarpEntries(res)["gone-landed"]
	if e.Deleted || e.Error != "" || !strings.Contains(e.Reason, "teardown lock") {
		t.Errorf("gone-landed = %+v; want kept with a reason naming the teardown lock and no Error", e)
	}
	if !originHasBranch(t, h.CodeBare, "gone-landed") {
		t.Errorf("gone-landed was deleted from origin despite the held lock")
	}
}

func TestCleanupRemoteWarp_NilOpenPRHeadsRefusesEveryDeletion(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := sweepFixture(t)

	res, err := h.Topology.CleanupRemoteWarp(h.Location, true, nil)
	if err != nil {
		t.Fatalf("CleanupRemoteWarp error = %v", err)
	}
	for _, e := range res.Entries {
		if e.Deleted || e.Candidate {
			t.Errorf("%s = %+v; want nothing deleted or offered without the open-PR set", e.Branch, e)
		}
	}
	if !originHasBranch(t, h.CodeBare, "gone-landed") {
		t.Errorf("gone-landed was deleted without the open-PR set")
	}
}

func TestCleanupRemoteWarp_MovedTipKeepsTheBranchWithAnError(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := sweepFixture(t)

	res, err := fabricengine.CleanupRemoteWarpWithHookForTest(h.Topology, h.Location, true, map[string]bool{}, func() {
		pushUnlandedLeftover(t, h, "gone-landed", "moved.txt")
	})
	if err != nil {
		t.Fatalf("CleanupRemoteWarp error = %v", err)
	}
	e := remoteWarpEntries(res)["gone-landed"]
	if e.Deleted || e.Error == "" {
		t.Errorf("gone-landed = %+v; want kept with an error after its tip moved", e)
	}
	if !originHasBranch(t, h.CodeBare, "gone-landed") {
		t.Errorf("gone-landed was deleted despite the moved tip")
	}
}
