//go:build integration

// warplayout_test.go pins warpLayoutFor's two branches against each other: the spawn-free hub-sibling
// fast path and the spawning lyxcwd.ResolveWorktree fallback are documented as equivalent, so this
// asserts they actually produce the same Location for the same worktree rather than one branch
// quietly omitting a field.

package fabricengine

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/lyxtest"
)

func TestWarpLayoutFor_FastPathMatchesResolveWorktree(t *testing.T) {
	t.Parallel()

	fixture := lyxtest.CopyWarpHub(t)

	base, err := lyxcwd.Resolve(fixture.Hub)
	if err != nil {
		t.Fatalf("lyxcwd.Resolve(%s): %v", fixture.Hub, err)
	}

	fast, err := warpLayoutFor(base, fixture.Hub)
	if err != nil {
		t.Fatalf("warpLayoutFor(hub sibling): %v", err)
	}
	slow, err := lyxcwd.ResolveWorktree(fixture.Hub)
	if err != nil {
		t.Fatalf("lyxcwd.ResolveWorktree(%s): %v", fixture.Hub, err)
	}

	if *fast != *slow {
		t.Errorf("warpLayoutFor fast path = %+v; want it to equal the ResolveWorktree fallback %+v", *fast, *slow)
	}
}
