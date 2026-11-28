// autonomy.go is the one reader of `beam.autonomy` for a run or an orch.
// It also places the hub's prime from the git files, with no git process.

package hubgeom

import (
	"errors"
	"fmt"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/dobbyengine"
	"github.com/Knatte18/loomyard/internal/dotgit"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
)

// autonomyFailures keeps one episode per worktree whose autonomy read failed, so a failure that repeats warns once.
var autonomyFailures logger.EpisodeSet

// Autonomy returns the beam.autonomy level of the worktree slug, read at the moment of the call.
// A run's reader passes the run's own pair slug.
// An orch's reader passes its own worktree's name, so the lookup starts at its own worktree and never at its parent's.
// The prime's own name reads the prime's chain, the same as an empty slug.
// location is any worktree of the hub and only locates it.
// The prime is resolved from the hub's git files, so the reader spawns no git.
// Any failure reads AutonomyOff and warns once per episode.
// A failure is a prime that does not resolve, a chain that does not read, or a stack that does not load.
// A value the key does not take is one of these failures.
// AutonomyOff falls back to asking the operator and never to skipping one.
func Autonomy(location *lymcwd.Location, slug string) dobbyengine.AutonomyLevel {
	level, _, _ := AutonomyWithRefusal(location, slug)
	return level
}

// AutonomyWithRefusal returns the level Autonomy returns, read and warned the same way.
// When the read fails on a value the key does not take, it also returns that refusal.
// The refusal names the file that set the value, the values the key takes and the way forward.
// Every other failure comes back with a nil refusal.
// read reports that the read succeeded, which a caller needs to tell a good value from a failure with no refusal.
func AutonomyWithRefusal(location *lymcwd.Location, slug string) (level dobbyengine.AutonomyLevel, refusal error, read bool) {
	name := slug
	if name == "" {
		name = location.WorktreeName
	}
	prime, err := PrimeWithoutGit(location)
	if err != nil {
		return autonomyReadFailed(name, err), nil, false
	}
	if slug == prime.WorktreeName {
		slug = ""
	}
	if slug == "" {
		name = prime.WorktreeName
	}
	chain, err := TaskSettingChain(prime, slug)
	if err != nil {
		return autonomyReadFailed(name, err), nil, false
	}
	level, err = dobbyengine.LookupAutonomy(chain)
	if err != nil {
		if errors.Is(err, configengine.ErrInvalid) {
			refusal = err
		}
		return autonomyReadFailed(name, err), refusal, false
	}
	autonomyFailures.Recover(name, "hubgeom: read autonomy recovered", "worktree", name)
	return level, nil, true
}

// autonomyReadFailed warns of a failed read of the worktree name's autonomy, once per episode.
// It answers the AutonomyOff that a failed read reads as.
func autonomyReadFailed(name string, cause error) dobbyengine.AutonomyLevel {
	autonomyFailures.Fail(name, "hubgeom: read autonomy failed, reading false", "worktree", name, "cause", cause)
	return dobbyengine.AutonomyOff
}

// PrimeWithoutGit returns the Location of the hub's main worktree, the prime, from the hub's git files.
// It spawns no git, so a caller that must start no process can place the prime.
// location is any worktree of the hub.
func PrimeWithoutGit(location *lymcwd.Location) (*lymcwd.Location, error) {
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
