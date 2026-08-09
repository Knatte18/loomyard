//go:build integration

// reconcile_stale_removal_test.go covers batch 2's declarative convergence:
// Reconcile now converges every warp worktree to the repo-wide `pathspec`
// (fabricengine.BoardDir(Hub)'s fabric.yaml) in both directions — wiring a
// junction missing on disk (already-existing add-missing behavior) AND
// removing an on-disk junction absent from the repo-wide set
// (applyStaleRemoval, new this batch) — with a fail-closed guard when the
// repo-wide config cannot be loaded, and hub-reserved names (_board,
// _portals, _launchers) permanently excluded from the sweep.
//
// It also proves the repo-wide-base regression: the sites card 7 migrated to
// RepoWiredNames (Healthy, Topology.Checkout, Topology.Remove, and
// transitively checkJunctionHealth via the add/no-op/stale-removal cases
// above) resolve the junction name-set from fabricengine.BoardDir(Hub) alone
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
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
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

// TestReconcile_AddsMissingRemovesStaleNoOpsCorrect exercises all three per-junction convergence
// outcomes in a single pair: a missing desired junction (_extra) is added, a stale on-disk
// junction absent from the repo-wide pathspec (_other) is removed, and an already-correct junction
// (_lyx) is left untouched.
func TestReconcile_AddsMissingRemovesStaleNoOpsCorrect(t *testing.T) {
	t.Parallel()

	const slug = "stale-removal-add-remove-noop"
	fixture := newFabricFixture(t)
	l := fixture.Layout
	boardDir := fabricengine.BoardDir(l.HubPath)
	cfgPath := configengine.ConfigFile(boardDir, "fabric")

	// Narrow the repo-wide pathspec to "_lyx" alone before Add: batch 5 card
	// 20 makes Add eagerly wire the repo-wide pathspec it sees at call time
	// (RepoWiredNames(l)), so leaving the fixture's default empty pathspec in
	// place would still leave Add wiring only the structural set — but this
	// explicit "_lyx" write keeps the setup self-documenting and immune to a
	// future default-pathspec change.
	if err := os.WriteFile(cfgPath, []byte("branch_prefix: \"\"\npathspec: _lyx\n"), 0o644); err != nil {
		t.Fatalf("narrow repo-wide pathspec: %v", err)
	}

	topology := fabricengine.NewTopology(fabricengine.Config{})
	if _, err := topology.Add(l, slug, fabricengine.AddOptions{SkipPush: true}); err != nil {
		t.Fatalf("setup Add: %v", err)
	}

	warpLayout, err := lyxcwd.Resolve(fabricengine.WorktreePath(l, slug))
	if err != nil {
		t.Fatalf("lyxcwd.Resolve(warp): %v", err)
	}

	// Wire _other (on disk but not in the repo-wide pathspec) alongside the
	// _lyx junction Add already wired above — this test's stale-removal case.
	if err := fabricengine.WireJunctions(warpLayout, slug, []string{"_lyx", "_other"}); err != nil {
		t.Fatalf("setup WireJunctions: %v", err)
	}

	// Widen the repo-wide pathspec to "_lyx _extra" so _extra is genuinely
	// missing on disk relative to the desired set Reconcile is about to
	// converge against — the add-missing shape this test's name promises.
	if err := os.WriteFile(cfgPath, []byte("branch_prefix: \"\"\npathspec: _lyx _extra\n"), 0o644); err != nil {
		t.Fatalf("widen repo-wide pathspec: %v", err)
	}

	result, err := topology.Reconcile(l)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	weftPath := fabricengine.WeftWorktreePath(l, slug)
	pair := findReconcilePair(t, result.Pairs, weftPath)
	if pair.Error != "" {
		t.Errorf("Error = %q; want empty", pair.Error)
	}

	// Add-missing: _extra was absent on disk and present in the desired
	// set, so it is wired.
	extraLink := filepath.Join(warpLayout.WorktreePath(), warpLayout.AnchorRel, "_extra")
	if isLink, err := fslink.IsLink(extraLink); err != nil || !isLink {
		t.Errorf("missing _extra junction not added by Reconcile: isLink=%v err=%v", isLink, err)
	}

	// Stale-removal: _other was on disk but absent from the desired set, so
	// it is removed.
	otherLink := filepath.Join(warpLayout.WorktreePath(), "_other")
	if _, statErr := os.Lstat(otherLink); !os.IsNotExist(statErr) {
		t.Errorf("stale _other junction %s not removed by Reconcile; want removed", otherLink)
	}
	if !strings.Contains(pair.Detail, "_other") {
		t.Errorf("Detail = %q; want it to name the removed stale junction _other", pair.Detail)
	}

	// No-op: _lyx was already correct and desired, so it is left untouched by
	// either step.
	lyxLink := fabricengine.WarpLyxLinkHere(warpLayout)
	if isLink, err := fslink.IsLink(lyxLink); err != nil || !isLink {
		t.Errorf("_lyx junction broken by Reconcile: isLink=%v err=%v", isLink, err)
	}
}

// TestReconcile_CorrectJunctionsAreNoOp asserts that a pair whose on-disk junctions already exactly
// match the repo-wide pathspec produces a true no-op: ReconcileActionAlreadyHealthy with an empty
// Detail, not a spurious ReconcileActionStaleRemoved from an empty stale set.
func TestReconcile_CorrectJunctionsAreNoOp(t *testing.T) {
	t.Parallel()

	const slug = "stale-removal-correct-noop"
	fixture := newFabricFixture(t)
	l := fixture.Layout
	topology := fabricengine.NewTopology(fabricengine.Config{})
	if _, err := topology.Add(l, slug, fabricengine.AddOptions{SkipPush: true}); err != nil {
		t.Fatalf("setup Add: %v", err)
	}
	warpLayout, err := lyxcwd.Resolve(fabricengine.WorktreePath(l, slug))
	if err != nil {
		t.Fatalf("lyxcwd.Resolve(warp): %v", err)
	}
	if err := fabricengine.WireJunctions(warpLayout, slug, []string{"_lyx", "_extra"}); err != nil {
		t.Fatalf("setup WireJunctions: %v", err)
	}

	// Widen the repo-wide pathspec to match exactly what is wired on disk
	// above: the fixture's default empty pathspec alone would make _extra
	// stale, so this explicit widening is what keeps this a genuine no-op
	// rather than a stale-removal case.
	boardDir := fabricengine.BoardDir(l.HubPath)
	cfgPath := configengine.ConfigFile(boardDir, "fabric")
	if err := os.WriteFile(cfgPath, []byte("branch_prefix: \"\"\npathspec: _lyx _extra\n"), 0o644); err != nil {
		t.Fatalf("widen repo-wide pathspec: %v", err)
	}

	result, err := topology.Reconcile(l)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	pair := findReconcilePair(t, result.Pairs, fabricengine.WeftWorktreePath(l, slug))
	if pair.Action != fabricengine.ReconcileActionAlreadyHealthy {
		t.Errorf("Action = %q; want %q (fully correct pair, nothing to converge)", pair.Action, fabricengine.ReconcileActionAlreadyHealthy)
	}
	if pair.Detail != "" {
		t.Errorf("Detail = %q; want empty for a true no-op", pair.Detail)
	}
}

// TestReconcile_ConvergesAllWorktreesToRepoWidePathspec proves the "repo-wide, not per-pair"
// property: two independently wired pairs both gain a new, non-reserved junction name after the
// repo-wide pathspec is widened, without either pair's own fabric.yaml (which does not exist) ever
// being consulted.
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
	// already wired the fixture's default (empty) pathspec on both pairs at
	// Add time — the structural sets alone — so this call converges to a
	// no-op here — included for realism (a pair reconciled before ever being
	// widened), not because it changes anything on disk. The genuine
	// add-missing convergence this test proves is the second Reconcile call
	// below, against the widened pathspec.
	if _, err := topology.Reconcile(l); err != nil {
		t.Fatalf("Reconcile (initial): %v", err)
	}

	// Widen the repo-wide pathspec with a non-reserved optional name.
	boardDir := fabricengine.BoardDir(l.HubPath)
	cfgPath := configengine.ConfigFile(boardDir, "fabric")
	if err := os.WriteFile(cfgPath, []byte("branch_prefix: \"\"\npathspec: _lyx _extra\n"), 0o644); err != nil {
		t.Fatalf("widen repo-wide pathspec: %v", err)
	}

	if _, err := topology.Reconcile(l); err != nil {
		t.Fatalf("Reconcile (converge): %v", err)
	}

	for _, slug := range slugs {
		warpLayout, err := lyxcwd.Resolve(fabricengine.WorktreePath(l, slug))
		if err != nil {
			t.Fatalf("lyxcwd.Resolve(%s): %v", slug, err)
		}
		extraLink := filepath.Join(warpLayout.WorktreePath(), "_extra")
		if isLink, err := fslink.IsLink(extraLink); err != nil || !isLink {
			t.Errorf("worktree %s did not gain _extra junction after widening the repo-wide pathspec: isLink=%v err=%v", slug, isLink, err)
		}
	}
}

// TestReconcile_EmptyDefaultPathspecRemovesOptionalJunctionKeepsStructural pins the actual
// junction-teardown behaviour this task delivers: against the fixture's unmodified default (empty)
// repo-wide pathspec, an on-disk optional junction is stale by definition and Reconcile removes it,
// while the _lyx and .lyx structural junctions are left untouched.
// This deliberately never touches the repo-wide config -- newFabricFixture already seeds
// fabricengine.ConfigTemplate()'s real empty default, so this is the genuine post-change production
// shape, not a hand-widened one like the other cases in this file.
func TestReconcile_EmptyDefaultPathspecRemovesOptionalJunctionKeepsStructural(t *testing.T) {
	t.Parallel()

	const slug = "stale-removal-empty-default"
	fixture := newFabricFixture(t)
	l := fixture.Layout
	topology := fabricengine.NewTopology(fabricengine.Config{})
	if _, err := topology.Add(l, slug, fabricengine.AddOptions{SkipPush: true}); err != nil {
		t.Fatalf("setup Add: %v", err)
	}
	warpLayout, err := lyxcwd.Resolve(fabricengine.WorktreePath(l, slug))
	if err != nil {
		t.Fatalf("lyxcwd.Resolve(warp): %v", err)
	}

	// Wire an optional junction on disk that the empty default pathspec does
	// not name -- stale by definition against an empty desired set.
	if err := fabricengine.WireJunctions(warpLayout, slug, []string{"_lyx", "_extra"}); err != nil {
		t.Fatalf("setup WireJunctions: %v", err)
	}

	if _, err := topology.Reconcile(l); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	extraLink := filepath.Join(warpLayout.WorktreePath(), "_extra")
	if _, statErr := os.Lstat(extraLink); !os.IsNotExist(statErr) {
		t.Errorf("optional junction %s not removed against the empty default pathspec; want removed", extraLink)
	}

	lyxLink := fabricengine.WarpLyxLinkHere(warpLayout)
	if isLink, err := fslink.IsLink(lyxLink); err != nil || !isLink {
		t.Errorf("_lyx junction removed against the empty default pathspec; want untouched: isLink=%v err=%v", isLink, err)
	}
	dotLyxLink := filepath.Join(warpLayout.WorktreePath(), warpLayout.AnchorRel, lyxdirs.DotLyxDirName)
	if isLink, err := fslink.IsLink(dotLyxLink); err != nil || !isLink {
		t.Errorf(".lyx junction removed against the empty default pathspec; want untouched: isLink=%v err=%v", isLink, err)
	}
}

// TestReconcile_StaleRemovalFailsClosedOnUnparseableRepoWideConfig proves the fail-closed guard:
// when the repo-wide fabric.yaml cannot be loaded, Reconcile strips no junction and records the
// abort rather than treating the load failure as an empty (blanket-sweep) pathspec.
func TestReconcile_StaleRemovalFailsClosedOnUnparseableRepoWideConfig(t *testing.T) {
	t.Parallel()

	const slug = "stale-removal-failclosed"
	fixture := newFabricFixture(t)
	l := fixture.Layout
	topology := fabricengine.NewTopology(fabricengine.Config{})
	if _, err := topology.Add(l, slug, fabricengine.AddOptions{SkipPush: true}); err != nil {
		t.Fatalf("setup Add: %v", err)
	}
	warpLayout, err := lyxcwd.Resolve(fabricengine.WorktreePath(l, slug))
	if err != nil {
		t.Fatalf("lyxcwd.Resolve(warp): %v", err)
	}
	if err := fabricengine.WireJunctions(warpLayout, slug, []string{"_lyx", "_extra"}); err != nil {
		t.Fatalf("setup WireJunctions: %v", err)
	}

	// Corrupt the repo-wide fabric.yaml into unparseable YAML.
	boardDir := fabricengine.BoardDir(l.HubPath)
	cfgPath := configengine.ConfigFile(boardDir, "fabric")
	if err := os.WriteFile(cfgPath, []byte("not-valid-yaml: [unterminated"), 0o644); err != nil {
		t.Fatalf("corrupt repo-wide config: %v", err)
	}

	result, err := topology.Reconcile(l)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	pair := findReconcilePair(t, result.Pairs, fabricengine.WeftWorktreePath(l, slug))
	combined := pair.Detail + pair.Error
	if !strings.Contains(combined, "fabric.yaml") {
		t.Errorf("Detail=%q Error=%q; want the load failure recorded (fail-closed abort), not silently ignored", pair.Detail, pair.Error)
	}

	// The load failure must never be interpreted as an empty pathspec: every
	// junction wired before the corruption must still be present.
	lyxLink := fabricengine.WarpLyxLinkHere(warpLayout)
	if isLink, err := fslink.IsLink(lyxLink); err != nil || !isLink {
		t.Errorf("_lyx junction removed despite fail-closed guard: isLink=%v err=%v", isLink, err)
	}
	extraLink := filepath.Join(warpLayout.WorktreePath(), warpLayout.AnchorRel, "_extra")
	if isLink, err := fslink.IsLink(extraLink); err != nil || !isLink {
		t.Errorf("_extra junction removed despite fail-closed guard: isLink=%v err=%v", isLink, err)
	}
}

// TestReconcile_NeverRemovesReservedHubName proves scanOnDiskJunctionNames' exclusion of
// fabricengine.HubReservedNames() holds end-to-end through Reconcile: a hub-structural name present
// on disk (here _portals) is never swept even though it is absent from the repo-wide pathspec.
func TestReconcile_NeverRemovesReservedHubName(t *testing.T) {
	t.Parallel()

	const slug = "stale-removal-reserved"
	fixture := newFabricFixture(t)
	l := fixture.Layout
	topology := fabricengine.NewTopology(fabricengine.Config{})
	if _, err := topology.Add(l, slug, fabricengine.AddOptions{SkipPush: true}); err != nil {
		t.Fatalf("setup Add: %v", err)
	}
	warpLayout, err := lyxcwd.Resolve(fabricengine.WorktreePath(l, slug))
	if err != nil {
		t.Fatalf("lyxcwd.Resolve(warp): %v", err)
	}
	if err := fabricengine.WireJunctions(warpLayout, slug, []string{"_lyx", "_extra"}); err != nil {
		t.Fatalf("setup WireJunctions: %v", err)
	}

	// Seed a hub-structural reserved name directly on disk, as a link, under
	// the warp worktree root — never wired via WireJunctions (which never
	// takes a reserved name), matching how a hub-reserved passenger like
	// _portals would actually appear on disk. _raddle is no longer reserved
	// (card 19), so _portals stands in as the still-reserved exemplar; _board
	// itself is unusable here because Add already wires a real per-worktree
	// board junction at this same path (see junction.go), which would collide
	// with a hand-seeded link of the same name.
	reservedLink := filepath.Join(warpLayout.WorktreePath(), warpLayout.AnchorRel, "_portals")
	if err := fslink.CreateDirLink(reservedLink, t.TempDir()); err != nil {
		t.Fatalf("seed reserved link: %v", err)
	}

	if _, err := topology.Reconcile(l); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	if isLink, err := fslink.IsLink(reservedLink); err != nil || !isLink {
		t.Errorf("reserved _portals link removed by stale-removal; want untouched: isLink=%v err=%v", isLink, err)
	}
}

// TestRepoWideMigratedSites_ResolveFromBoardDirWithNoPerPairConfig is the repo-wide-base regression
// coverage card 10 calls for: with ONLY the repo-wide fabric.yaml seeded (no per-pair weft-base
// fabric.yaml exists anywhere in this fixture), the four sites card 7 migrated to RepoWiredNames
// still function — Healthy returns a real health verdict rather than "junction check unavailable",
// Topology.Checkout succeeds rather than hard-failing/rolling back on a name-set load error, and
// Topology.Remove still tears down junctions.
func TestRepoWideMigratedSites_ResolveFromBoardDirWithNoPerPairConfig(t *testing.T) {
	t.Parallel()

	fixture := lyxtest.CopyPairedLocal(t)
	// Deliberately skip lyxtest.SeedConfig at fixture.WeftPrime — the point
	// of this test is that no per-pair fabric.yaml exists, mirroring the new
	// clone flow where only the repo-wide config is ever materialized.
	seedRepoWideFabricConfig(t, fixture.Layout.HubPath)
	lyxtest.MustRun(t, fixture.WeftPrime, "git", "checkout", "-b", fabricengine.WeftBranchName("main"))

	l := fixture.Layout
	topology := fabricengine.NewTopology(fabricengine.Config{})
	slug := filepath.Base(fixture.Hub)

	if err := fabricengine.WireJunctions(l, slug, []string{"_lyx", lyxdirs.DotLyxDirName, "_extra"}); err != nil {
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
		t.Errorf("Healthy ok = false (reason %+v); want true (repo-wide-only config)", reason)
	}
	if reason.Cause == fabricengine.CauseConfigLoadFailed {
		t.Errorf("Healthy reason = %+v; want a real verdict, not CauseConfigLoadFailed", reason)
	}

	// Topology.Checkout must not hard-fail/rollback re-pointing junctions
	// after the branch switch.
	lyxtest.MustRun(t, l.WorktreePath(), "git", "branch", "repo-wide-checkout-target")
	if _, err := topology.Checkout(l, "repo-wide-checkout-target"); err != nil {
		t.Errorf("Checkout with repo-wide-only config = %v; want success", err)
	}

	// Topology.Remove must still tear down a pair's junctions using the
	// repo-wide name-set.
	const removeSlug = "repo-wide-remove-target"
	if _, err := topology.Add(l, removeSlug, fabricengine.AddOptions{SkipPush: true}); err != nil {
		t.Fatalf("setup Add(%s): %v", removeSlug, err)
	}
	removeWarpLayout, err := lyxcwd.Resolve(fabricengine.WorktreePath(l, removeSlug))
	if err != nil {
		t.Fatalf("lyxcwd.Resolve(%s): %v", removeSlug, err)
	}
	if err := fabricengine.WireJunctions(removeWarpLayout, removeSlug, []string{"_lyx", "_extra"}); err != nil {
		t.Fatalf("WireJunctions(%s): %v", removeSlug, err)
	}
	lyxLink := fabricengine.WarpLyxLinkHere(removeWarpLayout)
	extraLink := filepath.Join(removeWarpLayout.WorktreePath(), removeWarpLayout.AnchorRel, "_extra")

	if _, err := topology.Remove(l, removeSlug, true); err != nil {
		t.Fatalf("Remove(%s): %v", removeSlug, err)
	}
	if _, statErr := os.Lstat(lyxLink); !os.IsNotExist(statErr) {
		t.Errorf("_lyx junction %s still exists after Remove", lyxLink)
	}
	if _, statErr := os.Lstat(extraLink); !os.IsNotExist(statErr) {
		t.Errorf("_extra junction %s still exists after Remove", extraLink)
	}
}

// TestReconcile_PreservesUserSymlinkAtAnchor proves stale-removal claims only links fabric itself
// could have created. A hand-authored symlink checked into the warp repo sits in the same anchored
// directory as fabric's junctions and is absent from the repo-wide pathspec, so a link-ness-only
// sweep deleted it out of the user's working tree; ownership is decided by the resolved target
// instead, and a link pointing inside the warp worktree is never fabric's.
func TestReconcile_PreservesUserSymlinkAtAnchor(t *testing.T) {
	t.Parallel()

	const slug = "stale-removal-user-symlink"
	fixture := newFabricFixture(t)
	l := fixture.Layout
	topology := fabricengine.NewTopology(fabricengine.Config{})
	if _, err := topology.Add(l, slug, fabricengine.AddOptions{SkipPush: true}); err != nil {
		t.Fatalf("setup Add: %v", err)
	}

	warpLayout, err := lyxcwd.Resolve(fabricengine.WorktreePath(l, slug))
	if err != nil {
		t.Fatalf("lyxcwd.Resolve(warp): %v", err)
	}
	anchorDir := filepath.Join(warpLayout.WorktreePath(), warpLayout.AnchorRel)

	// A real directory in the user's own repo, plus a hand-authored symlink beside fabric's
	// junctions pointing at it — the shape a checked-in "latest -> v2" link has.
	userTargetDir := filepath.Join(anchorDir, "versions")
	if err := os.MkdirAll(userTargetDir, 0o755); err != nil {
		t.Fatalf("create user target dir: %v", err)
	}
	userLink := filepath.Join(anchorDir, "latest")
	if err := fslink.CreateDirLink(userLink, userTargetDir); err != nil {
		t.Fatalf("create user symlink: %v", err)
	}

	// A genuine stale fabric junction alongside it, so the sweep is provably still running and
	// this test cannot pass merely because stale-removal did nothing at all.
	if err := fabricengine.WireJunctions(warpLayout, slug, []string{"_lyx", "_other"}); err != nil {
		t.Fatalf("setup WireJunctions: %v", err)
	}

	if _, err := topology.Reconcile(l); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	if _, statErr := os.Lstat(userLink); statErr != nil {
		t.Errorf("Reconcile removed the hand-authored symlink %s: %v; want it preserved", userLink, statErr)
	}
	staleLink := filepath.Join(anchorDir, "_other")
	if _, statErr := os.Lstat(staleLink); !os.IsNotExist(statErr) {
		t.Errorf("stale fabric junction %s not removed by Reconcile; the sweep did not run", staleLink)
	}
}

// TestUnwire_PreservesUserSymlinkAtAnchor is the Unwire half of
// TestReconcile_PreservesUserSymlinkAtAnchor: Unwire enumerates the same on-disk scan, so a
// hand-authored symlink must survive a full deactivation too.
func TestUnwire_PreservesUserSymlinkAtAnchor(t *testing.T) {
	t.Parallel()

	const slug = "unwire-user-symlink"
	fixture := newFabricFixture(t)
	l := fixture.Layout
	topology := fabricengine.NewTopology(fabricengine.Config{})
	if _, err := topology.Add(l, slug, fabricengine.AddOptions{SkipPush: true}); err != nil {
		t.Fatalf("setup Add: %v", err)
	}

	warpLayout, err := lyxcwd.Resolve(fabricengine.WorktreePath(l, slug))
	if err != nil {
		t.Fatalf("lyxcwd.Resolve(warp): %v", err)
	}
	anchorDir := filepath.Join(warpLayout.WorktreePath(), warpLayout.AnchorRel)

	userTargetDir := filepath.Join(anchorDir, "versions")
	if err := os.MkdirAll(userTargetDir, 0o755); err != nil {
		t.Fatalf("create user target dir: %v", err)
	}
	userLink := filepath.Join(anchorDir, "latest")
	if err := fslink.CreateDirLink(userLink, userTargetDir); err != nil {
		t.Fatalf("create user symlink: %v", err)
	}

	result, err := fabricengine.Unwire(anchorDir)
	if err != nil {
		t.Fatalf("Unwire: %v", err)
	}

	if _, statErr := os.Lstat(userLink); statErr != nil {
		t.Errorf("Unwire removed the hand-authored symlink %s: %v; want it preserved", userLink, statErr)
	}
	for _, name := range result.JunctionsRemoved {
		if name == "latest" {
			t.Errorf("JunctionsRemoved names the hand-authored symlink %q; want fabric junctions only", name)
		}
	}
	if len(result.JunctionsRemoved) == 0 {
		t.Errorf("JunctionsRemoved is empty; Unwire removed no fabric junction, so this test proves nothing")
	}
}
