// target.go names the worktree a pusher serves and the flags and lock path that address its process.

package runpusher

import (
	"fmt"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/lymdirs"
)

const (
	// WorktreeFlag is the flag of the pusher verb that names the code worktree it serves.
	WorktreeFlag = "worktree"
	// SlugFlag is the flag of the pusher verb that names the run slug of a pair.
	SlugFlag = "slug"
)

// Target names the worktree one pusher serves.
type Target struct {
	// Location is the code worktree whose records worktree the pusher pushes.
	Location *lymcwd.Location
	// Slug is the run the pair's pusher serves, empty for the prime.
	Slug string
	// Pair reports whether Location is a task pair rather than the prime.
	Pair bool
}

// TargetOf builds the target of the worktree l for the run slug.
// The prime's target carries an empty slug whatever slug is passed:
// its pusher carries every batten run's commits and names no one run.
func TargetOf(l *lymcwd.Location, slug string) (Target, error) {
	prime, err := fabricengine.IsPrimeWorktree(l)
	if err != nil {
		return Target{}, fmt.Errorf("runpusher: tell whether %s is the prime: %w", l.WorktreePath(), err)
	}
	if prime {
		return Target{Location: l}, nil
	}
	return Target{Location: l, Slug: slug, Pair: true}, nil
}

// LockPath returns the lock file of the pusher of t.
// It is one file per records worktree, under the hub's never-tracked directory.
func LockPath(t Target) string {
	name := filepath.Base(fabricengine.RecordsWorktree(t.Location)) + ".lock"
	return filepath.Join(fabriclayout.HubScratchDir(t.Location.HubPath), lymdirs.PusherDirName, name)
}
