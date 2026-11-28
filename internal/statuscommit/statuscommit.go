// Package statuscommit is the shared core of a shed module's per-transition status commit.
// It commits the run's status file onto the fabric sibling and spawns the push that publishes it.
//
// A caller wraps the closure with the steps that are its own.
// It passes its commit-message prefix and log prefix, so the texts stay its own.
//
// The package imports the standard library and `internal/logger` only and derives no path.
package statuscommit

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/logger"
)

// Deps carries the three fabric calls the seam drives.
// They are plain function values, so a test drives the seam's branching from stub closures alone.
type Deps struct {
	// MergeActive reports whether the fabric sibling worktree that carries the status file is mid-merge at the git level.
	MergeActive func() (bool, error)
	// Commit commits the module's own status file with msg and reports whether it made a commit.
	Commit func(msg string) (committed bool, err error)
	// Spawn starts the detached push of the module's run records, and whatever else its caller's push publishes with them.
	// It starts the push and does not wait for it.
	Spawn func() error
}

// Message renders the commit message for a per-transition status commit as "<prefix>: <producer> -> <state>".
// It names the transition, since the seam fires once per transition.
// Identical messages would make the log unreadable to an operator resuming a run.
func Message(prefix, producer, state string) string {
	return fmt.Sprintf("%s: %s -> %s", prefix, producer, state)
}

// New builds the status commit closure from deps.
// commitPrefix heads every commit message (see Message) and logPrefix heads every log line.
// afterCommit, when non-nil, runs after a successful commit and before the spawn.
//
// The closure applies three dispositions, in order: skip first, then commit, then spawn:
//
//  1. skip-while-mid-merge: MergeActive reporting true skips both Commit and Spawn, logged at warn.
//     A non-nil error from MergeActive counts as true:
//     an unreadable probe is the same "git state cannot be trusted right now" case the skip exists for,
//     and probe I/O failures cluster while foreign merge machinery is touching the repo.
//     The probe is not locked against a merge that starts after it,
//     so a Commit failure is probed again before it counts as a hard error.
//  2. commit-hard-errors: a Commit failure returns an error from the seam, which halts the run.
//     A git fault on the run's own bookkeeping is infrastructure breakage.
//     The exception is a failure the re-probe explains as a merge that started after the first probe;
//     it takes the skip disposition instead.
//  3. spawn-warns: Spawn runs only after a Commit that made a commit.
//     A Spawn failure logs a warning and returns nil, since a push that cannot start must not kill an autonomous run;
//     the next transition's spawn carries the commits.
//     The seam never sees the push itself: the detached child logs a rejection or an unreachable remote at warn.
//     A rejection means another machine advanced the branch:
//     that is a human decision, not something a background persist may rewrite history over.
func New(deps Deps, commitPrefix, logPrefix string, afterCommit func(producer, state string)) func(producer, state string) error {
	return func(producer, state string) error {
		active, err := deps.MergeActive()
		if err != nil {
			logger.Warn(logPrefix+": skip status commit, merge-state probe failed", "producer", producer, "state", state, "error", err)
			return nil
		}
		if active {
			logger.Warn(logPrefix+": skip status commit, fabric sibling is mid-merge", "producer", producer, "state", state)
			return nil
		}

		committed, err := deps.Commit(Message(commitPrefix, producer, state))
		if err != nil {
			return failureDisposition(deps, logPrefix, producer, state, err)
		}

		if afterCommit != nil {
			afterCommit(producer, state)
		}

		if !committed {
			return nil
		}
		if err := deps.Spawn(); err != nil {
			logger.Warn(logPrefix+": status push spawn failed; the next transition's spawn carries the commits", "producer", producer, "state", state, "error", err)
		}
		return nil
	}
}

// failureDisposition decides what a failed status Commit means.
// The mid-merge probe is not locked.
// Nothing fabric can hold serialises against an operator running plain git in the fabric sibling worktree.
// The Fabric Git Invariant's carve-out permits that.
// So a merge can start between MergeActive answering false and the commit running.
// The commit then fails with git's "cannot do a partial commit during a merge".
// A path-scoped commit is a partial commit by definition.
//
// Without the re-probe, that lost race would take the hard-error disposition and kill the whole run.
// That is the situation the mid-merge skip exists to absorb.
// With the re-probe, a live merge explains the failure:
// the run continues, and the next transition retries once the operator is done.
// A probe that errors on the re-read counts as active, for the same reason the first probe does:
// An unreadable probe is the same untrustworthy-git-state category.
// Probe I/O failures cluster while foreign merge machinery is touching the repo.
//
// Every other commit failure takes the hard-error disposition:
// a git fault on the run's own bookkeeping with no merge to explain it is infrastructure breakage.
//
// One residual remains: gitrepo.StageAndCommit runs `git add` before `git commit`,
// so a commit that loses this race has already staged the status file into the foreign merge's index,
// and the operator's own conclude carries it.
// Closing it would take a lock the operator does not take, so the residual is stated, not defended against.
func failureDisposition(deps Deps, logPrefix, producer, state string, commitErr error) error {
	active, probeErr := deps.MergeActive()
	if probeErr != nil {
		logger.Warn(logPrefix+": status commit failed and the merge-state re-probe failed too; continuing",
			"producer", producer, "state", state, "commit_error", commitErr, "probe_error", probeErr)
		return nil
	}
	if active {
		logger.Warn(logPrefix+": status commit failed because the fabric sibling went mid-merge after the probe; skipping this transition",
			"producer", producer, "state", state, "error", commitErr)
		return nil
	}
	return commitErr
}
