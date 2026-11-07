// reconcile_stale_removal_test.go covers Reconcile converging warp worktree junctions to the repo-wide pathspec.
// That pathspec comes from fabricengine.BoardDir(Hub)'s fabric.yaml: missing junctions are added, stale ones removed.
// It also covers the fail-closed guard for an unloadable config and that hub-reserved names are never swept.

package fabricengine_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fslink"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// findReconcilePair returns the ReconcilePairResult for weftPath, failing the test if none is found.
// Result paths are emitted forward-slashed, so weftPath is normalized the same way before comparing.
func findReconcilePair(t *testing.T, pairs []fabricengine.ReconcilePairResult, weftPath string) *fabricengine.ReconcilePairResult {
	t.Helper()
	want := filepath.ToSlash(weftPath)
	for i := range pairs {
		if pairs[i].RecordsWorktree == want {
			return &pairs[i]
		}
	}
	t.Fatalf("Reconcile result has no pair for weft path %s: %+v", weftPath, pairs)
	return nil
}

// TestReconcile_AddsMissingRemovesStaleNoOpsCorrect covers the three per-junction convergence outcomes in one pair.
// A missing desired junction (_extra) is added.
// A stale on-disk junction absent from the repo-wide pathspec (_other) is removed.
// An already-correct junction (_lym) is left untouched.
func TestReconcile_AddsMissingRemovesStaleNoOpsCorrect(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	const slug = "stale-removal-add-remove-noop"
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	l := h.Location
	boardDir := fabricengine.BoardDir(l.HubPath)
	cfgPath := configengine.ConfigFile(boardDir, "fabric")

	// Add wires the repo-wide pathspec it reads at call time (RepoWiredNames(l)), so narrow it to "_lym" first.
	// The explicit write keeps the setup self-documenting and immune to a change in the default pathspec.
	if err := os.WriteFile(cfgPath, []byte("branch_prefix: \"\"\npathspec: _lym\n"), 0o644); err != nil {
		t.Fatalf("narrow repo-wide pathspec: %v", err)
	}

	topology := h.Topology
	hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{SkipPush: true})

	warpLayout, err := lymcwd.Resolve(fabricengine.WorktreePath(l, slug))
	if err != nil {
		t.Fatalf("lymcwd.Resolve(warp): %v", err)
	}

	// Wire _other alongside the _lym junction Add already wired.
	// _other is on disk but absent from the repo-wide pathspec, so it is the stale junction the sweep removes.
	if err := fabricengine.WireJunctions(warpLayout, slug, []string{"_lym", "_other"}); err != nil {
		t.Fatalf("setup WireJunctions: %v", err)
	}

	// Widen the repo-wide pathspec to "_lym _extra".
	// That leaves _extra missing on disk, so Reconcile must add it.
	if err := os.WriteFile(cfgPath, []byte("branch_prefix: \"\"\npathspec: _lym _extra\n"), 0o644); err != nil {
		t.Fatalf("widen repo-wide pathspec: %v", err)
	}

	result, err := topology.Reconcile(l)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	weftPath := fabricengine.RecordsWorktreePath(l, slug)
	pair := findReconcilePair(t, result.Pairs, weftPath)
	if pair.Error != "" {
		t.Errorf("Error = %q; want empty", pair.Error)
	}

	// Add-missing: _extra was absent on disk and present in the desired set, so it is wired.
	extraLink := filepath.Join(warpLayout.WorktreePath(), warpLayout.AnchorRel, "_extra")
	if isLink, err := fslink.IsLink(extraLink); err != nil || !isLink {
		t.Errorf("missing _extra junction not added by Reconcile: isLink=%v err=%v", isLink, err)
	}

	// Stale-removal: _other was on disk but absent from the desired set, so it is removed.
	otherLink := filepath.Join(warpLayout.WorktreePath(), "_other")
	if _, statErr := os.Lstat(otherLink); !os.IsNotExist(statErr) {
		t.Errorf("stale _other junction %s not removed by Reconcile; want removed", otherLink)
	}
	if !strings.Contains(pair.Detail, "_other") {
		t.Errorf("Detail = %q; want it to name the removed stale junction _other", pair.Detail)
	}

	// No-op: _lym was already correct and desired, so it is left untouched by either step.
	lymLink := fabricengine.WarpLymLinkHere(warpLayout)
	if isLink, err := fslink.IsLink(lymLink); err != nil || !isLink {
		t.Errorf("_lym junction broken by Reconcile: isLink=%v err=%v", isLink, err)
	}
}

// TestReconcile_CorrectJunctionsAreNoOp asserts that a pair with correct junctions is a true no-op.
// The action is ReconcileActionAlreadyHealthy with an empty Detail.
// It is never ReconcileActionStaleRemoved from an empty stale set.
func TestReconcile_CorrectJunctionsAreNoOp(t *testing.T) {
	tiergate.Require(t, 1)
	// Serial: the logger capture below replaces process-global logger output.
	const slug = "stale-removal-correct-noop"
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	l := h.Location
	topology := h.Topology
	hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{SkipPush: true})
	warpLayout, err := lymcwd.Resolve(fabricengine.WorktreePath(l, slug))
	if err != nil {
		t.Fatalf("lymcwd.Resolve(warp): %v", err)
	}
	if err := fabricengine.WireJunctions(warpLayout, slug, []string{"_lym", "_extra"}); err != nil {
		t.Fatalf("setup WireJunctions: %v", err)
	}

	// Widen the repo-wide pathspec to match what is wired on disk above.
	// The fixture's default empty pathspec alone would make _extra stale,
	// so this widening keeps the pair a genuine no-op rather than a stale-removal case.
	boardDir := fabricengine.BoardDir(l.HubPath)
	cfgPath := configengine.ConfigFile(boardDir, "fabric")
	if err := os.WriteFile(cfgPath, []byte("branch_prefix: \"\"\npathspec: _lym _extra\n"), 0o644); err != nil {
		t.Fatalf("widen repo-wide pathspec: %v", err)
	}

	logs := logcapture.Capture(t)
	result, err := topology.Reconcile(l)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	pair := findReconcilePair(t, result.Pairs, fabricengine.RecordsWorktreePath(l, slug))
	if pair.Action != fabricengine.ReconcileActionAlreadyHealthy {
		t.Errorf("Action = %q; want %q (fully correct pair, nothing to converge)", pair.Action, fabricengine.ReconcileActionAlreadyHealthy)
	}
	if pair.Detail != "" {
		t.Errorf("Detail = %q; want empty for a true no-op", pair.Detail)
	}

	root, target := scratchPaths(l, slug)
	if got, want := describeEntry(t, root), "link:"+target; got != want {
		t.Errorf("root .scratch = %q; want %q", got, want)
	}
	if lines := gitkit.ExcludeLines(t, fabricengine.WorktreePath(l, slug)); !containsLine(lines, "/.scratch") {
		t.Errorf("exclude lines = %v; want /.scratch kept", lines)
	}
	if hasMutation(result.Mutations, fabricengine.KindLinkRemoved, scratchHubRelative(t, l, root), "") {
		t.Errorf("record %+v holds link_removed at the root .scratch", result.Mutations.Entries())
	}
	if strings.Contains(logs.String(), "stale-junction removal refused") {
		t.Errorf("logs %q hold a refused stale-junction removal; the sweep must never claim the scratch link", logs.String())
	}
}

// TestReconcile_ConvergesAllWorktreesToRepoWidePathspec proves convergence to the repo-wide pathspec.
// It is repo-wide, not per-pair.
// Two independently wired pairs both gain a new, non-reserved junction after the pathspec is widened.
// Neither pair has a fabric.yaml of its own, and none is consulted.
func TestReconcile_ConvergesAllWorktreesToRepoWidePathspec(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	l := h.Location
	topology := h.Topology

	slugs := []string{"converge-all-a", "converge-all-b"}
	for _, slug := range slugs {
		hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{SkipPush: true})
	}

	// First Reconcile: Add's eager wiring (RepoWiredNames) already wired the empty default pathspec.
	// Both pairs therefore hold only the structural sets, so this call is a no-op.
	// It runs for realism, as a pair reconciled before it is ever widened, and changes nothing on disk.
	// The add-missing convergence this test proves is the second Reconcile call below, against the widened pathspec.
	if _, err := topology.Reconcile(l); err != nil {
		t.Fatalf("Reconcile (initial): %v", err)
	}

	// Widen the repo-wide pathspec with a non-reserved optional name.
	boardDir := fabricengine.BoardDir(l.HubPath)
	cfgPath := configengine.ConfigFile(boardDir, "fabric")
	if err := os.WriteFile(cfgPath, []byte("branch_prefix: \"\"\npathspec: _lym _extra\n"), 0o644); err != nil {
		t.Fatalf("widen repo-wide pathspec: %v", err)
	}

	if _, err := topology.Reconcile(l); err != nil {
		t.Fatalf("Reconcile (converge): %v", err)
	}

	for _, slug := range slugs {
		warpLayout, err := lymcwd.Resolve(fabricengine.WorktreePath(l, slug))
		if err != nil {
			t.Fatalf("lymcwd.Resolve(%s): %v", slug, err)
		}
		extraLink := filepath.Join(warpLayout.WorktreePath(), "_extra")
		if isLink, err := fslink.IsLink(extraLink); err != nil || !isLink {
			t.Errorf("worktree %s did not gain _extra junction after widening the repo-wide pathspec: isLink=%v err=%v", slug, isLink, err)
		}
	}
}

// TestReconcile_EmptyDefaultPathspecRemovesOptionalJunctionKeepsStructural pins junction teardown.
// The fixture's unmodified default pathspec is empty.
// An on-disk optional junction that the pathspec does not name is stale, so Reconcile removes it.
// The _lym and .lym structural junctions are left untouched.
// The test does not write the repo-wide config, so the fixture's default applies.
func TestReconcile_EmptyDefaultPathspecRemovesOptionalJunctionKeepsStructural(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	const slug = "stale-removal-empty-default"
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	l := h.Location
	topology := h.Topology
	hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{SkipPush: true})
	warpLayout, err := lymcwd.Resolve(fabricengine.WorktreePath(l, slug))
	if err != nil {
		t.Fatalf("lymcwd.Resolve(warp): %v", err)
	}

	// Wire an optional junction on disk that the empty default pathspec does not name.
	// Against an empty desired set it is stale.
	if err := fabricengine.WireJunctions(warpLayout, slug, []string{"_lym", "_extra"}); err != nil {
		t.Fatalf("setup WireJunctions: %v", err)
	}

	if _, err := topology.Reconcile(l); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	extraLink := filepath.Join(warpLayout.WorktreePath(), "_extra")
	if _, statErr := os.Lstat(extraLink); !os.IsNotExist(statErr) {
		t.Errorf("optional junction %s not removed against the empty default pathspec; want removed", extraLink)
	}

	lymLink := fabricengine.WarpLymLinkHere(warpLayout)
	if isLink, err := fslink.IsLink(lymLink); err != nil || !isLink {
		t.Errorf("_lym junction removed against the empty default pathspec; want untouched: isLink=%v err=%v", isLink, err)
	}
	dotLymLink := filepath.Join(warpLayout.WorktreePath(), warpLayout.AnchorRel, lymdirs.DotLymDirName)
	if isLink, err := fslink.IsLink(dotLymLink); err != nil || !isLink {
		t.Errorf(".lym junction removed against the empty default pathspec; want untouched: isLink=%v err=%v", isLink, err)
	}
}

// TestReconcile_StaleRemovalFailsClosedOnUnparseableRepoWideConfig proves the fail-closed guard.
// When the repo-wide fabric.yaml cannot be loaded, Reconcile strips no junction and records the abort.
// It does not treat the load failure as an empty pathspec, which would sweep every junction.
func TestReconcile_StaleRemovalFailsClosedOnUnparseableRepoWideConfig(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	const slug = "stale-removal-failclosed"
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	l := h.Location
	topology := h.Topology
	hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{SkipPush: true})
	warpLayout, err := lymcwd.Resolve(fabricengine.WorktreePath(l, slug))
	if err != nil {
		t.Fatalf("lymcwd.Resolve(warp): %v", err)
	}
	if err := fabricengine.WireJunctions(warpLayout, slug, []string{"_lym", "_extra"}); err != nil {
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

	pair := findReconcilePair(t, result.Pairs, fabricengine.RecordsWorktreePath(l, slug))
	combined := pair.Detail + pair.Error
	if !strings.Contains(combined, "fabric.yaml") {
		t.Errorf("Detail=%q Error=%q; want the load failure recorded (fail-closed abort), not silently ignored", pair.Detail, pair.Error)
	}

	// The load failure must never be read as an empty pathspec.
	// Every junction wired before the corruption must still be present.
	lymLink := fabricengine.WarpLymLinkHere(warpLayout)
	if isLink, err := fslink.IsLink(lymLink); err != nil || !isLink {
		t.Errorf("_lym junction removed despite fail-closed guard: isLink=%v err=%v", isLink, err)
	}
	extraLink := filepath.Join(warpLayout.WorktreePath(), warpLayout.AnchorRel, "_extra")
	if isLink, err := fslink.IsLink(extraLink); err != nil || !isLink {
		t.Errorf("_extra junction removed despite fail-closed guard: isLink=%v err=%v", isLink, err)
	}
}

// TestReconcile_NeverRemovesReservedHubName proves that Reconcile never sweeps a hub-reserved name.
// Here _portals stands in for one: it is on disk but absent from the repo-wide pathspec, yet it survives.
func TestReconcile_NeverRemovesReservedHubName(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	const slug = "stale-removal-reserved"
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	l := h.Location
	topology := h.Topology
	hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{SkipPush: true})
	warpLayout, err := lymcwd.Resolve(fabricengine.WorktreePath(l, slug))
	if err != nil {
		t.Fatalf("lymcwd.Resolve(warp): %v", err)
	}
	if err := fabricengine.WireJunctions(warpLayout, slug, []string{"_lym", "_extra"}); err != nil {
		t.Fatalf("setup WireJunctions: %v", err)
	}

	// Seed a hub-reserved name directly on disk as a link under the warp worktree root.
	// WireJunctions never takes a reserved name, so the link cannot come from it.
	// _portals stands in for the reserved names here, because the Bolt checkout cannot serve:
	// Add already wires a per-worktree board junction at that same path.
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

// TestRepoWideMigratedSites_ResolveFromBoardDirWithNoPerPairConfig checks that the call sites still work.
// They read the name set through RepoWiredNames, and the test seeds only the repo-wide fabric.yaml.
// No per-pair weft-base fabric.yaml exists in this fixture.
// Healthy returns a real health verdict rather than "junction check unavailable".
// Topology.Checkout succeeds rather than rolling back on a name-set load error.
// Topology.Remove still tears down junctions.
func TestRepoWideMigratedSites_ResolveFromBoardDirWithNoPerPairConfig(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	// hubforge.NewHub's CloneAndWire checks the weft primary out on its suffixed branch.
	// It materializes only the repo-wide fabric.yaml, as a real hub does.
	// No per-pair weft-base fabric.yaml exists in a real hub either, so no per-pair SeedConfig call is needed here.
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})

	l := h.Location
	topology := h.Topology
	slug := l.WorktreeName

	if err := fabricengine.WireJunctions(l, slug, []string{"_lym", lymdirs.DotLymDirName, "_extra"}); err != nil {
		t.Fatalf("WireJunctions(primary): %v", err)
	}

	// Healthy resolves the name-set from the repo-wide BoardDir base and reports a real verdict.
	// The "unavailable" degraded reason is what a per-pair weft-base read would give, and no such file exists here.
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
	gitkit.MustRun(t, l.WorktreePath(), "git", "branch", "repo-wide-checkout-target")
	if _, err := topology.Checkout(l, "repo-wide-checkout-target"); err != nil {
		t.Errorf("Checkout with repo-wide-only config = %v; want success", err)
	}

	// Topology.Remove must still tear down a pair's junctions using the
	// repo-wide name-set.
	const removeSlug = "repo-wide-remove-target"
	hubforge.AddPairWith(t, h, removeSlug, fabricengine.AddOptions{SkipPush: true})
	removeWarpLayout, err := lymcwd.Resolve(fabricengine.WorktreePath(l, removeSlug))
	if err != nil {
		t.Fatalf("lymcwd.Resolve(%s): %v", removeSlug, err)
	}
	if err := fabricengine.WireJunctions(removeWarpLayout, removeSlug, []string{"_lym", "_extra"}); err != nil {
		t.Fatalf("WireJunctions(%s): %v", removeSlug, err)
	}
	lymLink := fabricengine.WarpLymLinkHere(removeWarpLayout)
	extraLink := filepath.Join(removeWarpLayout.WorktreePath(), removeWarpLayout.AnchorRel, "_extra")

	if _, err := topology.Remove(l, removeSlug, true, false); err != nil {
		t.Fatalf("Remove(%s): %v", removeSlug, err)
	}
	if _, statErr := os.Lstat(lymLink); !os.IsNotExist(statErr) {
		t.Errorf("_lym junction %s still exists after Remove", lymLink)
	}
	if _, statErr := os.Lstat(extraLink); !os.IsNotExist(statErr) {
		t.Errorf("_extra junction %s still exists after Remove", extraLink)
	}
}

// TestReconcile_PreservesUserSymlinkAtAnchor checks that stale-removal removes only fabric's own links.
// A hand-authored symlink sits in the same anchored directory as fabric's junctions,
// and the repo-wide pathspec does not list it, so a sweep that only checks for links would delete it.
// Ownership is decided by the resolved target instead,
// and a link pointing inside the warp worktree is never fabric's.
func TestReconcile_PreservesUserSymlinkAtAnchor(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	const slug = "stale-removal-user-symlink"
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	l := h.Location
	topology := h.Topology
	hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{SkipPush: true})

	warpLayout, err := lymcwd.Resolve(fabricengine.WorktreePath(l, slug))
	if err != nil {
		t.Fatalf("lymcwd.Resolve(warp): %v", err)
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
	if err := fabricengine.WireJunctions(warpLayout, slug, []string{"_lym", "_other"}); err != nil {
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

// TestUnwire_PreservesUserSymlinkAtAnchor checks that Unwire leaves a hand-authored symlink at the anchor in place.
// Unwire scans the anchor the same way stale-removal does, so a full deactivation must spare such a link too.
//
//testtiming:keep Unwire leaving a hand-authored symlink at the anchor in place; coverage of its blocks by other tests does not show an assertion of this
func TestUnwire_PreservesUserSymlinkAtAnchor(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	const slug = "unwire-user-symlink"
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	l := h.Location
	hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{SkipPush: true})

	warpLayout, err := lymcwd.Resolve(fabricengine.WorktreePath(l, slug))
	if err != nil {
		t.Fatalf("lymcwd.Resolve(warp): %v", err)
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

	result, err := fabricengine.Unwire(anchorDir, true)
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

// TestReconcile_RefusedStaleRemovalReportsNothing checks that a refused stale removal reports nothing.
// The destructive gate refuses removal of the only stale junction.
// applyStaleRemoval must not set Action to stale_removed or append a removed-detail naming it.
// It must also keep the junction's .git/info/exclude entry.
// Stripping that entry would leave the still-present junction showing as untracked dirt.
//
// The drift keeps the junction fabric-owned while making its removal refusable.
// scanOnDiskJunctionNames still claims it, since it resolves into the weft worktree.
// ownedWiredJunction refuses it when the link's raw one-hop target differs from the nominal weft target.
func TestReconcile_RefusedStaleRemovalReportsNothing(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	const slug = "stale-removal-refused"
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	l := h.Location
	boardDir := fabricengine.BoardDir(l.HubPath)
	cfgPath := configengine.ConfigFile(boardDir, "fabric")

	// Wire _lym + _other with _other correct, then narrow the repo-wide pathspec so _other is stale.
	if err := os.WriteFile(cfgPath, []byte("branch_prefix: \"\"\npathspec: _lym _other\n"), 0o644); err != nil {
		t.Fatalf("seed repo-wide pathspec: %v", err)
	}
	topology := h.Topology
	hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{SkipPush: true})
	warpLayout, err := lymcwd.Resolve(fabricengine.WorktreePath(l, slug))
	if err != nil {
		t.Fatalf("lymcwd.Resolve(warp): %v", err)
	}

	// Drift _other's target: re-point it at a decoy directory inside the same weft worktree, so it
	// still resolves into weft (scanOnDiskJunctionNames keeps claiming it) but no longer matches its
	// nominal weft target (ownedWiredJunction refuses removal).
	otherLink := filepath.Join(warpLayout.WorktreePath(), warpLayout.AnchorRel, "_other")
	weftDecoy := filepath.Join(fabricengine.RecordsWorktreePath(l, slug), warpLayout.AnchorRel, "_decoy")
	if err := os.MkdirAll(weftDecoy, 0o755); err != nil {
		t.Fatalf("create weft decoy dir: %v", err)
	}
	if err := os.Remove(otherLink); err != nil {
		t.Fatalf("remove _other link before re-pointing: %v", err)
	}
	if err := fslink.CreateDirLink(otherLink, weftDecoy); err != nil {
		t.Fatalf("re-point _other at decoy: %v", err)
	}

	// Narrow the repo-wide pathspec so that _other falls outside the desired set and becomes stale.
	if err := os.WriteFile(cfgPath, []byte("branch_prefix: \"\"\npathspec: _lym\n"), 0o644); err != nil {
		t.Fatalf("narrow repo-wide pathspec: %v", err)
	}

	result, err := topology.Reconcile(l)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	pair := findReconcilePair(t, result.Pairs, fabricengine.RecordsWorktreePath(l, slug))
	if pair.Action == fabricengine.ReconcileActionStaleRemoved {
		t.Errorf("Action = %q; a refused stale removal must not report stale_removed", pair.Action)
	}
	if strings.Contains(pair.Detail, "removed") {
		t.Errorf("Detail = %q; must not claim a junction was removed when the removal was refused", pair.Detail)
	}

	// The refused junction must still be on disk — nothing removed it.
	if _, statErr := os.Lstat(otherLink); statErr != nil {
		t.Errorf("refused stale junction %s was removed after all: %v", otherLink, statErr)
	}

	// Its exclude entry must survive.
	// Stripping it while the junction is still present would make the junction show as untracked dirt.
	// A clean `git status --porcelain` in the warp worktree proves the entry was not stripped.
	if status := gitkit.GitStatusPorcelain(t, warpLayout.WorktreePath()); status != "" {
		t.Errorf("warp worktree is dirty after a refused stale removal (exclude entry stripped?):\n%s", status)
	}
}
