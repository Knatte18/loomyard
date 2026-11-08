// cleanupremote_integration_test.go covers Cleanup's remote branch deletion under --remote.
// Deleting an orphan weft branch's remote copy is opt-in, and the default-off path is pinned here.
// The tests also cover the dry-run no-op, the idempotent never-pushed path, and the protected-entry carve-out.
// They cover the non-fatal remote failure and the once-per-verb no-origin pre-check across apply and remote.
//
// Every hub here is built through hubforge.CopyHub.
// The hub's RecordsBare field is its private weft bare remote.
// A test pushes an orphan branch to it and then asserts the ref is gone.
//
// Package fabricengine_test; assertions go through the exported API.
// The package shares the single TestMain in testmain_test.go.

package fabricengine_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// mustCreateOrphanWeftBranch creates branch in the weft repo at weftRoot, pointed at HEAD.
// It leaves no worktree checkout.
// Cleanup treats such a branch as an orphan candidate only with the "-weft" suffix that `WeftWarpSlug` requires.
func mustCreateOrphanWeftBranch(t *testing.T, weftRoot, branch string) {
	t.Helper()
	gitkit.MustRun(t, weftRoot, "git", "branch", branch, "HEAD")
}

// mustPushBranch pushes branch from repoRoot to its configured origin remote.
func mustPushBranch(t *testing.T, repoRoot, branch string) {
	t.Helper()
	gitkit.MustRun(t, repoRoot, "git", "push", "origin", branch)
}

// mustBreakOrigin points repoRoot's origin remote at a filesystem path that does not exist.
// A push against it fails locally, with no network involved.
func mustBreakOrigin(t *testing.T, repoRoot string) {
	t.Helper()
	gitkit.MustRun(t, repoRoot, "git", "remote", "set-url", "origin", filepath.Join(t.TempDir(), "no-such-remote.git"))
}

// mustRemoveOrigin removes repoRoot's origin remote entirely.
func mustRemoveOrigin(t *testing.T, repoRoot string) {
	t.Helper()
	gitkit.MustRun(t, repoRoot, "git", "remote", "remove", "origin")
}

// TestCleanup_RemoteTrueDeletesLocalAndRemoteOrphan checks apply and remote both true.
// Both the local orphan weft branch and its copy on the remote are deleted.
//
//testtiming:keep the orphan weft branch deleted on origin as well as locally under remote=true; coverage of its blocks by other tests does not show an assertion of this
func TestCleanup_RemoteTrueDeletesLocalAndRemoteOrphan(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	const branch = "cleanup-remote-both-weft"
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	l := h.Location
	weftRoot := mustRecordsRepoRoot(t, l)

	mustCreateOrphanWeftBranch(t, weftRoot, branch)
	mustPushBranch(t, weftRoot, branch)

	topology := h.Topology
	res, err := topology.Cleanup(l, true, false, true)
	if err != nil {
		t.Fatalf("Cleanup(apply=true, remote=true) error = %v", err)
	}

	entry := findCleanupEntry(t, res.Entries, branch)
	if !entry.Deleted {
		t.Errorf("entry.Deleted = false; want true")
	}
	if !entry.RemoteDeleted {
		t.Errorf("entry.RemoteDeleted = false; want true")
	}
	if entry.RemoteError != "" {
		t.Errorf("entry.RemoteError = %q; want empty", entry.RemoteError)
	}
	if gitkit.BranchExists(t, weftRoot, branch) {
		t.Errorf("branch %q still exists locally after Cleanup(apply=true)", branch)
	}
	if gitkit.BranchExists(t, h.RecordsBare, branch) {
		t.Errorf("branch %q still exists on the remote after Cleanup(apply=true, remote=true)", branch)
	}
}

// TestCleanup_PrefixSetOverBareSlugOrphan covers an orphan weft branch from before branch_prefix was set.
// The branch is read as its bare slug, so the sweep archives and deletes it under that slug.
//
//testtiming:keep the archive tag of a bare-slug orphan named under archive/<slug>/ after a branch prefix was set
func TestCleanup_PrefixSetOverBareSlugOrphan(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	const slug = "cleanup-bare-before-prefix"
	branch := fabricengine.RecordsBranchName(slug)
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	l := h.Location
	weftRoot := mustRecordsRepoRoot(t, l)
	mustCreateOrphanWeftBranch(t, weftRoot, branch)
	mustPushBranch(t, weftRoot, branch)

	res, err := topologyWithPrefix(t, l, laterBranchPrefix).Cleanup(l, true, false, true)
	if err != nil {
		t.Fatalf("Cleanup(apply=true, remote=true) error = %v", err)
	}

	entry := findCleanupEntry(t, res.Entries, branch)
	if !entry.Deleted || !entry.RemoteDeleted || entry.Error != "" {
		t.Errorf("entry = %+v; want the bare-slug orphan deleted locally and on the remote with no error", entry)
	}
	if !strings.HasPrefix(entry.ArchiveTag, fabricengine.ArchiveTagPrefix+slug+"/") {
		t.Errorf("entry.ArchiveTag = %q; want a tag under %q", entry.ArchiveTag, fabricengine.ArchiveTagPrefix+slug+"/")
	}
}

// TestCleanup_RemoteFalseLeavesRemoteCopyIntact covers apply true with remote false.
// It guards the existing default: the remote deletion is opt-in.
//
//testtiming:keep the origin copy of a deleted orphan left intact when remote is off; coverage of its blocks by other tests does not show an assertion of this
func TestCleanup_RemoteFalseLeavesRemoteCopyIntact(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	const branch = "cleanup-remote-off-weft"
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	l := h.Location
	weftRoot := mustRecordsRepoRoot(t, l)

	mustCreateOrphanWeftBranch(t, weftRoot, branch)
	mustPushBranch(t, weftRoot, branch)

	topology := h.Topology
	res, err := topology.Cleanup(l, true, false, false)
	if err != nil {
		t.Fatalf("Cleanup(apply=true, remote=false) error = %v", err)
	}

	entry := findCleanupEntry(t, res.Entries, branch)
	if !entry.Deleted {
		t.Errorf("entry.Deleted = false; want true")
	}
	if entry.RemoteDeleted {
		t.Errorf("entry.RemoteDeleted = true; want false — remote is opt-in")
	}
	if !gitkit.BranchExists(t, h.RecordsBare, branch) {
		t.Errorf("branch %q no longer exists on the remote after Cleanup(apply=true, remote=false); remote must be opt-in", branch)
	}
}

// TestCleanup_DryRunWithRemoteDeletesNeither checks that a dry run with remote true deletes on neither side.
//
//testtiming:keep a dry run with remote=true deleting the orphan branch neither locally nor on origin; coverage of its blocks by other tests does not show an assertion of this
func TestCleanup_DryRunWithRemoteDeletesNeither(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	const branch = "cleanup-dry-remote-weft"
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	l := h.Location
	weftRoot := mustRecordsRepoRoot(t, l)

	mustCreateOrphanWeftBranch(t, weftRoot, branch)
	mustPushBranch(t, weftRoot, branch)

	topology := h.Topology
	res, err := topology.Cleanup(l, false, false, true)
	if err != nil {
		t.Fatalf("Cleanup(apply=false, remote=true) error = %v", err)
	}

	entry := findCleanupEntry(t, res.Entries, branch)
	if entry.Deleted || entry.RemoteDeleted {
		t.Errorf("entry = %+v; want no deletion on a dry run", entry)
	}
	if !gitkit.BranchExists(t, weftRoot, branch) {
		t.Errorf("branch %q was removed locally on a dry run", branch)
	}
	if !gitkit.BranchExists(t, h.RecordsBare, branch) {
		t.Errorf("branch %q was removed on the remote on a dry run", branch)
	}
}

// TestCleanup_NeverPushedOrphanIsIdempotentOnRemote covers an orphan branch that was never pushed.
// It deletes locally and reports no remote deletion or remote error.
// It records no KindRemoteBranchDeleted entry.
//
//testtiming:keep no remote_branch_deleted mutation record for a branch that was never on origin; coverage of its blocks by other tests does not show an assertion of this
func TestCleanup_NeverPushedOrphanIsIdempotentOnRemote(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	const branch = "cleanup-never-pushed-weft"
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	l := h.Location
	weftRoot := mustRecordsRepoRoot(t, l)

	mustCreateOrphanWeftBranch(t, weftRoot, branch)
	// Never pushed, so the remote has no ref to delete.

	topology := h.Topology
	res, err := topology.Cleanup(l, true, false, true)
	if err != nil {
		t.Fatalf("Cleanup(apply=true, remote=true) error = %v", err)
	}

	entry := findCleanupEntry(t, res.Entries, branch)
	if !entry.Deleted {
		t.Errorf("entry.Deleted = false; want true")
	}
	if entry.RemoteDeleted {
		t.Errorf("entry.RemoteDeleted = true; want false — the ref was never on the remote")
	}
	if entry.RemoteError != "" {
		t.Errorf("entry.RemoteError = %q; want empty — an already-absent remote ref is idempotent success", entry.RemoteError)
	}

	for _, m := range res.Mutated().Entries() {
		if m.Kind == fabricengine.KindRemoteBranchDeleted && m.Target == branch {
			t.Errorf("mutation record has a %s entry for %q; want none, since nothing was deleted on the remote", fabricengine.KindRemoteBranchDeleted, branch)
		}
	}
}

// TestCleanup_ProtectedEntryUntouchedOnRemote checks that a protected entry keeps its local and remote copies.
// Here the protected entry is an unmanaged branch, and remote true must not touch either copy.
//
//testtiming:keep a protected unmanaged branch left untouched on origin and locally under remote=true; coverage of its blocks by other tests does not show an assertion of this
func TestCleanup_ProtectedEntryUntouchedOnRemote(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	const branch = "cleanup-unmanaged-legacy"
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	l := h.Location
	weftRoot := mustRecordsRepoRoot(t, l)

	// Without the "-weft" suffix the branch is unmanaged: reported, never deletable, as `WeftWarpSlug` rejects it.
	mustCreateOrphanWeftBranch(t, weftRoot, branch)
	mustPushBranch(t, weftRoot, branch)

	topology := h.Topology
	res, err := topology.Cleanup(l, true, false, true)
	if err != nil {
		t.Fatalf("Cleanup(apply=true, remote=true) error = %v", err)
	}

	entry := findCleanupEntry(t, res.Entries, branch)
	if !entry.Protected {
		t.Errorf("entry.Protected = false; want true — unmanaged branches are reported but never deleted")
	}
	if entry.Deleted || entry.RemoteDeleted {
		t.Errorf("entry = %+v; want neither local nor remote deletion of a protected entry", entry)
	}
	if !gitkit.BranchExists(t, weftRoot, branch) {
		t.Errorf("protected branch %q was removed locally", branch)
	}
	if !gitkit.BranchExists(t, h.RecordsBare, branch) {
		t.Errorf("protected branch %q was removed on the remote", branch)
	}
}

// TestCleanup_RemoteFailureIsNonFatal checks that a failed remote deletion returns a nil error.
// The local branch is still deleted, and RemoteError is populated.
// The test makes the bare origin refuse ref deletions.
// The archive tag push still succeeds ahead of the failing branch deletion, and no network is needed.
func TestCleanup_RemoteFailureIsNonFatal(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	const branch = "cleanup-remote-fail-weft"
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	l := h.Location
	weftRoot := mustRecordsRepoRoot(t, l)

	mustCreateOrphanWeftBranch(t, weftRoot, branch)
	mustPushBranch(t, weftRoot, branch)
	gitkit.MustRun(t, h.RecordsBare, "git", "config", "receive.denyDeletes", "true")

	topology := h.Topology
	res, err := topology.Cleanup(l, true, false, true)
	if err != nil {
		t.Fatalf("Cleanup(apply=true, remote=true) error = %v; want nil — a remote failure is non-fatal", err)
	}

	entry := findCleanupEntry(t, res.Entries, branch)
	if !entry.Deleted {
		t.Errorf("entry.Deleted = false; want true — the local deletion must still succeed")
	}
	if entry.RemoteError == "" {
		t.Errorf("entry.RemoteError is empty; want a reason naming the remote deletion failure")
	}
	if gitkit.BranchExists(t, weftRoot, branch) {
		t.Errorf("branch %q still exists locally after Cleanup(apply=true)", branch)
	}
}

// TestCleanup_NoOriginUnderApplyAndRemoteSkipsOnceReportsOnce covers a weft repo with no origin.
// Under apply and remote both true, every orphan weft branch is still deleted locally.
// RemoteSkippedReason is non-empty exactly once on the result, and no entry carries a RemoteError.
// The verb returns a nil error.
func TestCleanup_NoOriginUnderApplyAndRemoteSkipsOnceReportsOnce(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	l := h.Location
	weftRoot := mustRecordsRepoRoot(t, l)

	branches := []string{"cleanup-no-origin-one-weft", "cleanup-no-origin-two-weft"}
	for _, b := range branches {
		mustCreateOrphanWeftBranch(t, weftRoot, b)
	}
	mustRemoveOrigin(t, weftRoot)

	topology := h.Topology
	res, err := topology.Cleanup(l, true, false, true)
	if err != nil {
		t.Fatalf("Cleanup(apply=true, remote=true) error = %v", err)
	}

	if res.RemoteSkippedReason == "" {
		t.Errorf("RemoteSkippedReason is empty; want a reason naming the missing origin remote")
	}

	for _, b := range branches {
		entry := findCleanupEntry(t, res.Entries, b)
		if !entry.Deleted {
			t.Errorf("entry for %q: Deleted = false; want true — the local sweep must still run", b)
		}
		if entry.RemoteError != "" {
			t.Errorf("entry for %q: RemoteError = %q; want empty — the reason lives on the verb-level field, not here", b, entry.RemoteError)
		}
		if gitkit.BranchExists(t, weftRoot, b) {
			t.Errorf("branch %q still exists locally after Cleanup(apply=true)", b)
		}
	}
}
