// edits.go reads what an arbiter changed in the task's code worktree:
// the commits it made since its spawn, and whether it left a worktree the run may go on from.

package shedarbiter

import (
	"fmt"
	"maps"
	"strings"

	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/sheddriver"
)

// WorktreeSeam reads the task's code worktree an arbiter may edit and commit in.
type WorktreeSeam struct {
	// Snapshot reads the worktree's HEAD, branch and the content hash of each tracked uncommitted path;
	// it never lists an untracked file.
	Snapshot func() (sheddriver.WorktreeSnapshot, error)
	// Changes maps every uncommitted path of the worktree to the SHA-256 of its content.
	// Untracked files are included, and a file that is gone maps to "deleted".
	Changes func() (map[string]string, error)
	// IsAncestor reports whether the commit sha is an ancestor of the commit ref.
	IsAncestor func(sha, ref string) (bool, error)
	// CommitLog lists the commits reachable from to and not from from, newest first, each a short SHA and its subject.
	CommitLog func(from, to string) ([]string, error)
}

// attemptEdits is what an attempt's arbiter changed in the worktree.
// problem names why the run may not go on from the worktree the arbiter left, and is empty when it may.
type attemptEdits struct {
	commits []string
	problem string
}

// readEdits compares the worktree with the snapshot attempt recorded at spawn.
// A call with no worktree seam, or an attempt with no snapshot, reads no edits.
// The arbiter must leave every change committed on the branch it found, on top of the HEAD it found.
// Each of these is a problem: a switched branch, an uncommitted change,
// and a HEAD that no longer descends from the spawn HEAD.
// An uncommitted change is any uncommitted path, tracked or untracked.
// It counts when it was absent at spawn or its content differs from the spawn's.
// So is a worktree the seam cannot read.
// When HEAD moved but its commits cannot be read, the problem names the move.
// The parent can then inspect the range.
func (c *Call) readEdits(attempt loomengine.ArbiterAttempt) attemptEdits {
	seam, spawned := c.deps.Worktree, attempt.Snapshot
	if seam == nil || spawned == nil {
		return attemptEdits{}
	}
	current, err := seam.Snapshot()
	if err != nil {
		return attemptEdits{problem: fmt.Sprintf("the worktree could not be read after the arbiter: %v", err)}
	}
	if current.Branch != spawned.Branch {
		return attemptEdits{problem: fmt.Sprintf("the arbiter left branch %q for %q", spawned.Branch, current.Branch)}
	}
	var edits attemptEdits
	if current.Head != spawned.Head {
		descends, err := seam.IsAncestor(spawned.Head, current.Head)
		if err != nil {
			return attemptEdits{problem: fmt.Sprintf("HEAD moved from %s to %s and the arbiter's commits could not be read: %v", spawned.Head, current.Head, err)}
		}
		if !descends {
			return attemptEdits{problem: fmt.Sprintf("the arbiter rewrote the branch's history: HEAD %s does not descend from %s, the HEAD it found", current.Head, spawned.Head)}
		}
		commits, err := seam.CommitLog(spawned.Head, current.Head)
		if err != nil {
			return attemptEdits{problem: fmt.Sprintf("HEAD moved from %s to %s and the arbiter's commits could not be read: %v", spawned.Head, current.Head, err)}
		}
		edits.commits = commits
	}
	changes, err := seam.Changes()
	if err != nil {
		edits.problem = fmt.Sprintf("the worktree's uncommitted paths could not be read after the arbiter: %v", err)
		return edits
	}
	if !maps.Equal(current.Dirty, spawned.Dirty) || !maps.Equal(changes, attempt.Changes) {
		edits.problem = "the arbiter left uncommitted changes in the worktree"
	}
	return edits
}

// appendTo returns question followed by what the parent must look at in the worktree:
// the arbiter's commits to keep or revert, and the problem it left.
// It returns question unchanged when there is neither.
func (e attemptEdits) appendTo(question string) string {
	var notes []string
	if len(e.commits) > 0 {
		notes = append(notes, "The arbiter committed on the task branch before the stop was handed on: "+strings.Join(e.commits, "; ")+"; keep or revert those commits.")
	}
	if e.problem != "" {
		notes = append(notes, "Check the worktree first: "+e.problem+".")
	}
	if len(notes) == 0 {
		return question
	}
	return question + " " + strings.Join(notes, " ")
}
