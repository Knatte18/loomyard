// prcontinue.go implements the `pr-continue` loom verb.
// The PR-Review agent runs it to tell the run the operator is satisfied with the review.
// The verb sets Continue in the run's pull-request record and clears the head whose gate failure was mailed.
// Under a review's recorded autonomy it needs the parent's satisfied answer.
// That answer must stand at the current head.
// It then commits the record, so the flag reaches the pair's records.
// The PR-Review row reads that flag, records the ending and ends the row, leaving the pull request open.
// A cleared mailed head makes the row gate that head again, so a failure from outside the change is retried.
// Finalize then merges the pull request on GitHub under its own gate and credit block.

package loomcli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/Knatte18/loomyard/internal/burlerengine"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/landingshed"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/pullrequest"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/spf13/cobra"
)

// prContinueReadWayForward is the way forward of a failed read the verb can only retry.
const prContinueReadWayForward = "; way forward: run lym loom pr-continue again, and tell the operator when it keeps failing"

// prContinueReview holds the reads the verb makes of a review recorded under autonomy.
// The head is the task worktree's current head, and the standing is the parent's PR-Review answer at that head.
// The closed read reports whether the record's pull request is closed on GitHub without a merge.
type prContinueReview struct {
	head     func() (string, error)
	standing func(head string) (landingshed.PRReviewStanding, error)
	closed   func(rec pullrequest.Record) (bool, error)
}

// prContinueNoRecord refuses a run that has opened no pull request yet.
const prContinueNoRecord = "loom: pr-continue: this run has no pull request record, so there is no review to end; way forward: wait until the PR-Review row has opened the pull request, then run lym loom pr-continue again"

// prContinueVerb sets Continue in the pull-request record at recordPath, under the lock at lockPath.
// It also clears the record's mailed head, so the PR-Review row gates again a head whose gate failure it mailed.
// It reports a JSON envelope naming the pull request and returns the exit code.
// A record carrying ReviewEnded, or Continue with no mailed head, succeeds the same way and is not written again.
// A record with autonomy recorded true that has not ended sets Continue only on the parent's satisfied answer.
// That answer must stand at the review's head.
// A closed pull request is refused there.
// Without a satisfied answer, a mailed head is cleared so the row gates it again.
// The verb then answers ok with continue false.
// Any other such record is refused.
// A run with no record is refused, and an unreadable or unwritable record is an error envelope carrying the error.
func prContinueVerb(out io.Writer, recordPath, lockPath string, review prContinueReview) int {
	rec, found, err := pullrequest.ReadRecord(recordPath, lockPath)
	if err != nil {
		return output.Err(out, "loom: pr-continue: read the pull request record: "+err.Error()+prContinueReadWayForward)
	}
	if !found {
		return output.Err(out, prContinueNoRecord)
	}
	if rec.Autonomy != nil && *rec.Autonomy && !rec.ReviewEnded {
		return prContinueUnderAutonomy(out, recordPath, lockPath, rec, review)
	}
	if !rec.ReviewEnded && (!rec.Continue || rec.MailedHead != "") {
		err := pullrequest.UpdateRecord(recordPath, lockPath, func(rec *pullrequest.Record) error {
			rec.Continue = true
			rec.MailedHead = ""
			return nil
		})
		if errors.Is(err, pullrequest.ErrNoRecord) {
			return output.Err(out, prContinueNoRecord)
		}
		if err != nil {
			return output.Err(out, "loom: pr-continue: write the pull request record: "+err.Error()+"; way forward: run lym loom pr-continue again, and tell the operator when it keeps failing")
		}
	}
	return output.Ok(out, map[string]any{
		"continue": true,
		"url":      rec.URL,
		"note":     "the run ends the review and Finalize merges pull request " + rec.URL + " on GitHub",
	})
}

// prContinueUnderAutonomy runs the verb for a review recorded under autonomy that has not ended.
// Only the parent's satisfied answer at the current head sets Continue.
func prContinueUnderAutonomy(out io.Writer, recordPath, lockPath string, rec pullrequest.Record, review prContinueReview) int {
	head, err := review.head()
	if err != nil {
		return output.Err(out, "loom: pr-continue: read the task worktree's head: "+err.Error()+prContinueReadWayForward)
	}
	closed, err := review.closed(rec)
	if err != nil {
		return output.Err(out, "loom: pr-continue: read the pull request's state: "+err.Error()+prContinueReadWayForward)
	}
	if closed {
		return output.Err(out, fmt.Sprintf("loom: pr-continue: pull request %s is closed without a merge, so the review cannot end; way forward: leave the pull request alone and end the turn, since the PR-Review row halts on its next wake and your parent settles that halt", rec.URL))
	}
	standing, err := review.standing(head)
	if err != nil {
		return output.Err(out, "loom: pr-continue: read the parent's PR-Review answer: "+err.Error()+prContinueReadWayForward)
	}
	satisfied := standing == landingshed.PRReviewSatisfied
	if !satisfied && rec.MailedHead == "" {
		return output.Err(out, fmt.Sprintf("loom: pr-continue: no satisfied answer from your parent stands at %s, so the review cannot end; way forward: %s", head, prContinueParentStep(standing)))
	}
	err = pullrequest.UpdateRecord(recordPath, lockPath, func(rec *pullrequest.Record) error {
		rec.Continue = satisfied
		rec.MailedHead = ""
		return nil
	})
	if errors.Is(err, pullrequest.ErrNoRecord) {
		return output.Err(out, prContinueNoRecord)
	}
	if err != nil {
		return output.Err(out, "loom: pr-continue: write the pull request record: "+err.Error()+prContinueReadWayForward)
	}
	if satisfied {
		return output.Ok(out, map[string]any{
			"continue": true,
			"url":      rec.URL,
			"note":     "the run ends the review and Finalize merges pull request " + rec.URL + " on GitHub",
		})
	}
	return output.Ok(out, map[string]any{
		"continue": false,
		"url":      rec.URL,
		"note":     fmt.Sprintf("the review stays open at %s: no satisfied answer from your parent stands at it, and the row gates that head again; %s", head, prContinueParentStep(standing)),
	})
}

// prContinueParentStep names the verb that gets the parent's answer at the current head, given the standing there.
func prContinueParentStep(standing landingshed.PRReviewStanding) string {
	if standing == landingshed.PRReviewAwaiting {
		return "wait for your parent's answer with lym loom parent-review await"
	}
	return "ask your parent again with lym loom parent-review ask"
}

// prContinueReviewOf builds the reads of a review recorded under autonomy over the worktree of c.
func (c *loomCLI) prContinueReviewOf(ctx context.Context) prContinueReview {
	return prContinueReview{
		head: func() (string, error) {
			f, err := fabricengine.Open(c.location)
			if err != nil {
				return "", err
			}
			return f.HeadSHA()
		},
		standing: func(head string) (landingshed.PRReviewStanding, error) {
			return landingshed.ReadPRReviewStanding(burlerengine.ParentReviewFilesOf(c.location.AnchorPath()), head, time.Now())
		},
		closed: func(rec pullrequest.Record) (bool, error) {
			deps := pullRequestDeps(c.location, rec.Base, false, nil)
			origin, err := deps.Origin()
			if err != nil {
				return false, err
			}
			api, err := deps.API(origin.URL)
			if err != nil {
				return false, err
			}
			status, err := api.Status(ctx, rec.Number)
			if err != nil {
				return false, err
			}
			return status.State == pullrequest.StateClosed, nil
		},
	}
}

// prContinueCmd builds the `pr-continue` subcommand.
func (c *loomCLI) prContinueCmd() *cobra.Command {
	return &cobra.Command{
		Use:         "pr-continue",
		Short:       "tell the run the operator is done with the pull request review, so it goes on",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceRole},
		Long: `pr-continue sets the run's continue flag in its pull request record. The
PR-Review agent runs it in its task worktree when the operator asks it to
continue. The PR-Review row then ends the review and leaves the pull request
open, and the run goes on through Finalize, which merges it on GitHub under its
own gate. Run again after the row mailed a gate failure that lies outside the
change, it makes the row gate the same head again. It takes no arguments, and
is refused until the PR-Review row has opened the pull request.

Under a review whose record holds autonomy true, it sets the flag only on the
parent's recorded satisfied answer at the current head. Without one it is
refused, or, after a mailed gate failure, it clears the mailed head and answers
that the review stays open. It is refused on a pull request closed without a
merge. A review whose record holds autonomy false or none ends with no parent
answer.

Example:
  lym loom pr-continue`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			var verbOut bytes.Buffer
			code := prContinueVerb(&verbOut, shedrun.PullRequestRecord(c.location, shedrun.SelfRunID), shedrun.PullRequestRecordLock(c.location, shedrun.SelfRunID), c.prContinueReviewOf(cmd.Context()))
			if code == 0 {
				if err := pullRequestRecordCommitter(newPrePushCheck(c.location, shedrun.SelfRunID), c.location)(); err != nil {
					code = output.Err(cmd.OutOrStdout(), "loom: pr-continue: commit the pull request record: "+err.Error()+"; way forward: run lym loom pr-continue again, and tell the operator when it keeps failing")
					clihelp.SetExit(cmd.Context(), code)
					return nil
				}
			}
			if _, err := cmd.OutOrStdout().Write(verbOut.Bytes()); err != nil {
				return err
			}
			clihelp.SetExit(cmd.Context(), code)
			return nil
		},
	}
}
