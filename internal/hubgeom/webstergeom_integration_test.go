//go:build integration

// webstergeom_integration_test.go drives WebsterGeometry against a real hub from hubforge:
// a task worktree's geometry carries the parent its origin record names, and none when the origin names none.

package hubgeom_test

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

func TestWebsterGeometry_ParentNameFromOriginRecord(t *testing.T) {
	h := hubforge.NewHub(t, ".")
	const slug = "webster-parent-task"
	hubforge.AddPair(t, h, slug)

	l, err := lyxcwd.ResolveWorktree(h.PairWarpWorktree(slug))
	if err != nil {
		t.Fatalf("ResolveWorktree: %v", err)
	}

	if got, want := hubgeom.WebsterGeometry(l).ParentName, hubforge.TestShortname+":orch"; got != want {
		t.Errorf("ParentName = %q; want %q (the prime's orch, from the origin record)", got, want)
	}

	origin, found, err := fabricengine.ReadOriginFor(l, slug)
	if err != nil || !found {
		t.Fatalf("ReadOriginFor = %v, found %v", err, found)
	}
	origin.ParentWorktree = ""
	if err := fabricengine.WriteOrigin(&fabricengine.Mutations{}, l, slug, origin); err != nil {
		t.Fatalf("WriteOrigin without parent_worktree: %v", err)
	}
	if got := hubgeom.WebsterGeometry(l).ParentName; got != "" {
		t.Errorf("ParentName without a parent worktree = %q; want empty", got)
	}
}
