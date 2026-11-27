// remove_junctions_integration_test.go covers Remove tearing down every warp junction.
// Junctions nested under a non-"." RelPath lie beyond the worktree-root scan, so this file drives that nested case.
// Removal must not rely on a _lym-specific path.
//
// It shares the TestMain in testmain_test.go.

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
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestRemove_TearsDownNestedJunction wires a junction one level below the worktree root, at RelPath "sub".
// It asserts Remove leaves no junction at that nested path.
func TestRemove_TearsDownNestedJunction(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	const slug = "remove-nested-junction"
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	l := h.Location
	topology := h.Topology

	hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{SkipPush: true})

	// Resolve a nested layout: the same worktree, anchored one level deeper, so AnchorRel becomes "sub".
	// Every sibling worktree nests at the same AnchorRel offset.
	// The strict cwd gate needs the anchor recorded before Resolve(subDir) can succeed at that subpath.
	subDir := filepath.Join(l.WorktreePath(), "sub")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", subDir, err)
	}
	seedAnchorMarker(t, h, "sub")
	nestedLayout, err := lymcwd.Resolve(subDir)
	if err != nil {
		t.Fatalf("lymcwd.Resolve(%s): %v", subDir, err)
	}
	if nestedLayout.AnchorRel != "sub" {
		t.Fatalf("nestedLayout.AnchorRel = %q; want %q", nestedLayout.AnchorRel, "sub")
	}

	if err := fabricengine.WireJunctions(nestedLayout, slug, []string{"_lym", "_extra"}); err != nil {
		t.Fatalf("WireJunctions(nested): %v", err)
	}

	nestedLymLink := fabricengine.CodeLymLink(nestedLayout, slug)
	if isLink, err := fslink.IsLink(nestedLymLink); err != nil || !isLink {
		t.Fatalf("setup: nested _lym junction %s not wired: isLink=%v err=%v", nestedLymLink, isLink, err)
	}
	nestedPatternLink := filepath.Join(fabricengine.WorktreePath(nestedLayout, slug), nestedLayout.AnchorRel, "_extra")
	if isLink, err := fslink.IsLink(nestedPatternLink); err != nil || !isLink {
		t.Fatalf("setup: nested _extra junction %s not wired: isLink=%v err=%v", nestedPatternLink, isLink, err)
	}

	// Remove loads the repo-wide config, best-effort, to find the nested junctions to tear down.
	// The hub fixture seeds that config, so the name-load finds the configured junctions for any RelPath,
	// and this test exercises the nested teardown rather than the path that removes nothing.
	if _, err := topology.Remove(nestedLayout, slug, true, false); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	if _, statErr := os.Lstat(nestedLymLink); !os.IsNotExist(statErr) {
		t.Errorf("nested _lym junction %s still exists after Remove", nestedLymLink)
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

		result, err := h.Topology.Remove(h.Location, reanchored, true, false)
		if err != nil {
			t.Fatalf("Remove: %v", err)
		}

		// The wired junctions, _lym and .lym, sit at the pair's own anchor.
		if result.LinksRemoved != 2 {
			t.Errorf("LinksRemoved = %d; want 2 (_lym, .lym at <slug>/sub)", result.LinksRemoved)
		}
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

// TestRemove_SweepsAnchoredLinksOnSubpathHub asserts Remove sweeps the pair's anchored directory, not the root.
// On a subpath-anchored hub every junction sits in the anchored directory, so the root holds none.
//
//testtiming:keep the scratch link_removed mutation recorded once, ahead of the warp worktree_removed; coverage of its blocks by other tests does not show an assertion of this
func TestRemove_SweepsAnchoredLinksOnSubpathHub(t *testing.T) {
	tiergate.Require(t, 1)
	// Serial: t.Setenv("FABRIC_SKIP_PUSH") sets a process-global variable.
	t.Setenv("FABRIC_SKIP_PUSH", "1")

	const slug = "remove-anchored-sweep"
	const anchor = "backend"

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	// PrimeName resolves the hub from the anchored directory.
	// The directory must exist in the prime worktree before the anchor is recorded.
	if err := os.MkdirAll(filepath.Join(h.Location.WorktreePath(), anchor), 0o755); err != nil {
		t.Fatalf("create anchored directory in prime: %v", err)
	}
	seedAnchorMarker(t, h, anchor)

	l, err := lymcwd.ResolveWorktree(h.Location.WorktreePath())
	if err != nil {
		t.Fatalf("lymcwd.ResolveWorktree: %v", err)
	}
	if l.AnchorRel != anchor {
		t.Fatalf("fixture AnchorRel = %q; want %q — the anchored geometry this test needs is not in place", l.AnchorRel, anchor)
	}

	topology := h.Topology
	if _, err := topology.Add(l, slug, fabricengine.AddOptions{SkipPush: true}); err != nil {
		t.Fatalf("Add: %v", err)
	}

	anchorDir := filepath.Join(fabricengine.WorktreePath(l, slug), anchor)
	lymLink := filepath.Join(anchorDir, lymdirs.LymDirName)
	if isLink, linkErr := fslink.IsLink(lymLink); linkErr != nil || !isLink {
		t.Fatalf("setup: %s is not a junction (isLink=%v err=%v)", lymLink, isLink, linkErr)
	}

	result, err := topology.Remove(l, slug, true, false)
	if err != nil {
		t.Fatalf("Remove: %v", err)
	}
	// The anchored sweep is the only contributor to LinksRemoved, so an exact count proves the sweep ran.
	// Expect the two wired junctions, _lym and .lym, and no others.
	if result.LinksRemoved != 2 {
		t.Errorf("LinksRemoved = %d; want 2 (_lym, .lym at the anchored directory)", result.LinksRemoved)
	}

	// The scratch link at the anchor is no junction, so it is unlinked separately and ahead of the warp worktree removal.
	link, _ := scratchPaths(l, slug)
	unlinked := mutationIndexes(result.Mutations, fabricengine.KindLinkRemoved, scratchHubRelative(t, l, link))
	warpRemoved := mutationIndexes(result.Mutations, fabricengine.KindWorktreeRemoved, scratchHubRelative(t, l, fabricengine.WorktreePath(l, slug)))
	if len(unlinked) != 1 || len(warpRemoved) != 1 || unlinked[0] > warpRemoved[0] {
		t.Errorf("record %+v; want one link_removed at the anchor .scratch ahead of the warp worktree_removed", result.Mutations.Entries())
	}
}

// seedAnchorMarker records anchor as the prime's subpath, as a real clone does.
// It commits the marker at the root of the prime's weft worktree, where it takes precedence over any Bolt marker.
func seedAnchorMarker(t *testing.T, h *hubforge.Hub, anchor string) {
	t.Helper()
	gitkit.CommitFile(t, h.PrimeRecords(), lymcwd.AnchorFileName, anchor+"\n", "test fixture: re-anchor the prime at "+anchor)
}
