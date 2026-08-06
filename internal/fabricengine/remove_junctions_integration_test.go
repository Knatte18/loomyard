//go:build integration

// remove_junctions_integration_test.go proves card 9's fix: Remove tears down
// every host junction — not just the worktree-root case
// fslink.RemoveLinksIn's safety net already covers — including one nested
// under a non-"." RelPath, where that safety net (which scans only the
// worktree root's immediate children) cannot see it. At RelPath == "." the
// safety net masks the bug entirely, which is why this file drives the
// nested case specifically. From card 15 onward HostJunctions returns two
// entries (_lyx and _pattern), so this is now a true discriminator against
// the old _lyx-hardcoded form: _pattern's nested removal has no _lyx-shaped
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
	"github.com/Knatte18/loomyard/internal/lyxcwd"
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

	// Resolve a nested layout: same worktree (l.WorktreePath()), but anchored
	// one level deeper — AnchorRel becomes "sub", matching the hub-wide
	// nesting convention HostLyxLink/WeftLyxDirFor assume (every sibling
	// worktree nests at the same AnchorRel offset as the caller's own). The
	// strict cwd gate requires the anchor to actually be recorded before
	// Resolve(subDir) can succeed at that subpath.
	subDir := filepath.Join(l.WorktreePath(), "sub")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", subDir, err)
	}
	anchorPath := filepath.Join(lyxcwd.BoardDir(l.HubPath), lyxcwd.AnchorFileName)
	if err := os.MkdirAll(filepath.Dir(anchorPath), 0o755); err != nil {
		t.Fatalf("mkdir board dir: %v", err)
	}
	if err := os.WriteFile(anchorPath, []byte("sub"), 0o644); err != nil {
		t.Fatalf("write %s: %v", anchorPath, err)
	}
	nestedLayout, err := lyxcwd.Resolve(subDir)
	if err != nil {
		t.Fatalf("lyxcwd.Resolve(%s): %v", subDir, err)
	}
	if nestedLayout.AnchorRel != "sub" {
		t.Fatalf("nestedLayout.AnchorRel = %q; want %q", nestedLayout.AnchorRel, "sub")
	}

	if err := fabricengine.WireJunctions(nestedLayout, slug, []string{"_lyx", "_pattern"}); err != nil {
		t.Fatalf("WireJunctions(nested): %v", err)
	}

	nestedLyxLink := fabricengine.HostLyxLink(nestedLayout, slug)
	if isLink, err := fslink.IsLink(nestedLyxLink); err != nil || !isLink {
		t.Fatalf("setup: nested _lyx junction %s not wired: isLink=%v err=%v", nestedLyxLink, isLink, err)
	}
	nestedPatternLink := nestedLayout.HostPatternLink(slug)
	if isLink, err := fslink.IsLink(nestedPatternLink); err != nil || !isLink {
		t.Fatalf("setup: nested _pattern junction %s not wired: isLink=%v err=%v", nestedPatternLink, isLink, err)
	}

	// Remove loads the repo-wide config (best-effort) to know which nested
	// junctions to tear down — newFabricFixture already materialized it at
	// lyxcwd.BoardDir(l.HubPath) via seedRepoWideFabricConfig, so Remove's
	// name-load finds "_lyx _pattern" (the default pathspec) regardless of
	// this pair's RelPath, and the happy-path nested teardown below is
	// actually exercised, not just the degraded nothing-removed path.
	if _, err := topology.Remove(nestedLayout, slug, true); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	if _, statErr := os.Lstat(nestedLyxLink); !os.IsNotExist(statErr) {
		t.Errorf("nested _lyx junction %s still exists after Remove", nestedLyxLink)
	}
	if _, statErr := os.Lstat(nestedPatternLink); !os.IsNotExist(statErr) {
		t.Errorf("nested _pattern junction %s still exists after Remove", nestedPatternLink)
	}
}
