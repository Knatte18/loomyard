//go:build integration

// remove_remotetask_integration_test.go covers Remove's deletion of the pair's task branch on the warp repo's origin under remote:
// a landed tip goes, an unlanded one stays with a reason (and is then the only copy of its work, the local step having deleted the pushed local branch), a tip that moved after it was observed fails the lease, and without remote the origin copy is untouched.
//
// Every hub is built through hubforge.NewHub with an empty branch_prefix, so the pair's warp branch is the bare slug, and h.CodeBare is the origin.
// Package fabricengine_test; shares the single TestMain in testmain_test.go.

package fabricengine_test

import (
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
)

// originHasBranch reports whether the bare repo at bare holds branch.
func originHasBranch(t *testing.T, bare, branch string) bool {
	t.Helper()

	out, err := gitexec.Run([]string{"branch", "--list", branch}, bare)
	if err != nil {
		t.Fatalf("list branches in %s: %v", bare, err)
	}
	return strings.TrimSpace(out) != ""
}

// pushedPair adds slug, commits one file on its task branch and pushes the branch to origin, returning the hub.
func pushedPair(t *testing.T, slug string) *hubforge.Hub {
	t.Helper()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{})
	warpBranchCommit(t, h.Location, slug, "work.txt")
	gitkit.MustRun(t, fabricengine.WorktreePath(h.Location, slug), "git", "push", "origin", slug)
	return h
}

// squashLand lands slug's work on the prime with a different commit than the branch's own.
func squashLand(t *testing.T, h *hubforge.Hub, slug string) {
	t.Helper()

	prime := h.Location.WorktreePath()
	gitkit.MustRun(t, prime, "git", "merge", "--squash", slug)
	gitkit.Git(t, prime, "commit", "-m", "land "+slug)
}

func TestRemove_RemoteDeletesSquashLandedTaskBranchOnOrigin(t *testing.T) {
	t.Parallel()

	const slug = "rt-landed"
	h := pushedPair(t, slug)
	squashLand(t, h, slug)

	res, err := h.Topology.Remove(h.Location, slug, false, true)
	if err != nil {
		t.Fatalf("Remove(%q) error = %v", slug, err)
	}
	if !res.RemoteCodeBranchDeleted || res.RemoteCodeBranchKeptReason != "" {
		t.Errorf("RemoteCodeBranchDeleted = %v, kept reason = %q; want deleted", res.RemoteCodeBranchDeleted, res.RemoteCodeBranchKeptReason)
	}
	if originHasBranch(t, h.CodeBare, slug) {
		t.Errorf("task branch %q still on origin", slug)
	}
	found := false
	for _, step := range res.Steps {
		found = found || step == fabricengine.RemoveStepTaskBranchOnOrigin
	}
	if !found {
		t.Errorf("Steps = %v; want %q among them", res.Steps, fabricengine.RemoveStepTaskBranchOnOrigin)
	}
}

func TestRemove_RemoteKeepsUnlandedTaskBranchOnOrigin(t *testing.T) {
	t.Parallel()

	const slug = "rt-unlanded"
	h := pushedPair(t, slug)

	res, err := h.Topology.Remove(h.Location, slug, false, true)
	if err != nil {
		t.Fatalf("Remove(%q) error = %v; a kept branch is not a failure", slug, err)
	}
	if res.RemoteCodeBranchDeleted {
		t.Errorf("RemoteCodeBranchDeleted = true; want the unlanded branch kept")
	}
	if !strings.Contains(res.RemoteCodeBranchKeptReason, "commit(s)") {
		t.Errorf("RemoteCodeBranchKeptReason = %q; want it to name the unlanded commits", res.RemoteCodeBranchKeptReason)
	}
	if !originHasBranch(t, h.CodeBare, slug) {
		t.Errorf("task branch %q was deleted on origin despite unlanded work", slug)
	}
	// The local step counts the pushed origin copy as holding the work, so it deletes the local branch; the origin copy is then the only one left, which is why the remote gate excludes copies of the branch itself.
	if !res.CodeBranchDeleted {
		t.Errorf("CodeBranchDeleted = false (kept reason %q); the pushed local branch is deleted by the local step", res.CodeBranchKeptReason)
	}
}

func TestRemove_WithoutRemoteLeavesTaskBranchOnOrigin(t *testing.T) {
	t.Parallel()

	const slug = "rt-noremote"
	h := pushedPair(t, slug)
	squashLand(t, h, slug)

	res, err := h.Topology.Remove(h.Location, slug, false, false)
	if err != nil {
		t.Fatalf("Remove(%q) error = %v", slug, err)
	}
	if res.RemoteCodeBranchDeleted || res.RemoteCodeBranchKeptReason != "" {
		t.Errorf("RemoteCodeBranchDeleted = %v, kept reason = %q; want neither without remote", res.RemoteCodeBranchDeleted, res.RemoteCodeBranchKeptReason)
	}
	if !originHasBranch(t, h.CodeBare, slug) {
		t.Errorf("task branch %q was deleted on origin without remote", slug)
	}
}

func TestDeleteTaskBranchAtTip_MovedTipFailsLease(t *testing.T) {
	t.Parallel()

	const slug = "rt-lease"
	h := pushedPair(t, slug)
	squashLand(t, h, slug)
	l := h.Location
	task := fabricengine.WorktreePath(l, slug)

	origin, found, err := fabricengine.ReadOriginFor(l, slug)
	if err != nil || !found {
		t.Fatalf("ReadOriginFor(%q) = (found=%v, err=%v); want the recorded parent", slug, found, err)
	}
	observed := gitkit.RevParse(t, task, "HEAD")
	warpBranchCommit(t, l, slug, "later.txt")
	gitkit.MustRun(t, task, "git", "push", "origin", slug)

	deleted, reason := fabricengine.DeleteTaskBranchAtTipForTest(l, slug, origin.ParentBranch, observed)
	if deleted {
		t.Errorf("deleted = true; want the lease to fail on a moved tip")
	}
	if reason == "" {
		t.Errorf("kept reason is empty; want the lease failure")
	}
	if !originHasBranch(t, h.CodeBare, slug) {
		t.Errorf("task branch %q was deleted on origin despite the moved tip", slug)
	}
}
