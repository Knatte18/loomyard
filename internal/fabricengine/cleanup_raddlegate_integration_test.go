//go:build integration

// cleanup_raddlegate_integration_test.go pins the post-raddle-gate-removal behaviour of `lyx fabric
// cleanup`: an orphan weft branch is now deletable under --apply alone, while the primary weft branch
// and an unmanaged (non-"-weft"-suffixed) weft branch stay protected exactly as before — removing
// raddleFoldedBack narrowed only the orphan-branch carve-out it used to add, not the primary or
// unmanaged carve-outs, which come from independent checks.
//
// Package fabricengine_test to reuse newFabricFixture, findCleanupEntry, branchExistsAt and
// mustWeftRepoRoot from reconcile_stale_registration_test.go and add_rollback_adopt_test.go; shares
// the single TestMain in testmain_test.go.

package fabricengine_test

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
)

// TestCleanup_ApplyDeletesOrphanWeftBranchWithoutForce proves an orphan weft branch — one whose
// paired warp branch no warp worktree is on — is deleted outright by Cleanup(l, true, false): apply
// alone, no --force, since no fold-back gate stands between it and deletion any more.
func TestCleanup_ApplyDeletesOrphanWeftBranchWithoutForce(t *testing.T) {
	t.Parallel()

	fixture := newFabricFixture(t)
	l := fixture.Layout
	weftRepoRoot := mustWeftRepoRoot(t, l)

	// An orphan weft branch: paired to a warp branch ("cleanup-raddlegate-orphan") that no warp
	// worktree is checked out on.
	orphan := fabricengine.WeftBranchName("cleanup-raddlegate-orphan")
	gitkit.MustRun(t, weftRepoRoot, "git", "branch", orphan, fabricengine.WeftBranchName("main"))

	topology := fabricengine.NewTopology(fabricengine.Config{})
	result, err := topology.Cleanup(l, true, false)
	if err != nil {
		t.Fatalf("Cleanup(apply=true, force=false) error = %v", err)
	}

	entry := findCleanupEntry(t, result.Entries, orphan)
	if !entry.Deleted {
		t.Errorf("Deleted = false for orphan branch %q; want true (no fold-back gate protects it)", orphan)
	}
	if entry.Protected {
		t.Errorf("Protected = true for orphan branch %q; want false", orphan)
	}
	if entry.Error != "" {
		t.Errorf("Error = %q; want empty", entry.Error)
	}
	if branchExistsAt(t, weftRepoRoot, orphan) {
		t.Errorf("orphan branch %q still exists in the weft repo after --apply; want deleted", orphan)
	}
}

// TestCleanup_PrimaryWeftBranchStaysProtectedWithoutForce moves the prime pair off the repo's
// default branch, exactly as TestCleanup_ProtectsPrimaryWeftBranchAfterCheckout does for
// --apply --force, and asserts the primary weft branch stays protected under --apply alone: removing
// the raddle gate did not widen the primary carve-out, which is independent of it.
func TestCleanup_PrimaryWeftBranchStaysProtectedWithoutForce(t *testing.T) {
	t.Parallel()

	fixture := newFabricFixture(t)
	l := fixture.Layout
	primaryWeftBranch := fabricengine.WeftBranchName("main")

	// Move the prime pair off the default branch, exactly as `lyx fabric checkout` does. The
	// fixture's <Hub>/_board worktree stays on "main", which is what records the repo's primary
	// warp branch.
	gitkit.MustRun(t, l.WorktreePath(), "git", "checkout", "-b", "alt")
	gitkit.MustRun(t, fixture.WeftPrime, "git", "checkout", "-b", fabricengine.WeftBranchName("alt"))

	topology := fabricengine.NewTopology(fabricengine.Config{})
	result, err := topology.Cleanup(l, true, false)
	if err != nil {
		t.Fatalf("Cleanup(apply=true, force=false) error = %v", err)
	}

	entry := findCleanupEntry(t, result.Entries, primaryWeftBranch)
	if entry.Deleted {
		t.Errorf("Cleanup deleted the primary weft branch %q", primaryWeftBranch)
	}
	if !entry.Protected {
		t.Errorf("Cleanup entry for %q has Protected = false; want the primary weft branch protected", primaryWeftBranch)
	}

	if !branchExistsAt(t, mustWeftRepoRoot(t, l), primaryWeftBranch) {
		t.Fatalf("primary weft branch %q no longer exists after cleanup --apply", primaryWeftBranch)
	}
}

// TestCleanup_UnmanagedNonSuffixedBranchStaysProtectedWithoutForce covers a weft branch with no
// "-weft" suffix: it is not fabric-managed, whatever its origin, and Cleanup(l, true, false) must
// still report it and still leave it alone — the unmanaged carve-out is independent of the removed
// raddle gate.
func TestCleanup_UnmanagedNonSuffixedBranchStaysProtectedWithoutForce(t *testing.T) {
	t.Parallel()

	fixture := newFabricFixture(t)
	l := fixture.Layout
	weftRepoRoot := mustWeftRepoRoot(t, l)

	const unmanagedBranch = "cleanup-raddlegate-unmanaged"
	gitkit.MustRun(t, weftRepoRoot, "git", "branch", unmanagedBranch, fabricengine.WeftBranchName("main"))

	topology := fabricengine.NewTopology(fabricengine.Config{})
	result, err := topology.Cleanup(l, true, false)
	if err != nil {
		t.Fatalf("Cleanup(apply=true, force=false) error = %v", err)
	}

	entry := findCleanupEntry(t, result.Entries, unmanagedBranch)
	if !entry.Protected {
		t.Errorf("Protected = false for non-suffixed branch %q; want true (not fabric-managed)", unmanagedBranch)
	}
	if entry.Deleted {
		t.Errorf("Deleted = true for non-suffixed branch %q; want false", unmanagedBranch)
	}
	if !branchExistsAt(t, weftRepoRoot, unmanagedBranch) {
		t.Errorf("non-suffixed branch %q deleted; want intact", unmanagedBranch)
	}
}
