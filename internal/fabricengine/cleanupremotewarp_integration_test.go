//go:build integration

// cleanupremotewarp_integration_test.go covers Topology.CleanupRemoteWarp against hubforge bare origins:
// a dry run classifies every branch on the warp origin, apply deletes only the candidate, and a tip that moves after it was observed keeps the branch.
//
// Every hub is built through hubforge.NewHub with an empty branch_prefix, so a branch is fabric-managed when the weft origin holds its weft branch.
// Package fabricengine_test; shares the single TestMain in testmain_test.go.

package fabricengine_test

import (
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
)

// markManaged gives branch a weft branch on the weft origin, which is what makes it fabric-managed.
func markManaged(t *testing.T, h *hubforge.Hub, branch string) {
	t.Helper()

	gitkit.MustRun(t, mustRecordsRepoRoot(t, h.Location), "git", "push", "origin", "HEAD:refs/heads/"+fabricengine.RecordsBranchName(branch))
}

// pushLandedLeftover pushes the prime's HEAD to origin as branch, so all its work is already on the default branch.
func pushLandedLeftover(t *testing.T, h *hubforge.Hub, branch string) {
	t.Helper()

	gitkit.MustRun(t, h.Location.WorktreePath(), "git", "push", "origin", "HEAD:refs/heads/"+branch)
}

// pushUnlandedLeftover pushes a commit reachable from no other ref to origin as branch, with force so it can also move an existing tip.
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

	h := hubforge.NewHub(t, ".")
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

func TestCleanupRemoteWarp_DryRunClassifiesEveryBranch(t *testing.T) {
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
}

func TestCleanupRemoteWarp_NilOpenPRHeadsRefusesEveryDeletion(t *testing.T) {
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
