//go:build integration

// cleanupremote_integration_test.go covers Cleanup's remote branch deletion under --remote: the
// opt-in deletion of an orphan weft branch's copy on the remote, its default-off regression guard,
// the dry-run no-op, the idempotent never-pushed path, the protected-entry carve-out, the non-fatal
// remote failure, and the once-per-verb no-origin pre-check across every combination of apply and
// remote -- plus RemovePairBranch, which deletes through the same gated helpers.
//
// Every hub here is built through hubforge.NewHub per the hubforge Fabric-Fixture Invariant (via
// newFabricFixture), using the hub's own WeftBare field as the weft remote to assert against — this
// hub's private copy of the weft bare remote, so a test can push an orphan branch to it and then
// assert the ref is gone.
//
// Package fabricengine_test to reuse newFabricFixture (reconcile_stale_registration_test.go) and
// mustWeftRepoRoot/branchExistsAt (add_rollback_adopt_test.go / reconcile_stale_registration_test.go)
// — every assertion here goes through exported API; shares the single TestMain in testmain_test.go.

package fabricengine_test

import (
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
)

// mustCreateOrphanWeftBranch creates branch in the weft repo at weftRoot, pointed at HEAD, with no
// worktree checkout — the shape Cleanup treats as an orphan candidate once it also carries the
// "-weft" suffix WeftWarpSlug requires.
func mustCreateOrphanWeftBranch(t *testing.T, weftRoot, branch string) {
	t.Helper()
	gitkit.MustRun(t, weftRoot, "git", "branch", branch, "HEAD")
}

// mustPushBranch pushes branch from repoRoot to its configured origin remote.
func mustPushBranch(t *testing.T, repoRoot, branch string) {
	t.Helper()
	gitkit.MustRun(t, repoRoot, "git", "push", "origin", branch)
}

// mustBreakOrigin points repoRoot's origin remote at a filesystem path that does not exist, so a
// push against it fails locally with no network involved.
func mustBreakOrigin(t *testing.T, repoRoot string) {
	t.Helper()
	gitkit.MustRun(t, repoRoot, "git", "remote", "set-url", "origin", filepath.Join(t.TempDir(), "no-such-remote.git"))
}

// mustRemoveOrigin removes repoRoot's origin remote entirely.
func mustRemoveOrigin(t *testing.T, repoRoot string) {
	t.Helper()
	gitkit.MustRun(t, repoRoot, "git", "remote", "remove", "origin")
}

// TestCleanup_RemoteTrueDeletesLocalAndRemoteOrphan covers case 1: apply and remote both true delete
// both the local orphan weft branch and its copy on the remote.
func TestCleanup_RemoteTrueDeletesLocalAndRemoteOrphan(t *testing.T) {
	t.Parallel()

	const branch = "cleanup-remote-both-weft"
	fixture := newFabricFixture(t)
	l := fixture.Layout
	weftRoot := mustWeftRepoRoot(t, l)

	mustCreateOrphanWeftBranch(t, weftRoot, branch)
	mustPushBranch(t, weftRoot, branch)

	topology := fabricengine.NewTopology(fabricengine.Config{})
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
	if branchExistsAt(t, weftRoot, branch) {
		t.Errorf("branch %q still exists locally after Cleanup(apply=true)", branch)
	}
	if branchExistsAt(t, fixture.WeftBare, branch) {
		t.Errorf("branch %q still exists on the remote after Cleanup(apply=true, remote=true)", branch)
	}
}

// TestCleanup_RemoteFalseLeavesRemoteCopyIntact covers case 2: apply true and remote false is the
// regression guard on the existing default — the feature is opt-in.
func TestCleanup_RemoteFalseLeavesRemoteCopyIntact(t *testing.T) {
	t.Parallel()

	const branch = "cleanup-remote-off-weft"
	fixture := newFabricFixture(t)
	l := fixture.Layout
	weftRoot := mustWeftRepoRoot(t, l)

	mustCreateOrphanWeftBranch(t, weftRoot, branch)
	mustPushBranch(t, weftRoot, branch)

	topology := fabricengine.NewTopology(fabricengine.Config{})
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
	if !branchExistsAt(t, fixture.WeftBare, branch) {
		t.Errorf("branch %q no longer exists on the remote after Cleanup(apply=true, remote=false); remote must be opt-in", branch)
	}
}

// TestCleanup_DryRunWithRemoteDeletesNeither covers case 3: a dry run with remote true performs no
// deletion on either side.
func TestCleanup_DryRunWithRemoteDeletesNeither(t *testing.T) {
	t.Parallel()

	const branch = "cleanup-dry-remote-weft"
	fixture := newFabricFixture(t)
	l := fixture.Layout
	weftRoot := mustWeftRepoRoot(t, l)

	mustCreateOrphanWeftBranch(t, weftRoot, branch)
	mustPushBranch(t, weftRoot, branch)

	topology := fabricengine.NewTopology(fabricengine.Config{})
	res, err := topology.Cleanup(l, false, false, true)
	if err != nil {
		t.Fatalf("Cleanup(apply=false, remote=true) error = %v", err)
	}

	entry := findCleanupEntry(t, res.Entries, branch)
	if entry.Deleted || entry.RemoteDeleted {
		t.Errorf("entry = %+v; want no deletion on a dry run", entry)
	}
	if !branchExistsAt(t, weftRoot, branch) {
		t.Errorf("branch %q was removed locally on a dry run", branch)
	}
	if !branchExistsAt(t, fixture.WeftBare, branch) {
		t.Errorf("branch %q was removed on the remote on a dry run", branch)
	}
}

// TestCleanup_NeverPushedOrphanIsIdempotentOnRemote covers case 4: an orphan branch that was never
// pushed deletes locally, reports no remote deletion and no remote error, and records no
// KindRemoteBranchDeleted entry.
func TestCleanup_NeverPushedOrphanIsIdempotentOnRemote(t *testing.T) {
	t.Parallel()

	const branch = "cleanup-never-pushed-weft"
	fixture := newFabricFixture(t)
	l := fixture.Layout
	weftRoot := mustWeftRepoRoot(t, l)

	mustCreateOrphanWeftBranch(t, weftRoot, branch)
	// Deliberately never pushed: the remote never had this ref to begin with.

	topology := fabricengine.NewTopology(fabricengine.Config{})
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

// TestCleanup_ProtectedEntryUntouchedOnRemote covers case 5: a protected (here, unmanaged) branch
// has neither its local nor its remote copy touched with remote true.
func TestCleanup_ProtectedEntryUntouchedOnRemote(t *testing.T) {
	t.Parallel()

	const branch = "cleanup-unmanaged-legacy"
	fixture := newFabricFixture(t)
	l := fixture.Layout
	weftRoot := mustWeftRepoRoot(t, l)

	// No "-weft" suffix: unmanaged, reported but never deletable — WeftWarpSlug rejects it.
	mustCreateOrphanWeftBranch(t, weftRoot, branch)
	mustPushBranch(t, weftRoot, branch)

	topology := fabricengine.NewTopology(fabricengine.Config{})
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
	if !branchExistsAt(t, weftRoot, branch) {
		t.Errorf("protected branch %q was removed locally", branch)
	}
	if !branchExistsAt(t, fixture.WeftBare, branch) {
		t.Errorf("protected branch %q was removed on the remote", branch)
	}
}

// TestCleanup_RemoteFailureIsNonFatal covers case 6: a remote deletion failure leaves the verb
// returning a nil error, the local branch deleted, and RemoteError populated. Induced by pointing
// the weft repo's origin at a filesystem path that no longer exists, so no network is needed.
func TestCleanup_RemoteFailureIsNonFatal(t *testing.T) {
	t.Parallel()

	const branch = "cleanup-remote-fail-weft"
	fixture := newFabricFixture(t)
	l := fixture.Layout
	weftRoot := mustWeftRepoRoot(t, l)

	mustCreateOrphanWeftBranch(t, weftRoot, branch)
	mustBreakOrigin(t, weftRoot)

	topology := fabricengine.NewTopology(fabricengine.Config{})
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
	if branchExistsAt(t, weftRoot, branch) {
		t.Errorf("branch %q still exists locally after Cleanup(apply=true)", branch)
	}
}

// TestCleanup_NoOriginUnderApplyAndRemoteSkipsOnceReportsOnce covers case 7: a weft repo with no
// origin configured, under apply and remote both true: every orphan weft branch is deleted locally,
// RemoteSkippedReason is non-empty exactly once on the result, every entry's RemoteError is empty,
// and the verb returns a nil error.
func TestCleanup_NoOriginUnderApplyAndRemoteSkipsOnceReportsOnce(t *testing.T) {
	t.Parallel()

	fixture := newFabricFixture(t)
	l := fixture.Layout
	weftRoot := mustWeftRepoRoot(t, l)

	branches := []string{"cleanup-no-origin-one-weft", "cleanup-no-origin-two-weft"}
	for _, b := range branches {
		mustCreateOrphanWeftBranch(t, weftRoot, b)
	}
	mustRemoveOrigin(t, weftRoot)

	topology := fabricengine.NewTopology(fabricengine.Config{})
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
		if branchExistsAt(t, weftRoot, b) {
			t.Errorf("branch %q still exists locally after Cleanup(apply=true)", b)
		}
	}
}

// TestCleanup_NoOriginUnderRemoteWithoutApplyIsStillReportedAndDeletesNothing covers case 8: the
// dry-run half of the no-origin pre-check.
func TestCleanup_NoOriginUnderRemoteWithoutApplyIsStillReportedAndDeletesNothing(t *testing.T) {
	t.Parallel()

	const branch = "cleanup-no-origin-dry-weft"
	fixture := newFabricFixture(t)
	l := fixture.Layout
	weftRoot := mustWeftRepoRoot(t, l)

	mustCreateOrphanWeftBranch(t, weftRoot, branch)
	mustRemoveOrigin(t, weftRoot)

	topology := fabricengine.NewTopology(fabricengine.Config{})
	res, err := topology.Cleanup(l, false, false, true)
	if err != nil {
		t.Fatalf("Cleanup(apply=false, remote=true) error = %v", err)
	}

	if res.RemoteSkippedReason == "" {
		t.Errorf("RemoteSkippedReason is empty; want a reason naming the missing origin remote, even on a dry run")
	}

	entry := findCleanupEntry(t, res.Entries, branch)
	if entry.Deleted {
		t.Errorf("entry.Deleted = true; want false — apply is false")
	}
	if !branchExistsAt(t, weftRoot, branch) {
		t.Errorf("branch %q was removed on a dry run", branch)
	}
}

// TestCleanup_NoOriginWithRemoteFalseReportsNoReason covers case 9: this guards the remote guard
// itself — without it, a plain cleanup --apply against a remoteless repo would start reporting a
// reason, an observable change to a path this task does not otherwise touch.
func TestCleanup_NoOriginWithRemoteFalseReportsNoReason(t *testing.T) {
	t.Parallel()

	const branch = "cleanup-no-origin-remote-off-weft"
	fixture := newFabricFixture(t)
	l := fixture.Layout
	weftRoot := mustWeftRepoRoot(t, l)

	mustCreateOrphanWeftBranch(t, weftRoot, branch)
	mustRemoveOrigin(t, weftRoot)

	topology := fabricengine.NewTopology(fabricengine.Config{})
	res, err := topology.Cleanup(l, true, false, false)
	if err != nil {
		t.Fatalf("Cleanup(apply=true, remote=false) error = %v", err)
	}

	if res.RemoteSkippedReason != "" {
		t.Errorf("RemoteSkippedReason = %q; want empty — remote is false, so the pre-check must not run at all", res.RemoteSkippedReason)
	}
}

// TestRemovePairBranch_DeletesLocalAndRemoteAndRefusesALivePair covers RemovePairBranch: it refuses
// while the pair's other-side worktree is still on disk, and once the pair is gone deletes the
// branch locally and on the remote, then answers an already-clean repeat with no error.
func TestRemovePairBranch_DeletesLocalAndRemoteAndRefusesALivePair(t *testing.T) {
	t.Parallel()

	const slug = "remove-pair-branch"
	branch := fabricengine.WeftBranchName(slug)
	fixture := newFabricFixture(t)
	l := fixture.Layout
	weftRoot := mustWeftRepoRoot(t, l)
	mustCreateOrphanWeftBranch(t, weftRoot, branch)
	mustPushBranch(t, weftRoot, branch)
	topology := fabricengine.NewTopology(fabricengine.Config{})

	sibling, _, err := fabricengine.PairSiblingRemnant(l, slug)
	if err != nil {
		t.Fatalf("PairSiblingRemnant: %v", err)
	}
	gitkit.MustRun(t, weftRoot, "git", "worktree", "add", sibling, branch)
	if _, err := topology.RemovePairBranch(l, slug); err == nil {
		t.Fatal("RemovePairBranch() with the other-side worktree on disk = nil error; want a refusal")
	}
	gitkit.MustRun(t, weftRoot, "git", "worktree", "remove", "--force", sibling)

	res, err := topology.RemovePairBranch(l, slug)
	if err != nil {
		t.Fatalf("RemovePairBranch() error = %v", err)
	}
	if !res.LocalDeleted || !res.RemoteDeleted {
		t.Errorf("RemovePairBranch() = %+v; want LocalDeleted and RemoteDeleted", res)
	}
	if branchExistsAt(t, weftRoot, branch) || branchExistsAt(t, fixture.WeftBare, branch) {
		t.Errorf("branch %q still present locally or on the remote", branch)
	}
	if _, err := topology.RemovePairBranch(l, slug); err != nil {
		t.Errorf("RemovePairBranch() repeated = %v; want nil for an already-deleted branch", err)
	}
}
