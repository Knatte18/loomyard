// wiring.go builds the seams Reset is told from a hub's task worktree location:
// the geometry, the shuttle runner and reed engine, and fabric's reset, history, read and commit calls.

package weavercli

import (
	"fmt"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/shuttleengines"
	"github.com/Knatte18/loomyard/internal/weaverengine"
)

// resetDepsFor builds the ResetDeps of the run in the task worktree loc names.
func resetDepsFor(loc *lymcwd.Location) (weaverengine.ResetDeps, error) {
	geom := hubgeom.WeaverGeometry(loc)

	shuttleCfg, err := shuttleengine.LoadConfig(fabricengine.ConfigDirs(loc), "shuttle")
	if err != nil {
		return weaverengine.ResetDeps{}, err
	}
	reedCfg, err := reedengine.LoadConfig(fabricengine.ConfigDirs(loc), "reed")
	if err != nil {
		return weaverengine.ResetDeps{}, err
	}
	reedGeom, err := hubgeom.ReedGeometry(loc)
	if err != nil {
		return weaverengine.ResetDeps{}, err
	}
	reedEngine := reedengine.New(reedCfg, reedGeom, shuttleengines.EnvScrub())
	engines, err := shuttleengines.New(shuttleCfg, fabricengine.ConfigDirs(loc))
	if err != nil {
		return weaverengine.ResetDeps{}, err
	}
	runner := shuttleengine.NewRunner(reedEngine, engines, reedGeom.AnchorPath, reedGeom.WorktreeRoot, shuttleCfg)

	planRel, err := filepath.Rel(geom.AnchorRoot, geom.PlanDir)
	if err != nil {
		return weaverengine.ResetDeps{}, fmt.Errorf("weaver: place the plan directory under the anchor: %w", err)
	}
	planRel = filepath.ToSlash(planRel)
	recordRels := []string{weaverengine.DirRel(), planRel}
	scoped := fabriclayout.ScopedPathspec(loc.AnchorRel, recordRels)

	return weaverengine.ResetDeps{
		Geom:   geom,
		Runner: runner,
		Reed:   reedEngine,
		StateHistory: func() ([]weaverengine.StateVersion, error) {
			versions, err := fabricengine.AnchoredFileFirstParentHistory(loc, weaverengine.StateRel())
			if err != nil {
				return nil, err
			}
			history := make([]weaverengine.StateVersion, len(versions))
			for i, version := range versions {
				history[i] = weaverengine.StateVersion{Commit: version.SHA, Data: version.Data}
			}
			return history, nil
		},
		ReadPlanAt: func(rev, name string) ([]byte, bool, error) {
			return fabricengine.AnchoredFileAt(loc, rev, path.Join(planRel, name))
		},
		ResetCode: func(sha string, ownPaths []string) error {
			fab, err := fabricengine.Open(loc)
			if err != nil {
				return err
			}
			origin, found, err := fabricengine.ReadOrigin(loc)
			if err != nil {
				return err
			}
			if !found {
				return fmt.Errorf("the pair has no fabric origin record, so its parent branch is unknown; way forward: run `lym fabric reconcile` to repair the pair")
			}
			return fab.ResetPairCode(fabricengine.NewMutations(""), sha, origin.ParentBranch, ownPaths, fabricengine.EnvSyncOptions())
		},
		RecordPending: func() (bool, error) {
			changed, err := gitrepo.New(fabricengine.RecordsWorktree(loc)).WorktreeChangedFiles()
			if err != nil {
				return false, err
			}
			for _, file := range changed {
				for _, prefix := range scoped {
					if strings.HasPrefix(filepath.ToSlash(file), filepath.ToSlash(prefix)) {
						return true, nil
					}
				}
			}
			return false, nil
		},
		Commit: func(msg string) error {
			var present []string
			for _, rel := range recordRels {
				has, err := fabricengine.AnchoredPathHasContent(loc, rel)
				if err != nil {
					return err
				}
				if has {
					present = append(present, rel)
				}
			}
			if len(present) == 0 {
				return nil
			}
			_, _, err := fabricengine.CommitAnchoredPaths(fabricengine.NewMutations(""), loc, present, msg, fabricengine.EnvSyncOptions())
			return err
		},
		DirtyTracked: func() ([]string, error) {
			repo := gitrepo.New(geom.WorktreeRoot)
			changed, err := repo.WorktreeChangedFiles()
			if err != nil {
				return nil, err
			}
			untracked, err := repo.UntrackedFiles()
			if err != nil {
				return nil, err
			}
			var tracked []string
			for _, file := range changed {
				isUntracked := false
				for _, other := range untracked {
					if other == file {
						isUntracked = true
					}
				}
				if !isUntracked {
					tracked = append(tracked, file)
				}
			}
			return tracked, nil
		},
		Now: time.Now,
	}, nil
}
