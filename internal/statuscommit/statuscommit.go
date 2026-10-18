// Package statuscommit is the shared core of a shed module's per-transition status commit seam: the `shedengine.Shed.CommitStatus` closure that commits a run's status file onto the fabric sibling and spawns its push.
//
// `loomcli` and `battencli` each wrap it with what is theirs alone: loom's board-status write ahead of the core, batten's on-disk no-op-transition marker ahead of it and its marker write after a successful commit.
// The per-module texts stay theirs by parameter: each caller tells the core its commit-message prefix and its log prefix.
//
// The package imports the standard library and `internal/logger` only and derives no path.
package statuscommit

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/logger"
)

// Deps carries the three fabric calls the seam drives, injected as plain function values so its branching is drivable from stub closures, with no hub fixture and no git spawn.
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
// It is a function rather than a bare constant because the seam fires once per transition, and an unreadable stream of identical messages is the log a resuming operator has to read.
func Message(prefix, producer, state string) string {
	return fmt.Sprintf("%s: %s -> %s", prefix, producer, state)
}

// New builds the status commit closure from deps.
// commitPrefix heads every commit message (see Message) and logPrefix heads every log line, so each caller's texts read as they did before the core was shared.
// afterCommit, when non-nil, runs after a successful commit and before the spawn.
//
// It implements three dispositions, evaluated in the order they appear in the closure body -- skip first, then commit, then spawn:
//
//  1. skip-while-mid-merge: MergeActive reporting true skips both Commit and Spawn, logged at warn.
//     A non-nil error from MergeActive is treated exactly like true -- an unreadable probe is the same "git state cannot be trusted right now" category the skip exists for, and probe I/O failures cluster precisely when foreign merge machinery is touching the repo.
//     The probe is unlocked, so this disposition also has a second half at the other end of the window: see failureDisposition.
//  2. commit-hard-errors: a Commit failure returns an error from the seam and therefore halts the run -- a git fault on the run's own bookkeeping is infrastructure breakage.
//     The one exception is a failure the re-probe explains as a merge that went live after the first probe, which takes the skip disposition instead.
//  3. spawn-warns: Spawn runs only after a Commit that made a commit, and a Spawn failure logs a warning and returns nil -- a push that cannot start must not kill an autonomous run, and the next transition's spawn carries the commits.
//     The seam never sees the push itself: the detached child logs a rejection or an unreachable remote at warn, and a rejection means another machine advanced the branch, which is a human decision rather than something a background persist may rewrite history over.
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

// failureDisposition decides what a failed status Commit means, and it exists because the mid-merge probe is unlocked by construction:
// nothing fabric can hold serialises against an operator running plain git inside the fabric sibling worktree, which the Fabric Git Invariant's own carve-out permits.
// So a merge can become live in the window between MergeActive answering false and the commit running, and when it does the commit fails on git's own "cannot do a partial commit during a merge" -- a path-scoped commit is a partial commit by definition.
//
// Without this re-probe that lost race took the commit-hard-errors disposition and killed the whole run, in precisely the situation skip-while-mid-merge exists to absorb gracefully.
// Re-probing turns it back into the skip it was always meant to be: if a merge is live NOW, the commit failure is explained and the run continues, and the next transition retries once the operator is done.
// A probe that errors on the re-read is treated as active for the same reason the first probe is:
// an unreadable probe is the same untrustworthy-git-state category, and probe I/O failures cluster exactly when foreign merge machinery is touching the repo.
//
// Every other commit failure keeps the hard-error disposition unchanged: a git fault on the run's own bookkeeping with no merge to explain it is real infrastructure breakage.
//
// One residual is NOT closed here and must not be read as closed: gitrepo.StageAndCommit runs `git add` before `git commit`, so a commit that loses this race has already staged the status file into the foreign merge's index, and the operator's own conclude will carry it.
// Closing that would take a lock the operator does not take, so it is stated rather than defended against.
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
