//go:build integration

// reconcile_stale_registration_test.go proves Reconcile repairs a weft
// worktree that was deleted by hand (plain rm, not `git worktree remove`),
// the drift shape that leaves a stale git worktree registration still
// claiming the weft branch. Pre-fix, `git worktree add` refused with
// "missing but already registered worktree" on every reconcile run, so the
// drift was permanently unrepairable by any fabric verb (a live review round
// confirmed this); Reconcile now prunes stale registrations before adopting.
//
// This file also carries the fabric-only standalone regression guards
// backfilled from the now-deleted reconcile_differential_test.go: Prune's
// portal/launcher teardown on apply (R6) and its once-only stale-registration
// reporting (F2/F3), Cleanup's primary-weft-branch protection when the
// primary is parked off its branch (F1), its refusal to ever delete a
// non-suffixed (not fabric-managed) weft branch, and its checked-out protection for
// a pair whose host sits on a detached HEAD (R5); and PairInSync's "real
// directory in the junction's place" wording (R10). Each guards the same real
// bug the differential file's subtest name/doc comment names — see git
// history for reconcile_differential_test.go for the original differential
// framing.
//
// Package fabricengine_test to reuse the external-test-package fixture idiom
// formerly shared with lifecycle_differential_test.go; shares the single
// TestMain in testmain_test.go.

package fabricengine_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/hubgeometry"
	"github.com/Knatte18/loomyard/internal/lyxtest"
)

// TestReconcile_RecreatesHandDeletedWeftWorktree deletes a pair's weft
// worktree directory with plain os.RemoveAll (leaving the stale registration
// behind) and asserts one Reconcile run recreates it from the surviving
// branch with no per-pair error.
func TestReconcile_RecreatesHandDeletedWeftWorktree(t *testing.T) {
	t.Parallel()

	const slug = "stale-reg-recreate"
	fixture := newFabricFixture(t)

	l := fixture.Layout
	topology := fabricengine.NewTopology(fabricengine.Config{})
	if _, err := topology.Add(l, slug, fabricengine.AddOptions{SkipPush: true}); err != nil {
		t.Fatalf("setup Add: %v", err)
	}

	// The drift injection: delete the weft worktree directory out from under
	// git, exactly as a stray rm would — the registration and branch survive.
	weftPath := l.WeftWorktreePath(slug)
	if err := os.RemoveAll(weftPath); err != nil {
		t.Fatalf("hand-delete weft worktree: %v", err)
	}

	result, err := topology.Reconcile(l)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	// Find the pair for our slug and assert a clean recreate. Result paths are
	// emitted forward-slashed, so normalize the expectation the same way.
	var found bool
	for _, pair := range result.Pairs {
		if pair.WeftWorktree != filepath.ToSlash(weftPath) {
			continue
		}
		found = true
		if pair.Action != fabricengine.ReconcileActionWeftRecreated {
			t.Errorf("Action = %q; want %q", pair.Action, fabricengine.ReconcileActionWeftRecreated)
		}
		if pair.Error != "" {
			t.Errorf("Error = %q; want empty (stale registration must be pruned before adopt)", pair.Error)
		}
	}
	if !found {
		t.Fatalf("Reconcile result has no pair for slug %q: %+v", slug, result.Pairs)
	}

	// The weft worktree is back on disk, checked out on its suffixed branch.
	if info, err := os.Stat(weftPath); err != nil || !info.IsDir() {
		t.Fatalf("weft worktree not recreated at %s: %v", weftPath, err)
	}
	if got, want := currentBranchOf(t, weftPath), fabricengine.WeftBranchName(slug); got != want {
		t.Errorf("recreated weft worktree branch = %q; want %q", got, want)
	}
}

// newFabricFixture returns a lyxtest.CopyPairedLocal fixture seeded with a
// fabric config and its weft prime already on fabricengine.WeftBranchName
// ("main") — mirroring CloneHub's post-clone state so a fresh Add/Checkout
// fork has the suffixed primary branch it expects. Shared setup for every
// standalone regression guard in this file.
func newFabricFixture(t *testing.T) lyxtest.PairedFixture {
	t.Helper()

	fixture := lyxtest.CopyPairedLocal(t)
	lyxtest.SeedConfig(t, fixture.WeftPrime, map[string]string{
		"fabric": fabricengine.ConfigTemplate(),
	})
	lyxtest.MustRun(t, fixture.WeftPrime, "git", "checkout", "-b", fabricengine.WeftBranchName("main"))
	return fixture
}

// currentBranchOf returns the branch currently checked out at dir via
// git rev-parse --abbrev-ref HEAD, failing the test on any git error. Shared
// by every regression guard in this file plus checkout_rollback_test.go and
// checkout_index_refresh_test.go, which reference it across the shared
// fabricengine_test package.
func currentBranchOf(t *testing.T, dir string) string {
	t.Helper()

	out, _, exitCode, err := gitexec.RunGit([]string{"rev-parse", "--abbrev-ref", "HEAD"}, dir)
	if err != nil || exitCode != 0 {
		t.Fatalf("rev-parse --abbrev-ref HEAD in %s: err=%v exit=%d", dir, err, exitCode)
	}
	return strings.TrimSpace(out)
}

// branchExistsAt reports whether branch exists as a local ref in the repo at repoRoot.
func branchExistsAt(t *testing.T, repoRoot, branch string) bool {
	t.Helper()

	_, _, exitCode, err := gitexec.RunGit([]string{"rev-parse", "--verify", "refs/heads/" + branch}, repoRoot)
	if err != nil {
		t.Fatalf("rev-parse --verify refs/heads/%s in %s: %v", branch, repoRoot, err)
	}
	return exitCode == 0
}

// findPruneEntryByWeftPath returns the fabricengine.PruneEntry matching
// weftPath, failing the test if none is found.
func findPruneEntryByWeftPath(t *testing.T, entries []fabricengine.PruneEntry, weftPath string) *fabricengine.PruneEntry {
	t.Helper()
	for i := range entries {
		if filepath.Clean(entries[i].WeftWorktree) == filepath.Clean(weftPath) {
			return &entries[i]
		}
	}
	t.Fatalf("Prune: no entry for weft path %s", weftPath)
	return nil
}

// countPruneEntriesForWeft counts how many prune entries reference weftPath.
func countPruneEntriesForWeft(entries []fabricengine.PruneEntry, weftPath string) int {
	n := 0
	for i := range entries {
		if filepath.Clean(entries[i].WeftWorktree) == filepath.Clean(weftPath) {
			n++
		}
	}
	return n
}

// findCleanupEntry returns the fabricengine.CleanupBranchEntry for branch,
// failing the test if none is found.
func findCleanupEntry(t *testing.T, entries []fabricengine.CleanupBranchEntry, branch string) *fabricengine.CleanupBranchEntry {
	t.Helper()
	for i := range entries {
		if entries[i].Branch == branch {
			return &entries[i]
		}
	}
	t.Fatalf("Cleanup: no entry for branch %q; entries=%+v", branch, entries)
	return nil
}

// TestPrune_ApplyRemovesPortalAndLaunchers is the R6 fix regression guard:
// when the host worktree is deleted by hand, apply Prune removes the
// orphaned weft — but before the fix it left the dead slug's portal junction
// dangling and its launcher directory behind forever, since no other verb
// ever revisits a pruned slug (Remove would have, but the pair is already
// gone).
func TestPrune_ApplyRemovesPortalAndLaunchers(t *testing.T) {
	t.Parallel()

	const slug = "prune-portal-r6"
	fixture := newFabricFixture(t)
	l := fixture.Layout
	topology := fabricengine.NewTopology(fabricengine.Config{})
	if _, err := topology.Add(l, slug, fabricengine.AddOptions{SkipPush: true}); err != nil {
		t.Fatalf("setup Add: %v", err)
	}

	hostPath := l.WorktreePath(slug)
	portalLink := l.PortalLink(slug)
	launcherDir := l.LauncherDir(slug)

	// Sanity: Add wired both; Lstat for the portal since it dangles once the
	// host directory is gone.
	if _, err := os.Lstat(portalLink); err != nil {
		t.Fatalf("setup: portal link missing after Add: %v", err)
	}
	if _, err := os.Stat(launcherDir); err != nil {
		t.Fatalf("setup: launcher dir missing after Add: %v", err)
	}

	// Delete the host by hand (bare removal, not `git worktree remove`) — the
	// live-reproduced R6 scenario, leaving a stale registration.
	if err := os.RemoveAll(hostPath); err != nil {
		t.Fatalf("remove host dir: %v", err)
	}

	res, err := topology.Prune(l, true)
	if err != nil {
		t.Fatalf("Prune(apply=true): %v", err)
	}

	weftPath := l.WeftWorktreePath(slug)
	entry := findPruneEntryByWeftPath(t, res.Entries, weftPath)
	if !entry.Removed {
		t.Errorf("Removed = false after apply; want true (error=%q)", entry.Error)
	}
	if _, err := os.Lstat(portalLink); !os.IsNotExist(err) {
		t.Errorf("portal link %s still present after apply Prune; want removed", portalLink)
	}
	if _, err := os.Stat(launcherDir); !os.IsNotExist(err) {
		t.Errorf("launcher dir %s still present after apply Prune; want removed", launcherDir)
	}
}

// TestPrune_StaleRegistrationReportedOnce is the F2/F3 regression guard: when
// a host worktree directory is deleted by a bare removal (leaving a stale git
// worktree registration) rather than `git worktree remove`, Pass 1 (missing
// registered host) and Pass 2 (weft with no host sibling) both see the same
// orphaned weft. Prune must report it exactly once so dry-run and --apply
// agree, and must not claim Removed=true when there is no weft directory to
// delete.
func TestPrune_StaleRegistrationReportedOnce(t *testing.T) {
	t.Run("DoubleCountAvoided", func(t *testing.T) {
		t.Parallel()

		const slug = "prune-stale-reg-f2"
		fixture := newFabricFixture(t)
		l := fixture.Layout
		topology := fabricengine.NewTopology(fabricengine.Config{})
		if _, err := topology.Add(l, slug, fabricengine.AddOptions{SkipPush: true}); err != nil {
			t.Fatalf("setup Add: %v", err)
		}
		hostPath := l.WorktreePath(slug)
		weftPath := l.WeftWorktreePath(slug)

		// Bare removal of the host directory leaves the git worktree
		// registration stale (unlike `git worktree remove`), so both prune
		// passes see the orphan.
		if err := os.RemoveAll(hostPath); err != nil {
			t.Fatalf("bare-remove host worktree: %v", err)
		}

		dry, err := topology.Prune(l, false)
		if err != nil {
			t.Fatalf("Prune(dry-run): %v", err)
		}
		if got := countPruneEntriesForWeft(dry.Entries, weftPath); got != 1 {
			t.Errorf("dry-run reported weft %s %d times; want exactly 1", weftPath, got)
		}

		apply, err := topology.Prune(l, true)
		if err != nil {
			t.Fatalf("Prune(apply): %v", err)
		}
		if got := countPruneEntriesForWeft(apply.Entries, weftPath); got != 1 {
			t.Errorf("apply reported weft %s %d times; want exactly 1", weftPath, got)
		}
		if len(dry.Entries) != len(apply.Entries) {
			t.Errorf("dry-run entry count %d != apply entry count %d; want agreement", len(dry.Entries), len(apply.Entries))
		}
		if _, err := os.Stat(weftPath); !os.IsNotExist(err) {
			t.Errorf("weft dir %s still exists after apply Prune", weftPath)
		}
	})

	t.Run("RemovedFalseWhenNoWeftDir", func(t *testing.T) {
		t.Parallel()

		const slug = "prune-stale-reg-f3"
		fixture := newFabricFixture(t)
		l := fixture.Layout
		topology := fabricengine.NewTopology(fabricengine.Config{})
		if _, err := topology.Add(l, slug, fabricengine.AddOptions{SkipPush: true}); err != nil {
			t.Fatalf("setup Add: %v", err)
		}
		hostPath := l.WorktreePath(slug)
		weftPath := l.WeftWorktreePath(slug)

		// Bare-remove BOTH sides, leaving both registrations stale. With the
		// weft directory gone, Pass 1's removeStalePair has nothing to
		// delete.
		if err := os.RemoveAll(hostPath); err != nil {
			t.Fatalf("bare-remove host worktree: %v", err)
		}
		if err := os.RemoveAll(weftPath); err != nil {
			t.Fatalf("bare-remove weft worktree: %v", err)
		}

		apply, err := topology.Prune(l, true)
		if err != nil {
			t.Fatalf("Prune(apply): %v", err)
		}
		entry := findPruneEntryByWeftPath(t, apply.Entries, weftPath)
		if entry.Removed {
			t.Errorf("Removed = true for a weft worktree that no longer existed; want false")
		}
		if entry.Error != "" {
			t.Errorf("unexpected error for missing weft dir: %q", entry.Error)
		}
	})
}

// TestCleanup_PrimaryBranchSurvivesForceWhenNotCheckedOut is the F1
// regression guard: it moves the fabric weft primary OFF main-weft (exactly
// what a coordinated checkout of the primary to another branch produces) so
// main-weft is no longer the checked-out branch, then runs apply+force
// Cleanup. main-weft must still survive — protected by fabric's
// live-host-branch logic, not merely by git's refusal to delete a
// checked-out branch. Before the fix, main-weft was deleted here.
func TestCleanup_PrimaryBranchSurvivesForceWhenNotCheckedOut(t *testing.T) {
	t.Parallel()

	fixture := newFabricFixture(t)
	l := fixture.Layout
	topology := fabricengine.NewTopology(fabricengine.Config{})

	mainWeft := fabricengine.WeftBranchName("main")
	weftPrime := l.WeftWorktree()

	// Move the weft primary off main-weft so main-weft is not the
	// checked-out branch.
	lyxtest.MustRun(t, weftPrime, "git", "checkout", "-b", "primary-parked")

	res, err := topology.Cleanup(l, true, true)
	if err != nil {
		t.Fatalf("Cleanup(apply, force): %v", err)
	}

	for _, entry := range res.Entries {
		if entry.Branch == mainWeft {
			t.Errorf("Cleanup reported/handled primary weft branch %q; want not reported (live pair)", mainWeft)
		}
	}
	if !branchExistsAt(t, l.WeftRepoRoot(), mainWeft) {
		t.Errorf("main-weft branch deleted after force Cleanup with primary parked elsewhere; want intact (F1 regression)")
	}
}

// TestCleanup_NonSuffixedBranchNeverDeleted covers a weft branch with no
// "-weft" suffix: it is not fabric-managed, whatever its origin; fabric must
// never delete it, even under apply+force.
func TestCleanup_NonSuffixedBranchNeverDeleted(t *testing.T) {
	t.Parallel()

	fixture := newFabricFixture(t)
	l := fixture.Layout
	topology := fabricengine.NewTopology(fabricengine.Config{})

	const warpManagedBranch = "cleanup-warp-owned"
	lyxtest.MustRun(t, l.WeftRepoRoot(), "git", "branch", warpManagedBranch, fabricengine.WeftBranchName("main"))

	res, err := topology.Cleanup(l, true, true)
	if err != nil {
		t.Fatalf("Cleanup(apply, force): %v", err)
	}

	entry := findCleanupEntry(t, res.Entries, warpManagedBranch)
	if !entry.Protected {
		t.Errorf("Protected = false for non-suffixed branch %q; want true (not fabric-managed)", warpManagedBranch)
	}
	if entry.Deleted {
		t.Errorf("Deleted = true for non-suffixed branch %q; want false even under force", warpManagedBranch)
	}
	if !branchExistsAt(t, l.WeftRepoRoot(), warpManagedBranch) {
		t.Errorf("non-suffixed branch %q deleted; want intact", warpManagedBranch)
	}
}

// TestCleanup_DetachedHostHeadProtectsCheckedOutWeftBranch is the R5 fix: a
// live pair whose host worktree sits on a detached HEAD is invisible to the
// live-host-branch liveness check (readBranch reports the literal "HEAD"),
// so before the fix Cleanup listed the pair's weft branch as a deletable
// orphan and apply+force attempted a git branch -D that only git's own
// checked-out refusal stopped. The checked-out protection must report the
// branch Protected in every mode instead.
func TestCleanup_DetachedHostHeadProtectsCheckedOutWeftBranch(t *testing.T) {
	t.Parallel()

	const slug = "cleanup-detached-r5"
	fixture := newFabricFixture(t)
	l := fixture.Layout
	topology := fabricengine.NewTopology(fabricengine.Config{})
	if _, err := topology.Add(l, slug, fabricengine.AddOptions{SkipPush: true}); err != nil {
		t.Fatalf("setup Add: %v", err)
	}

	// Detach the host worktree's HEAD so branch-space liveness cannot see the
	// pair is live; only the checked-out protection stands between Cleanup
	// and the pair's weft branch.
	hostPath := l.WorktreePath(slug)
	lyxtest.MustRun(t, hostPath, "git", "checkout", "--detach")

	weftBranch := fabricengine.WeftBranchName(slug)

	// Dry-run must already report the branch protected, so dry-run and apply
	// agree about its fate.
	dry, err := topology.Cleanup(l, false, false)
	if err != nil {
		t.Fatalf("Cleanup(dry-run): %v", err)
	}
	dryEntry := findCleanupEntry(t, dry.Entries, weftBranch)
	if !dryEntry.Protected {
		t.Errorf("dry-run Protected = false for checked-out weft branch %q; want true", weftBranch)
	}

	forced, err := topology.Cleanup(l, true, true)
	if err != nil {
		t.Fatalf("Cleanup(apply, force): %v", err)
	}
	forcedEntry := findCleanupEntry(t, forced.Entries, weftBranch)
	if !forcedEntry.Protected || forcedEntry.Deleted {
		t.Errorf("apply+force entry: Protected=%v Deleted=%v; want Protected=true Deleted=false", forcedEntry.Protected, forcedEntry.Deleted)
	}
	if forcedEntry.Error != "" {
		t.Errorf("apply+force entry Error = %q; want empty (no doomed delete attempt)", forcedEntry.Error)
	}
	if !branchExistsAt(t, l.WeftRepoRoot(), weftBranch) {
		t.Errorf("checked-out weft branch %q deleted; want intact", weftBranch)
	}
}

// TestPairInSync_RealDirNotAJunction is the R10 fix: a real (non-link)
// directory sitting where the _lyx junction belongs must be reported as
// "host _lyx is not a junction" — the wording checkJunctionHealth already
// uses for the same drift shape — not as "junction missing". The loom
// preflight will consume this reason string after cutover.
func TestPairInSync_RealDirNotAJunction(t *testing.T) {
	t.Parallel()

	const slug = "pairinsync-realdir"
	fixture := newFabricFixture(t)
	l := fixture.Layout
	topology := fabricengine.NewTopology(fabricengine.Config{})
	if _, err := topology.Add(l, slug, fabricengine.AddOptions{SkipPush: true}); err != nil {
		t.Fatalf("setup Add: %v", err)
	}
	if err := fabricengine.WireJunctions(l, slug); err != nil {
		t.Fatalf("WireJunctions: %v", err)
	}

	hostLayout, err := hubgeometry.Resolve(l.WorktreePath(slug))
	if err != nil {
		t.Fatalf("hubgeometry.Resolve(host): %v", err)
	}

	// Replace the junction with a real directory — the drift shape a
	// pre-weft repo migration or a hand-created _lyx leaves behind.
	hostLink := hostLayout.HostLyxLinkHere()
	if err := os.Remove(hostLink); err != nil {
		t.Fatalf("remove host junction: %v", err)
	}
	if err := os.Mkdir(hostLink, 0o755); err != nil {
		t.Fatalf("mkdir real dir in junction's place: %v", err)
	}

	ok, reason, err := fabricengine.PairInSync(hostLayout)
	if err != nil {
		t.Fatalf("fabricengine.PairInSync: %v", err)
	}
	if ok {
		t.Errorf("PairInSync = true with a real _lyx directory; want false")
	}
	if reason != "host _lyx is not a junction" {
		t.Errorf("PairInSync reason = %q; want %q", reason, "host _lyx is not a junction")
	}
}
