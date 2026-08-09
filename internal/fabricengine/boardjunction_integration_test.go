//go:build integration

// boardjunction_integration_test.go covers the operator-convenience _board junction wireBoardLink
// wires: its creation at clone and add, its unconditional-with-respect-to-junction-health repair on
// reconcile (the placement this batch's card 38 gets right precisely because checkJunctionHealth
// never inspects it), its exclusion from every pathspec-derived route (filterHubReserved via
// WiredNames, and ScopedPathspec over the real loaded config), its removal on Unwire, and its
// absence until a pair is fully wired (RawAdopted leaves it unwired; the immediately following pass
// wires it).
//
// Package fabricengine_test to reuse newFabricFixture/seedRepoWideFabricConfig
// (reconcile_stale_registration_test.go), makeBareRemote (clone_adopt_test.go), findReconcilePair
// (reconcile_stale_removal_test.go), and readExcludeLines (junction_pattern_integration_test.go);
// shares the single TestMain in testmain_test.go — no new TestMain is added here.
package fabricengine_test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fslink"
	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

// TestBoardJunction_WiredAtClone asserts that CloneHub itself wires the _board link at
// <PrimeCwd>/_board, pointing at res.BoardDir — the "clone" half of card 37's "wired at clone and
// add" requirement.
func TestBoardJunction_WiredAtClone(t *testing.T) {
	fixtures := t.TempDir()
	warpBare := makeBareRemote(t, fixtures, "board-clone-warp")
	weftBare := makeBareRemote(t, fixtures, "board-clone-weft")

	cloneParent := t.TempDir()
	// ForceBootstrap: true — weftBare is an ordinary seeded bare remote standing in for a weft,
	// not a repo that has ever been one, so it carries no .lyx-anchor.
	res, err := fabricengine.CloneHub(cloneParent, fabricengine.CloneOptions{
		WeftURL:        filepath.ToSlash(weftBare),
		WarpURL:        filepath.ToSlash(warpBare),
		Subpath:        ".",
		ForceBootstrap: true,
	})
	if err != nil {
		t.Fatalf("CloneHub() error = %v; want nil", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(res.HubPath) })

	boardLink := filepath.Join(res.PrimeCwd, fabricengine.BoardDirName)
	isLink, err := fslink.IsLink(boardLink)
	if err != nil || !isLink {
		t.Fatalf("board link %s missing/not-a-link after CloneHub: isLink=%v err=%v", boardLink, isLink, err)
	}

	got, err := fslink.PointsTo(boardLink)
	if err != nil {
		t.Fatalf("resolve board link: %v", err)
	}
	want, err := filepath.EvalSymlinks(res.BoardDir)
	if err != nil {
		t.Fatalf("resolve BoardDir: %v", err)
	}
	if got != want {
		t.Errorf("board link target = %q; want %q", got, want)
	}
}

// TestBoardJunction_WiredAtAddAndSurvivesReconcileThenUnwireRemoves covers the "add" half of card
// 37's wiring requirement, the git-exclude seed, the stale-sweep protection (a reconcile pass
// immediately after wiring must not remove it — scanOnDiskJunctionNames' HubReservedNames skip,
// which this batch inherits rather than designs in), and Unwire's explicitly-named removal of both
// the link and the exclude entry.
func TestBoardJunction_WiredAtAddAndSurvivesReconcileThenUnwireRemoves(t *testing.T) {
	t.Setenv("WEFT_SKIP_PUSH", "1")

	const slug = "board-add-survive-unwire"
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
	boardLink := filepath.Join(warpLayout.WorktreePath(), warpLayout.AnchorRel, fabricengine.BoardDirName)

	isLink, err := fslink.IsLink(boardLink)
	if err != nil || !isLink {
		t.Fatalf("board link %s missing after Add: isLink=%v err=%v", boardLink, isLink, err)
	}
	got, err := fslink.PointsTo(boardLink)
	if err != nil {
		t.Fatalf("resolve board link: %v", err)
	}
	want, err := filepath.EvalSymlinks(fabricengine.BoardDir(l.HubPath))
	if err != nil {
		t.Fatalf("resolve BoardDir: %v", err)
	}
	if got != want {
		t.Errorf("board link target = %q; want %q", got, want)
	}

	lines := readExcludeLines(t, l, slug)
	if !slices.Contains(lines, fabricengine.BoardDirName) {
		t.Errorf(".git/info/exclude = %v; want it to contain %q after Add", lines, fabricengine.BoardDirName)
	}

	// A reconcile run immediately after wiring must not remove the link:
	// scanOnDiskJunctionNames skips every HubReservedNames() entry, so
	// applyStaleRemoval's sweep never sees _board as a candidate.
	if _, err := topology.Reconcile(l); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if isLink, err := fslink.IsLink(boardLink); err != nil || !isLink {
		t.Errorf("board link removed by an immediately-following Reconcile pass: isLink=%v err=%v", isLink, err)
	}

	if _, err := fabricengine.Unwire(warpLayout.WorktreePath()); err != nil {
		t.Fatalf("Unwire: %v", err)
	}
	if _, statErr := os.Lstat(boardLink); !os.IsNotExist(statErr) {
		t.Errorf("board link %s still exists after Unwire (stat err: %v)", boardLink, statErr)
	}
	lines = readExcludeLines(t, l, slug)
	if slices.Contains(lines, fabricengine.BoardDirName) {
		t.Errorf(".git/info/exclude = %v; want it to no longer contain %q after Unwire", lines, fabricengine.BoardDirName)
	}
}

// TestBoardJunction_ReconcileRepairsOutsideHealthCheck is card 38's central regression: it
// constructs a pair with only the _board link broken (missing) and every pathspec junction intact,
// so checkJunctionHealth — which only ever inspects the pathspec name-set — reports the pair
// healthy.
// It asserts both that Healthy() still reports true (the wire-only-and-unmonitored decision: a
// broken convenience link must never block loom preflight) and that Reconcile still re-wires the
// link despite reporting ReconcileActionAlreadyHealthy — the assertion that would have caught the
// board re-wire being placed inside the `!junctionHealthy` branch instead of next to it.
func TestBoardJunction_ReconcileRepairsOutsideHealthCheck(t *testing.T) {
	t.Setenv("WEFT_SKIP_PUSH", "1")

	const slug = "board-repair-outside-health"
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
	boardLink := filepath.Join(warpLayout.WorktreePath(), warpLayout.AnchorRel, fabricengine.BoardDirName)

	if err := os.Remove(boardLink); err != nil {
		t.Fatalf("break board link: %v", err)
	}

	ok, reason, err := fabricengine.Healthy(warpLayout)
	if err != nil {
		t.Fatalf("Healthy: %v", err)
	}
	if !ok || reason != (fabricengine.HealthReason{}) {
		t.Errorf("Healthy() = (%v, %+v); want (true, HealthReason{}) — a broken _board link must never report unhealthy", ok, reason)
	}

	result, err := topology.Reconcile(l)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	pair := findReconcilePair(t, result.Pairs, fabricengine.WeftWorktreePath(l, slug))
	if pair.Error != "" {
		t.Errorf("Error = %q; want empty", pair.Error)
	}
	if pair.Action != fabricengine.ReconcileActionAlreadyHealthy {
		t.Errorf("Action = %q; want %q — proves the board re-wire sits outside the !junctionHealthy branch", pair.Action, fabricengine.ReconcileActionAlreadyHealthy)
	}

	if isLink, err := fslink.IsLink(boardLink); err != nil || !isLink {
		t.Errorf("board link not re-created by Reconcile despite AlreadyHealthy: isLink=%v err=%v", isLink, err)
	}
}

// TestBoardJunction_ReconcileRepointsWrongTarget covers the mispointed shape of the same repair (as
// opposed to TestBoardJunction_ReconcileRepairsOutsideHealthCheck's missing shape): a _board link
// present but resolving to the wrong directory is re-pointed at the canonical target by Reconcile.
func TestBoardJunction_ReconcileRepointsWrongTarget(t *testing.T) {
	t.Setenv("WEFT_SKIP_PUSH", "1")

	const slug = "board-repoint-wrong-target"
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
	boardLink := filepath.Join(warpLayout.WorktreePath(), warpLayout.AnchorRel, fabricengine.BoardDirName)

	if err := fslink.Remove(boardLink); err != nil {
		t.Fatalf("remove board link: %v", err)
	}
	wrongTarget := t.TempDir()
	if err := fslink.CreateDirLink(boardLink, wrongTarget); err != nil {
		t.Fatalf("create wrong-target link: %v", err)
	}

	result, err := topology.Reconcile(l)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	pair := findReconcilePair(t, result.Pairs, fabricengine.WeftWorktreePath(l, slug))
	if pair.Error != "" {
		t.Errorf("Error = %q; want empty", pair.Error)
	}

	got, err := fslink.PointsTo(boardLink)
	if err != nil {
		t.Fatalf("resolve board link: %v", err)
	}
	want, err := filepath.EvalSymlinks(fabricengine.BoardDir(l.HubPath))
	if err != nil {
		t.Fatalf("resolve BoardDir: %v", err)
	}
	if got != want {
		t.Errorf("board link target = %q; want %q (re-pointed at the canonical target)", got, want)
	}
}

// TestBoardJunction_AbsentUntilPairFullyWired covers the missing-weft deferral: a raw warp worktree
// created directly via `git worktree add` (bypassing topology.Add entirely) has no junction wired
// at all, including _board.
// The first Reconcile pass adopts it (ReconcileActionRawAdopted), creating only a dormant weft
// worktree — no wiring happens on this action, so the board link stays absent.
// The immediately following pass takes the weftWorktreeExists branch, wiring both the pathspec
// junctions and _board together.
func TestBoardJunction_AbsentUntilPairFullyWired(t *testing.T) {
	t.Parallel()

	const slug = "board-raw-adopt"
	fixture := newFabricFixture(t)
	l := fixture.Layout
	topology := fabricengine.NewTopology(fabricengine.Config{})

	target := fabricengine.WorktreePath(l, slug)
	if _, _, exitCode, err := gitexec.RunGit([]string{"worktree", "add", "-b", slug, target}, l.WorktreePath()); err != nil || exitCode != 0 {
		t.Fatalf("git worktree add -b %s %s: err=%v exit=%d", slug, target, err, exitCode)
	}

	weftPath := fabricengine.WeftWorktreePath(l, slug)
	boardLink := filepath.Join(target, l.AnchorRel, fabricengine.BoardDirName)

	result, err := topology.Reconcile(l)
	if err != nil {
		t.Fatalf("Reconcile (first pass): %v", err)
	}
	pair := findReconcilePair(t, result.Pairs, weftPath)
	if pair.Action != fabricengine.ReconcileActionRawAdopted {
		t.Fatalf("Action = %q; want %q", pair.Action, fabricengine.ReconcileActionRawAdopted)
	}
	if _, statErr := os.Lstat(boardLink); !os.IsNotExist(statErr) {
		t.Errorf("board link %s exists after a RawAdopted pass; want absent (no wiring happens on this action)", boardLink)
	}

	if _, err := topology.Reconcile(l); err != nil {
		t.Fatalf("Reconcile (second pass): %v", err)
	}
	got, err := fslink.PointsTo(boardLink)
	if err != nil {
		t.Fatalf("resolve board link after the follow-up reconcile pass: %v", err)
	}
	want, err := filepath.EvalSymlinks(fabricengine.BoardDir(l.HubPath))
	if err != nil {
		t.Fatalf("resolve BoardDir: %v", err)
	}
	if got != want {
		t.Errorf("board link target = %q; want %q", got, want)
	}
}

// TestBoardJunction_ExcludedFromPathspecRoutes guards the batch-local decision against a later
// "simplification": _board must appear in neither WiredNames' output (the exported wrapper over
// filterHubReserved) nor ScopedPathspec's output over the real loaded config's raw Dirs() — the two
// routes that respectively drive junction wiring and the weft commit pathspec.
func TestBoardJunction_ExcludedFromPathspecRoutes(t *testing.T) {
	t.Parallel()

	fixture := newFabricFixture(t)
	l := fixture.Layout
	boardDir := fabricengine.BoardDir(l.HubPath)

	names, err := fabricengine.WiredNames(boardDir)
	if err != nil {
		t.Fatalf("WiredNames: %v", err)
	}
	if slices.Contains(names, fabricengine.BoardDirName) {
		t.Errorf("WiredNames() = %v; want it to never include %q", names, fabricengine.BoardDirName)
	}

	cfg, err := fabricengine.LoadConfig(boardDir)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	scoped := fabricengine.ScopedPathspec(l.AnchorRel, cfg.Dirs())
	for _, entry := range scoped {
		if filepath.Base(entry) == fabricengine.BoardDirName {
			t.Errorf("ScopedPathspec(%q, %v) = %v; want no entry named %q", l.AnchorRel, cfg.Dirs(), scoped, fabricengine.BoardDirName)
		}
	}
}
