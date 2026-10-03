// reviewstatus.go fills shedverbs.Hooks.Waiting for loom, so `lyx loom status` reports a run that waits on its reviewer.

package loomcli

import (
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/parentreview"
	"github.com/Knatte18/loomyard/internal/verifytree"
)

// reviewWaitPrefix leads the note so a status reader sees which wait it is.
const reviewWaitPrefix = "parent review: "

// reviewWaiting returns the Waiting hook over a parent-review store rooted at root, with its lock files under lockDir.
// Only the latest round is reported, and an absent root yields an empty note rather than an error.
func reviewWaiting(root, lockDir string) func() (string, error) {
	store := parentreview.Store{Root: root, LockDir: lockDir}
	return func() (string, error) {
		note, err := store.WaitNote()
		if err != nil || note == "" {
			return "", err
		}
		return reviewWaitPrefix + note, nil
	}
}

// verifyWaiting returns the Waiting hook that reports a running verify from the marker at markerPath, and falls through to next otherwise.
// A marker whose pid is dead reads as absent, so a crashed verify never sticks in the status line.
func verifyWaiting(markerPath string, next func() (string, error)) func() (string, error) {
	return func() (string, error) {
		m, live, err := verifytree.ReadMarker(markerPath)
		if err != nil {
			return "", err
		}
		if live {
			return m.Note(), nil
		}
		return next()
	}
}

// reviewWaitingFor builds the Waiting hook for loc's worktree, or nil when no location is wired.
// A running verify is reported ahead of the parent-review note.
func reviewWaitingFor(loc *lyxcwd.Location) func() (string, error) {
	if loc == nil {
		return nil
	}
	review := reviewWaiting(loomengine.LoomParentReviewDir(loc), loomengine.LoomParentReviewLockDir(loc))
	markerPath := verifytree.NewPaths(loc.WorktreePath(), verifytree.Dir(loc.AnchorPath())).Marker
	return verifyWaiting(markerPath, review)
}
