//go:build tier1

// dobbydeps_integration_test.go pins the seams hubDobbyDeps fills differently for a beam's dobby and the prime's, over a hubforge hub.

package beamcli

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/hubgeom"
)

func TestHubDobbyDeps_PrimeTidyShuttle(t *testing.T) {
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	hubforge.AddPair(t, h, "task1")
	prime := h.Location

	beamGeom, err := hubgeom.DobbyGeometry(prime, "task1")
	if err != nil {
		t.Fatalf("DobbyGeometry(task1) = %v; want nil", err)
	}
	beamDeps, err := hubDobbyDeps(prime, beamGeom)
	if err != nil {
		t.Fatalf("hubDobbyDeps(beam) = %v; want nil", err)
	}
	stopper, ok := beamDeps.PrimeTidyShuttle.(*runnerTidyShuttle)
	if !ok || stopper.worktree != prime {
		t.Errorf("beam PrimeTidyShuttle = %#v; want a *runnerTidyShuttle over the prime's location", beamDeps.PrimeTidyShuttle)
	}

	primeGeom, err := hubgeom.DobbyGeometry(prime, prime.WorktreeName)
	if err != nil {
		t.Fatalf("DobbyGeometry(prime) = %v; want nil", err)
	}
	primeDeps, err := hubDobbyDeps(prime, primeGeom)
	if err != nil {
		t.Fatalf("hubDobbyDeps(prime) = %v; want nil", err)
	}
	if primeDeps.PrimeTidyShuttle != nil {
		t.Errorf("prime PrimeTidyShuttle = %#v; want nil", primeDeps.PrimeTidyShuttle)
	}
}
