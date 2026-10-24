// deps.go fills the verbs' deps over a real hub: the board reads and the owner write, the pair add, mark and records reset, the pair's push, the run's end and clear, and the workspace regeneration.

package beamcli

import (
	"context"
	"os"

	"github.com/Knatte18/loomyard/internal/battencli"
	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/machinename"
)

// hubCreateDeps fills createDeps over the hub the prime belongs to.
func hubCreateDeps(prime *lyxcwd.Location) (createDeps, error) {
	config, err := fabricengine.LoadConfig(fabricengine.ConfigDirs(prime))
	if err != nil {
		return createDeps{}, err
	}
	topology := fabricengine.NewTopology(config)
	boardDir := fabricengine.BoardDir(prime.HubPath)

	return createDeps{
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
		AddPair: func(slug string, opts fabricengine.AddOptions) (fabricengine.AddResult, error) {
			return topology.Add(prime, slug, opts)
		},
		MarkBeam: func(rec *fabricengine.Mutations, slug string) (bool, error) {
			return fabricengine.MarkBeam(rec, prime, slug)
		},
		SpawnPush: func(slug string) error {
			return spawnPairPush(prime, slug)
		},
		RegenerateWorkspace: func(rec *fabricengine.Mutations) (string, error) {
			return fabricengine.RegenerateHubWorkspace(rec, prime)
		},
		WorktreePath: func(slug string) string { return fabricengine.WorktreePath(prime, slug) },
		HubPath:      prime.HubPath,
	}, nil
}

// hubPromoteDeps fills promoteDeps over the hub the prime belongs to.
func hubPromoteDeps(prime *lyxcwd.Location) (promoteDeps, error) {
	create, err := hubCreateDeps(prime)
	if err != nil {
		return promoteDeps{}, err
	}
	return promoteDeps{
		createDeps: create,
		ReadTasks: func() ([]boardengine.Task, error) {
			return boardengine.ReadTasks(fabricengine.BoardDir(prime.HubPath))
		},
		OriginRecord: func(slug string) (fabricengine.Origin, bool, error) {
			return fabricengine.ReadOriginFor(prime, slug)
		},
		EndRun: func(ctx context.Context, slug string) error {
			return battencli.EndRunForPromote(ctx, prime, slug)
		},
		ClearRun: func(ctx context.Context, slug string) error {
			return battencli.ClearRunForPromote(ctx, prime, slug)
		},
		ResetRecords: func(rec *fabricengine.Mutations, slug string) (fabricengine.ResetRecordsResult, error) {
			return fabricengine.ResetRecordsToParent(rec, prime, slug)
		},
	}, nil
}

// hubLandDeps fills landDeps over the hub the prime belongs to.
func hubLandDeps(prime *lyxcwd.Location) (landDeps, error) {
	primeName, err := fabricengine.PrimeName(prime)
	if err != nil {
		return landDeps{}, err
	}
	return landDeps{
		ReadOwners: func() (boardengine.Owners, error) {
			return boardengine.ReadOwners(fabricengine.BoardDir(prime.HubPath))
		},
		PairPresent: func(slug string) bool { return pairWorktreePresent(prime, slug) },
		OriginRecord: func(slug string) (fabricengine.Origin, bool, error) {
			return fabricengine.ReadOriginFor(prime, slug)
		},
		ParentWorktree: func(slug string) (string, error) {
			origin, found, err := fabricengine.ReadOriginFor(prime, slug)
			if err != nil {
				return "", err
			}
			if found && origin.ParentWorktree != "" {
				return origin.ParentWorktree, nil
			}
			return primeName, nil
		},
		Refusal: func(slug string) error { return battencli.BeamLandRefusal(prime, slug) },
		StartLanding: func(slug string) (map[string]any, error) {
			return battencli.ResumeInWindow(prime, slug)
		},
	}, nil
}

// pairWorktreePresent reports whether this machine holds the worktree of slug's pair.
func pairWorktreePresent(prime *lyxcwd.Location, slug string) bool {
	info, err := os.Stat(fabricengine.WorktreePath(prime, slug))
	return err == nil && info.IsDir()
}

// spawnPairPush launches the detached push of slug's pair, resolved from the prime's hub.
func spawnPairPush(prime *lyxcwd.Location, slug string) error {
	pair, err := lyxcwd.ResolveWorktree(fabricengine.WorktreePath(prime, slug))
	if err != nil {
		return err
	}
	return fabricengine.SpawnPairPush(pair)
}
