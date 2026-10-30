//go:build tier1

// dobbydeps_integration_test.go pins the seams hubDobbyDeps fills, those that differ between a beam's dobby and the prime's and those it fills for both, over a hubforge hub.

package beamcli

import (
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/dobbyengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/shedrun"
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

	// The seam reads the host's real cgroup, so it is checked for being filled and never called.
	if beamDeps.TaskLimit == nil {
		t.Errorf("beam TaskLimit = nil; want the task-limit reader")
	}
	if primeDeps.TaskLimit == nil {
		t.Errorf("prime TaskLimit = nil; want the task-limit reader")
	}

	// Both dobbies send the run notices, with the batten recipe's quiet window and the marker in the run's records directory.
	for name, notices := range map[string]dobbyengine.RunNoticeDeps{"beam": beamDeps.RunNotices, "prime": primeDeps.RunNotices} {
		if notices.ReadChild == nil || notices.DriverAlive == nil || notices.Agents == nil || notices.Pause == nil || notices.ReviveAPIError == nil || notices.MarkerPath == nil {
			t.Errorf("%s RunNotices = %+v; want every seam filled", name, notices)
			continue
		}
		if notices.Quiet != 30*time.Minute {
			t.Errorf("%s RunNotices.Quiet = %s; want the Run-Shed row's 30m", name, notices.Quiet)
		}
		if got, want := notices.MarkerPath("task1"), shedrun.NoticeEpisodeFile(prime, "task1"); got != want {
			t.Errorf("%s RunNotices.MarkerPath(task1) = %q; want %q", name, got, want)
		}
	}
}
