//go:build integration

// hubcontainment_integration_test.go gives the Hub Containment Invariant teeth: no hub-level
// container is ever junctioned into a worktree, so <worktree>/<anchorRel>/_board must not exist
// after any verb. One case per verb that used to wire the deleted _board junction — clone, add, and
// reconcile — asserts absence, never removal: nothing in this task sweeps a pre-existing junction, so
// a case that wires one by hand and expects it gone would be testing behaviour the plan deliberately
// does not build.
//
// Package fabricengine_test to reuse newFabricFixture (reconcile_stale_registration_test.go),
// makeBareRemote (clone_adopt_test.go), and readExcludeLines (junction_pattern_integration_test.go);
// shares the single TestMain in testmain_test.go — no new TestMain is added here.
package fabricengine_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

// TestHubContainment_CloneWiresNoBoardJunction drives fabricengine.CloneHub and asserts the resolved
// prime worktree carries no _board junction and no _board line in the warp repo's
// .git/info/exclude — the clone-time call this batch deleted used to wire both.
func TestHubContainment_CloneWiresNoBoardJunction(t *testing.T) {
	t.Parallel()

	fixtures := t.TempDir()
	warpBare := makeBareRemote(t, fixtures, "hubcontainment-clone-warp")
	weftBare := makeBareRemote(t, fixtures, "hubcontainment-clone-weft")

	cloneParent := t.TempDir()
	// ForceBootstrap: true — weftBare is an ordinary seeded bare remote standing in for a weft, not a
	// repo that has ever been one, so it carries no .lyx-anchor.
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
	if _, statErr := os.Lstat(boardLink); !os.IsNotExist(statErr) {
		t.Errorf("board link %s exists after CloneHub; want absent (stat err: %v)", boardLink, statErr)
	}

	l, err := lyxcwd.Resolve(res.PrimeCwd)
	if err != nil {
		t.Fatalf("lyxcwd.Resolve(PrimeCwd): %v", err)
	}
	slug := filepath.Base(l.WorktreePath())
	boardPattern := fabricengine.ExcludePatternForTest(l.AnchorRel, fabricengine.BoardDirName)
	for _, line := range readExcludeLines(t, l, slug) {
		if line == boardPattern {
			t.Errorf(".git/info/exclude carries %q after CloneHub; want no _board exclude line", boardPattern)
		}
	}
}

// TestHubContainment_AddWiresNoBoardJunction drives Topology.Add on a fixture and asserts the new
// pair's anchored directory carries no _board junction and no _board line in the warp repo's
// .git/info/exclude — the add-time call this batch deleted used to wire both.
func TestHubContainment_AddWiresNoBoardJunction(t *testing.T) {
	t.Setenv("WEFT_SKIP_PUSH", "1")

	const slug = "hubcontainment-add"
	fixture := newFabricFixture(t)
	l := fixture.Layout
	topology := fabricengine.NewTopology(fabricengine.Config{})
	res, err := topology.Add(l, slug, fabricengine.AddOptions{SkipPush: true})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}

	boardLink := filepath.Join(res.Path, l.AnchorRel, fabricengine.BoardDirName)
	if _, statErr := os.Lstat(boardLink); !os.IsNotExist(statErr) {
		t.Errorf("board link %s exists after Add; want absent (stat err: %v)", boardLink, statErr)
	}

	boardPattern := fabricengine.ExcludePatternForTest(l.AnchorRel, fabricengine.BoardDirName)
	for _, line := range readExcludeLines(t, l, slug) {
		if line == boardPattern {
			t.Errorf(".git/info/exclude carries %q after Add; want no _board exclude line", boardPattern)
		}
	}
}

// TestHubContainment_ReconcileWiresNoBoardJunction drives Topology.Reconcile and asserts every pair
// it converges carries no _board junction and no _board line in its warp repo's
// .git/info/exclude — reconcile is the verb that used to re-wire the link unconditionally on every
// pass, regardless of junction health.
func TestHubContainment_ReconcileWiresNoBoardJunction(t *testing.T) {
	t.Setenv("WEFT_SKIP_PUSH", "1")

	const slug = "hubcontainment-reconcile"
	fixture := newFabricFixture(t)
	l := fixture.Layout
	topology := fabricengine.NewTopology(fabricengine.Config{})
	if _, err := topology.Add(l, slug, fabricengine.AddOptions{SkipPush: true}); err != nil {
		t.Fatalf("Add: %v", err)
	}

	result, err := topology.Reconcile(l)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if len(result.Pairs) == 0 {
		t.Fatalf("Reconcile returned no pairs; want at least the one just added")
	}

	boardPattern := fabricengine.ExcludePatternForTest(l.AnchorRel, fabricengine.BoardDirName)
	for _, pair := range result.Pairs {
		warpPath := filepath.FromSlash(pair.WarpWorktree)
		boardLink := filepath.Join(warpPath, l.AnchorRel, fabricengine.BoardDirName)
		if _, statErr := os.Lstat(boardLink); !os.IsNotExist(statErr) {
			t.Errorf("board link %s exists after Reconcile; want absent (stat err: %v)", boardLink, statErr)
		}

		pairSlug := filepath.Base(warpPath)
		for _, line := range readExcludeLines(t, l, pairSlug) {
			if line == boardPattern {
				t.Errorf("%s .git/info/exclude carries %q after Reconcile; want no _board exclude line", warpPath, boardPattern)
			}
		}
	}
}
