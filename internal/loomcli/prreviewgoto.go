// prreviewgoto.go clears a PR-Review review's recorded autonomy, parent-review ask mark and session.
// It does so when a goto moves the run onto the row.

package loomcli

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/pullrequest"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// prReviewEntryReset returns a goto hook that runs reset when the target is the PR-Review row and returns its error.
// It does nothing for any other target.
// The engine refuses a target past the current row before the move,
// so a move onto PR-Review always finds the run at or past that row.
func prReviewEntryReset(reset func() error) func(target string) error {
	return func(target string) error {
		if target != loomshed.NamePRReview {
			return nil
		}
		return reset()
	}
}

// clearPRReviewOnGoto clears the review fields of the pull request record at recordPath.
// It clears them under the lock at lockPath, then runs commit.
// A record that names a strand has that strand's live session stopped first through stopLiveSession.
// That keeps the re-entry from starting a second session beside it.
// A missing record clears nothing and commits nothing.
// Every error names the goto that repeats the clear, since the move it follows is already written.
func clearPRReviewOnGoto(recordPath, lockPath string, stopLiveSession func(guid string) error, commit func() error) error {
	rec, found, err := pullrequest.ReadRecord(recordPath, lockPath)
	if err != nil {
		return prReviewClearFailure("read the pull request record", err)
	}
	if !found {
		return nil
	}
	if rec.StrandGUID != "" {
		if err := stopLiveSession(rec.StrandGUID); err != nil {
			return prReviewClearFailure("stop the recorded PR-Review session", err)
		}
	}
	if err := pullrequest.ClearReviewSession(recordPath, lockPath); err != nil {
		return prReviewClearFailure("clear the pull request record's review fields", err)
	}
	if err := commit(); err != nil {
		return prReviewClearFailure("commit the pull request record", err)
	}
	return nil
}

// prReviewClearFailure words a failed step of the clear, naming the goto that repeats it.
func prReviewClearFailure(step string, cause error) error {
	return fmt.Errorf("loom: the goto onto %s is written, but it could not %s: %w; way forward: repair that, then run lym loom goto --to %s again to repeat the clear", loomshed.NamePRReview, step, cause, loomshed.NamePRReview)
}

// prReviewGotoAdmitted returns the goto hook that clears the run's PR-Review record once a move onto the row is written.
// The shuttle runner that stops the recorded session is resolved inside the stop, at call time.
// A goto runs on a lightly wired receiver that holds none.
// A receiver with no location returns nil.
func (c *loomCLI) prReviewGotoAdmitted() func(target string) error {
	if c.location == nil {
		return nil
	}
	location := c.location
	return prReviewEntryReset(func() error {
		return clearPRReviewOnGoto(
			shedrun.PullRequestRecord(location, shedrun.SelfRunID),
			shedrun.PullRequestRecordLock(location, shedrun.SelfRunID),
			func(guid string) error {
				runner, err := c.stepStopRunner()
				if err != nil {
					return fmt.Errorf("load the shuttle and reed config to stop the session: %w", err)
				}
				return runner.StopStrand(guid)
			},
			pullRequestRecordCommitter(newPrePushCheck(location, shedrun.SelfRunID), location),
		)
	})
}
