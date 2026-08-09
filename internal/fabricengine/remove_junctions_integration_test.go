//go:build integration

// remove_junctions_integration_test.go proves card 9's fix: Remove tears down
// every warp junction — not just the worktree-root case
// fslink.RemoveLinksIn's safety net already covers — including one nested
// under a non-"." RelPath, where that safety net (which scans only the
// worktree root's immediate children) cannot see it. At RelPath == "." the
// safety net masks the bug entirely, which is why this file drives the
// nested case specifically. From card 15 onward WarpJunctions returns two
// entries (_lyx and a second, non-_lyx junction), so this is now a true
// discriminator against the old _lyx-hardcoded form: the second junction's
// nested removal has no _lyx-shaped shortcut to fall back on.
//
// Package fabricengine_test to reuse newFabricFixture from
// reconcile_stale_registration_test.go; shares the single TestMain in
// testmain_test.go.

package fabricengine_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fslink"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
	"github.com/Knatte18/loomyard/internal/lyxtest"
)

// TestRemove_TearsDownNestedJunction wires a junction nested one level below the worktree root
// (RelPath "sub") and asserts Remove leaves no junction behind at that nested path.
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
	// nesting convention WarpLyxLink/WeftLyxDirFor assume (every sibling
	// worktree nests at the same AnchorRel offset as the caller's own). The
	// strict cwd gate requires the anchor to actually be recorded before
	// Resolve(subDir) can succeed at that subpath.
	subDir := filepath.Join(l.WorktreePath(), "sub")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", subDir, err)
	}
	anchorPath := filepath.Join(fabricengine.BoardDir(l.HubPath), lyxcwd.AnchorFileName)
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

	if err := fabricengine.WireJunctions(nestedLayout, slug, []string{"_lyx", "_extra"}); err != nil {
		t.Fatalf("WireJunctions(nested): %v", err)
	}

	nestedLyxLink := fabricengine.WarpLyxLink(nestedLayout, slug)
	if isLink, err := fslink.IsLink(nestedLyxLink); err != nil || !isLink {
		t.Fatalf("setup: nested _lyx junction %s not wired: isLink=%v err=%v", nestedLyxLink, isLink, err)
	}
	nestedPatternLink := filepath.Join(fabricengine.WorktreePath(nestedLayout, slug), nestedLayout.AnchorRel, "_extra")
	if isLink, err := fslink.IsLink(nestedPatternLink); err != nil || !isLink {
		t.Fatalf("setup: nested _extra junction %s not wired: isLink=%v err=%v", nestedPatternLink, isLink, err)
	}

	// Remove loads the repo-wide config (best-effort) to know which nested
	// junctions to tear down — newFabricFixture already materialized it at
	// fabricengine.BoardDir(l.HubPath) via seedRepoWideFabricConfig, so Remove's
	// name-load finds the configured pathspec's junctions regardless of this
	// pair's RelPath, and the happy-path nested teardown below is actually
	// exercised, not just the degraded nothing-removed path.
	if _, err := topology.Remove(nestedLayout, slug, true); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	if _, statErr := os.Lstat(nestedLyxLink); !os.IsNotExist(statErr) {
		t.Errorf("nested _lyx junction %s still exists after Remove", nestedLyxLink)
	}
	if _, statErr := os.Lstat(nestedPatternLink); !os.IsNotExist(statErr) {
		t.Errorf("nested _extra junction %s still exists after Remove", nestedPatternLink)
	}
}

// TestRemove_SweepsAnchoredLinksOnSubpathHub proves Remove's link sweep reads the pair's ANCHORED
// directory rather than the worktree root.
// On a subpath-anchored hub the root holds no junction at all — every one of them sits at
// <worktree>/<anchorRel> — so a root sweep saw nothing, left the links behind, and reported
// LinksRemoved: 0 while three junctions existed one directory down.
func TestRemove_SweepsAnchoredLinksOnSubpathHub(t *testing.T) {
	t.Setenv("WEFT_SKIP_PUSH", "1")

	const slug = "remove-anchored-sweep"
	const anchor = "backend"

	fixture := newFabricFixture(t)
	// PrimeName resolves the hub from the anchored directory, so it must exist in the prime
	// worktree before the anchor is recorded — the same precondition CloneHub's own subpath guard
	// enforces against the freshly cloned warp.
	if err := os.MkdirAll(filepath.Join(fixture.Layout.WorktreePath(), anchor), 0o755); err != nil {
		t.Fatalf("create anchored directory in prime: %v", err)
	}
	seedAnchorMarker(t, fixture.Layout.HubPath, anchor)

	l, err := lyxcwd.ResolveWorktree(fixture.Layout.WorktreePath())
	if err != nil {
		t.Fatalf("lyxcwd.ResolveWorktree: %v", err)
	}
	if l.AnchorRel != anchor {
		t.Fatalf("fixture AnchorRel = %q; want %q — the anchored geometry this test needs is not in place", l.AnchorRel, anchor)
	}

	topology := fabricengine.NewTopology(fabricengine.Config{})
	if _, err := topology.Add(l, slug, fabricengine.AddOptions{SkipPush: true}); err != nil {
		t.Fatalf("Add: %v", err)
	}

	anchorDir := filepath.Join(fabricengine.WorktreePath(l, slug), anchor)
	lyxLink := filepath.Join(anchorDir, lyxdirs.LyxDirName)
	if isLink, linkErr := fslink.IsLink(lyxLink); linkErr != nil || !isLink {
		t.Fatalf("setup: %s is not a junction (isLink=%v err=%v)", lyxLink, isLink, linkErr)
	}

	result, err := topology.Remove(l, slug, true)
	if err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if result.LinksRemoved == 0 {
		t.Errorf("LinksRemoved = 0; want the anchored junctions counted — the sweep read the worktree root instead")
	}
}

// TestRemove_FailedWeftTeardownIsReported locks the weft worktree so `git worktree remove --force`
// refuses it, and asserts Remove reports the surviving weft worktree instead of returning success —
// the silent `_ =` swallow used to leave a half-torn pair behind with an ok verdict.
func TestRemove_FailedWeftTeardownIsReported(t *testing.T) {
	t.Setenv("WEFT_SKIP_PUSH", "1")

	const slug = "remove-locked-pair"
	fixture := newFabricFixture(t)
	l := fixture.Layout

	topology := fabricengine.NewTopology(fabricengine.Config{})
	if _, err := topology.Add(l, slug, fabricengine.AddOptions{SkipPush: true}); err != nil {
		t.Fatalf("Add: %v", err)
	}

	weftTarget := fabricengine.WeftWorktreePath(l, slug)
	lyxtest.MustRun(t, fixture.WeftPrime, "git", "worktree", "lock", weftTarget)

	_, err := topology.Remove(l, slug, true)
	if err == nil {
		t.Fatal("Remove() with a locked weft worktree error = nil; want the surviving weft worktree reported")
	}
	if !strings.Contains(err.Error(), "weft teardown failed") {
		t.Errorf("Remove() error = %q; want it to report the failed weft teardown", err)
	}
	if _, statErr := os.Stat(weftTarget); statErr != nil {
		t.Fatalf("test premise broken: locked weft worktree %s was removed anyway: %v", weftTarget, statErr)
	}
}

// seedAnchorMarker records anchor as the hub-wide subpath the same way a real clone does, by writing
// the marker into the hub's board directory.
func seedAnchorMarker(t *testing.T, hubPath, anchor string) {
	t.Helper()
	boardDir := fabricengine.BoardDir(hubPath)
	if err := os.MkdirAll(boardDir, 0o755); err != nil {
		t.Fatalf("mkdir board dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(boardDir, lyxcwd.AnchorFileName), []byte(anchor+"\n"), 0o644); err != nil {
		t.Fatalf("write anchor marker: %v", err)
	}
}
