//go:build integration

// remove_junctions_integration_test.go proves card 9's fix: Remove tears down
// every host junction — not just the worktree-root case
// fslink.RemoveLinksIn's safety net already covers — including one nested
// under a non-"." RelPath, where that safety net (which scans only the
// worktree root's immediate children) cannot see it. At RelPath == "." the
// safety net masks the bug entirely, which is why this file drives the
// nested case specifically. HostJunctions returns exactly one entry (_lyx)
// today, so this is a behaviour-preservation guard for the generalised
// machinery rather than a regression discriminator against the old
// _lyx-hardcoded form (the two are identical for one junction, by this
// batch's own design) — it becomes a true discriminator once batch 5 adds a
// second, non-_lyx junction whose nested removal has no _lyx-shaped
// shortcut to fall back on.
//
// Package fabricengine_test to reuse newFabricFixture from
// reconcile_stale_registration_test.go; shares the single TestMain in
// testmain_test.go.

package fabricengine_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fslink"
	"github.com/Knatte18/loomyard/internal/hubgeometry"
)

// TestRemove_TearsDownNestedJunction wires a junction nested one level below
// the worktree root (RelPath "sub") and asserts Remove leaves no junction
// behind at that nested path.
func TestRemove_TearsDownNestedJunction(t *testing.T) {
	t.Parallel()

	const slug = "remove-nested-junction"
	fixture := newFabricFixture(t)
	l := fixture.Layout
	topology := fabricengine.NewTopology(fabricengine.Config{})

	if _, err := topology.Add(l, slug, fabricengine.AddOptions{SkipPush: true}); err != nil {
		t.Fatalf("setup Add: %v", err)
	}

	// Resolve a nested layout: same worktree (l.WorktreeRoot), but Cwd one
	// level deeper — RelPath becomes "sub", matching the hub-wide nesting
	// convention HostLyxLink/WeftLyxDirFor assume (every sibling worktree
	// nests at the same RelPath offset as the caller's own).
	subDir := filepath.Join(l.WorktreeRoot, "sub")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", subDir, err)
	}
	nestedLayout, err := hubgeometry.Resolve(subDir)
	if err != nil {
		t.Fatalf("hubgeometry.Resolve(%s): %v", subDir, err)
	}
	if nestedLayout.RelPath != "sub" {
		t.Fatalf("nestedLayout.RelPath = %q; want %q", nestedLayout.RelPath, "sub")
	}

	if err := fabricengine.WireJunctions(nestedLayout, slug); err != nil {
		t.Fatalf("WireJunctions(nested): %v", err)
	}

	nestedLink := nestedLayout.HostLyxLink(slug)
	if isLink, err := fslink.IsLink(nestedLink); err != nil || !isLink {
		t.Fatalf("setup: nested junction %s not wired: isLink=%v err=%v", nestedLink, isLink, err)
	}

	if _, err := topology.Remove(nestedLayout, slug, true); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	if _, statErr := os.Lstat(nestedLink); !os.IsNotExist(statErr) {
		t.Errorf("nested junction %s still exists after Remove", nestedLink)
	}
}
