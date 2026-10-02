//go:build integration

// reedgeom_integration_test.go drives ReedGeometry against a real hub from hubforge:
// the prime's geometry carries the hub's code with no slug or parent, a task worktree carries its slug and the parent its default run's seed records,
// and a hub with no .lyx-code yields an empty code and no error.

package hubgeom_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

func TestReedGeometry_PrimeCarriesCodeOnly(t *testing.T) {
	h := hubforge.NewHub(t, ".")

	geom, err := hubgeom.ReedGeometry(h.Location)
	if err != nil {
		t.Fatalf("ReedGeometry(prime): %v", err)
	}
	if geom.NameCode != hubforge.TestCode {
		t.Errorf("NameCode = %q; want %q", geom.NameCode, hubforge.TestCode)
	}
	if geom.NameSlug != "" || geom.ParentName != "" {
		t.Errorf("prime NameSlug/ParentName = %q/%q; want both empty", geom.NameSlug, geom.ParentName)
	}
}

func TestReedGeometry_TaskWorktreeCarriesSlugAndSeededParent(t *testing.T) {
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
	if before.NameCode != hubforge.TestCode || before.NameSlug != slug {
		t.Errorf("task NameCode/NameSlug = %q/%q; want %q/%q", before.NameCode, before.NameSlug, hubforge.TestCode, slug)
	}
	if before.ParentName != "" {
		t.Errorf("ParentName before seeding = %q; want empty", before.ParentName)
	}

	const parent = "tst:hub"
	seed := shedrun.Seed{Recipe: shedrun.RecipeNames()[0], Driver: shedrun.DriverGo, Parent: parent}
	if err := shedrun.WriteSeed(l, shedrun.SelfRunID, seed); err != nil {
		t.Fatalf("WriteSeed: %v", err)
	}
	after, err := hubgeom.ReedGeometry(l)
	if err != nil {
		t.Fatalf("ReedGeometry(task) after seeding: %v", err)
	}
	if after.ParentName != parent {
		t.Errorf("ParentName after seeding = %q; want %q", after.ParentName, parent)
	}
}

func TestReedGeometry_UncodedHubYieldsEmptyCode(t *testing.T) {
	h := hubforge.NewHub(t, ".")
	if err := os.Remove(filepath.Join(h.BoardDir(), fabricengine.CodeFileName)); err != nil {
		t.Fatalf("remove code record: %v", err)
	}

	geom, err := hubgeom.ReedGeometry(h.Location)
	if err != nil {
		t.Fatalf("ReedGeometry on an uncoded hub: %v", err)
	}
	if geom.NameCode != "" {
		t.Errorf("NameCode = %q; want empty", geom.NameCode)
	}
}
