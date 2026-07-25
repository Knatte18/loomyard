//go:build integration

// reconcile_differential_test.go runs warpengine's and fabricengine's Reconcile,
// Status, Prune, Cleanup, PairInSync, and HostClean back to back against twin
// lyxtest fixtures (via lifecycle_differential_test.go's buildDiffPair) and
// asserts equivalent end state, modulo fabric's suffixed weft-branch naming.
//
// Package fabricengine_test (external test package), reusing the single
// TestMain declared in testmain_test.go (package fabricengine) and the
// buildDiffPair/currentBranchOf/branchExistsAt helpers from
// lifecycle_differential_test.go (card 20), per the test-package-discipline
// decision.

package fabricengine_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fslink"
	"github.com/Knatte18/loomyard/internal/hubgeometry"
	"github.com/Knatte18/loomyard/internal/lyxtest"
	"github.com/Knatte18/loomyard/internal/warpengine"
)

// wireDiffPairJunctions wires the host _lyx junction for the prime worktree on
// both sides of dp, mirroring warp's setupReconcileFixture / setupStatusFixture
// / setupPruneFixture / setupCleanupFixture pattern so WireJunctions-dependent
// verbs (Reconcile, Status) have a real junction to inspect.
func wireDiffPairJunctions(t *testing.T, dp diffPair) {
	t.Helper()

	warpSlug := filepath.Base(dp.WarpFixture.Hub)
	fabricSlug := filepath.Base(dp.FabricFixture.Hub)
	if err := warpengine.WireJunctions(dp.WarpFixture.Layout, warpSlug); err != nil {
		t.Fatalf("warpengine.WireJunctions: %v", err)
	}
	if err := fabricengine.WireJunctions(dp.FabricFixture.Layout, fabricSlug); err != nil {
		t.Fatalf("fabricengine.WireJunctions: %v", err)
	}
}

// findWarpReconcilePair returns the ReconcilePairResult whose HostWorktree matches
// hostPath, failing the test if none is found.
func findWarpReconcilePair(t *testing.T, r warpengine.ReconcileResult, hostPath string) *warpengine.ReconcilePairResult {
	t.Helper()
	for i := range r.Pairs {
		if filepath.Clean(r.Pairs[i].HostWorktree) == filepath.Clean(hostPath) {
			return &r.Pairs[i]
		}
	}
	t.Fatalf("warpengine Reconcile(): no pair result for host worktree %s", hostPath)
	return nil
}

// findFabricReconcilePair returns the ReconcilePairResult whose HostWorktree matches
// hostPath, failing the test if none is found.
func findFabricReconcilePair(t *testing.T, r fabricengine.ReconcileResult, hostPath string) *fabricengine.ReconcilePairResult {
	t.Helper()
	for i := range r.Pairs {
		if filepath.Clean(r.Pairs[i].HostWorktree) == filepath.Clean(hostPath) {
			return &r.Pairs[i]
		}
	}
	t.Fatalf("fabricengine Reconcile(): no pair result for host worktree %s", hostPath)
	return nil
}

// TestReconcile_DifferentialEquivalence covers the already-healthy, weft-recreated,
// and junction-repointed Reconcile actions, asserting corresponding actions and
// equivalent end state between warpengine and fabricengine.
func TestReconcile_DifferentialEquivalence(t *testing.T) {
	t.Run("AlreadyHealthy", func(t *testing.T) {
		t.Parallel()

		dp := buildDiffPair(t, "")
		wireDiffPairJunctions(t, dp)

		warpRes, err := dp.Warp.Reconcile(dp.WarpFixture.Layout)
		if err != nil {
			t.Fatalf("warpengine Reconcile: %v", err)
		}
		fabricRes, err := dp.Fabric.Reconcile(dp.FabricFixture.Layout)
		if err != nil {
			t.Fatalf("fabricengine Reconcile: %v", err)
		}

		warpPair := findWarpReconcilePair(t, warpRes, dp.WarpFixture.Hub)
		fabricPair := findFabricReconcilePair(t, fabricRes, dp.FabricFixture.Hub)

		if warpPair.Action != warpengine.ReconcileActionAlreadyHealthy {
			t.Errorf("warp Action = %q; want %q", warpPair.Action, warpengine.ReconcileActionAlreadyHealthy)
		}
		if fabricPair.Action != fabricengine.ReconcileActionAlreadyHealthy {
			t.Errorf("fabric Action = %q; want %q", fabricPair.Action, fabricengine.ReconcileActionAlreadyHealthy)
		}
		if string(warpPair.Action) != string(fabricPair.Action) {
			t.Errorf("Action strings diverge: warp=%q fabric=%q; want equal", warpPair.Action, fabricPair.Action)
		}
	})

	t.Run("WeftRecreated", func(t *testing.T) {
		t.Parallel()

		const testSlug = "diff-reconcile-recreate"
		dp := buildDiffPair(t, "")
		wireDiffPairJunctions(t, dp)

		if _, err := dp.Warp.Add(dp.WarpFixture.Layout, testSlug, warpengine.AddOptions{SkipGit: true}); err != nil {
			t.Fatalf("setup warpengine Add: %v", err)
		}
		if _, err := dp.Fabric.Add(dp.FabricFixture.Layout, testSlug, fabricengine.AddOptions{SkipGit: true}); err != nil {
			t.Fatalf("setup fabricengine Add: %v", err)
		}

		warpWeftPath := dp.WarpFixture.Layout.WeftWorktreePath(testSlug)
		fabricWeftPath := dp.FabricFixture.Layout.WeftWorktreePath(testSlug)

		// Remove only the weft worktree directory on both sides (keeping the branch),
		// so Reconcile takes the recreate path.
		lyxtest.MustRun(t, dp.WarpFixture.Layout.WeftRepoRoot(), "git", "worktree", "remove", "--force", warpWeftPath)
		lyxtest.MustRun(t, dp.FabricFixture.Layout.WeftRepoRoot(), "git", "worktree", "remove", "--force", fabricWeftPath)

		warpRes, err := dp.Warp.Reconcile(dp.WarpFixture.Layout)
		if err != nil {
			t.Fatalf("warpengine Reconcile: %v", err)
		}
		fabricRes, err := dp.Fabric.Reconcile(dp.FabricFixture.Layout)
		if err != nil {
			t.Fatalf("fabricengine Reconcile: %v", err)
		}

		var warpFound *warpengine.ReconcilePairResult
		for i := range warpRes.Pairs {
			if filepath.Clean(warpRes.Pairs[i].WeftWorktree) == filepath.Clean(warpWeftPath) {
				warpFound = &warpRes.Pairs[i]
				break
			}
		}
		if warpFound == nil {
			t.Fatalf("warp: no pair result for weft path %s", warpWeftPath)
		}
		var fabricFound *fabricengine.ReconcilePairResult
		for i := range fabricRes.Pairs {
			if filepath.Clean(fabricRes.Pairs[i].WeftWorktree) == filepath.Clean(fabricWeftPath) {
				fabricFound = &fabricRes.Pairs[i]
				break
			}
		}
		if fabricFound == nil {
			t.Fatalf("fabric: no pair result for weft path %s", fabricWeftPath)
		}

		if warpFound.Action != warpengine.ReconcileActionWeftRecreated {
			t.Errorf("warp Action = %q; want %q", warpFound.Action, warpengine.ReconcileActionWeftRecreated)
		}
		if fabricFound.Action != fabricengine.ReconcileActionWeftRecreated {
			t.Errorf("fabric Action = %q; want %q", fabricFound.Action, fabricengine.ReconcileActionWeftRecreated)
		}
		if warpFound.Error != "" {
			t.Errorf("warp Error = %q; want empty", warpFound.Error)
		}
		if fabricFound.Error != "" {
			t.Errorf("fabric Error = %q; want empty", fabricFound.Error)
		}

		if _, err := os.Stat(warpWeftPath); err != nil {
			t.Errorf("warp weft worktree %s missing after Reconcile: %v", warpWeftPath, err)
		}
		if _, err := os.Stat(fabricWeftPath); err != nil {
			t.Errorf("fabric weft worktree %s missing after Reconcile: %v", fabricWeftPath, err)
		}

		// The deliberate delta: fabric recreates on the suffixed branch.
		warpWeftBranch := currentBranchOf(t, warpWeftPath)
		fabricWeftBranch := currentBranchOf(t, fabricWeftPath)
		if want := fabricengine.WeftBranchName(warpWeftBranch); fabricWeftBranch != want {
			t.Errorf("fabric recreated weft branch = %q; want %q", fabricWeftBranch, want)
		}
	})

	t.Run("JunctionRepointed", func(t *testing.T) {
		t.Parallel()

		dp := buildDiffPair(t, "")
		wireDiffPairJunctions(t, dp)

		warpSlug := filepath.Base(dp.WarpFixture.Hub)
		fabricSlug := filepath.Base(dp.FabricFixture.Hub)

		warpLink := dp.WarpFixture.Layout.HostLyxLink(warpSlug)
		fabricLink := dp.FabricFixture.Layout.HostLyxLink(fabricSlug)
		if err := fslink.Remove(warpLink); err != nil {
			t.Fatalf("remove warp junction: %v", err)
		}
		if err := fslink.Remove(fabricLink); err != nil {
			t.Fatalf("remove fabric junction: %v", err)
		}

		warpRes, err := dp.Warp.Reconcile(dp.WarpFixture.Layout)
		if err != nil {
			t.Fatalf("warpengine Reconcile: %v", err)
		}
		fabricRes, err := dp.Fabric.Reconcile(dp.FabricFixture.Layout)
		if err != nil {
			t.Fatalf("fabricengine Reconcile: %v", err)
		}

		warpPair := findWarpReconcilePair(t, warpRes, dp.WarpFixture.Hub)
		fabricPair := findFabricReconcilePair(t, fabricRes, dp.FabricFixture.Hub)

		if warpPair.Action != warpengine.ReconcileActionJunctionRepointed {
			t.Errorf("warp Action = %q; want %q", warpPair.Action, warpengine.ReconcileActionJunctionRepointed)
		}
		if fabricPair.Action != fabricengine.ReconcileActionJunctionRepointed {
			t.Errorf("fabric Action = %q; want %q", fabricPair.Action, fabricengine.ReconcileActionJunctionRepointed)
		}

		if isLink, _ := fslink.IsLink(warpLink); !isLink {
			t.Errorf("warp junction at %s not restored after Reconcile", warpLink)
		}
		if isLink, _ := fslink.IsLink(fabricLink); !isLink {
			t.Errorf("fabric junction at %s not restored after Reconcile", fabricLink)
		}
	})
}

// TestStatus_DifferentialEquivalence covers Status's InSync verdict on a healthy
// pair and a deliberately-drifted pair, asserting fabric's InSync is true exactly
// when warp's is, given corresponding fixtures.
func TestStatus_DifferentialEquivalence(t *testing.T) {
	t.Run("InSync", func(t *testing.T) {
		t.Parallel()

		dp := buildDiffPair(t, "")
		wireDiffPairJunctions(t, dp)

		warpRes, err := dp.Warp.Status(dp.WarpFixture.Layout)
		if err != nil {
			t.Fatalf("warpengine Status: %v", err)
		}
		fabricRes, err := dp.Fabric.Status(dp.FabricFixture.Layout)
		if err != nil {
			t.Fatalf("fabricengine Status: %v", err)
		}

		var warpPair *warpengine.PairStatus
		for i := range warpRes.Pairs {
			if filepath.Clean(warpRes.Pairs[i].HostWorktree) == filepath.Clean(dp.WarpFixture.Hub) {
				warpPair = &warpRes.Pairs[i]
				break
			}
		}
		var fabricPair *fabricengine.PairStatus
		for i := range fabricRes.Pairs {
			if filepath.Clean(fabricRes.Pairs[i].HostWorktree) == filepath.Clean(dp.FabricFixture.Hub) {
				fabricPair = &fabricRes.Pairs[i]
				break
			}
		}
		if warpPair == nil {
			t.Fatalf("warp: no pair for %s", dp.WarpFixture.Hub)
		}
		if fabricPair == nil {
			t.Fatalf("fabric: no pair for %s", dp.FabricFixture.Hub)
		}

		if !warpPair.InSync {
			t.Errorf("warp InSync = false; want true (DriftReason=%q)", warpPair.DriftReason)
		}
		if !fabricPair.InSync {
			t.Errorf("fabric InSync = false; want true (DriftReason=%q)", fabricPair.DriftReason)
		}
		if warpPair.InSync != fabricPair.InSync {
			t.Errorf("InSync diverges: warp=%v fabric=%v; want equal", warpPair.InSync, fabricPair.InSync)
		}
	})

	t.Run("Drifted", func(t *testing.T) {
		t.Parallel()

		dp := buildDiffPair(t, "")
		wireDiffPairJunctions(t, dp)

		// Move each side's weft prime to an unrelated branch, diverging it from the
		// host's branch. The literal branch name "drifted" is arbitrary and equally
		// applicable to both sides.
		lyxtest.MustRun(t, dp.WarpFixture.Layout.WeftWorktree(), "git", "checkout", "-b", "drifted")
		lyxtest.MustRun(t, dp.FabricFixture.Layout.WeftWorktree(), "git", "checkout", "-b", "drifted")

		warpRes, err := dp.Warp.Status(dp.WarpFixture.Layout)
		if err != nil {
			t.Fatalf("warpengine Status: %v", err)
		}
		fabricRes, err := dp.Fabric.Status(dp.FabricFixture.Layout)
		if err != nil {
			t.Fatalf("fabricengine Status: %v", err)
		}

		var warpPair *warpengine.PairStatus
		for i := range warpRes.Pairs {
			if filepath.Clean(warpRes.Pairs[i].HostWorktree) == filepath.Clean(dp.WarpFixture.Hub) {
				warpPair = &warpRes.Pairs[i]
				break
			}
		}
		var fabricPair *fabricengine.PairStatus
		for i := range fabricRes.Pairs {
			if filepath.Clean(fabricRes.Pairs[i].HostWorktree) == filepath.Clean(dp.FabricFixture.Hub) {
				fabricPair = &fabricRes.Pairs[i]
				break
			}
		}
		if warpPair == nil {
			t.Fatalf("warp: no pair for %s", dp.WarpFixture.Hub)
		}
		if fabricPair == nil {
			t.Fatalf("fabric: no pair for %s", dp.FabricFixture.Hub)
		}

		if warpPair.InSync {
			t.Errorf("warp InSync = true for drifted pair; want false")
		}
		if fabricPair.InSync {
			t.Errorf("fabric InSync = true for drifted pair; want false")
		}
		if warpPair.DriftReason == "" {
			t.Errorf("warp DriftReason empty for drifted pair; want non-empty")
		}
		if fabricPair.DriftReason == "" {
			t.Errorf("fabric DriftReason empty for drifted pair; want non-empty")
		}
	})
}

// TestPrune_DifferentialEquivalence covers Prune's stale-weft dry-run/apply cycle
// and its never-touch-a-live-pair guarantee, asserting equivalent behavior between
// warpengine and fabricengine.
func TestPrune_DifferentialEquivalence(t *testing.T) {
	t.Run("StaleWeft", func(t *testing.T) {
		t.Parallel()

		const testSlug = "diff-prune-stale"
		dp := buildDiffPair(t, "")
		wireDiffPairJunctions(t, dp)

		if _, err := dp.Warp.Add(dp.WarpFixture.Layout, testSlug, warpengine.AddOptions{SkipGit: true}); err != nil {
			t.Fatalf("setup warpengine Add: %v", err)
		}
		if _, err := dp.Fabric.Add(dp.FabricFixture.Layout, testSlug, fabricengine.AddOptions{SkipGit: true}); err != nil {
			t.Fatalf("setup fabricengine Add: %v", err)
		}

		warpHostPath := dp.WarpFixture.Layout.WorktreePath(testSlug)
		fabricHostPath := dp.FabricFixture.Layout.WorktreePath(testSlug)
		warpWeftPath := dp.WarpFixture.Layout.WeftWorktreePath(testSlug)
		fabricWeftPath := dp.FabricFixture.Layout.WeftWorktreePath(testSlug)

		// Remove only the host worktree on both sides, leaving the weft directory
		// orphaned. The host branch name is unaffected by fabric's suffix (only the
		// weft side is suffixed), so this step is identical on both sides.
		lyxtest.MustRun(t, dp.WarpFixture.Hub, "git", "worktree", "remove", "--force", warpHostPath)
		lyxtest.MustRun(t, dp.WarpFixture.Hub, "git", "branch", "-D", testSlug)
		lyxtest.MustRun(t, dp.FabricFixture.Hub, "git", "worktree", "remove", "--force", fabricHostPath)
		lyxtest.MustRun(t, dp.FabricFixture.Hub, "git", "branch", "-D", testSlug)

		// Dry-run: reported, not removed, on both sides.
		warpDry, err := dp.Warp.Prune(dp.WarpFixture.Layout, false)
		if err != nil {
			t.Fatalf("warpengine Prune(apply=false): %v", err)
		}
		fabricDry, err := dp.Fabric.Prune(dp.FabricFixture.Layout, false)
		if err != nil {
			t.Fatalf("fabricengine Prune(apply=false): %v", err)
		}

		warpDryEntry := findPruneEntryByWeftPath(t, warpDry.Entries, warpWeftPath)
		fabricDryEntry := findFabricPruneEntryByWeftPath(t, fabricDry.Entries, fabricWeftPath)
		if warpDryEntry.Removed || fabricDryEntry.Removed {
			t.Errorf("dry-run Removed: warp=%v fabric=%v; want both false", warpDryEntry.Removed, fabricDryEntry.Removed)
		}
		if _, err := os.Stat(warpWeftPath); err != nil {
			t.Errorf("warp weft dir removed by dry-run: %v", err)
		}
		if _, err := os.Stat(fabricWeftPath); err != nil {
			t.Errorf("fabric weft dir removed by dry-run: %v", err)
		}

		// Apply: removed on both sides.
		warpApply, err := dp.Warp.Prune(dp.WarpFixture.Layout, true)
		if err != nil {
			t.Fatalf("warpengine Prune(apply=true): %v", err)
		}
		fabricApply, err := dp.Fabric.Prune(dp.FabricFixture.Layout, true)
		if err != nil {
			t.Fatalf("fabricengine Prune(apply=true): %v", err)
		}

		warpApplyEntry := findPruneEntryByWeftPath(t, warpApply.Entries, warpWeftPath)
		fabricApplyEntry := findFabricPruneEntryByWeftPath(t, fabricApply.Entries, fabricWeftPath)
		if !warpApplyEntry.Removed {
			t.Errorf("warp Removed = false after apply; want true (error=%q)", warpApplyEntry.Error)
		}
		if !fabricApplyEntry.Removed {
			t.Errorf("fabric Removed = false after apply; want true (error=%q)", fabricApplyEntry.Error)
		}
		if _, err := os.Stat(warpWeftPath); !os.IsNotExist(err) {
			t.Errorf("warp weft dir %s still exists after apply Prune", warpWeftPath)
		}
		if _, err := os.Stat(fabricWeftPath); !os.IsNotExist(err) {
			t.Errorf("fabric weft dir %s still exists after apply Prune", fabricWeftPath)
		}
	})

	t.Run("LivePairNeverTouched", func(t *testing.T) {
		t.Parallel()

		const testSlug = "diff-prune-live"
		dp := buildDiffPair(t, "")
		wireDiffPairJunctions(t, dp)

		if _, err := dp.Warp.Add(dp.WarpFixture.Layout, testSlug, warpengine.AddOptions{SkipGit: true}); err != nil {
			t.Fatalf("setup warpengine Add: %v", err)
		}
		if _, err := dp.Fabric.Add(dp.FabricFixture.Layout, testSlug, fabricengine.AddOptions{SkipGit: true}); err != nil {
			t.Fatalf("setup fabricengine Add: %v", err)
		}

		warpWeftPath := dp.WarpFixture.Layout.WeftWorktreePath(testSlug)
		fabricWeftPath := dp.FabricFixture.Layout.WeftWorktreePath(testSlug)

		warpRes, err := dp.Warp.Prune(dp.WarpFixture.Layout, true)
		if err != nil {
			t.Fatalf("warpengine Prune(apply=true): %v", err)
		}
		fabricRes, err := dp.Fabric.Prune(dp.FabricFixture.Layout, true)
		if err != nil {
			t.Fatalf("fabricengine Prune(apply=true): %v", err)
		}

		for _, entry := range warpRes.Entries {
			if filepath.Clean(entry.WeftWorktree) == filepath.Clean(warpWeftPath) {
				t.Errorf("warp Prune reported live weft %s as stale", warpWeftPath)
			}
		}
		for _, entry := range fabricRes.Entries {
			if filepath.Clean(entry.WeftWorktree) == filepath.Clean(fabricWeftPath) {
				t.Errorf("fabric Prune reported live weft %s as stale", fabricWeftPath)
			}
		}
		if _, err := os.Stat(warpWeftPath); err != nil {
			t.Errorf("warp live weft %s deleted by Prune: %v", warpWeftPath, err)
		}
		if _, err := os.Stat(fabricWeftPath); err != nil {
			t.Errorf("fabric live weft %s deleted by Prune: %v", fabricWeftPath, err)
		}
	})

	// ApplyRemovesPortalAndLaunchers is fabric-only (the R6 fix): when the host
	// worktree is deleted by hand, apply Prune removes the orphaned weft — but
	// before the fix it left the dead slug's portal junction dangling and its
	// launcher directory behind forever, since no other verb ever revisits a
	// pruned slug (Remove would have, but the pair is already gone).
	t.Run("ApplyRemovesPortalAndLaunchers", func(t *testing.T) {
		t.Parallel()

		const testSlug = "diff-prune-portal"
		dp := buildDiffPair(t, "")
		wireDiffPairJunctions(t, dp)

		if _, err := dp.Fabric.Add(dp.FabricFixture.Layout, testSlug, fabricengine.AddOptions{SkipGit: true}); err != nil {
			t.Fatalf("setup fabricengine Add: %v", err)
		}

		hostPath := dp.FabricFixture.Layout.WorktreePath(testSlug)
		portalLink := dp.FabricFixture.Layout.PortalLink(testSlug)
		launcherDir := dp.FabricFixture.Layout.LauncherDir(testSlug)

		// Sanity: Add wired both; Lstat for the portal since it dangles once
		// the host directory is gone.
		if _, err := os.Lstat(portalLink); err != nil {
			t.Fatalf("setup: portal link missing after Add: %v", err)
		}
		if _, err := os.Stat(launcherDir); err != nil {
			t.Fatalf("setup: launcher dir missing after Add: %v", err)
		}

		// Delete the host by hand (bare removal, not `git worktree remove`) —
		// the live-reproduced R6 scenario, leaving a stale registration.
		if err := os.RemoveAll(hostPath); err != nil {
			t.Fatalf("remove host dir: %v", err)
		}

		res, err := dp.Fabric.Prune(dp.FabricFixture.Layout, true)
		if err != nil {
			t.Fatalf("fabricengine Prune(apply=true): %v", err)
		}

		weftPath := dp.FabricFixture.Layout.WeftWorktreePath(testSlug)
		entry := findFabricPruneEntryByWeftPath(t, res.Entries, weftPath)
		if !entry.Removed {
			t.Errorf("Removed = false after apply; want true (error=%q)", entry.Error)
		}
		if _, err := os.Lstat(portalLink); !os.IsNotExist(err) {
			t.Errorf("portal link %s still present after apply Prune; want removed", portalLink)
		}
		if _, err := os.Stat(launcherDir); !os.IsNotExist(err) {
			t.Errorf("launcher dir %s still present after apply Prune; want removed", launcherDir)
		}
	})
}

// findPruneEntryByWeftPath returns the warpengine.PruneEntry matching weftPath,
// failing the test if none is found.
func findPruneEntryByWeftPath(t *testing.T, entries []warpengine.PruneEntry, weftPath string) *warpengine.PruneEntry {
	t.Helper()
	for i := range entries {
		if filepath.Clean(entries[i].WeftWorktree) == filepath.Clean(weftPath) {
			return &entries[i]
		}
	}
	t.Fatalf("warpengine Prune: no entry for weft path %s", weftPath)
	return nil
}

// findFabricPruneEntryByWeftPath returns the fabricengine.PruneEntry matching
// weftPath, failing the test if none is found.
func findFabricPruneEntryByWeftPath(t *testing.T, entries []fabricengine.PruneEntry, weftPath string) *fabricengine.PruneEntry {
	t.Helper()
	for i := range entries {
		if filepath.Clean(entries[i].WeftWorktree) == filepath.Clean(weftPath) {
			return &entries[i]
		}
	}
	t.Fatalf("fabricengine Prune: no entry for weft path %s", weftPath)
	return nil
}

// TestPrune_StaleRegistrationReportedOnce is the F2/F3 regression guard (fabric-only,
// no warp comparison — warp shares the original double-count). When a host worktree
// directory is deleted by a bare removal (leaving a stale git worktree registration)
// rather than `git worktree remove`, Pass 1 (missing registered host) and Pass 2 (weft
// with no host sibling) both see the same orphaned weft. Prune must report it exactly
// once so dry-run and --apply agree, and must not claim Removed=true when there is no
// weft directory to delete.
func TestPrune_StaleRegistrationReportedOnce(t *testing.T) {
	t.Run("DoubleCountAvoided", func(t *testing.T) {
		t.Parallel()

		const slug = "diff-prune-stale-reg"
		dp := buildDiffPair(t, "")
		wireDiffPairJunctions(t, dp)

		if _, err := dp.Fabric.Add(dp.FabricFixture.Layout, slug, fabricengine.AddOptions{SkipGit: true}); err != nil {
			t.Fatalf("setup fabricengine Add: %v", err)
		}
		hostPath := dp.FabricFixture.Layout.WorktreePath(slug)
		weftPath := dp.FabricFixture.Layout.WeftWorktreePath(slug)

		// Bare removal of the host directory leaves the git worktree registration stale
		// (unlike `git worktree remove`), so both prune passes see the orphan.
		if err := os.RemoveAll(hostPath); err != nil {
			t.Fatalf("bare-remove host worktree: %v", err)
		}

		dry, err := dp.Fabric.Prune(dp.FabricFixture.Layout, false)
		if err != nil {
			t.Fatalf("fabricengine Prune(dry-run): %v", err)
		}
		if got := countPruneEntriesForWeft(dry.Entries, weftPath); got != 1 {
			t.Errorf("dry-run reported weft %s %d times; want exactly 1", weftPath, got)
		}

		apply, err := dp.Fabric.Prune(dp.FabricFixture.Layout, true)
		if err != nil {
			t.Fatalf("fabricengine Prune(apply): %v", err)
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

		const slug = "diff-prune-noweft"
		dp := buildDiffPair(t, "")
		wireDiffPairJunctions(t, dp)

		if _, err := dp.Fabric.Add(dp.FabricFixture.Layout, slug, fabricengine.AddOptions{SkipGit: true}); err != nil {
			t.Fatalf("setup fabricengine Add: %v", err)
		}
		hostPath := dp.FabricFixture.Layout.WorktreePath(slug)
		weftPath := dp.FabricFixture.Layout.WeftWorktreePath(slug)

		// Bare-remove BOTH sides, leaving both registrations stale. With the weft
		// directory gone, Pass 1's removeStalePair has nothing to delete.
		if err := os.RemoveAll(hostPath); err != nil {
			t.Fatalf("bare-remove host worktree: %v", err)
		}
		if err := os.RemoveAll(weftPath); err != nil {
			t.Fatalf("bare-remove weft worktree: %v", err)
		}

		apply, err := dp.Fabric.Prune(dp.FabricFixture.Layout, true)
		if err != nil {
			t.Fatalf("fabricengine Prune(apply): %v", err)
		}
		entry := findFabricPruneEntryByWeftPath(t, apply.Entries, weftPath)
		if entry.Removed {
			t.Errorf("Removed = true for a weft worktree that no longer existed; want false")
		}
		if entry.Error != "" {
			t.Errorf("unexpected error for missing weft dir: %q", entry.Error)
		}
	})
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

// TestCleanup_DifferentialEquivalence covers Cleanup's full dry-run/apply/force
// matrix, the accidental-but-real protection a currently-checked-out primary weft
// branch gets from git's own refusal (main on warp, main-weft on fabric — the
// "protected set... mapped through WeftBranchName" the batch's requirements
// describe), and live-pair branches never being reported.
func TestCleanup_DifferentialEquivalence(t *testing.T) {
	t.Run("DryRunReportsWithoutDeleting", func(t *testing.T) {
		t.Parallel()

		dp := buildDiffPair(t, "")
		wireDiffPairJunctions(t, dp)

		const warpOrphan = "diff-cleanup-orphan"
		lyxtest.MustRun(t, dp.WarpFixture.Layout.WeftRepoRoot(), "git", "branch", warpOrphan, "main")
		fabricOrphan := fabricengine.WeftBranchName(warpOrphan)
		lyxtest.MustRun(t, dp.FabricFixture.Layout.WeftRepoRoot(), "git", "branch", fabricOrphan, fabricengine.WeftBranchName("main"))

		warpRes, err := dp.Warp.Cleanup(dp.WarpFixture.Layout, false, false)
		if err != nil {
			t.Fatalf("warpengine Cleanup(dry-run): %v", err)
		}
		fabricRes, err := dp.Fabric.Cleanup(dp.FabricFixture.Layout, false, false)
		if err != nil {
			t.Fatalf("fabricengine Cleanup(dry-run): %v", err)
		}

		warpEntry := findWarpCleanupEntry(t, warpRes.Entries, warpOrphan)
		fabricEntry := findFabricCleanupEntry(t, fabricRes.Entries, fabricOrphan)
		if warpEntry.Deleted || fabricEntry.Deleted {
			t.Errorf("dry-run Deleted: warp=%v fabric=%v; want both false", warpEntry.Deleted, fabricEntry.Deleted)
		}
		if !branchExistsAt(t, dp.WarpFixture.Layout.WeftRepoRoot(), warpOrphan) {
			t.Errorf("warp orphan branch %q deleted by dry-run", warpOrphan)
		}
		if !branchExistsAt(t, dp.FabricFixture.Layout.WeftRepoRoot(), fabricOrphan) {
			t.Errorf("fabric orphan branch %q deleted by dry-run", fabricOrphan)
		}
	})

	t.Run("ApplyProtectsThenForceDeletes", func(t *testing.T) {
		t.Parallel()

		dp := buildDiffPair(t, "")
		wireDiffPairJunctions(t, dp)

		const warpOrphan = "diff-cleanup-force"
		lyxtest.MustRun(t, dp.WarpFixture.Layout.WeftRepoRoot(), "git", "branch", warpOrphan, "main")
		fabricOrphan := fabricengine.WeftBranchName(warpOrphan)
		lyxtest.MustRun(t, dp.FabricFixture.Layout.WeftRepoRoot(), "git", "branch", fabricOrphan, fabricengine.WeftBranchName("main"))

		// apply=true, force=false: gate-protected (raddleFoldedBack always false).
		warpProtected, err := dp.Warp.Cleanup(dp.WarpFixture.Layout, true, false)
		if err != nil {
			t.Fatalf("warpengine Cleanup(apply, no force): %v", err)
		}
		fabricProtected, err := dp.Fabric.Cleanup(dp.FabricFixture.Layout, true, false)
		if err != nil {
			t.Fatalf("fabricengine Cleanup(apply, no force): %v", err)
		}

		warpProtEntry := findWarpCleanupEntry(t, warpProtected.Entries, warpOrphan)
		fabricProtEntry := findFabricCleanupEntry(t, fabricProtected.Entries, fabricOrphan)
		if !warpProtEntry.Protected || warpProtEntry.Deleted {
			t.Errorf("warp entry: Protected=%v Deleted=%v; want Protected=true Deleted=false", warpProtEntry.Protected, warpProtEntry.Deleted)
		}
		if !fabricProtEntry.Protected || fabricProtEntry.Deleted {
			t.Errorf("fabric entry: Protected=%v Deleted=%v; want Protected=true Deleted=false", fabricProtEntry.Protected, fabricProtEntry.Deleted)
		}

		// apply=true, force=true: deleted on both sides.
		warpForced, err := dp.Warp.Cleanup(dp.WarpFixture.Layout, true, true)
		if err != nil {
			t.Fatalf("warpengine Cleanup(apply, force): %v", err)
		}
		fabricForced, err := dp.Fabric.Cleanup(dp.FabricFixture.Layout, true, true)
		if err != nil {
			t.Fatalf("fabricengine Cleanup(apply, force): %v", err)
		}

		warpForcedEntry := findWarpCleanupEntry(t, warpForced.Entries, warpOrphan)
		fabricForcedEntry := findFabricCleanupEntry(t, fabricForced.Entries, fabricOrphan)
		if !warpForcedEntry.Deleted {
			t.Errorf("warp Deleted = false after force; want true (error=%q)", warpForcedEntry.Error)
		}
		if !fabricForcedEntry.Deleted {
			t.Errorf("fabric Deleted = false after force; want true (error=%q)", fabricForcedEntry.Error)
		}
		if branchExistsAt(t, dp.WarpFixture.Layout.WeftRepoRoot(), warpOrphan) {
			t.Errorf("warp orphan branch %q still exists after force delete", warpOrphan)
		}
		if branchExistsAt(t, dp.FabricFixture.Layout.WeftRepoRoot(), fabricOrphan) {
			t.Errorf("fabric orphan branch %q still exists after force delete", fabricOrphan)
		}
	})

	// PrimaryBranchSurvivesForce asserts the primary pair's weft branch survives even
	// apply+force Cleanup — with a DELIBERATE divergence between the two modules.
	//
	// warp's primary weft branch "main" is unsuffixed, so WeftHostSlug rejects it and
	// warp reports it as an unmanaged (Protected), never-deleted branch. fabric's
	// primary weft branch "main-weft" IS suffixed; fabric protects it by recognising
	// that a host worktree (the primary) is on its paired host branch "main", so fabric
	// never even reports it as a Cleanup candidate. (Before the F1 fix, fabric misclassified
	// "main-weft" as a deletable orphan and relied only on git's refusal to delete a
	// checked-out branch — an accident that fails the moment the primary weft is moved off
	// main-weft; see PrimaryBranchSurvivesForceWhenNotCheckedOut below.)
	t.Run("PrimaryBranchSurvivesForce", func(t *testing.T) {
		t.Parallel()

		dp := buildDiffPair(t, "")
		wireDiffPairJunctions(t, dp)

		warpRes, err := dp.Warp.Cleanup(dp.WarpFixture.Layout, true, true)
		if err != nil {
			t.Fatalf("warpengine Cleanup(apply, force): %v", err)
		}
		fabricRes, err := dp.Fabric.Cleanup(dp.FabricFixture.Layout, true, true)
		if err != nil {
			t.Fatalf("fabricengine Cleanup(apply, force): %v", err)
		}

		// warp: "main" is reported (unmanaged/Protected) but never deleted.
		warpMainEntry := findWarpCleanupEntry(t, warpRes.Entries, "main")
		if warpMainEntry.Deleted {
			t.Errorf("warp main entry Deleted = true; want false")
		}

		// fabric: "main-weft" is a live pair (primary host on "main"), so it is never
		// reported as a Cleanup candidate at all.
		mainWeft := fabricengine.WeftBranchName("main")
		for _, entry := range fabricRes.Entries {
			if entry.Branch == mainWeft {
				t.Errorf("fabric Cleanup reported primary weft branch %q; want not reported (live pair)", mainWeft)
			}
		}

		if !branchExistsAt(t, dp.WarpFixture.Layout.WeftRepoRoot(), "main") {
			t.Errorf("warp main branch deleted; want intact")
		}
		if !branchExistsAt(t, dp.FabricFixture.Layout.WeftRepoRoot(), mainWeft) {
			t.Errorf("fabric main-weft branch deleted; want intact")
		}
	})

	// PrimaryBranchSurvivesForceWhenNotCheckedOut is the F1 regression guard: it moves
	// the fabric weft primary OFF main-weft (exactly what a coordinated checkout of the
	// primary to another branch produces) so main-weft is no longer the checked-out
	// branch, then runs apply+force Cleanup. main-weft must still survive — protected by
	// fabric's live-host-branch logic, not merely by git's refusal to delete a
	// checked-out branch. Before the fix, main-weft was deleted here.
	t.Run("PrimaryBranchSurvivesForceWhenNotCheckedOut", func(t *testing.T) {
		t.Parallel()

		dp := buildDiffPair(t, "")
		wireDiffPairJunctions(t, dp)

		mainWeft := fabricengine.WeftBranchName("main")
		weftPrime := dp.FabricFixture.Layout.WeftWorktree()

		// Move the weft primary off main-weft so main-weft is not the checked-out branch.
		lyxtest.MustRun(t, weftPrime, "git", "checkout", "-b", "diff-primary-parked")

		fabricRes, err := dp.Fabric.Cleanup(dp.FabricFixture.Layout, true, true)
		if err != nil {
			t.Fatalf("fabricengine Cleanup(apply, force): %v", err)
		}

		for _, entry := range fabricRes.Entries {
			if entry.Branch == mainWeft {
				t.Errorf("fabric Cleanup reported/handled primary weft branch %q; want not reported (live pair)", mainWeft)
			}
		}
		if !branchExistsAt(t, dp.FabricFixture.Layout.WeftRepoRoot(), mainWeft) {
			t.Errorf("fabric main-weft branch deleted after force Cleanup with primary parked elsewhere; want intact (F1 regression)")
		}
	})

	// LiveBranchNeverDeleted asserts that a live pair's weft branch is never even
	// reported by Cleanup, on either side.
	t.Run("LiveBranchNeverDeleted", func(t *testing.T) {
		t.Parallel()

		const liveSlug = "diff-cleanup-live"
		dp := buildDiffPair(t, "")
		wireDiffPairJunctions(t, dp)

		if _, err := dp.Warp.Add(dp.WarpFixture.Layout, liveSlug, warpengine.AddOptions{SkipGit: true}); err != nil {
			t.Fatalf("setup warpengine Add: %v", err)
		}
		if _, err := dp.Fabric.Add(dp.FabricFixture.Layout, liveSlug, fabricengine.AddOptions{SkipGit: true}); err != nil {
			t.Fatalf("setup fabricengine Add: %v", err)
		}

		warpRes, err := dp.Warp.Cleanup(dp.WarpFixture.Layout, true, true)
		if err != nil {
			t.Fatalf("warpengine Cleanup(apply, force): %v", err)
		}
		fabricRes, err := dp.Fabric.Cleanup(dp.FabricFixture.Layout, true, true)
		if err != nil {
			t.Fatalf("fabricengine Cleanup(apply, force): %v", err)
		}

		for _, entry := range warpRes.Entries {
			if entry.Branch == liveSlug {
				t.Errorf("warp Cleanup reported live branch %q; want not reported", liveSlug)
			}
		}
		fabricLiveWeft := fabricengine.WeftBranchName(liveSlug)
		for _, entry := range fabricRes.Entries {
			if entry.Branch == fabricLiveWeft {
				t.Errorf("fabric Cleanup reported live branch %q; want not reported", fabricLiveWeft)
			}
		}
		if !branchExistsAt(t, dp.WarpFixture.Layout.WeftRepoRoot(), liveSlug) {
			t.Errorf("warp live branch %q missing after Cleanup", liveSlug)
		}
		if !branchExistsAt(t, dp.FabricFixture.Layout.WeftRepoRoot(), fabricLiveWeft) {
			t.Errorf("fabric live branch %q missing after Cleanup", fabricLiveWeft)
		}
	})

	// NonSuffixedBranchNeverDeleted is fabric-only: it covers the delta Card 18
	// introduced (a weft branch with no "-weft" suffix is, during the
	// parallel-build period, a warp-managed branch sharing the same weft repo;
	// fabric must never delete it, even under apply+force).
	t.Run("NonSuffixedBranchNeverDeleted", func(t *testing.T) {
		t.Parallel()

		dp := buildDiffPair(t, "")
		wireDiffPairJunctions(t, dp)

		const warpManagedBranch = "diff-cleanup-warp-owned"
		lyxtest.MustRun(t, dp.FabricFixture.Layout.WeftRepoRoot(), "git", "branch", warpManagedBranch, fabricengine.WeftBranchName("main"))

		r, err := dp.Fabric.Cleanup(dp.FabricFixture.Layout, true, true)
		if err != nil {
			t.Fatalf("fabricengine Cleanup(apply, force): %v", err)
		}

		entry := findFabricCleanupEntry(t, r.Entries, warpManagedBranch)
		if !entry.Protected {
			t.Errorf("Protected = false for non-suffixed branch %q; want true (not fabric-managed)", warpManagedBranch)
		}
		if entry.Deleted {
			t.Errorf("Deleted = true for non-suffixed branch %q; want false even under force", warpManagedBranch)
		}
		if !branchExistsAt(t, dp.FabricFixture.Layout.WeftRepoRoot(), warpManagedBranch) {
			t.Errorf("non-suffixed branch %q deleted; want intact", warpManagedBranch)
		}
	})

	// DetachedHostHeadProtectsCheckedOutWeftBranch is fabric-only (the R5 fix):
	// a live pair whose host worktree sits on a detached HEAD is invisible to
	// the live-host-branch liveness check (readBranch reports the literal
	// "HEAD"), so before the fix Cleanup listed the pair's weft branch as a
	// deletable orphan and apply+force attempted a git branch -D that only
	// git's own checked-out refusal stopped. The checked-out protection must
	// report the branch Protected in every mode instead.
	t.Run("DetachedHostHeadProtectsCheckedOutWeftBranch", func(t *testing.T) {
		t.Parallel()

		const detachedSlug = "diff-cleanup-detached"
		dp := buildDiffPair(t, "")
		wireDiffPairJunctions(t, dp)

		if _, err := dp.Fabric.Add(dp.FabricFixture.Layout, detachedSlug, fabricengine.AddOptions{SkipGit: true}); err != nil {
			t.Fatalf("setup fabricengine Add: %v", err)
		}

		// Detach the host worktree's HEAD so branch-space liveness cannot see
		// the pair is live; only the checked-out protection stands between
		// Cleanup and the pair's weft branch.
		hostPath := dp.FabricFixture.Layout.WorktreePath(detachedSlug)
		lyxtest.MustRun(t, hostPath, "git", "checkout", "--detach")

		weftBranch := fabricengine.WeftBranchName(detachedSlug)

		// Dry-run must already report the branch protected, so dry-run and
		// apply agree about its fate.
		dry, err := dp.Fabric.Cleanup(dp.FabricFixture.Layout, false, false)
		if err != nil {
			t.Fatalf("fabricengine Cleanup(dry-run): %v", err)
		}
		dryEntry := findFabricCleanupEntry(t, dry.Entries, weftBranch)
		if !dryEntry.Protected {
			t.Errorf("dry-run Protected = false for checked-out weft branch %q; want true", weftBranch)
		}

		forced, err := dp.Fabric.Cleanup(dp.FabricFixture.Layout, true, true)
		if err != nil {
			t.Fatalf("fabricengine Cleanup(apply, force): %v", err)
		}
		forcedEntry := findFabricCleanupEntry(t, forced.Entries, weftBranch)
		if !forcedEntry.Protected || forcedEntry.Deleted {
			t.Errorf("apply+force entry: Protected=%v Deleted=%v; want Protected=true Deleted=false", forcedEntry.Protected, forcedEntry.Deleted)
		}
		if forcedEntry.Error != "" {
			t.Errorf("apply+force entry Error = %q; want empty (no doomed delete attempt)", forcedEntry.Error)
		}
		if !branchExistsAt(t, dp.FabricFixture.Layout.WeftRepoRoot(), weftBranch) {
			t.Errorf("checked-out weft branch %q deleted; want intact", weftBranch)
		}
	})
}

// findWarpCleanupEntry returns the warpengine.CleanupBranchEntry for branch,
// failing the test if none is found.
func findWarpCleanupEntry(t *testing.T, entries []warpengine.CleanupBranchEntry, branch string) *warpengine.CleanupBranchEntry {
	t.Helper()
	for i := range entries {
		if entries[i].Branch == branch {
			return &entries[i]
		}
	}
	t.Fatalf("warpengine Cleanup: no entry for branch %q; entries=%+v", branch, entries)
	return nil
}

// findFabricCleanupEntry returns the fabricengine.CleanupBranchEntry for branch,
// failing the test if none is found.
func findFabricCleanupEntry(t *testing.T, entries []fabricengine.CleanupBranchEntry, branch string) *fabricengine.CleanupBranchEntry {
	t.Helper()
	for i := range entries {
		if entries[i].Branch == branch {
			return &entries[i]
		}
	}
	t.Fatalf("fabricengine Cleanup: no entry for branch %q; entries=%+v", branch, entries)
	return nil
}

// TestPairInSyncAndHostClean_DifferentialEquivalence covers PairInSync on clean
// and drifted fixtures, and HostClean on clean and dirty fixtures, asserting
// equivalent verdicts between warpengine and fabricengine.
func TestPairInSyncAndHostClean_DifferentialEquivalence(t *testing.T) {
	t.Run("PairInSync_Clean", func(t *testing.T) {
		t.Parallel()

		const slug = "diff-pairinsync-clean"
		dp := buildDiffPair(t, "")

		if _, err := dp.Warp.Add(dp.WarpFixture.Layout, slug, warpengine.AddOptions{SkipPush: true}); err != nil {
			t.Fatalf("setup warpengine Add: %v", err)
		}
		if _, err := dp.Fabric.Add(dp.FabricFixture.Layout, slug, fabricengine.AddOptions{SkipPush: true}); err != nil {
			t.Fatalf("setup fabricengine Add: %v", err)
		}
		if err := warpengine.WireJunctions(dp.WarpFixture.Layout, slug); err != nil {
			t.Fatalf("warpengine.WireJunctions: %v", err)
		}
		if err := fabricengine.WireJunctions(dp.FabricFixture.Layout, slug); err != nil {
			t.Fatalf("fabricengine.WireJunctions: %v", err)
		}

		warpHostLayout, err := hubgeometry.Resolve(dp.WarpFixture.Layout.WorktreePath(slug))
		if err != nil {
			t.Fatalf("hubgeometry.Resolve(warp host): %v", err)
		}
		fabricHostLayout, err := hubgeometry.Resolve(dp.FabricFixture.Layout.WorktreePath(slug))
		if err != nil {
			t.Fatalf("hubgeometry.Resolve(fabric host): %v", err)
		}

		warpOK, warpReason, err := warpengine.PairInSync(warpHostLayout)
		if err != nil {
			t.Fatalf("warpengine.PairInSync: %v", err)
		}
		fabricOK, fabricReason, err := fabricengine.PairInSync(fabricHostLayout)
		if err != nil {
			t.Fatalf("fabricengine.PairInSync: %v", err)
		}

		if !warpOK {
			t.Errorf("warp PairInSync = (false, %q); want (true, \"\")", warpReason)
		}
		if !fabricOK {
			t.Errorf("fabric PairInSync = (false, %q); want (true, \"\")", fabricReason)
		}
	})

	t.Run("PairInSync_Drifted", func(t *testing.T) {
		t.Parallel()

		const slug = "diff-pairinsync-drift"
		dp := buildDiffPair(t, "")

		if _, err := dp.Warp.Add(dp.WarpFixture.Layout, slug, warpengine.AddOptions{SkipPush: true}); err != nil {
			t.Fatalf("setup warpengine Add: %v", err)
		}
		if _, err := dp.Fabric.Add(dp.FabricFixture.Layout, slug, fabricengine.AddOptions{SkipPush: true}); err != nil {
			t.Fatalf("setup fabricengine Add: %v", err)
		}
		if err := warpengine.WireJunctions(dp.WarpFixture.Layout, slug); err != nil {
			t.Fatalf("warpengine.WireJunctions: %v", err)
		}
		if err := fabricengine.WireJunctions(dp.FabricFixture.Layout, slug); err != nil {
			t.Fatalf("fabricengine.WireJunctions: %v", err)
		}

		warpHostPath := dp.WarpFixture.Layout.WorktreePath(slug)
		fabricHostPath := dp.FabricFixture.Layout.WorktreePath(slug)
		lyxtest.MustRun(t, warpHostPath, "git", "checkout", "-b", "diverge-test")
		lyxtest.MustRun(t, fabricHostPath, "git", "checkout", "-b", "diverge-test")

		warpHostLayout, err := hubgeometry.Resolve(warpHostPath)
		if err != nil {
			t.Fatalf("hubgeometry.Resolve(warp host): %v", err)
		}
		fabricHostLayout, err := hubgeometry.Resolve(fabricHostPath)
		if err != nil {
			t.Fatalf("hubgeometry.Resolve(fabric host): %v", err)
		}

		warpOK, warpReason, err := warpengine.PairInSync(warpHostLayout)
		if err != nil {
			t.Fatalf("warpengine.PairInSync: %v", err)
		}
		fabricOK, fabricReason, err := fabricengine.PairInSync(fabricHostLayout)
		if err != nil {
			t.Fatalf("fabricengine.PairInSync: %v", err)
		}

		if warpOK {
			t.Errorf("warp PairInSync = true for drifted pair; want false")
		}
		if fabricOK {
			t.Errorf("fabric PairInSync = true for drifted pair; want false")
		}
		if !strings.Contains(warpReason, "host on") {
			t.Errorf("warp PairInSync reason = %q; want branch-divergence message", warpReason)
		}
		if !strings.Contains(fabricReason, "host on") {
			t.Errorf("fabric PairInSync reason = %q; want branch-divergence message", fabricReason)
		}
	})

	// PairInSync_RealDirNotAJunction is fabric-only (the R10 fix): a real
	// (non-link) directory sitting where the _lyx junction belongs must be
	// reported as "host _lyx is not a junction" — the wording
	// checkJunctionHealth already uses for the same drift shape — not as
	// "junction missing". The loom preflight will consume this reason string
	// after cutover.
	t.Run("PairInSync_RealDirNotAJunction", func(t *testing.T) {
		t.Parallel()

		const slug = "diff-pairinsync-realdir"
		dp := buildDiffPair(t, "")

		if _, err := dp.Fabric.Add(dp.FabricFixture.Layout, slug, fabricengine.AddOptions{SkipPush: true}); err != nil {
			t.Fatalf("setup fabricengine Add: %v", err)
		}
		if err := fabricengine.WireJunctions(dp.FabricFixture.Layout, slug); err != nil {
			t.Fatalf("fabricengine.WireJunctions: %v", err)
		}

		fabricHostLayout, err := hubgeometry.Resolve(dp.FabricFixture.Layout.WorktreePath(slug))
		if err != nil {
			t.Fatalf("hubgeometry.Resolve(fabric host): %v", err)
		}

		// Replace the junction with a real directory — the drift shape a
		// pre-weft repo migration or a hand-created _lyx leaves behind.
		hostLink := fabricHostLayout.HostLyxLinkHere()
		if err := os.Remove(hostLink); err != nil {
			t.Fatalf("remove host junction: %v", err)
		}
		if err := os.Mkdir(hostLink, 0o755); err != nil {
			t.Fatalf("mkdir real dir in junction's place: %v", err)
		}

		ok, reason, err := fabricengine.PairInSync(fabricHostLayout)
		if err != nil {
			t.Fatalf("fabricengine.PairInSync: %v", err)
		}
		if ok {
			t.Errorf("PairInSync = true with a real _lyx directory; want false")
		}
		if reason != "host _lyx is not a junction" {
			t.Errorf("PairInSync reason = %q; want %q", reason, "host _lyx is not a junction")
		}
	})

	t.Run("HostClean_CleanAndDirty", func(t *testing.T) {
		t.Parallel()

		dp := buildDiffPair(t, "")

		warpClean, warpReason, err := warpengine.HostClean(dp.WarpFixture.Layout)
		if err != nil {
			t.Fatalf("warpengine.HostClean: %v", err)
		}
		fabricClean, fabricReason, err := fabricengine.HostClean(dp.FabricFixture.Layout)
		if err != nil {
			t.Fatalf("fabricengine.HostClean: %v", err)
		}
		if !warpClean {
			t.Errorf("warp HostClean = (false, %q); want (true, \"\") on a fresh fixture", warpReason)
		}
		if !fabricClean {
			t.Errorf("fabric HostClean = (false, %q); want (true, \"\") on a fresh fixture", fabricReason)
		}

		// Dirty both host worktrees with an untracked-only file — the strict-policy
		// case neither implementation's --untracked-files=no pre-Add check would catch.
		if err := os.WriteFile(filepath.Join(dp.WarpFixture.Hub, "scratch.txt"), []byte("stray\n"), 0o644); err != nil {
			t.Fatalf("write warp scratch.txt: %v", err)
		}
		if err := os.WriteFile(filepath.Join(dp.FabricFixture.Hub, "scratch.txt"), []byte("stray\n"), 0o644); err != nil {
			t.Fatalf("write fabric scratch.txt: %v", err)
		}

		warpDirty, warpDirtyReason, err := warpengine.HostClean(dp.WarpFixture.Layout)
		if err != nil {
			t.Fatalf("warpengine.HostClean (dirty): %v", err)
		}
		fabricDirty, fabricDirtyReason, err := fabricengine.HostClean(dp.FabricFixture.Layout)
		if err != nil {
			t.Fatalf("fabricengine.HostClean (dirty): %v", err)
		}
		if warpDirty {
			t.Errorf("warp HostClean = true after untracked file; want false (strict policy)")
		}
		if fabricDirty {
			t.Errorf("fabric HostClean = true after untracked file; want false (strict policy)")
		}
		if !strings.Contains(warpDirtyReason, "scratch.txt") {
			t.Errorf("warp HostClean reason = %q; want it to mention scratch.txt", warpDirtyReason)
		}
		if !strings.Contains(fabricDirtyReason, "scratch.txt") {
			t.Errorf("fabric HostClean reason = %q; want it to mention scratch.txt", fabricDirtyReason)
		}
	})
}
