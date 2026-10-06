//go:build integration

// geometry_integration_test.go drives ReedGeometry and WebsterGeometry against one real hub from hubforge:
// the prime's reed geometry carries the hub's shortname with no slug or parent, a task worktree carries its slug and the parent its origin record names, or none when the origin names none, and a hub with no .lyx-shortname yields an empty shortname and no error.

package hubgeom_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

// TestGeometryOverRealHub builds one hub and runs each geometry case as a step over it, in the order below.
// The steps run serially, and the last removes the hub's shortname record, so it must stay last.
// The top-level test calls t.Parallel and no step does, because the steps share the one hub fixture.
func TestGeometryOverRealHub(t *testing.T) {
	t.Parallel()

	h := hubforge.NewHub(t, ".")

	if !t.Run("reed prime carries the shortname only", func(t *testing.T) {
		geom, err := hubgeom.ReedGeometry(h.Location)
		if err != nil {
			t.Fatalf("ReedGeometry(prime): %v", err)
		}
		if geom.NameShortname != hubforge.TestShortname {
			t.Errorf("NameShortname = %q; want %q", geom.NameShortname, hubforge.TestShortname)
		}
		if geom.NameSlug != "" || geom.ParentName != "" {
			t.Errorf("prime NameSlug/ParentName = %q/%q; want both empty", geom.NameSlug, geom.ParentName)
		}
	}) {
		return
	}

	if !t.Run("webster parent name comes from the origin record", func(t *testing.T) {
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
	}) {
		return
	}

	if !t.Run("reed task worktree carries its slug and the origin's parent", func(t *testing.T) {
		const slug = "naming-task"
		hubforge.AddPair(t, h, slug)

		l, err := lyxcwd.ResolveWorktree(h.PairWarpWorktree(slug))
		if err != nil {
			t.Fatalf("ResolveWorktree: %v", err)
		}

		before, err := hubgeom.ReedGeometry(l)
		if err != nil {
			t.Fatalf("ReedGeometry(task) before seeding: %v", err)
		}
		if before.NameShortname != hubforge.TestShortname || before.NameSlug != slug {
			t.Errorf("task NameShortname/NameSlug = %q/%q; want %q/%q", before.NameShortname, before.NameSlug, hubforge.TestShortname, slug)
		}
		if want := hubforge.TestShortname + ":orch"; before.ParentName != want {
			t.Errorf("ParentName before seeding = %q; want %q (the prime's orch, from the origin record)", before.ParentName, want)
		}

		origin, found, err := fabricengine.ReadOriginFor(l, slug)
		if err != nil || !found {
			t.Fatalf("ReadOriginFor = %v, found %v", err, found)
		}
		origin.ParentWorktree = "other-task"
		if err := fabricengine.WriteOrigin(&fabricengine.Mutations{}, l, slug, origin); err != nil {
			t.Fatalf("WriteOrigin with another parent_worktree: %v", err)
		}
		const parent = hubforge.TestShortname + ":other-task:orch"
		after, err := hubgeom.ReedGeometry(l)
		if err != nil {
			t.Fatalf("ReedGeometry(task) after replanting the parent: %v", err)
		}
		if after.ParentName != parent {
			t.Errorf("ParentName after replanting = %q; want %q", after.ParentName, parent)
		}

		origin.ParentWorktree = ""
		if err := fabricengine.WriteOrigin(&fabricengine.Mutations{}, l, slug, origin); err != nil {
			t.Fatalf("WriteOrigin without parent_worktree: %v", err)
		}
		none, err := hubgeom.ReedGeometry(l)
		if err != nil {
			t.Fatalf("ReedGeometry(task) without a parent worktree: %v", err)
		}
		if none.ParentName != "" {
			t.Errorf("ParentName without a parent worktree = %q; want empty", none.ParentName)
		}
	}) {
		return
	}

	// This step removes the shortname record every earlier step read, so it runs last.
	t.Run("hub with no shortname yields an empty shortname", func(t *testing.T) {
		if err := os.Remove(filepath.Join(h.BoardDir(), fabricengine.ShortnameFileName)); err != nil {
			t.Fatalf("remove shortname record: %v", err)
		}

		geom, err := hubgeom.ReedGeometry(h.Location)
		if err != nil {
			t.Fatalf("ReedGeometry on a hub with no shortname: %v", err)
		}
		if geom.NameShortname != "" {
			t.Errorf("NameShortname = %q; want empty", geom.NameShortname)
		}
	})
}
