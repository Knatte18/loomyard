//go:build integration

// reedgeom_integration_test.go drives ReedGeometry against a real hub from hubforge:
// the prime's geometry carries the hub's shortname with no slug or parent, a task worktree carries its slug and the parent its origin record names, or none when the origin names none,
// and a hub with no .lyx-shortname yields an empty shortname and no error.

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

func TestReedGeometry_PrimeCarriesShortnameOnly(t *testing.T) {
	h := hubforge.NewHub(t, ".")

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
}

func TestReedGeometry_TaskWorktreeCarriesSlugAndOriginParent(t *testing.T) {
	h := hubforge.NewHub(t, ".")
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
}

func TestReedGeometry_HubWithNoShortnameYieldsEmptyShortname(t *testing.T) {
	h := hubforge.NewHub(t, ".")
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
}
