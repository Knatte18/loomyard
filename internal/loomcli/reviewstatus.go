// reviewstatus.go fills shedverbs.Hooks.Waiting for loom, so `lyx loom status` reports a run that waits on its reviewer.

package loomcli

import (
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/parentreview"
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

// reviewWaitingFor builds the Waiting hook for loc's worktree, or nil when no location is wired.
func reviewWaitingFor(loc *lyxcwd.Location) func() (string, error) {
	if loc == nil {
		return nil
	}
	return reviewWaiting(loomengine.LoomParentReviewDir(loc), loomengine.LoomParentReviewLockDir(loc))
}
