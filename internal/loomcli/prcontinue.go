// prcontinue.go implements the `pr-continue` loom verb, the ending of a pull request's review on the operator's word.
// The verb sets Continue in the run's pull-request record and clears the head whose gate failure was mailed.
// It then commits the record, so the flag reaches the pair's records.
// The PR-Review row reads that flag, records the ending and ends the row, leaving the pull request open.
// A cleared mailed head makes the row gate that head again, so a failure from outside the change is retried.
// Finalize then merges the pull request on GitHub under its own gate and credit block.
// The operator runs it from a shell, and the PR-Review session or an orch runs it on the operator's word.
// It refuses every other lym agent session.
// The check reads the strand name reed exports to a session's pane: a guardrail against a mistaken call, not a barrier.

package loomcli

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/Knatte18/loomyard/internal/agentconfig"
	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/pullrequest"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/spf13/cobra"
)

// prContinueWayForward is the way forward of a failed read or write the verb can only retry.
const prContinueWayForward = "; way forward: run lym loom pr-continue again"

// prContinueNoRecord refuses a run that has opened no pull request yet.
const prContinueNoRecord = "loom: pr-continue: this run has no pull request record, so there is no review to end; way forward: wait until the PR-Review row has opened the pull request, then run lym loom pr-continue again"

// prContinueAgentRefusal refuses a call from the lym agent session it names.
const prContinueAgentRefusal = "loom: pr-continue: this call runs in the lym agent session %s, and only the operator, the run's PR-Review session or an orch ends a pull request's review; way forward: tell the operator the run's pull request waits for their review, which they end by approving or merging it on GitHub, or by telling the PR-Review session in the task's window to continue"

// prContinueAdmitsStrand reports whether the session strand names may end a review on the operator's word.
// The operator's shell, which reed did not spawn and which carries no strand name, is admitted.
// So is an orch, by the role its name carries.
// A PR-Review session is admitted with runSessionOnly set: the caller admits it only as the run's own recorded session.
// Any other strand is refused.
func prContinueAdmitsStrand(strand string) (admitted, runSessionOnly bool) {
	if strand == "" {
		return true, false
	}
	name, err := agentname.Parse(strand)
	if err != nil {
		return false, false
	}
	if agentname.MatchesRole(name.Role, string(agentconfig.RolePRReviewer)) {
		return true, true
	}
	return agentname.MatchesRole(name.Role, string(agentconfig.RoleOrch)), false
}

// prContinueVerb sets Continue in the pull-request record at recordPath, under the lock at lockPath.
// It also clears the record's mailed head, so the PR-Review row gates again a head whose gate failure it mailed.
// strand is the caller's agent name as reed exported it, empty for a shell reed did not spawn.
// It reports a JSON envelope naming the pull request and returns the exit code.
// A caller whose strand the verb does not admit is refused before anything is read.
// A PR-Review session that is not the one the record names is refused once the record is read.
// A record carrying ReviewEnded, or Continue with no mailed head, succeeds the same way and is not written again.
// A run with no record is refused, and an unreadable or unwritable record is an error envelope carrying the error.
func prContinueVerb(out io.Writer, strand, recordPath, lockPath string) int {
	admitted, runSessionOnly := prContinueAdmitsStrand(strand)
	if !admitted {
		return output.Err(out, fmt.Sprintf(prContinueAgentRefusal, strand))
	}
	rec, found, err := pullrequest.ReadRecord(recordPath, lockPath)
	if err != nil {
		return output.Err(out, "loom: pr-continue: read the pull request record: "+err.Error()+prContinueWayForward)
	}
	if !found {
		return output.Err(out, prContinueNoRecord)
	}
	if runSessionOnly && strand != rec.StrandName {
		return output.Err(out, fmt.Sprintf(prContinueAgentRefusal, strand))
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
			return output.Err(out, "loom: pr-continue: write the pull request record: "+err.Error()+prContinueWayForward)
		}
	}
	return output.Ok(out, map[string]any{
		"continue": true,
		"url":      rec.URL,
		"note":     "the run ends the review and Finalize merges pull request " + rec.URL + " on GitHub",
	})
}

// prContinueCmd builds the `pr-continue` subcommand.
func (c *loomCLI) prContinueCmd() *cobra.Command {
	return &cobra.Command{
		Use:         "pr-continue",
		Short:       "end the pull request's review on the operator's word, so the run goes on to merge it",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceRole},
		Long: `pr-continue sets the run's continue flag in its pull request record. It
ends the review of the run's pull request on the operator's word: the operator
runs it in the task worktree, or the run's PR-Review session or an orch runs it
when the operator says so. The PR-Review row then ends the review and leaves
the pull request open, and the run goes on through Finalize, which merges it on
GitHub under its own gate. An approval or a merge on GitHub ends the review
too. It takes no arguments, and is refused until the PR-Review row has opened
the pull request.

Run after a gate failure of the PR-Review row, it makes the row gate the same
head again, for a failure that lies outside the change. It also ends the review
once a head passes the gate, so it needs the operator's word then too.

It is refused from any other lym agent session.

Example:
  lym loom pr-continue`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			var verbOut bytes.Buffer
			code := prContinueVerb(&verbOut, os.Getenv(agentname.StrandNameEnv), shedrun.PullRequestRecord(c.location, shedrun.SelfRunID), shedrun.PullRequestRecordLock(c.location, shedrun.SelfRunID))
			if code == 0 {
				if err := pullRequestRecordCommitter(newPrePushCheck(c.location, shedrun.SelfRunID), c.location)(); err != nil {
					code = output.Err(cmd.OutOrStdout(), "loom: pr-continue: commit the pull request record: "+err.Error()+prContinueWayForward)
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
