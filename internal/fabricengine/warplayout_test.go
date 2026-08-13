//go:build integration

// warplayout_test.go pins warpLayoutFor's two branches against each other: the spawn-free hub-sibling
// fast path and the spawning lyxcwd.ResolveWorktree fallback are documented as equivalent, so this
// asserts they actually produce the same Location for the same worktree rather than one branch
// quietly omitting a field.
//
// Against a real hub, the prime warp worktree's own directory level also carries _board, _portals and
// _launchers — every name fabricengine.HubReservedNames() returns — a level the old CopyWarpHub
// fixture never populated with anything else at all.
// This additionally pins that the fast path resolves the prime worktree's own name, not one of those
// hub-reserved siblings that now share the same parent directory.

package fabricengine_test

import (
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

func TestWarpLayoutFor_FastPathMatchesResolveWorktree(t *testing.T) {
	t.Parallel()

	h := hubforge.NewHub(t, ".")
	worktreeRoot := h.PrimeWorktree()

	fast, err := fabricengine.WarpLayoutForForTest(h.Location, worktreeRoot)
	if err != nil {
		t.Fatalf("warpLayoutFor(hub sibling): %v", err)
	}
	slow, err := lyxcwd.ResolveWorktree(worktreeRoot)
	if err != nil {
		t.Fatalf("lyxcwd.ResolveWorktree(%s): %v", worktreeRoot, err)
	}

	if *fast != *slow {
		t.Errorf("warpLayoutFor fast path = %+v; want it to equal the ResolveWorktree fallback %+v", *fast, *slow)
	}

	// On the old CopyWarpHub fixture the hub-sibling directory level held nothing else, so
	// fast.WorktreeName being "the one repo dir" proved nothing about the fast path actually
	// distinguishing a warp worktree from anything else at that level. On a real hub,
	// fabricengine.HubReservedNames() (_board, _portals, _launchers) sit at that same level, so pin
	// that the fast path resolved the prime warp worktree's own name, not one of those reserved
	// siblings.
	for _, reserved := range fabricengine.HubReservedNames() {
		if fast.WorktreeName == reserved {
			t.Errorf("warpLayoutFor resolved WorktreeName = %q, a hub-reserved name; want the prime worktree's own name", fast.WorktreeName)
		}
	}
	if fast.WorktreeName != filepath.Base(worktreeRoot) {
		t.Errorf("warpLayoutFor WorktreeName = %q; want %q (the prime worktree's own base name)", fast.WorktreeName, filepath.Base(worktreeRoot))
	}
}
