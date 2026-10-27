// deps.go fills the verbs' deps over a real hub: the board reads and the owner write, the batten run's status and first pass, and the landing's refusal and land request.

package beamcli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/Knatte18/loomyard/internal/battencli"
	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/machinename"
	"github.com/Knatte18/loomyard/internal/pawlengine"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/state"
)

// hubCreateDeps fills createDeps over the hub the prime belongs to.
func hubCreateDeps(prime *lymcwd.Location) (createDeps, error) {
	config, err := fabricengine.LoadConfig(fabricengine.ConfigDirs(prime))
	if err != nil {
		return createDeps{}, err
	}
	topology := fabricengine.NewTopology(config)
	boardDir := fabricengine.BoardDir(prime.HubPath)

	return createDeps{
		PrimeName:  prime.WorktreeName,
		Machine:    machinename.Name,
		ReadOwners: func() (boardengine.Owners, error) { return boardengine.ReadOwners(boardDir) },
		ParentOf: func(slug string) (string, error) {
			return hubgeom.ParentOf(prime, slug)
		},
		ParentBeam: func(slug string) (string, error) {
			tasks, err := boardengine.ReadTasks(boardDir)
			if err != nil {
				return "", err
			}
			return boardengine.ParentBeam(tasks, slug), nil
		},
		PairPresent: func(slug string) bool { return pairWorktreePresent(prime, slug) },
		BranchOnOrigin: func(slug string) (bool, error) {
			return topology.TaskBranchOnOrigin(prime, slug)
		},
		SetOwners: func(req boardengine.OwnerRequest) (boardengine.OwnerWrite, error) {
			return boardengine.SetOwners(boardDir, req)
		},
		RunStatus: func(slug string) (shedengine.State, bool, error) {
			status, found, err := state.ReadJSON[shedengine.Status](battencli.StatusFile(prime, slug), shedrun.StatusLock(prime, slug))
			if errors.Is(err, fs.ErrNotExist) {
				return "", false, nil
			}
			return status.State, found, err
		},
		StartRun: func(slug, anchor string) (map[string]any, error) {
			return battencli.StartFirstPass(prime, slug, battencli.FirstPassOptions{Anchor: anchor})
		},
		WorktreePath: func(slug string) string { return fabricengine.WorktreePath(prime, slug) },
	}, nil
}

// hubLandDeps fills landDeps over the hub the prime belongs to.
func hubLandDeps(prime *lymcwd.Location) landDeps {
	beamOf := func(slug string) (*lymcwd.Location, error) {
		return lymcwd.ResolveWorktree(fabricengine.WorktreePath(prime, slug))
	}
	return landDeps{
		Machine: machinename.Name,
		ReadOwners: func() (boardengine.Owners, error) {
			return boardengine.ReadOwners(fabricengine.BoardDir(prime.HubPath))
		},
		ParentOf: func(slug string) (string, error) {
			return hubgeom.ParentOf(prime, slug)
		},
		PairPresent: func(slug string) bool { return pairWorktreePresent(prime, slug) },
		OriginRecord: func(slug string) (fabricengine.Origin, bool, error) {
			return fabricengine.ReadOriginFor(prime, slug)
		},
		ChildStatus: func(slug string) (shedengine.Status, bool, error) {
			beam, err := beamOf(slug)
			if err != nil {
				return shedengine.Status{}, false, err
			}
			statusPath := shedrun.StatusFile(beam, shedrun.SelfRunID)
			if _, err := os.Stat(statusPath); errors.Is(err, fs.ErrNotExist) {
				return shedengine.Status{}, false, nil
			}
			lockPath := shedrun.StatusLock(beam, shedrun.SelfRunID)
			if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
				return shedengine.Status{}, false, err
			}
			return state.ReadJSON[shedengine.Status](statusPath, lockPath)
		},
		RunID: func(slug string) (string, error) {
			beam, err := beamOf(slug)
			if err != nil {
				return "", err
			}
			seed, found, err := shedrun.ReadSeed(beam, shedrun.SelfRunID)
			switch {
			case err != nil:
				return "", err
			case !found || seed.StartedAt == "":
				return "", fmt.Errorf("the pawl run of %s has no seed start time", slug)
			}
			return seed.StartedAt, nil
		},
		Refusal: func(slug string) error { return battencli.BeamLandRefusal(prime, slug) },
		WriteRequest: func(slug string, req pawlengine.LandRequest) (string, error) {
			beam, err := beamOf(slug)
			if err != nil {
				return "", err
			}
			paths := hubgeom.PawlPaths(beam)
			return paths.LandRequest, pawlengine.WriteLandRequest(paths, req)
		},
		Now: time.Now,
	}
}

// pairWorktreePresent reports whether this machine holds the worktree of slug's pair.
func pairWorktreePresent(prime *lymcwd.Location, slug string) bool {
	info, err := os.Stat(fabricengine.WorktreePath(prime, slug))
	return err == nil && info.IsDir()
}
