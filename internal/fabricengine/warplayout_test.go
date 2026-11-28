// warplayout_test.go pins warpLayoutFor's two branches against each other.
// The hub-sibling fast path and the lymcwd.ResolveWorktree fallback are documented as equivalent,
// so this asserts that they produce the same Location for the same worktree and that neither branch omits a field.
//
// On a real hub, the prime warp worktree's directory level also holds the Bolt checkout, _portals and _launchers.
// Those are the names fabricengine.HubReservedNames() returns.
// This pins that the fast path resolves the prime worktree's own name, not one of those reserved siblings.

package fabricengine_test

import (
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

//testtiming:keep the fast-path layout equalling the resolved worktree layout; coverage of its blocks by other tests does not show an assertion of this
func TestWarpLayoutFor_FastPathMatchesResolveWorktree(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.SharedHub(t, hubforge.Shape{Anchor: "."})
	worktreeRoot := h.PrimeWorktree()

	fast, err := fabricengine.WarpLayoutForForTest(h.Location, worktreeRoot)
	if err != nil {
		t.Fatalf("warpLayoutFor(hub sibling): %v", err)
	}
	slow, err := lymcwd.ResolveWorktree(worktreeRoot)
	if err != nil {
		t.Fatalf("lymcwd.ResolveWorktree(%s): %v", worktreeRoot, err)
	}

	if *fast != *slow {
		t.Errorf("warpLayoutFor fast path = %+v; want it to equal the ResolveWorktree fallback %+v", *fast, *slow)
	}

	// Pin that the fast path resolved the prime warp worktree's own name, not a hub-reserved sibling.
	for _, reserved := range fabricengine.HubReservedNames() {
		if fast.WorktreeName == reserved {
			t.Errorf("warpLayoutFor resolved WorktreeName = %q, a hub-reserved name; want the prime worktree's own name", fast.WorktreeName)
		}
	}
	if fast.WorktreeName != filepath.Base(worktreeRoot) {
		t.Errorf("warpLayoutFor WorktreeName = %q; want %q (the prime worktree's own base name)", fast.WorktreeName, filepath.Base(worktreeRoot))
	}
}
