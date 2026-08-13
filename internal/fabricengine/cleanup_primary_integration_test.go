//go:build integration

// cleanup_primary_integration_test.go pins the carve-out that keeps `lyx fabric cleanup` from
// deleting the repo's primary weft branch.
// Branch-space liveness alone protected "main-weft" only while the prime warp worktree sat on the
// repo's default branch; one `lyx fabric checkout <other>` promoted the durable weft line to a
// deletable orphan, and `--apply --force` then deleted it together with any weft commit it alone
// carried.
//
// Package fabricengine_test to reuse newFabricFixture from
// reconcile_stale_registration_test.go; shares the single TestMain in testmain_test.go.

package fabricengine_test

import (
	"os"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
)

// TestCleanup_ProtectsPrimaryWeftBranchAfterCheckout materialises a real _board worktree on the
// warp's default branch, moves the prime pair onto another branch, and asserts cleanup reports the
// primary weft branch protected and leaves it on disk even under --apply --force.
func TestCleanup_ProtectsPrimaryWeftBranchAfterCheckout(t *testing.T) {
	t.Parallel()

	fixture := newFabricFixture(t)
	l := fixture.Layout
	primaryWeftBranch := fabricengine.WeftBranchName("main")

	// Move the prime pair off the default branch, exactly as `lyx fabric checkout` does.
	// The fixture's <Hub>/_board worktree stays on "main", which is what records the repo's
	// primary warp branch.
	gitkit.MustRun(t, l.WorktreePath(), "git", "checkout", "-b", "alt")
	gitkit.MustRun(t, fixture.WeftPrime, "git", "checkout", "-b", fabricengine.WeftBranchName("alt"))

	topology := fabricengine.NewTopology(fabricengine.Config{})

	result, err := topology.Cleanup(l, true, true)
	if err != nil {
		t.Fatalf("Cleanup(apply=true, force=true) error = %v", err)
	}

	var seen bool
	for _, entry := range result.Entries {
		if entry.Branch != primaryWeftBranch {
			continue
		}
		seen = true
		if entry.Deleted {
			t.Errorf("Cleanup deleted the primary weft branch %q", primaryWeftBranch)
		}
		if !entry.Protected {
			t.Errorf("Cleanup entry for %q has Protected = false; want the primary weft branch protected", primaryWeftBranch)
		}
	}
	if !seen {
		t.Fatalf("Cleanup reported no entry for %q; want it reported and protected", primaryWeftBranch)
	}

	if !branchExistsAt(t, mustWeftRepoRoot(t, l), primaryWeftBranch) {
		t.Fatalf("primary weft branch %q no longer exists after cleanup --apply --force", primaryWeftBranch)
	}
}

// TestCleanup_RefusesWhenPrimaryWeftBranchIsUndeterminable asserts the fail-closed direction: a hub
// whose board worktree is not a readable git checkout gets no orphan sweep at all, since Cleanup's
// deletions are irreversible.
func TestCleanup_RefusesWhenPrimaryWeftBranchIsUndeterminable(t *testing.T) {
	t.Parallel()

	fixture := newFabricFixture(t)
	if err := os.RemoveAll(fabricengine.BoardDir(fixture.Layout.HubPath)); err != nil {
		t.Fatalf("remove board worktree: %v", err)
	}

	topology := fabricengine.NewTopology(fabricengine.Config{})
	if _, err := topology.Cleanup(fixture.Layout, true, true); err == nil {
		t.Fatal("Cleanup() = nil error; want a refusal when the primary weft branch cannot be determined")
	}
}
