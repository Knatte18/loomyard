// hubcontainment_integration_test.go checks the Hub Containment Invariant.
// No hub-level container is junctioned into a worktree.
// So <worktree>/<anchorRel>/<Bolt directory> must not exist after any verb.
// There is one case per verb (clone, add, and reconcile), and each asserts absence, never removal.
// Nothing sweeps a pre-existing junction,
// so a case that wires one by hand and expects it gone would test behaviour the code lacks.
//
// Package fabricengine_test reuses makeBareRemote from clone_adopt_test.go.
// It shares the TestMain in testmain_test.go, and this file adds none.
package fabricengine_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestHubContainment_CloneWiresNoBoardJunction drives fabricengine.CloneHub.
// The resolved prime worktree carries no Bolt checkout junction, and the warp repo's
// .git/info/exclude carries no Bolt checkout line.
func TestHubContainment_CloneWiresNoBoardJunction(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	fixtures := t.TempDir()
	warpBare := makeBareRemote(t, fixtures, "hubcontainment-clone-warp")
	weftBare := makeBareRemote(t, fixtures, "hubcontainment-clone-weft")

	cloneParent := t.TempDir()
	// ForceBootstrap: true — weftBare is an ordinary seeded bare remote standing in for a weft, not a
	// repo that has ever been one, so it carries no .lym-anchor.
	res, err := fabricengine.CloneHub(cloneParent, fabricengine.CloneOptions{
		WeftURL:        filepath.ToSlash(weftBare),
		WarpURL:        filepath.ToSlash(warpBare),
		Subpath:        ".",
		ForceBootstrap: true,
		Shortname:      "tst",
	})
	if err != nil {
		t.Fatalf("CloneHub() error = %v; want nil", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(res.HubPath) })

	boardLink := filepath.Join(res.PrimeCwd, fabricengine.BoltDirName)
	if _, statErr := os.Lstat(boardLink); !os.IsNotExist(statErr) {
		t.Errorf("board link %s exists after CloneHub; want absent (stat err: %v)", boardLink, statErr)
	}

	l, err := lymcwd.Resolve(res.PrimeCwd)
	if err != nil {
		t.Fatalf("lymcwd.Resolve(PrimeCwd): %v", err)
	}
	slug := filepath.Base(l.WorktreePath())
	boardPattern := fabricengine.ExcludePatternForTest(l.AnchorRel, fabricengine.BoltDirName)
	for _, line := range gitkit.ExcludeLines(t, fabricengine.WorktreePath(l, slug)) {
		if line == boardPattern {
			t.Errorf(".git/info/exclude carries %q after CloneHub; want no Bolt checkout exclude line", boardPattern)
		}
	}
}

// TestHubContainment_AddWiresNoBoardJunction drives Topology.Add on a fixture.
// It asserts that the new pair carries no Bolt checkout junction in its anchored directory.
// It also asserts that the warp repo's .git/info/exclude has no Bolt checkout line.
func TestHubContainment_AddWiresNoBoardJunction(t *testing.T) {
	tiergate.Require(t, 1)
	// Serial: t.Setenv("FABRIC_SKIP_PUSH") sets a process-global variable.
	t.Setenv("FABRIC_SKIP_PUSH", "1")

	const slug = "hubcontainment-add"
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	l := h.Location
	topology := h.Topology
	res, err := topology.Add(l, slug, fabricengine.AddOptions{SkipPush: true})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}

	boardLink := filepath.Join(res.Path, l.AnchorRel, fabricengine.BoltDirName)
	if _, statErr := os.Lstat(boardLink); !os.IsNotExist(statErr) {
		t.Errorf("board link %s exists after Add; want absent (stat err: %v)", boardLink, statErr)
	}

	boardPattern := fabricengine.ExcludePatternForTest(l.AnchorRel, fabricengine.BoltDirName)
	for _, line := range gitkit.ExcludeLines(t, fabricengine.WorktreePath(l, slug)) {
		if line == boardPattern {
			t.Errorf(".git/info/exclude carries %q after Add; want no Bolt checkout exclude line", boardPattern)
		}
	}
}

// TestHubContainment_ReconcileWiresNoBoardJunction drives Topology.Reconcile.
// It asserts that every pair it converges carries no Bolt checkout junction.
// It also asserts that each pair's warp repo .git/info/exclude has no Bolt checkout line.
func TestHubContainment_ReconcileWiresNoBoardJunction(t *testing.T) {
	tiergate.Require(t, 1)
	// Serial: t.Setenv("FABRIC_SKIP_PUSH") sets a process-global variable.
	t.Setenv("FABRIC_SKIP_PUSH", "1")

	const slug = "hubcontainment-reconcile"
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	l := h.Location
	topology := h.Topology
	hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{SkipPush: true})

	result, err := topology.Reconcile(l)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if len(result.Pairs) == 0 {
		t.Fatalf("Reconcile returned no pairs; want at least the one just added")
	}

	boardPattern := fabricengine.ExcludePatternForTest(l.AnchorRel, fabricengine.BoltDirName)
	for _, pair := range result.Pairs {
		warpPath := filepath.FromSlash(pair.CodeWorktree)
		boardLink := filepath.Join(warpPath, l.AnchorRel, fabricengine.BoltDirName)
		if _, statErr := os.Lstat(boardLink); !os.IsNotExist(statErr) {
			t.Errorf("board link %s exists after Reconcile; want absent (stat err: %v)", boardLink, statErr)
		}

		pairSlug := filepath.Base(warpPath)
		for _, line := range gitkit.ExcludeLines(t, fabricengine.WorktreePath(l, pairSlug)) {
			if line == boardPattern {
				t.Errorf("%s .git/info/exclude carries %q after Reconcile; want no Bolt checkout exclude line", warpPath, boardPattern)
			}
		}
	}
}
