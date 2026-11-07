// removeremote_integration_test.go covers Remove's remote branch deletion under --remote.
// It covers the opt-in deletion of the pair's weft branch on the remote, and its default-off guard.
// It also covers the non-fatal remote-failure partial-teardown guarantee.
// The no-origin pre-check runs once per verb.
//
// Every hub here is built through hubforge, per the hubforge Fabric-Fixture Invariant.
// The assertions read the weft remote from the hub's RecordsBare field.
// The helpers mustBreakOrigin and mustRemoveOrigin come from cleanupremote_integration_test.go,
// and every assertion here goes through exported API.
//
// Package fabricengine_test; shares the single TestMain in testmain_test.go.

package fabricengine_test

import (
	"os"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestRemove_RemoteTrueDeletesWeftBranchOnRemote asserts that remote true deletes the pair's weft branch on the remote.
// It also asserts that RemoteBranchDeleted is set, and that exactly one KindRemoteBranchDeleted entry is recorded.
func TestRemove_RemoteTrueDeletesWeftBranchOnRemote(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	const slug = "remove-remote-both"
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	l := h.Location
	weftBranch := fabricengine.RecordsBranchName(slug)

	topology := h.Topology
	hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{})

	res, err := topology.Remove(l, slug, false, true)
	if err != nil {
		t.Fatalf("Remove(%q, remote=true) error = %v", slug, err)
	}
	if !res.RemoteBranchDeleted {
		t.Errorf("RemoteBranchDeleted = false; want true")
	}
	if res.RemoteBranchError != "" {
		t.Errorf("RemoteBranchError = %q; want empty", res.RemoteBranchError)
	}
	if gitkit.BranchExists(t, h.RecordsBare, weftBranch) {
		t.Errorf("weft branch %q still exists on the remote after Remove(remote=true)", weftBranch)
	}

	var seen int
	for _, m := range res.Mutated().Entries() {
		if m.Kind == fabricengine.KindRemoteBranchDeleted && m.Target == weftBranch {
			seen++
		}
	}
	if seen != 1 {
		t.Errorf("mutation record has %d %s entries for %q; want exactly 1", seen, fabricengine.KindRemoteBranchDeleted, weftBranch)
	}
}

// TestRemove_LeasedWeftDelete asserts the lease on Remove's delete of the weft branch on origin.
// The delete is leased on the tip the archive read or pushed;
// a branch that origin moved since is kept and reported,
// and an origin that held no branch gets no delete.
// One hub per row, each removing with remote true; the rows set a package-level hook, so neither the test nor its rows are parallel.
func TestRemove_LeasedWeftDelete(t *testing.T) {
	tiergate.Require(t, 1)

	newPairHub := func(t *testing.T, slug string) *hubforge.Hub {
		t.Helper()
		h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
		hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{})
		return h
	}

	t.Run("OriginMovedBeforeTheDeleteKeepsItsBranchAndReportsKept", func(t *testing.T) {
		const slug = "leased-moved"
		weftBranch := fabricengine.RecordsBranchName(slug)
		h := newPairHub(t, slug)
		var movedTip string
		fabricengine.SetRemoveBeforeWeftDeleteHookForTest(t, func() {
			pushCommitToOrigin(t, h.RecordsBare, weftBranch)
			movedTip = gitkit.RevParse(t, h.RecordsBare, weftBranch)
		})

		res, err := h.Topology.Remove(h.Location, slug, false, true)
		if err != nil {
			t.Fatalf("Remove error = %v; want a kept branch to leave the removal successful", err)
		}
		if res.RemoteBranchDeleted || res.RemoteBranchError != "" || !strings.Contains(res.RemoteBranchKeptReason, weftBranch) {
			t.Errorf("RemoteBranchDeleted = %v, RemoteBranchError = %q, RemoteBranchKeptReason = %q; want not deleted, no error and a kept reason naming %q", res.RemoteBranchDeleted, res.RemoteBranchError, res.RemoteBranchKeptReason, weftBranch)
		}
		if got := gitkit.RevParse(t, h.RecordsBare, weftBranch); got != movedTip {
			t.Errorf("origin weft branch = %s; want it kept at the moved tip %s", got, movedTip)
		}
	})

	t.Run("OriginWithoutABranchAndARefusedPushGetsNoDelete", func(t *testing.T) {
		const slug = "leased-no-origin-branch"
		weftBranch := fabricengine.RecordsBranchName(slug)
		h := newPairHub(t, slug)
		gitkit.Git(t, h.RecordsBare, "update-ref", "-d", "refs/heads/"+weftBranch)
		installUpdateHook(t, h.RecordsBare, "case \"$1\" in\nrefs/heads/"+weftBranch+")\n  echo \"weft branch updates refused\" >&2\n  exit 1\n  ;;\nesac")
		deletes := 0
		fabricengine.SetRemoveBeforeWeftDeleteHookForTest(t, func() { deletes++ })

		res, err := h.Topology.Remove(h.Location, slug, false, true)
		if err != nil {
			t.Fatalf("Remove error = %v", err)
		}
		if deletes != 0 || res.RemoteBranchDeleted || res.RemoteBranchError != "" || res.RemoteBranchKeptReason != "" {
			t.Errorf("deletes attempted = %d, RemoteBranchDeleted = %v, RemoteBranchError = %q, RemoteBranchKeptReason = %q; want no delete attempted and nothing reported", deletes, res.RemoteBranchDeleted, res.RemoteBranchError, res.RemoteBranchKeptReason)
		}
	})

	t.Run("OriginLackingTheLocalTipIsPushedThenDeletedUnderALeaseOnThePushedTip", func(t *testing.T) {
		const slug = "leased-pushed-tip"
		weftBranch := fabricengine.RecordsBranchName(slug)
		h := newPairHub(t, slug)
		tip := gitkit.CommitFile(t, h.PairRecordsSibling(slug), "_lym/record.txt", "run record\n", "record")
		var originAtDelete string
		fabricengine.SetRemoveBeforeWeftDeleteHookForTest(t, func() {
			originAtDelete = gitkit.RevParse(t, h.RecordsBare, weftBranch)
		})

		res, err := h.Topology.Remove(h.Location, slug, false, true)
		if err != nil {
			t.Fatalf("Remove error = %v", err)
		}
		if originAtDelete != tip {
			t.Errorf("origin weft branch right before the delete = %q; want the pushed local tip %s", originAtDelete, tip)
		}
		if !res.RemoteBranchDeleted || res.RemoteBranchError != "" || res.RemoteBranchKeptReason != "" {
			t.Errorf("RemoteBranchDeleted = %v, RemoteBranchError = %q, RemoteBranchKeptReason = %q; want the pushed branch deleted", res.RemoteBranchDeleted, res.RemoteBranchError, res.RemoteBranchKeptReason)
		}
		if gitkit.BranchExists(t, h.RecordsBare, weftBranch) {
			t.Errorf("weft branch %q still on origin", weftBranch)
		}
	})

	t.Run("OriginOnlyBranchIsDeletedUnderALeaseOnTheTipTheArchiveRead", func(t *testing.T) {
		const slug = "leased-origin-only"
		weftBranch := fabricengine.RecordsBranchName(slug)
		h := newPairHub(t, slug)
		readTip := gitkit.RevParse(t, h.RecordsBare, weftBranch)
		leaveSiblingBranchOnlyOnOrigin(t, h, slug)
		var originAtDelete string
		fabricengine.SetRemoveBeforeWeftDeleteHookForTest(t, func() {
			originAtDelete = gitkit.RevParse(t, h.RecordsBare, weftBranch)
		})

		res, err := h.Topology.Remove(h.Location, slug, false, true)
		if err != nil {
			t.Fatalf("Remove error = %v", err)
		}
		if originAtDelete != readTip {
			t.Errorf("origin weft branch right before the delete = %q; want the archived tip %s", originAtDelete, readTip)
		}
		if !res.RemoteBranchDeleted || res.RemoteBranchKeptReason != "" || gitkit.BranchExists(t, h.RecordsBare, weftBranch) {
			t.Errorf("RemoteBranchDeleted = %v, RemoteBranchKeptReason = %q; want the origin-only branch deleted", res.RemoteBranchDeleted, res.RemoteBranchKeptReason)
		}
	})

	t.Run("OriginOnlyBranchThatMovedIsKeptAndReported", func(t *testing.T) {
		const slug = "leased-origin-only-moved"
		weftBranch := fabricengine.RecordsBranchName(slug)
		h := newPairHub(t, slug)
		leaveSiblingBranchOnlyOnOrigin(t, h, slug)
		var movedTip string
		fabricengine.SetRemoveBeforeWeftDeleteHookForTest(t, func() {
			pushCommitToOrigin(t, h.RecordsBare, weftBranch)
			movedTip = gitkit.RevParse(t, h.RecordsBare, weftBranch)
		})

		res, err := h.Topology.Remove(h.Location, slug, false, true)
		if err != nil {
			t.Fatalf("Remove error = %v", err)
		}
		if res.RemoteBranchDeleted || res.RemoteBranchError != "" || !strings.Contains(res.RemoteBranchKeptReason, weftBranch) {
			t.Errorf("RemoteBranchDeleted = %v, RemoteBranchError = %q, RemoteBranchKeptReason = %q; want the moved origin-only branch kept and reported", res.RemoteBranchDeleted, res.RemoteBranchError, res.RemoteBranchKeptReason)
		}
		if got := gitkit.RevParse(t, h.RecordsBare, weftBranch); got != movedTip {
			t.Errorf("origin weft branch = %s; want it kept at the moved tip %s", got, movedTip)
		}
	})
}

// TestRemove_RemoteFalseLeavesRemoteBranchIntact asserts that remote false leaves the remote weft branch intact.
// It sets neither RemoteBranchDeleted nor a KindRemoteBranchDeleted entry.
func TestRemove_RemoteFalseLeavesRemoteBranchIntact(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	const slug = "remove-remote-off"
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	l := h.Location
	weftBranch := fabricengine.RecordsBranchName(slug)

	topology := h.Topology
	hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{})

	res, err := topology.Remove(l, slug, false, false)
	if err != nil {
		t.Fatalf("Remove(%q, remote=false) error = %v", slug, err)
	}
	if res.RemoteBranchDeleted {
		t.Errorf("RemoteBranchDeleted = true; want false — remote is opt-in")
	}
	if !gitkit.BranchExists(t, h.RecordsBare, weftBranch) {
		t.Errorf("weft branch %q no longer exists on the remote after Remove(remote=false)", weftBranch)
	}
}

// TestRemove_RemoteFailureLeavesPartialTeardownGuaranteesIntact asserts the partial-teardown guarantee.
// An unreachable weft origin fails the remote steps, which run after the local teardown.
// The weft worktree is gone and the archive failure is the returned error.
// The weft origin is broken by pointing it at an absent path.
func TestRemove_RemoteFailureLeavesPartialTeardownGuaranteesIntact(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	const slug = "remove-remote-fail"
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	l := h.Location
	weftRoot := mustRecordsRepoRoot(t, l)

	topology := h.Topology
	hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{})

	mustBreakOrigin(t, weftRoot)

	// The archive push runs before the remote deletion, so it is the step that fails.
	if _, err := topology.Remove(l, slug, false, true); err == nil {
		t.Fatalf("Remove(%q, remote=true) = nil error; want the archive push failure against the unreachable origin", slug)
	}

	weftTarget := fabricengine.RecordsWorktreePath(l, slug)
	if _, statErr := os.Stat(weftTarget); !os.IsNotExist(statErr) {
		t.Errorf("weft worktree at %s: Stat error = %v; want it removed before the remote steps", weftTarget, statErr)
	}
}

// TestRemove_NoOriginUnderRemoteReportsSkipReasonAndCompletesTeardown asserts the teardown completes.
// Under remote true with no origin, RemoteSkippedReason is populated.
// It also asserts that RemoteBranchError is empty and that the verb returns a nil error.
func TestRemove_NoOriginUnderRemoteReportsSkipReasonAndCompletesTeardown(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	const slug = "remove-no-origin"
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	l := h.Location
	weftRoot := mustRecordsRepoRoot(t, l)

	topology := h.Topology
	hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{SkipPush: true})

	mustRemoveOrigin(t, weftRoot)

	res, err := topology.Remove(l, slug, false, true)
	if err != nil {
		t.Fatalf("Remove(%q, remote=true) error = %v", slug, err)
	}
	if res.RemoteSkippedReason == "" {
		t.Errorf("RemoteSkippedReason is empty; want a reason naming the missing origin remote")
	}
	if res.RemoteBranchError != "" {
		t.Errorf("RemoteBranchError = %q; want empty when the pre-check itself skipped", res.RemoteBranchError)
	}

	weftTarget := fabricengine.RecordsWorktreePath(l, slug)
	if _, statErr := os.Stat(weftTarget); statErr == nil {
		t.Errorf("weft worktree still exists at %s; want the teardown to have completed", weftTarget)
	}
}
