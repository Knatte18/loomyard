// autonomy.go is the one reader of beam.yaml's `autonomy` for a run or an orch.

package hubgeom

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/dobbyengine"
	"github.com/Knatte18/loomyard/internal/dotgit"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
)

// autonomyFailures keeps one episode per worktree whose autonomy read failed, so a failure that repeats warns once.
var autonomyFailures logger.EpisodeSet

// Autonomy reports whether autonomy is on for the worktree slug, read at the moment of the call.
// A run's reader passes the run's own pair slug.
// An orch's reader passes its own worktree's name, so the lookup starts at its own worktree and never at its parent's.
// The prime's own name reads the prime's chain, the same as an empty slug.
// location is any worktree of the hub and only locates it.
// The prime is resolved from the hub's git files, so the reader spawns no git.
// Any failure reads false and warns once per episode.
// A failure is a prime that does not resolve, a chain that does not read, or a stack that does not load.
// False falls back to asking the operator and never to skipping one.
func Autonomy(location *lymcwd.Location, slug string) bool {
	name := slug
	if name == "" {
		name = location.WorktreeName
	}
	prime, err := resolvePrimeWithoutGit(location)
	if err != nil {
		return autonomyReadFailed(name, err)
	}
	if slug == prime.WorktreeName {
		slug = ""
	}
	if slug == "" {
		name = prime.WorktreeName
	}
	chain, err := TaskSettingChain(prime, slug)
	if err != nil {
		return autonomyReadFailed(name, err)
	}
	on, err := dobbyengine.LookupAutonomy(chain)
	if err != nil {
		return autonomyReadFailed(name, err)
	}
	autonomyFailures.Recover(name, "hubgeom: read autonomy recovered", "worktree", name)
	return on
}

// autonomyReadFailed warns of a failed read of the worktree name's autonomy, once per episode.
// It answers the false that a failed read reads as.
func autonomyReadFailed(name string, cause error) bool {
	autonomyFailures.Fail(name, "hubgeom: read autonomy failed, reading false", "worktree", name, "cause", cause)
	return false
}

// resolvePrimeWithoutGit returns the Location of the hub's main worktree, the prime, from the hub's git files.
func resolvePrimeWithoutGit(location *lymcwd.Location) (*lymcwd.Location, error) {
	worktrees, err := dotgit.ListWorktrees(location.WorktreePath())
	if err != nil {
		return nil, fmt.Errorf("hubgeom: list the worktrees of the hub: %w", err)
	}
	prime, err := lymcwd.ResolveWorktree(worktrees[0].Path)
	if err != nil {
		return nil, fmt.Errorf("hubgeom: resolve the prime worktree: %w", err)
	}
	return prime, nil
}
