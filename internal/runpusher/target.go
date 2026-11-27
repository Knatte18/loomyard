// target.go names the worktree a pusher serves and the flags and lock path that address its process.

package runpusher

import (
	"fmt"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/pushledger"
)

const (
	// WorktreeFlag is the flag of the pusher verb that names the code worktree it serves.
	WorktreeFlag = fabriclayout.PusherWorktreeFlag
	// SlugFlag is the flag of the pusher verb that names the run slug of a pair.
	SlugFlag = fabriclayout.PusherSlugFlag
)

// TargetKind says which branch a pusher brings to origin.
type TargetKind string

const (
	// KindRecords is the records of the prime or a pair, and the zero value.
	KindRecords TargetKind = ""
	// KindBoard is the board's branch.
	KindBoard TargetKind = "board"
	// KindLanding is the landing target of the prime's code branch.
	KindLanding TargetKind = "landing"
)

// Target names the branch one pusher serves.
type Target struct {
	// Location is the code worktree whose records worktree the pusher pushes; the prime for the board and landing kinds.
	Location *lymcwd.Location
	// Slug is the run the pair's pusher serves, empty for every other target.
	Slug string
	// Pair reports whether Location is a task pair rather than the prime.
	Pair bool
	// Kind says which branch the target pushes.
	Kind TargetKind
}

// BoardTarget returns the target of the board's branch of the hub of prime.
func BoardTarget(prime *lymcwd.Location) Target {
	return Target{Location: prime, Kind: KindBoard}
}

// LandingTarget returns the landing target of the code branch of prime.
func LandingTarget(prime *lymcwd.Location) Target {
	return Target{Location: prime, Kind: KindLanding}
}

// ledgerKey returns the key of t's entry in the push ledger.
func ledgerKey(t Target) pushledger.Key {
	switch t.Kind {
	case KindBoard:
		return pushledger.BoardKey
	case KindLanding:
		return pushledger.LandingKey
	default:
		return pushledger.RecordsKey(fabricengine.RecordsWorktree(t.Location))
	}
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
// It is one file per target, under the hub's never-tracked directory.
func LockPath(t Target) string {
	var name string
	switch t.Kind {
	case KindBoard, KindLanding:
		name = string(t.Kind)
	default:
		name = filepath.Base(fabricengine.RecordsWorktree(t.Location))
	}
	return filepath.Join(fabriclayout.HubScratchDir(t.Location.HubPath), lymdirs.PusherDirName, name+".lock")
}
