//go:build integration

// removeremote_integration_test.go covers Remove's remote branch deletion under --remote: the
// opt-in deletion of the pair's weft branch on the remote, its default-off regression guard, the
// non-fatal remote-failure partial-teardown guarantee, and the once-per-verb no-origin pre-check
// shared with Cleanup.
//
// Every hub here is built through hubforge.NewHub per the hubforge Fabric-Fixture Invariant (via
// newFabricFixture), using the hub's own WeftBare field as the weft remote to assert against.
// mustBreakOrigin/mustRemoveOrigin/branchExistsAt are shared with cleanupremote_integration_test.go
// and reconcile_stale_registration_test.go — every assertion here goes through exported API.
//
// Package fabricengine_test; shares the single TestMain in testmain_test.go.

package fabricengine_test

import (
	"os"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
)

// TestRemove_RemoteTrueDeletesWeftBranchOnRemote covers case 1: remote true deletes the pair's weft
// branch on the remote, sets RemoteBranchDeleted, and records exactly one KindRemoteBranchDeleted
// entry.
func TestRemove_RemoteTrueDeletesWeftBranchOnRemote(t *testing.T) {
	t.Parallel()

	const slug = "remove-remote-both"
	fixture := newFabricFixture(t)
	l := fixture.Layout
	weftBranch := fabricengine.WeftBranchName(slug)

	topology := fabricengine.NewTopology(fabricengine.Config{})
	if _, err := topology.Add(l, slug, fabricengine.AddOptions{}); err != nil {
		t.Fatalf("setup Add(%q): %v", slug, err)
	}

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
	if branchExistsAt(t, fixture.WeftBare, weftBranch) {
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

// TestRemove_RemoteFalseLeavesRemoteBranchIntact covers the same case's negative half: remote false
// does neither.
func TestRemove_RemoteFalseLeavesRemoteBranchIntact(t *testing.T) {
	t.Parallel()

	const slug = "remove-remote-off"
	fixture := newFabricFixture(t)
	l := fixture.Layout
	weftBranch := fabricengine.WeftBranchName(slug)

	topology := fabricengine.NewTopology(fabricengine.Config{})
	if _, err := topology.Add(l, slug, fabricengine.AddOptions{}); err != nil {
		t.Fatalf("setup Add(%q): %v", slug, err)
	}

	res, err := topology.Remove(l, slug, false, false)
	if err != nil {
		t.Fatalf("Remove(%q, remote=false) error = %v", slug, err)
	}
	if res.RemoteBranchDeleted {
		t.Errorf("RemoteBranchDeleted = true; want false — remote is opt-in")
	}
	if !branchExistsAt(t, fixture.WeftBare, weftBranch) {
		t.Errorf("weft branch %q no longer exists on the remote after Remove(remote=false)", weftBranch)
	}
}

// TestRemove_RemoteFailureLeavesPartialTeardownGuaranteesIntact covers case 2: Remove's existing
// partial-teardown guarantees are unchanged when the remote deletion fails: the weft worktree is
// gone, the verb returns a nil error, RemoteBranchError is populated, and the failure has not been
// picked up by the teardown's own error accumulation. Induced the same way as Cleanup's own case: a
// weft origin pointing at an absent path.
func TestRemove_RemoteFailureLeavesPartialTeardownGuaranteesIntact(t *testing.T) {
	t.Parallel()

	const slug = "remove-remote-fail"
	fixture := newFabricFixture(t)
	l := fixture.Layout
	weftRoot := mustWeftRepoRoot(t, l)

	topology := fabricengine.NewTopology(fabricengine.Config{})
	if _, err := topology.Add(l, slug, fabricengine.AddOptions{}); err != nil {
		t.Fatalf("setup Add(%q): %v", slug, err)
	}

	mustBreakOrigin(t, weftRoot)

	res, err := topology.Remove(l, slug, false, true)
	if err != nil {
		t.Fatalf("Remove(%q, remote=true) error = %v; want nil — a remote failure is non-fatal", slug, err)
	}
	if res.RemoteBranchError == "" {
		t.Errorf("RemoteBranchError is empty; want a reason naming the remote deletion failure")
	}

	weftTarget := fabricengine.WeftWorktreePath(l, slug)
	if _, statErr := os.Stat(weftTarget); statErr == nil {
		t.Errorf("weft worktree still exists at %s; want the local teardown to have completed despite the remote failure", weftTarget)
	}
}

// TestRemove_NoOriginUnderRemoteReportsSkipReasonAndCompletesTeardown covers case 3: a weft repo
// with no origin configured under remote true: the teardown completes, RemoteSkippedReason is
// populated, RemoteBranchError is empty, and the verb returns a nil error — the identical verdict
// Cleanup reports for the same configuration state.
func TestRemove_NoOriginUnderRemoteReportsSkipReasonAndCompletesTeardown(t *testing.T) {
	t.Parallel()

	const slug = "remove-no-origin"
	fixture := newFabricFixture(t)
	l := fixture.Layout
	weftRoot := mustWeftRepoRoot(t, l)

	topology := fabricengine.NewTopology(fabricengine.Config{})
	if _, err := topology.Add(l, slug, fabricengine.AddOptions{SkipPush: true}); err != nil {
		t.Fatalf("setup Add(%q): %v", slug, err)
	}

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

	weftTarget := fabricengine.WeftWorktreePath(l, slug)
	if _, statErr := os.Stat(weftTarget); statErr == nil {
		t.Errorf("weft worktree still exists at %s; want the teardown to have completed", weftTarget)
	}
}
