//go:build integration

// reconcile_stale_removal_test.go covers batch 2's declarative convergence:
// Reconcile now converges every host worktree to the repo-wide `pathspec`
// (hubgeometry.BoardDir(Hub)'s fabric.yaml) in both directions — wiring a
// junction missing on disk (already-existing add-missing behavior) AND
// removing an on-disk junction absent from the repo-wide set
// (applyStaleRemoval, new this batch) — with a fail-closed guard when the
// repo-wide config cannot be loaded, and hub-reserved names (_board,
// _portals, _launchers, _raddle) permanently excluded from the sweep.
//
// It also proves the repo-wide-base regression: the sites card 7 migrated to
// RepoWiredNames (Healthy, Topology.Checkout, Topology.Remove, and
// transitively checkJunctionHealth via the add/no-op/stale-removal cases
// above) resolve the junction name-set from hubgeometry.BoardDir(Hub) alone
// — no per-pair weft-base fabric.yaml is ever seeded in this file.
//
// Package fabricengine_test to reuse newFabricFixture/seedRepoWideFabricConfig
// from reconcile_stale_registration_test.go; shares the single TestMain in
// testmain_test.go.

package fabricengine_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fslink"
	"github.com/Knatte18/loomyard/internal/hubgeometry"
	"github.com/Knatte18/loomyard/internal/lyxtest"
)

// findReconcilePair returns the ReconcilePairResult for weftPath, failing the
// test if none is found. Result paths are emitted forward-slashed, so the
// comparison normalizes weftPath the same way, mirroring the pattern
// established in reconcile_stale_registration_test.go and
// junction_pattern_integration_test.go.
func findReconcilePair(t *testing.T, pairs []fabricengine.ReconcilePairResult, weftPath string) *fabricengine.ReconcilePairResult {
	t.Helper()
	want := filepath.ToSlash(weftPath)
	for i := range pairs {
		if pairs[i].WeftWorktree == want {
			return &pairs[i]
		}
	}
	t.Fatalf("Reconcile result has no pair for weft path %s: %+v", weftPath, pairs)
	return nil
}

// TestReconcile_AddsMissingRemovesStaleNoOpsCorrect exercises all three
// per-junction convergence outcomes in a single pair: a missing desired
// junction (_pattern) is added, a stale on-disk junction absent from the
// repo-wide pathspec (_extra) is removed, and an already-correct junction
// (_lyx) is left untouched.
func TestReconcile_AddsMissingRemovesStaleNoOpsCorrect(t *testing.T) {
	t.Parallel()

	const slug = "stale-removal-add-remove-noop"
	fixture := newFabricFixture(t)
	l := fixture.Layout
	boardDir := hubgeometry.BoardDir(l.Hub)
	cfgPath := configengine.ConfigFile(boardDir, "fabric")

	// Narrow the repo-wide pathspec to "_lyx" alone before Add: batch 5 card
	// 20 makes Add eagerly wire the repo-wide pathspec it sees at call time
	// (RepoWiredNames(l)), so leaving the fixture's default "_lyx _pattern"
	// pathspec in place would have Add itself wire _pattern too, leaving
	// nothing for Reconcile's own add-missing branch to genuinely add below.
	if err := os.WriteFile(cfgPath, []byte("branch_prefix: \"\"\npathspec: _lyx\n"), 0o644); err != nil {
		t.Fatalf("narrow repo-wide pathspec: %v", err)
	}

	topology := fabricengine.NewTopology(fabricengine.Config{})
	if _, err := topology.Add(l, slug, fabricengine.AddOptions{SkipPush: true}); err != nil {
		t.Fatalf("setup Add: %v", err)
	}

	hostLayout, err := hubgeometry.Resolve(fabricengine.WorktreePath(l, slug))
	if err != nil {
		t.Fatalf("hubgeometry.Resolve(host): %v", err)
	}

	// Wire _extra (on disk but not in the repo-wide pathspec) alongside the
	// _lyx junction Add already wired above — this test's stale-removal case.
	if err := fabricengine.WireJunctions(hostLayout, slug, []string{"_lyx", "_extra"}); err != nil {
		t.Fatalf("setup WireJunctions: %v", err)
	}

	// Widen the repo-wide pathspec back to the default "_lyx _pattern" so
	// _pattern is genuinely missing on disk relative to the desired set
	// Reconcile is about to converge against — the add-missing shape this
	// test's name promises.
	if err := os.WriteFile(cfgPath, []byte("branch_prefix: \"\"\npathspec: _lyx _pattern\n"), 0o644); err != nil {
		t.Fatalf("widen repo-wide pathspec: %v", err)
	}

	result, err := topology.Reconcile(l)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	weftPath := l.WeftWorktreePath(slug)
	pair := findReconcilePair(t, result.Pairs, weftPath)
	if pair.Error != "" {
		t.Errorf("Error = %q; want empty", pair.Error)
	}

	// Add-missing: _pattern was absent on disk and present in the desired
	// set, so it is wired.
	patternLink := hostLayout.HostPatternLinkHere()
	if isLink, err := fslink.IsLink(patternLink); err != nil || !isLink {
		t.Errorf("missing _pattern junction not added by Reconcile: isLink=%v err=%v", isLink, err)
	}

	// Stale-removal: _extra was on disk but absent from the desired set, so
	// it is removed.
	extraLink := filepath.Join(hostLayout.WorktreeRoot, "_extra")
	if _, statErr := os.Lstat(extraLink); !os.IsNotExist(statErr) {
		t.Errorf("stale _extra junction %s not removed by Reconcile; want removed", extraLink)
	}
	if !strings.Contains(pair.Detail, "_extra") {
		t.Errorf("Detail = %q; want it to name the removed stale junction _extra", pair.Detail)
	}

	// No-op: _lyx was already correct and desired, so it is left untouched by
	// either step.
	lyxLink := hostLayout.HostLyxLinkHere()
	if isLink, err := fslink.IsLink(lyxLink); err != nil || !isLink {
		t.Errorf("_lyx junction broken by Reconcile: isLink=%v err=%v", isLink, err)
	}
}

// TestReconcile_CorrectJunctionsAreNoOp asserts that a pair whose on-disk
// junctions already exactly match the repo-wide pathspec produces a true
// no-op: ReconcileActionAlreadyHealthy with an empty Detail, not a spurious
// ReconcileActionStaleRemoved from an empty stale set.
func TestReconcile_CorrectJunctionsAreNoOp(t *testing.T) {
	t.Parallel()

	const slug = "stale-removal-correct-noop"
	fixture := newFabricFixture(t)
	l := fixture.Layout
	topology := fabricengine.NewTopology(fabricengine.Config{})
	if _, err := topology.Add(l, slug, fabricengine.AddOptions{SkipPush: true}); err != nil {
		t.Fatalf("setup Add: %v", err)
	}
	hostLayout, err := hubgeometry.Resolve(fabricengine.WorktreePath(l, slug))
	if err != nil {
		t.Fatalf("hubgeometry.Resolve(host): %v", err)
	}
	if err := fabricengine.WireJunctions(hostLayout, slug, []string{"_lyx", "_pattern"}); err != nil {
		t.Fatalf("setup WireJunctions: %v", err)
	}

	result, err := topology.Reconcile(l)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	pair := findReconcilePair(t, result.Pairs, l.WeftWorktreePath(slug))
	if pair.Action != fabricengine.ReconcileActionAlreadyHealthy {
		t.Errorf("Action = %q; want %q (fully correct pair, nothing to converge)", pair.Action, fabricengine.ReconcileActionAlreadyHealthy)
	}
	if pair.Detail != "" {
		t.Errorf("Detail = %q; want empty for a true no-op", pair.Detail)
	}
}

// TestReconcile_ConvergesAllWorktreesToRepoWidePathspec proves the "repo-wide,
// not per-pair" property: two independently wired pairs both gain a new,
// non-reserved junction name after the repo-wide pathspec is widened,
// without either pair's own fabric.yaml (which does not exist) ever being
// consulted.
func TestReconcile_ConvergesAllWorktreesToRepoWidePathspec(t *testing.T) {
	t.Parallel()

	fixture := newFabricFixture(t)
	l := fixture.Layout
	topology := fabricengine.NewTopology(fabricengine.Config{})

	slugs := []string{"converge-all-a", "converge-all-b"}
	for _, slug := range slugs {
		if _, err := topology.Add(l, slug, fabricengine.AddOptions{SkipPush: true}); err != nil {
			t.Fatalf("setup Add(%s): %v", slug, err)
		}
	}

	// First Reconcile: Add's own eager wiring (RepoWiredNames, card 20)
	// already wired the default "_lyx _pattern" pathspec on both pairs at
	// Add time, so this call converges to a no-op here — included for
	// realism (a pair reconciled before ever being widened), not because it
	// changes anything on disk. The genuine add-missing convergence this
	// test proves is the second Reconcile call below, against the widened
	// pathspec.
	if _, err := topology.Reconcile(l); err != nil {
		t.Fatalf("Reconcile (initial): %v", err)
	}

	// Widen the repo-wide pathspec with a third, non-reserved name. _raddle
	// is reserved (hubgeometry.HubReservedNames), so a custom name is used
	// instead, per the batch's own test-plan wording.
	boardDir := hubgeometry.BoardDir(l.Hub)
	cfgPath := configengine.ConfigFile(boardDir, "fabric")
	if err := os.WriteFile(cfgPath, []byte("branch_prefix: \"\"\npathspec: _lyx _pattern _extra\n"), 0o644); err != nil {
		t.Fatalf("widen repo-wide pathspec: %v", err)
	}

	if _, err := topology.Reconcile(l); err != nil {
		t.Fatalf("Reconcile (converge): %v", err)
	}

	for _, slug := range slugs {
		hostLayout, err := hubgeometry.Resolve(fabricengine.WorktreePath(l, slug))
		if err != nil {
			t.Fatalf("hubgeometry.Resolve(%s): %v", slug, err)
		}
		extraLink := filepath.Join(hostLayout.WorktreeRoot, "_extra")
		if isLink, err := fslink.IsLink(extraLink); err != nil || !isLink {
			t.Errorf("worktree %s did not gain _extra junction after widening the repo-wide pathspec: isLink=%v err=%v", slug, isLink, err)
		}
	}
}

// TestReconcile_StaleRemovalFailsClosedOnUnparseableRepoWideConfig proves
// the fail-closed guard: when the repo-wide fabric.yaml cannot be loaded,
// Reconcile strips no junction and records the abort rather than treating
// the load failure as an empty (blanket-sweep) pathspec.
func TestReconcile_StaleRemovalFailsClosedOnUnparseableRepoWideConfig(t *testing.T) {
	t.Parallel()

	const slug = "stale-removal-failclosed"
	fixture := newFabricFixture(t)
	l := fixture.Layout
	topology := fabricengine.NewTopology(fabricengine.Config{})
	if _, err := topology.Add(l, slug, fabricengine.AddOptions{SkipPush: true}); err != nil {
		t.Fatalf("setup Add: %v", err)
	}
	hostLayout, err := hubgeometry.Resolve(fabricengine.WorktreePath(l, slug))
	if err != nil {
		t.Fatalf("hubgeometry.Resolve(host): %v", err)
	}
	if err := fabricengine.WireJunctions(hostLayout, slug, []string{"_lyx", "_pattern"}); err != nil {
		t.Fatalf("setup WireJunctions: %v", err)
	}

	// Corrupt the repo-wide fabric.yaml into unparseable YAML.
	boardDir := hubgeometry.BoardDir(l.Hub)
	cfgPath := configengine.ConfigFile(boardDir, "fabric")
	if err := os.WriteFile(cfgPath, []byte("not-valid-yaml: [unterminated"), 0o644); err != nil {
		t.Fatalf("corrupt repo-wide config: %v", err)
	}

	result, err := topology.Reconcile(l)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	pair := findReconcilePair(t, result.Pairs, l.WeftWorktreePath(slug))
	combined := pair.Detail + pair.Error
	if !strings.Contains(combined, "fabric.yaml") {
		t.Errorf("Detail=%q Error=%q; want the load failure recorded (fail-closed abort), not silently ignored", pair.Detail, pair.Error)
	}

	// The load failure must never be interpreted as an empty pathspec: every
	// junction wired before the corruption must still be present.
	lyxLink := hostLayout.HostLyxLinkHere()
	if isLink, err := fslink.IsLink(lyxLink); err != nil || !isLink {
		t.Errorf("_lyx junction removed despite fail-closed guard: isLink=%v err=%v", isLink, err)
	}
	patternLink := hostLayout.HostPatternLinkHere()
	if isLink, err := fslink.IsLink(patternLink); err != nil || !isLink {
		t.Errorf("_pattern junction removed despite fail-closed guard: isLink=%v err=%v", isLink, err)
	}
}

// TestReconcile_NeverRemovesReservedHubName proves scanOnDiskJunctionNames'
// exclusion of hubgeometry.HubReservedNames() holds end-to-end through
// Reconcile: a hub-structural name present on disk (here _raddle) is never
// swept even though it is absent from the repo-wide pathspec.
func TestReconcile_NeverRemovesReservedHubName(t *testing.T) {
	t.Parallel()

	const slug = "stale-removal-reserved"
	fixture := newFabricFixture(t)
	l := fixture.Layout
	topology := fabricengine.NewTopology(fabricengine.Config{})
	if _, err := topology.Add(l, slug, fabricengine.AddOptions{SkipPush: true}); err != nil {
		t.Fatalf("setup Add: %v", err)
	}
	hostLayout, err := hubgeometry.Resolve(fabricengine.WorktreePath(l, slug))
	if err != nil {
		t.Fatalf("hubgeometry.Resolve(host): %v", err)
	}
	if err := fabricengine.WireJunctions(hostLayout, slug, []string{"_lyx", "_pattern"}); err != nil {
		t.Fatalf("setup WireJunctions: %v", err)
	}

	// Seed a hub-structural reserved name directly on disk, as a link, under
	// the host worktree root — never wired via WireJunctions (which never
	// takes a reserved name), matching how a hub-reserved passenger like
	// _raddle would actually appear on disk.
	reservedLink := filepath.Join(hostLayout.WorktreeRoot, hostLayout.RelPath, "_raddle")
	if err := fslink.CreateDirLink(reservedLink, t.TempDir()); err != nil {
		t.Fatalf("seed reserved link: %v", err)
	}

	if _, err := topology.Reconcile(l); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	if isLink, err := fslink.IsLink(reservedLink); err != nil || !isLink {
		t.Errorf("reserved _raddle link removed by stale-removal; want untouched: isLink=%v err=%v", isLink, err)
	}
}

// TestRepoWideMigratedSites_ResolveFromBoardDirWithNoPerPairConfig is the
// repo-wide-base regression coverage card 10 calls for: with ONLY the
// repo-wide fabric.yaml seeded (no per-pair weft-base fabric.yaml exists
// anywhere in this fixture), the four sites card 7 migrated to
// RepoWiredNames still function — Healthy returns a real health verdict
// rather than "junction check unavailable", Topology.Checkout succeeds
// rather than hard-failing/rolling back on a name-set load error, and
// Topology.Remove still tears down junctions.
func TestRepoWideMigratedSites_ResolveFromBoardDirWithNoPerPairConfig(t *testing.T) {
	t.Parallel()

	fixture := lyxtest.CopyPairedLocal(t)
	// Deliberately skip lyxtest.SeedConfig at fixture.WeftPrime — the point
	// of this test is that no per-pair fabric.yaml exists, mirroring the new
	// clone flow where only the repo-wide config is ever materialized.
	seedRepoWideFabricConfig(t, fixture.Layout.Hub)
	lyxtest.MustRun(t, fixture.WeftPrime, "git", "checkout", "-b", fabricengine.WeftBranchName("main"))

	l := fixture.Layout
	topology := fabricengine.NewTopology(fabricengine.Config{})
	slug := filepath.Base(fixture.Hub)

	if err := fabricengine.WireJunctions(l, slug, []string{"_lyx", "_pattern"}); err != nil {
		t.Fatalf("WireJunctions(primary): %v", err)
	}

	// Healthy must resolve the name-set from the repo-wide BoardDir base
	// and report a real verdict, not the "unavailable" degraded reason a
	// per-pair-weft-base read would produce here (no such file exists).
	ok, reason, err := fabricengine.Healthy(l)
	if err != nil {
		t.Fatalf("Healthy: %v", err)
	}
	if !ok {
		t.Errorf("Healthy ok = false (reason %q); want true (repo-wide-only config)", reason)
	}
	if strings.Contains(reason, "unavailable") {
		t.Errorf("Healthy reason = %q; want a real verdict, not junction-check-unavailable", reason)
	}

	// Topology.Checkout must not hard-fail/rollback re-pointing junctions
	// after the branch switch.
	lyxtest.MustRun(t, l.WorktreeRoot, "git", "branch", "repo-wide-checkout-target")
	if _, err := topology.Checkout(l, "repo-wide-checkout-target"); err != nil {
		t.Errorf("Checkout with repo-wide-only config = %v; want success", err)
	}

	// Topology.Remove must still tear down a pair's junctions using the
	// repo-wide name-set.
	const removeSlug = "repo-wide-remove-target"
	if _, err := topology.Add(l, removeSlug, fabricengine.AddOptions{SkipPush: true}); err != nil {
		t.Fatalf("setup Add(%s): %v", removeSlug, err)
	}
	removeHostLayout, err := hubgeometry.Resolve(fabricengine.WorktreePath(l, removeSlug))
	if err != nil {
		t.Fatalf("hubgeometry.Resolve(%s): %v", removeSlug, err)
	}
	if err := fabricengine.WireJunctions(removeHostLayout, removeSlug, []string{"_lyx", "_pattern"}); err != nil {
		t.Fatalf("WireJunctions(%s): %v", removeSlug, err)
	}
	lyxLink := removeHostLayout.HostLyxLinkHere()
	patternLink := removeHostLayout.HostPatternLinkHere()

	if _, err := topology.Remove(l, removeSlug, true); err != nil {
		t.Fatalf("Remove(%s): %v", removeSlug, err)
	}
	if _, statErr := os.Lstat(lyxLink); !os.IsNotExist(statErr) {
		t.Errorf("_lyx junction %s still exists after Remove", lyxLink)
	}
	if _, statErr := os.Lstat(patternLink); !os.IsNotExist(statErr) {
		t.Errorf("_pattern junction %s still exists after Remove", patternLink)
	}
}
