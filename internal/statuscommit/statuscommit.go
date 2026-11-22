// Package statuscommit is the shared core of a shed module's per-transition status commit.
// It commits the run's status file onto the fabric sibling and pushes it synchronously,
// so the transition's records are on origin before the run's next producer starts.
// A caller that defers a transition's publication passes a Push that does nothing.
// Its own next push then carries the commit.
//
// A caller wraps the closure with the steps that are its own.
// It passes its commit-message prefix and log prefix, so the texts stay its own.
//
// The package imports the standard library and `internal/logger` only and derives no path.
package statuscommit

import (
	"fmt"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
)

const (
	// owedRetryInitial is the wait before the first retry of an owed push.
	owedRetryInitial = time.Second
	// owedRetryCeiling is the longest wait between two retries of an owed push.
	owedRetryCeiling = time.Minute
)

// Deps carries the calls the seam drives.
// They are plain function values, so a test drives the seam's branching from stub closures alone.
type Deps struct {
	// MergeActive reports whether the fabric sibling worktree that carries the status file is mid-merge at the git level.
	MergeActive func() (bool, error)
	// Commit commits the module's own status file with msg and reports whether it made a commit.
	Commit func(msg string) (committed bool, err error)
	// Push pushes the module's run records synchronously, and whatever else its caller's push publishes with them.
	// It is required: New panics on a nil Push.
	Push func() error
	// Admit takes the network gate's admission and returns the function that releases it.
	// A nil Admit takes none.
	Admit func() (release func())
	// Owed reports whether a failed push is owed, so the seam retries it.
	// A nil Owed reads no failure as owed.
	Owed func(err error) bool
	// Sleep waits between the retries of an owed push.
	// A nil Sleep is time.Sleep.
	Sleep func(time.Duration)
}

// Message renders the commit message for a per-transition status commit as "<prefix>: <producer> -> <state>".
// It names the transition, since the seam fires once per transition.
// Identical messages would make the log unreadable to an operator resuming a run.
func Message(prefix, producer, state string) string {
	return fmt.Sprintf("%s: %s -> %s", prefix, producer, state)
}

// New builds the status commit closure from deps.
// commitPrefix heads every commit message (see Message) and logPrefix heads every log line.
// afterCommit, when non-nil, runs after a successful commit and before the push.
// It panics when deps.Push is nil.
//
// The closure applies four dispositions, in order: skip first, then commit, then push:
//
//  1. skip-while-mid-merge: MergeActive reporting true skips both Commit and Push, logged at warn.
//     A non-nil error from MergeActive counts as true:
//     an unreadable probe is the same "git state cannot be trusted right now" case the skip exists for,
//     and probe I/O failures cluster while foreign merge machinery is touching the repo.
//     The probe is not locked against a merge that starts after it,
//     so a Commit failure is probed again before it counts as a hard error.
//  2. commit-hard-errors: a Commit failure returns an error from the seam, which halts the run.
//     A git fault on the run's own bookkeeping is infrastructure breakage.
//     The exception is a failure the re-probe explains as a merge that started after the first probe;
//     it takes the skip disposition instead.
//  3. push-lands: Push runs only after a Commit that made a commit, and the first attempt runs under the admission
//     Admit took before the commit, so a pause or hold written after the commit never stops it.
//     A landed push releases the admission and returns nil, and the next producer starts.
//  4. push-owed-retries: a failed push that Owed reads as owed releases the admission,
//     so the retries wait at the gate like any network call and a pause holds them,
//     then retries from one second, doubling to a minute, until it lands.
//     The wait is unbounded and lives as long as the run.
//     Any other push failure, a hand-over included, releases the admission and returns the error,
//     so the run halts at the transition with the failure recorded.
func New(deps Deps, commitPrefix, logPrefix string, afterCommit func(producer, state string)) func(producer, state string) error {
	if deps.Push == nil {
		panic("statuscommit: New needs a Push")
	}
	admit := deps.Admit
	if admit == nil {
		admit = func() func() { return func() {} }
	}
	owed := deps.Owed
	if owed == nil {
		owed = func(error) bool { return false }
	}
	sleep := deps.Sleep
	if sleep == nil {
		sleep = time.Sleep
	}

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

		release := admit()
		committed, err := deps.Commit(Message(commitPrefix, producer, state))
		if err != nil {
			release()
			return failureDisposition(deps, logPrefix, producer, state, err)
		}

		if afterCommit != nil {
			afterCommit(producer, state)
		}

		if !committed {
			release()
			return nil
		}
		pushErr := deps.Push()
		release()
		if pushErr == nil || !owed(pushErr) {
			return pushErr
		}
		logger.Warn(logPrefix+": status push is owed; retrying until it lands", "producer", producer, "state", state, "error", pushErr)
		return retryOwedPush(deps.Push, owed, sleep)
	}
}

// retryOwedPush runs push again, waiting before each attempt, until it lands or fails with an error that is not owed.
// The wait starts at owedRetryInitial and doubles up to owedRetryCeiling.
func retryOwedPush(push func() error, owed func(error) bool, sleep func(time.Duration)) error {
	wait := owedRetryInitial
	for {
		sleep(wait)
		err := push()
		if err == nil || !owed(err) {
			return err
		}
		wait = min(wait*2, owedRetryCeiling)
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
