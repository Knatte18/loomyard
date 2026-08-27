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
// a pair whose warp sits on a detached HEAD (R5); and Healthy's "real
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

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

// TestReconcile_RecreatesHandDeletedWeftWorktree deletes a pair's weft worktree directory with
// plain os.RemoveAll (leaving the stale registration behind) and asserts one Reconcile run
// recreates it from the surviving branch with no per-pair error.
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
	weftPath := fabricengine.WeftWorktreePath(l, slug)
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

// fabricFixture is the local field-mapping shape newFabricFixture returns over a real hub, replacing
// the deleted gitkit paired-fixture struct it used to hand-assemble from gitkit's own retired
// local-pair template.
// It is a package-local type so this file's many existing callers do not all need to change their
// field-access pattern.
type fabricFixture struct {
	Container string
	Hub       string
	Bare      string
	WeftPrime string
	WeftBare  string
	Layout    *lyxcwd.Location
}

// newFabricFixture returns a fabricFixture-shaped view over a real hub built by hubforge.NewHub.
// hubforge.NewHub's own CloneAndWire already produces the shape this fixture used to hand-assemble
// from gitkit's own retired local-pair template — the weft primary checked out on the suffixed
// primary branch, a real _board worktree on the warp's unsuffixed default branch (the shape CloneHub
// produces and the shape Cleanup reads the repo's primary weft branch from), and the repo-wide fabric.yaml committed inside
// it — so this is now a thin field-mapping wrapper over the mapping table's equivalents.
func newFabricFixture(t *testing.T) fabricFixture {
	t.Helper()

	h := hubforge.NewHub(t, ".")
	return fabricFixture{
		Container: h.Container,
		Hub:       h.PrimeWorktree(),
		Bare:      h.WarpBare,
		WeftPrime: h.PrimeWeft(),
		WeftBare:  h.WeftBare,
		Layout:    h.Location,
	}
}

// TestReconcile_MissingWeftRepoIsDiagnosedByName destroys the weft prime — the checkout holding the
// weft repo's gitdir — and asserts reconcile reports the missing weft repo by name with a remedy,
// instead of the raw chdir errors each corrective branch used to fail with.
func TestReconcile_MissingWeftRepoIsDiagnosedByName(t *testing.T) {
	t.Parallel()

	fixture := newFabricFixture(t)
	l := fixture.Layout

	if err := os.RemoveAll(fixture.WeftPrime); err != nil {
		t.Fatalf("remove weft prime: %v", err)
	}

	topology := fabricengine.NewTopology(fabricengine.Config{})
	result, err := topology.Reconcile(l)
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	if len(result.Pairs) == 0 {
		t.Fatal("Reconcile() returned no pairs; want the prime pair reported")
	}
	pr := result.Pairs[0]
	if !strings.Contains(pr.Error, "weft repo missing at") {
		t.Errorf("Reconcile() pair error = %q; want it to diagnose the missing weft repo by name", pr.Error)
	}
	if !strings.Contains(pr.Error, "re-clone") {
		t.Errorf("Reconcile() pair error = %q; want it to name a remedy", pr.Error)
	}
	if pr.Action != fabricengine.ReconcileActionUnmanagedReported {
		t.Errorf("Reconcile() pair action = %q; want %q (report, never a corrective attempt against a missing repo)", pr.Action, fabricengine.ReconcileActionUnmanagedReported)
	}
}

// seedRepoWideFabricConfig materializes the repo-wide fabric.yaml at
// fabricengine.BoardDir(hub) — <hub>/_board/_lyx/config/fabric.yaml — the
// base card 7's RepoWiredNames-migrated sites (checkJunctionHealth,
// Healthy, Reconcile, Topology.Checkout, Topology.Remove,
// junctionRepointedDetail) now read from. A real hub built by hubforge.NewHub
// materializes _board via CloneAndWire, but not this repo-wide fabric.yaml, so this creates the file
// (and its _lyx/config/) first; unlike gitkit.SeedConfig, _board is not a git repository, so the
// file is written directly with no git add/commit step. Shared by every
// fabricengine_test fixture that exercises a migrated read.
func seedRepoWideFabricConfig(t testing.TB, hub string) {
	t.Helper()

	boardDir := fabricengine.BoardDir(hub)
	if err := os.MkdirAll(configengine.ConfigDir(boardDir), 0o755); err != nil {
		t.Fatalf("mkdir repo-wide config dir: %v", err)
	}
	configPath := configengine.ConfigFile(boardDir, "fabric")
	if err := os.WriteFile(configPath, []byte(fabricengine.ConfigTemplate()), 0o644); err != nil {
		t.Fatalf("write repo-wide fabric config: %v", err)
	}
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

// TestPrune_ApplyRemovesPortalAndLaunchers is the R6 fix regression guard: when the warp worktree
// is deleted by hand, apply Prune removes the orphaned weft — but before the fix it left the dead
// slug's portal junction dangling and its launcher directory behind forever, since no other verb
// ever revisits a pruned slug (Remove would have, but the pair is already gone).
func TestPrune_ApplyRemovesPortalAndLaunchers(t *testing.T) {
	t.Parallel()

	const slug = "prune-portal-r6"
	fixture := newFabricFixture(t)
	l := fixture.Layout
	topology := fabricengine.NewTopology(fabricengine.Config{})
	if _, err := topology.Add(l, slug, fabricengine.AddOptions{SkipPush: true}); err != nil {
		t.Fatalf("setup Add: %v", err)
	}

	warpPath := fabricengine.WorktreePath(l, slug)
	portalLink := fabricengine.PortalLink(l, slug)
	launcherDir := fabricengine.LauncherDir(l, slug)

	// Sanity: Add wired both; Lstat for the portal since it dangles once the
	// warp directory is gone.
	if _, err := os.Lstat(portalLink); err != nil {
		t.Fatalf("setup: portal link missing after Add: %v", err)
	}
	if _, err := os.Stat(launcherDir); err != nil {
		t.Fatalf("setup: launcher dir missing after Add: %v", err)
	}

	// Delete the warp by hand (bare removal, not `git worktree remove`) — the
	// live-reproduced R6 scenario, leaving a stale registration.
	if err := os.RemoveAll(warpPath); err != nil {
		t.Fatalf("remove warp dir: %v", err)
	}

	res, err := topology.Prune(l, true, false)
	if err != nil {
		t.Fatalf("Prune(apply=true): %v", err)
	}

	weftPath := fabricengine.WeftWorktreePath(l, slug)
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

// TestPrune_StaleRegistrationReportedOnce is the F2/F3 regression guard: when a warp worktree
// directory is deleted by a bare removal (leaving a stale git worktree registration) rather than
// `git worktree remove`, Pass 1 (missing registered warp) and Pass 2 (weft with no warp sibling)
// both see the same orphaned weft.
// Prune must report it exactly once so dry-run and --apply agree,
// and must not claim Removed=true when there is no weft directory to delete.
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
		warpPath := fabricengine.WorktreePath(l, slug)
		weftPath := fabricengine.WeftWorktreePath(l, slug)

		// Bare removal of the warp directory leaves the git worktree
		// registration stale (unlike `git worktree remove`), so both prune
		// passes see the orphan.
		if err := os.RemoveAll(warpPath); err != nil {
			t.Fatalf("bare-remove warp worktree: %v", err)
		}

		dry, err := topology.Prune(l, false, false)
		if err != nil {
			t.Fatalf("Prune(dry-run): %v", err)
		}
		if got := countPruneEntriesForWeft(dry.Entries, weftPath); got != 1 {
			t.Errorf("dry-run reported weft %s %d times; want exactly 1", weftPath, got)
		}

		apply, err := topology.Prune(l, true, false)
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
		warpPath := fabricengine.WorktreePath(l, slug)
		weftPath := fabricengine.WeftWorktreePath(l, slug)

		// Bare-remove BOTH sides, leaving both registrations stale. With the
		// weft directory gone, Pass 1's removeStalePair has nothing to
		// delete.
		if err := os.RemoveAll(warpPath); err != nil {
			t.Fatalf("bare-remove warp worktree: %v", err)
		}
		if err := os.RemoveAll(weftPath); err != nil {
			t.Fatalf("bare-remove weft worktree: %v", err)
		}

		apply, err := topology.Prune(l, true, false)
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

// TestCleanup_PrimaryBranchSurvivesForceWhenNotCheckedOut is the F1 regression guard: it moves the
// fabric weft primary OFF main-weft (exactly what a coordinated checkout of the primary to another
// branch produces) so main-weft is no longer the checked-out branch, then runs apply+force Cleanup.
// main-weft must still survive — protected by fabric's live-warp-branch logic, not merely by git's
// refusal to delete a checked-out branch.
// Before the fix, main-weft was deleted here.
func TestCleanup_PrimaryBranchSurvivesForceWhenNotCheckedOut(t *testing.T) {
	t.Parallel()

	fixture := newFabricFixture(t)
	l := fixture.Layout
	topology := fabricengine.NewTopology(fabricengine.Config{})

	mainWeft := fabricengine.WeftBranchName("main")
	weftPrime := fabricengine.WeftWorktree(l)

	// Move the weft primary off main-weft so main-weft is not the
	// checked-out branch.
	gitkit.MustRun(t, weftPrime, "git", "checkout", "-b", "primary-parked")

	res, err := topology.Cleanup(l, true, true)
	if err != nil {
		t.Fatalf("Cleanup(apply, force): %v", err)
	}

	for _, entry := range res.Entries {
		if entry.Branch == mainWeft {
			t.Errorf("Cleanup reported/handled primary weft branch %q; want not reported (live pair)", mainWeft)
		}
	}
	if !branchExistsAt(t, mustWeftRepoRoot(t, l), mainWeft) {
		t.Errorf("main-weft branch deleted after force Cleanup with primary parked elsewhere; want intact (F1 regression)")
	}
}

// TestCleanup_NonSuffixedBranchNeverDeleted covers a weft branch with no "-weft" suffix: it is not
// fabric-managed, whatever its origin;
// fabric must never delete it, even under apply+force.
func TestCleanup_NonSuffixedBranchNeverDeleted(t *testing.T) {
	t.Parallel()

	fixture := newFabricFixture(t)
	l := fixture.Layout
	topology := fabricengine.NewTopology(fabricengine.Config{})

	const warpManagedBranch = "cleanup-warp-owned"
	gitkit.MustRun(t, mustWeftRepoRoot(t, l), "git", "branch", warpManagedBranch, fabricengine.WeftBranchName("main"))

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
	if !branchExistsAt(t, mustWeftRepoRoot(t, l), warpManagedBranch) {
		t.Errorf("non-suffixed branch %q deleted; want intact", warpManagedBranch)
	}
}

// TestCleanup_DetachedWarpHeadProtectsCheckedOutWeftBranch is the R5 fix: a live pair whose warp
// worktree sits on a detached HEAD is invisible to the live-warp-branch liveness check (readBranch
// reports the literal "HEAD"), so before the fix Cleanup listed the pair's weft branch as a
// deletable orphan and apply+force attempted a git branch -D that only git's own checked-out
// refusal stopped.
// The checked-out protection must report the branch Protected in every mode instead.
func TestCleanup_DetachedWarpHeadProtectsCheckedOutWeftBranch(t *testing.T) {
	t.Parallel()

	const slug = "cleanup-detached-r5"
	fixture := newFabricFixture(t)
	l := fixture.Layout
	topology := fabricengine.NewTopology(fabricengine.Config{})
	if _, err := topology.Add(l, slug, fabricengine.AddOptions{SkipPush: true}); err != nil {
		t.Fatalf("setup Add: %v", err)
	}

	// Detach the warp worktree's HEAD so branch-space liveness cannot see the
	// pair is live; only the checked-out protection stands between Cleanup
	// and the pair's weft branch.
	warpPath := fabricengine.WorktreePath(l, slug)
	gitkit.MustRun(t, warpPath, "git", "checkout", "--detach")

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
	if !branchExistsAt(t, mustWeftRepoRoot(t, l), weftBranch) {
		t.Errorf("checked-out weft branch %q deleted; want intact", weftBranch)
	}
}

// TestHealthy_RealDirNotAJunction is the R10 fix: a real (non-link) directory sitting where the
// _lyx junction belongs must be reported as "warp _lyx is not a junction" — the wording
// checkJunctionHealth already uses for the same drift shape — not as "junction missing".
// The loom preflight will consume this reason string after cutover.
func TestHealthy_RealDirNotAJunction(t *testing.T) {
	t.Parallel()

	const slug = "pairinsync-realdir"
	fixture := newFabricFixture(t)
	l := fixture.Layout
	topology := fabricengine.NewTopology(fabricengine.Config{})
	if _, err := topology.Add(l, slug, fabricengine.AddOptions{SkipPush: true}); err != nil {
		t.Fatalf("setup Add: %v", err)
	}
	if err := fabricengine.WireJunctions(l, slug, []string{"_lyx", "_extra"}); err != nil {
		t.Fatalf("WireJunctions: %v", err)
	}

	warpLayout, err := lyxcwd.Resolve(fabricengine.WorktreePath(l, slug))
	if err != nil {
		t.Fatalf("lyxcwd.Resolve(warp): %v", err)
	}

	// Replace the junction with a real directory — the drift shape a
	// pre-weft repo migration or a hand-created _lyx leaves behind.
	warpLink := fabricengine.WarpLyxLinkHere(warpLayout)
	if err := os.Remove(warpLink); err != nil {
		t.Fatalf("remove warp junction: %v", err)
	}
	if err := os.Mkdir(warpLink, 0o755); err != nil {
		t.Fatalf("mkdir real dir in junction's place: %v", err)
	}

	ok, reason, err := fabricengine.Healthy(warpLayout)
	if err != nil {
		t.Fatalf("fabricengine.Healthy: %v", err)
	}
	if ok {
		t.Errorf("Healthy = true with a real _lyx directory; want false")
	}
	if reason.Cause != fabricengine.CauseNotAJunction || reason.Detail != "_lyx is not a junction" {
		t.Errorf("Healthy reason = %+v; want {Cause: %q, Detail: %q}", reason, fabricengine.CauseNotAJunction, "_lyx is not a junction")
	}
}

// TestReconcile_RecreatedWeftIsWiredInTheSamePass proves a single reconcile pass fully repairs a
// pair whose weft worktree was deleted out from under it. Recreating the worktree alone leaves the
// warp junctions pointing at directories that vanished with it, so the pair stayed unhealthy — with
// a raw EvalSymlinks error as its reported reason — until a SECOND reconcile ran.
func TestReconcile_RecreatedWeftIsWiredInTheSamePass(t *testing.T) {
	t.Setenv("WEFT_SKIP_PUSH", "1")

	const slug = "reconcile-recreated-pair"
	fixture := newFabricFixture(t)
	l := fixture.Layout
	topology := fabricengine.NewTopology(fabricengine.Config{})
	if _, err := topology.Add(l, slug, fabricengine.AddOptions{SkipPush: true}); err != nil {
		t.Fatalf("Add: %v", err)
	}

	warpLayout, err := lyxcwd.Resolve(fabricengine.WorktreePath(l, slug))
	if err != nil {
		t.Fatalf("lyxcwd.Resolve(warp): %v", err)
	}

	weftRepoRoot, err := fabricengine.WeftRepoRoot(l)
	if err != nil {
		t.Fatalf("WeftRepoRoot: %v", err)
	}
	weftPath := fabricengine.WeftWorktreePath(l, slug)
	gitkit.MustRun(t, weftRepoRoot, "git", "worktree", "remove", "--force", weftPath)

	result, err := topology.Reconcile(l)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	pair := findReconcilePair(t, result.Pairs, weftPath)
	if pair.Error != "" {
		t.Fatalf("Error = %q; want empty", pair.Error)
	}
	if pair.Action != fabricengine.ReconcileActionWeftRecreated {
		t.Errorf("Action = %q; want %q — the repair must not relabel what happened",
			pair.Action, fabricengine.ReconcileActionWeftRecreated)
	}

	ok, reason, err := fabricengine.Healthy(warpLayout)
	if err != nil {
		t.Fatalf("Healthy: %v", err)
	}
	if !ok {
		t.Errorf("Healthy = false (reason %+v) after ONE reconcile pass; want the pair fully repaired", reason)
	}
}

// TestCleanup_DryRunMatchesApplyVerdict proves a dry run answers the question a dry run is for:
// what the same flags plus --apply would actually do. An orphan weft branch is unprotected in both
// the dry run and the apply, and --apply alone (no --force) actually deletes it — an orphan weft
// branch is deletable under --apply alone, with no fold-back gate standing between it and deletion.
func TestCleanup_DryRunMatchesApplyVerdict(t *testing.T) {
	t.Setenv("WEFT_SKIP_PUSH", "1")

	const slug = "cleanup-dryrun-parity"
	fixture := newFabricFixture(t)
	l := fixture.Layout
	topology := fabricengine.NewTopology(fabricengine.Config{})
	if _, err := topology.Add(l, slug, fabricengine.AddOptions{SkipPush: true}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if _, err := topology.Remove(l, slug, true); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	// Remove deletes the pair's weft branch, so re-create an orphaned one by hand: a weft branch
	// whose paired warp branch no warp worktree is on.
	weftRepoRoot, err := fabricengine.WeftRepoRoot(l)
	if err != nil {
		t.Fatalf("WeftRepoRoot: %v", err)
	}
	orphan := fabricengine.WeftBranchName(slug)
	gitkit.MustRun(t, weftRepoRoot, "git", "branch", orphan)

	findEntry := func(t *testing.T, entries []fabricengine.CleanupBranchEntry) fabricengine.CleanupBranchEntry {
		t.Helper()
		for _, e := range entries {
			if e.Branch == orphan {
				return e
			}
		}
		t.Fatalf("cleanup result has no entry for %q: %+v", orphan, entries)
		return fabricengine.CleanupBranchEntry{}
	}

	dry, err := topology.Cleanup(l, false, false)
	if err != nil {
		t.Fatalf("Cleanup(dry): %v", err)
	}
	applied, err := topology.Cleanup(l, true, false)
	if err != nil {
		t.Fatalf("Cleanup(apply): %v", err)
	}

	dryEntry := findEntry(t, dry.Entries)
	appliedEntry := findEntry(t, applied.Entries)
	if dryEntry.Protected != appliedEntry.Protected {
		t.Errorf("dry-run Protected = %v; --apply Protected = %v; want them to agree",
			dryEntry.Protected, appliedEntry.Protected)
	}
	if dryEntry.Protected {
		t.Errorf("dry-run Protected = true for orphan branch %q; want false (no fold-back gate protects it)", orphan)
	}
	if !appliedEntry.Deleted {
		t.Fatalf("--apply did not delete %q without --force; want it deleted (no fold-back gate protects an orphan)", orphan)
	}
	if branchExistsAt(t, weftRepoRoot, orphan) {
		t.Errorf("orphan branch %q still exists in the weft repo after --apply; want deleted", orphan)
	}
}

// TestReconcile_RestoresDeletedPortalAndLaunchers proves Reconcile repairs the hub-level portal
// junction and launcher directory.
// Add creates both and Remove/Prune tear both down, so they are part of the managed topology — but
// nothing repaired them: a pair whose portal had been deleted was reported already_healthy forever
// and could only be recovered by removing and re-adding the pair, which a leftover portal link then
// blocked.
func TestReconcile_RestoresDeletedPortalAndLaunchers(t *testing.T) {
	t.Parallel()

	fixture := newFabricFixture(t)
	l := fixture.Layout
	const slug = "portal-repair"

	topology := fabricengine.NewTopology(fabricengine.Config{})
	if _, err := topology.Add(l, slug, fabricengine.AddOptions{SkipPush: true}); err != nil {
		t.Fatalf("Add(%q) error = %v", slug, err)
	}

	portalLink := fabricengine.PortalLink(l, slug)
	launcherDir := fabricengine.LauncherDir(l, slug)
	if _, err := os.Lstat(portalLink); err != nil {
		t.Fatalf("Add(%q) did not create the portal at %s: %v", slug, portalLink, err)
	}
	if err := os.Remove(portalLink); err != nil {
		t.Fatalf("delete portal: %v", err)
	}
	if err := os.RemoveAll(launcherDir); err != nil {
		t.Fatalf("delete launcher dir: %v", err)
	}

	result, err := topology.Reconcile(l)
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	var repaired *fabricengine.ReconcilePairResult
	for i := range result.Pairs {
		if filepath.Base(result.Pairs[i].WarpWorktree) == slug {
			repaired = &result.Pairs[i]
		}
	}
	if repaired == nil {
		t.Fatalf("Reconcile() reported no pair for slug %q", slug)
	}

	if repaired.Action != fabricengine.ReconcileActionPortalRestored {
		t.Errorf("Reconcile() action for %q = %q; want %q — a missing portal must not read as healthy",
			slug, repaired.Action, fabricengine.ReconcileActionPortalRestored)
	}
	if _, err := os.Lstat(portalLink); err != nil {
		t.Errorf("Reconcile() left the portal missing at %s: %v", portalLink, err)
	}
	if _, err := os.Stat(launcherDir); err != nil {
		t.Errorf("Reconcile() left the launcher dir missing at %s: %v", launcherDir, err)
	}
}
