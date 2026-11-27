// engineworktree.go tells a launching engine the worktree its sessions run in, as a shuttleengine.Worktree.

package hubgeom

import (
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// EngineWorktree builds the shuttleengine.Worktree an engine is told for the worktree at l.
// Its Kind is prime when l is the hub's prime, beam when l's pair is a beam, and task otherwise.
// A worktree whose prime read fails is taken as the prime, so a worktree of unknown kind is never treated as a task's.
// A failed beam read is taken as a task, and both failures are logged at Warn.
// Its Names reads the hub's current worktree names when called, and does nothing before.
func EngineWorktree(l *lymcwd.Location) shuttleengine.Worktree {
	return shuttleengine.Worktree{
		Name:       l.WorktreeName,
		Kind:       engineWorktreeKind(l),
		AccountDir: fabriclayout.ClaudeAccountsDir(l.HubPath),
		Names: func() ([]string, error) {
			return liveWorktreeNames(l.WorktreePath())
		},
	}
}

// engineWorktreeKind reads l's kind for EngineWorktree.
func engineWorktreeKind(l *lymcwd.Location) shuttleengine.WorktreeKind {
	prime, err := IsPrime(l)
	if err != nil {
		logger.Warn("hubgeom: could not tell whether the worktree is the prime, so it is taken as the prime", "worktree", l.WorktreeName, "err", err)
		return shuttleengine.WorktreePrime
	}
	if prime {
		return shuttleengine.WorktreePrime
	}
	beam, err := pairIsBeam(l)
	if err != nil {
		logger.Warn("hubgeom: could not tell whether the pair is a beam, so it is taken as a task", "worktree", l.WorktreeName, "err", err)
		return shuttleengine.WorktreeTask
	}
	if beam {
		return shuttleengine.WorktreeBeam
	}
	return shuttleengine.WorktreeTask
}

// liveWorktreeNames returns the base names of the worktrees of the repository holding dir.
// It leaves out an entry git marks prunable and one whose directory is gone from disk.
func liveWorktreeNames(dir string) ([]string, error) {
	entries, err := fabricengine.List(dir)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.Prunable {
			continue
		}
		if _, err := os.Stat(entry.Path); err != nil {
			continue
		}
		names = append(names, filepath.Base(entry.Path))
	}
	return names, nil
}
