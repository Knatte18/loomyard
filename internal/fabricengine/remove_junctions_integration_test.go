//go:build tier1

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
// Package fabricengine_test; shares the single TestMain in testmain_test.go.

package fabricengine_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fslink"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
)

// TestRemove_TearsDownNestedJunction wires a junction nested one level below the worktree root
// (RelPath "sub") and asserts Remove leaves no junction behind at that nested path.
func TestRemove_TearsDownNestedJunction(t *testing.T) {
	t.Parallel()

	const slug = "remove-nested-junction"
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	l := h.Location
	topology := h.Topology

	hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{SkipPush: true})

	// Resolve a nested layout: same worktree (l.WorktreePath()), but anchored
	// one level deeper — AnchorRel becomes "sub", matching the hub-wide
	// nesting convention CodeLyxLink/WeftLyxDirFor assume (every sibling
	// worktree nests at the same AnchorRel offset as the caller's own). The
	// strict cwd gate requires the anchor to actually be recorded before
	// Resolve(subDir) can succeed at that subpath.
	subDir := filepath.Join(l.WorktreePath(), "sub")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", subDir, err)
	}
	seedAnchorMarker(t, h, "sub")
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

	nestedLyxLink := fabricengine.CodeLyxLink(nestedLayout, slug)
	if isLink, err := fslink.IsLink(nestedLyxLink); err != nil || !isLink {
		t.Fatalf("setup: nested _lyx junction %s not wired: isLink=%v err=%v", nestedLyxLink, isLink, err)
	}
	nestedPatternLink := filepath.Join(fabricengine.WorktreePath(nestedLayout, slug), nestedLayout.AnchorRel, "_extra")
	if isLink, err := fslink.IsLink(nestedPatternLink); err != nil || !isLink {
		t.Fatalf("setup: nested _extra junction %s not wired: isLink=%v err=%v", nestedPatternLink, isLink, err)
	}

	// Remove loads the repo-wide config (best-effort) to know which nested
	// junctions to tear down — hubforge.NewHub already materialized it at
	// fabricengine.BoardDir(l.HubPath) via seedRepoWideFabricConfig, so Remove's
	// name-load finds the configured pathspec's junctions regardless of this
	// pair's RelPath, and the happy-path nested teardown below is actually
	// exercised, not just the degraded nothing-removed path.
	if _, err := topology.Remove(nestedLayout, slug, true, false); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	if _, statErr := os.Lstat(nestedLyxLink); !os.IsNotExist(statErr) {
		t.Errorf("nested _lyx junction %s still exists after Remove", nestedLyxLink)
	}
	if _, statErr := os.Lstat(nestedPatternLink); !os.IsNotExist(statErr) {
		t.Errorf("nested _extra junction %s still exists after Remove", nestedPatternLink)
	}

	t.Run("PairAtItsOwnAnchorRemovedFromThePrime", func(t *testing.T) {
		t.Parallel()

		const reanchored = "remove-own-anchor"
		h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
		hubforge.AddPairWith(t, h, reanchored, fabricengine.AddOptions{SkipPush: true})
		hubforge.ReanchorPair(t, h, reanchored, "sub")
		pairLauncherDir := h.PairLauncherDir(reanchored)
		if err := os.MkdirAll(pairLauncherDir, 0o755); err != nil {
			t.Fatalf("plant the pair's launcher directory %s: %v", pairLauncherDir, err)
		}
		primeAnchorLauncherDir := fabricengine.LauncherDir(h.Location, reanchored)
		if _, err := os.Stat(primeAnchorLauncherDir); err != nil {
			t.Fatalf("setup: the launcher directory Add wrote at the prime's anchor %s: %v", primeAnchorLauncherDir, err)
		}

		result, err := h.Topology.Remove(h.Location, reanchored, true, false)
		if err != nil {
			t.Fatalf("Remove: %v", err)
		}

		// Exactly the two wired junctions (_lyx, .lyx) at the pair's own anchor.
		if result.LinksRemoved != 2 {
			t.Errorf("LinksRemoved = %d; want 2 (_lyx, .lyx at <slug>/sub)", result.LinksRemoved)
		}
		assertAbsent(t, pairLauncherDir)
		assertPresent(t, primeAnchorLauncherDir)
		assertAbsent(t, h.PairCodeWorktree(reanchored))
	})

	t.Run("PairWhoseRecordsWorktreeIsGone", func(t *testing.T) {
		t.Parallel()

		const reanchored = "remove-gone-records"
		h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
		hubforge.AddPairWith(t, h, reanchored, fabricengine.AddOptions{SkipPush: true})
		hubforge.ReanchorPair(t, h, reanchored, "sub")
		if err := os.RemoveAll(h.PairRecordsSibling(reanchored)); err != nil {
			t.Fatalf("delete the records worktree by hand: %v", err)
		}

		if _, found, err := fabricengine.ReadOriginFor(h.Location, reanchored); found || err != nil {
			t.Errorf("ReadOriginFor() = found %v, err %v; want not found with no error", found, err)
		}
		result, err := h.Topology.Remove(h.Location, reanchored, true, false)
		if err != nil {
			t.Fatalf("Remove: %v", err)
		}

		assertAbsent(t, h.PairCodeWorktree(reanchored))
		for _, m := range result.Mutations.Entries() {
			if strings.HasPrefix(filepath.ToSlash(m.Target), reanchored+"/sub") {
				t.Errorf("Remove recorded %+v; want no step under %s/sub", m, reanchored)
			}
		}
	})
}

// TestRemove_SweepsAnchoredLinksOnSubpathHub proves Remove's link sweep reads the pair's ANCHORED
// directory rather than the worktree root.
// On a subpath-anchored hub the root holds no junction at all — every one of them sits at
// <worktree>/<anchorRel> — so a root sweep saw nothing, left the links behind, and reported
// LinksRemoved: 0 while two junctions existed one directory down.
func TestRemove_SweepsAnchoredLinksOnSubpathHub(t *testing.T) {
	// Serial: t.Setenv("FABRIC_SKIP_PUSH") sets a process-global variable.
	t.Setenv("FABRIC_SKIP_PUSH", "1")

	const slug = "remove-anchored-sweep"
	const anchor = "backend"

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	// PrimeName resolves the hub from the anchored directory, so it must exist in the prime
	// worktree before the anchor is recorded — the same precondition CloneHub's own subpath guard
	// enforces against the freshly cloned warp.
	if err := os.MkdirAll(filepath.Join(h.Location.WorktreePath(), anchor), 0o755); err != nil {
		t.Fatalf("create anchored directory in prime: %v", err)
	}
	seedAnchorMarker(t, h, anchor)

	l, err := lyxcwd.ResolveWorktree(h.Location.WorktreePath())
	if err != nil {
		t.Fatalf("lyxcwd.ResolveWorktree: %v", err)
	}
	if l.AnchorRel != anchor {
		t.Fatalf("fixture AnchorRel = %q; want %q — the anchored geometry this test needs is not in place", l.AnchorRel, anchor)
	}

	topology := h.Topology
	if _, err := topology.Add(l, slug, fabricengine.AddOptions{SkipPush: true}); err != nil {
		t.Fatalf("Add: %v", err)
	}

	anchorDir := filepath.Join(fabricengine.WorktreePath(l, slug), anchor)
	lyxLink := filepath.Join(anchorDir, lyxdirs.LyxDirName)
	if isLink, linkErr := fslink.IsLink(lyxLink); linkErr != nil || !isLink {
		t.Fatalf("setup: %s is not a junction (isLink=%v err=%v)", lyxLink, isLink, linkErr)
	}

	result, err := topology.Remove(l, slug, true, false)
	if err != nil {
		t.Fatalf("Remove: %v", err)
	}
	// Exactly the two wired junctions (_lyx, .lyx) — an exact count, not merely non-zero: with the
	// _board special case gone, the anchored sweep is the sole contributor to LinksRemoved, so an
	// exact count is what proves the sweep ran at all.
	if result.LinksRemoved != 2 {
		t.Errorf("LinksRemoved = %d; want 2 (_lyx, .lyx at the anchored directory)", result.LinksRemoved)
	}

	// The root scratch link is no anchored junction, so it is unlinked separately and ahead of the warp worktree removal.
	root, _ := scratchPaths(l, slug)
	unlinked := mutationIndexes(result.Mutations, fabricengine.KindLinkRemoved, scratchHubRelative(t, l, root))
	warpRemoved := mutationIndexes(result.Mutations, fabricengine.KindWorktreeRemoved, scratchHubRelative(t, l, fabricengine.WorktreePath(l, slug)))
	if len(unlinked) != 1 || len(warpRemoved) != 1 || unlinked[0] > warpRemoved[0] {
		t.Errorf("record %+v; want one link_removed at the root .scratch ahead of the warp worktree_removed", result.Mutations.Entries())
	}
}

// TestRemove_FailedWeftTeardownIsReported locks the weft worktree so `git worktree remove --force`
// refuses it, and asserts Remove reports the surviving weft worktree instead of returning success —
// the silent `_ =` swallow used to leave a half-torn pair behind with an ok verdict.
func TestRemove_FailedWeftTeardownIsReported(t *testing.T) {
	// Serial: t.Setenv("FABRIC_SKIP_PUSH") sets a process-global variable.
	t.Setenv("FABRIC_SKIP_PUSH", "1")

	const slug = "remove-locked-pair"
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	l := h.Location

	topology := h.Topology
	hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{SkipPush: true})

	weftTarget := fabricengine.RecordsWorktreePath(l, slug)
	gitkit.MustRun(t, h.PrimeRecords(), "git", "worktree", "lock", weftTarget)

	_, err := topology.Remove(l, slug, true, false)
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

// seedAnchorMarker records anchor as the prime's subpath the same way a real clone does, by committing the marker at the root of the prime's weft worktree, where it wins over any Bolt marker.
func seedAnchorMarker(t *testing.T, h *hubforge.Hub, anchor string) {
	t.Helper()
	gitkit.CommitFile(t, h.PrimeRecords(), lyxcwd.AnchorFileName, anchor+"\n", "test fixture: re-anchor the prime at "+anchor)
}
