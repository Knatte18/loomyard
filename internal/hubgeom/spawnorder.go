// spawnorder.go tells reed the hub's worktrees in spawn order, so a restarted server's sessions are revived in the order the first server numbered them.

package hubgeom

import (
	"os"
	"slices"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// spawnCandidate is one worktree of the hub with what its place in spawn order is decided by.
type spawnCandidate struct {
	location *lyxcwd.Location
	// prime marks the hub's prime, which spawns before every pair.
	prime bool
	// started is the pair's run start time; meaningful only when dated.
	started time.Time
	dated   bool
}

// sortSpawnOrder returns candidates in spawn order: the prime first, then the pairs with a start time ascending, then the pairs without one by worktree name.
// Ties break by worktree name.
func sortSpawnOrder(candidates []spawnCandidate) []spawnCandidate {
	ordered := slices.Clone(candidates)
	slices.SortStableFunc(ordered, func(a, b spawnCandidate) int {
		if a.prime != b.prime {
			if a.prime {
				return -1
			}
			return 1
		}
		if a.dated != b.dated {
			if a.dated {
				return -1
			}
			return 1
		}
		if a.dated && !a.started.Equal(b.started) {
			return a.started.Compare(b.started)
		}
		return strings.Compare(a.location.WorktreeName, b.location.WorktreeName)
	})
	return ordered
}

// runStartTime reads the start time of l's run as the modification time of the run's seed, written once when the run is seeded.
// A pair whose seed is absent or unreadable has none.
// The seed sits in the fabric-synced _lyx tree, so a checkout or re-materialization that rewrites it moves the time.
func runStartTime(l *lyxcwd.Location) (time.Time, bool) {
	info, err := os.Stat(shedrun.SeedFile(l, shedrun.SelfRunID))
	if err != nil {
		return time.Time{}, false
	}
	return info.ModTime(), true
}

// spawnOrder returns the function reed calls, only when a revival is due, to list the hub's worktrees in spawn order.
// Prunable worktrees and worktrees that do not resolve are skipped, the latter with a warning.
func spawnOrder(l *lyxcwd.Location) func() ([]reedengine.ReviveEntry, error) {
	return func() ([]reedengine.ReviveEntry, error) {
		worktrees, err := fabricengine.List(l.WorktreePath())
		if err != nil {
			return nil, err
		}
		var candidates []spawnCandidate
		for _, worktree := range worktrees {
			if worktree.Prunable {
				continue
			}
			location, err := lyxcwd.ResolveWorktree(worktree.Path)
			if err != nil {
				logger.Warn("hubgeom: skipping a worktree that does not resolve in the spawn order", "worktree", worktree.Path, "err", err)
				continue
			}
			started, dated := runStartTime(location)
			candidates = append(candidates, spawnCandidate{location: location, prime: worktree.Main, started: started, dated: dated})
		}

		var entries []reedengine.ReviveEntry
		for _, candidate := range sortSpawnOrder(candidates) {
			location := candidate.location
			entries = append(entries, reedengine.ReviveEntry{
				Worktree: location.WorktreeName,
				Revive:   func() (bool, error) { return reviveWorktree(location) },
			})
		}
		return entries, nil
	}
}

// reviveWorktree builds the reed engine of l's worktree from its own reed.yaml and geometry and revives its recorded session.
func reviveWorktree(l *lyxcwd.Location) (bool, error) {
	cfg, err := reedengine.LoadConfig(l.AnchorPath(), "reed")
	if err != nil {
		return false, err
	}
	geom, err := ReedGeometry(l)
	if err != nil {
		return false, err
	}
	return reedengine.New(cfg, geom).Revive()
}
