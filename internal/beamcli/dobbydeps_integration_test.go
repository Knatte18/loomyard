//go:build tier1

// dobbydeps_integration_test.go pins the seams hubDobbyDeps fills, those that differ between a beam's dobby and the prime's and those it fills for both, over a hubforge hub.

package beamcli

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/dobbyengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/machinename"
	"github.com/Knatte18/loomyard/internal/pawlengine"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/state"
	"github.com/Knatte18/loomyard/internal/testkit/boardkit"
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
}

// landingHub returns a hubforge hub holding the beam b, owned by this machine, whose pawl run waits at Beam-Await, and the seam set of b's dobby over it.
func landingHub(t *testing.T) (*hubforge.Hub, dobbyengine.Deps) {
	t.Helper()
	boardkit.InstallReplay(t)
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	hubforge.AddPairWith(t, h, "b", fabricengine.AddOptions{SkipPush: true, Beam: true})

	cfg, err := boardengine.LoadConfig(configengine.Dirs{Board: h.BoardDir()}, "board")
	if err != nil {
		t.Fatalf("load board config: %v", err)
	}
	cfg.Path = h.BoardDir()
	if _, err := boardengine.New(cfg).UpsertTask(map[string]any{"slug": "b", "kind": "project", "title": "b", "labels": []string{"bug"}}); err != nil {
		t.Fatalf("seed the board entry b: %v", err)
	}
	machine, err := machinename.Name()
	if err != nil {
		t.Fatalf("machine name: %v", err)
	}
	if _, err := boardengine.SetOwners(h.BoardDir(), boardengine.OwnerRequest{Machine: machine, Slugs: []string{"b"}, Beam: true}); err != nil {
		t.Fatalf("flag b as a beam: %v", err)
	}

	beam, err := lymcwd.ResolveWorktree(fabricengine.WorktreePath(h.Location, "b"))
	if err != nil {
		t.Fatalf("resolve the beam worktree: %v", err)
	}
	if err := shedrun.WriteSeed(beam, shedrun.SelfRunID, shedrun.Seed{Recipe: "pawl", Driver: shedrun.DriverGo}); err != nil {
		t.Fatalf("write the pawl run's seed: %v", err)
	}
	lockPath := shedrun.StatusLock(beam, shedrun.SelfRunID)
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		t.Fatalf("create the status lock directory: %v", err)
	}
	waiting := shedengine.Status{State: shedengine.StateRunning, CurrentProducer: pawlengine.NameBeamAwait}
	if err := state.WriteJSON(shedrun.StatusFile(beam, shedrun.SelfRunID), lockPath, waiting); err != nil {
		t.Fatalf("write the pawl run's status: %v", err)
	}

	geom, err := hubgeom.DobbyGeometry(h.Location, "b")
	if err != nil {
		t.Fatalf("DobbyGeometry(b) = %v; want nil", err)
	}
	deps, err := hubDobbyDeps(h.Location, geom)
	if err != nil {
		t.Fatalf("hubDobbyDeps(b) = %v; want nil", err)
	}
	return h, deps
}

func TestHubDobbyDeps_LandsOnlyAFlaggedBeam(t *testing.T) {
	t.Parallel()

	h, deps := landingHub(t)
	beam, err := lymcwd.ResolveWorktree(fabricengine.WorktreePath(h.Location, "b"))
	if err != nil {
		t.Fatalf("resolve the beam worktree: %v", err)
	}
	requestPath := hubgeom.PawlPaths(beam).LandRequest

	reading, err := deps.CanLand()
	if err != nil || reading.Passes {
		t.Fatalf("CanLand() = %+v, %v for an unflagged beam; want a reading that does not pass", reading, err)
	}
	err = deps.Land()
	if err == nil || !strings.Contains(err.Error(), "lym beam ready b") {
		t.Fatalf("Land() = %v for an unflagged beam; want a refusal naming `lym beam ready b`", err)
	}
	if _, statErr := os.Stat(requestPath); !errors.Is(statErr, fs.ErrNotExist) {
		t.Errorf("land request after a refused Land: stat = %v; want none written", statErr)
	}

	if _, err := markReady(hubLandDeps(h.Location), "b"); err != nil {
		t.Fatalf("markReady(b) = %v; want nil", err)
	}
	reading, err = deps.CanLand()
	if err != nil || !reading.Passes || reading.ParentOwner != "" {
		t.Fatalf("CanLand() = %+v, %v for a flagged beam; want a passing reading with no foreign parent owner", reading, err)
	}
	if err := deps.Land(); err != nil {
		t.Fatalf("Land() = %v for a flagged beam; want nil", err)
	}
	if _, statErr := os.Stat(requestPath); statErr != nil {
		t.Errorf("land request after Land: stat = %v; want the request `lym beam land` writes", statErr)
	}
}
