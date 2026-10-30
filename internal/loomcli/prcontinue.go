// prcontinue.go implements the `pr-continue` loom verb: the PR-Review agent's flag that the operator is satisfied with the review and the run goes on.
// The verb only sets Continue in the run's pull-request record.
// The PR-Review row reads the flag, closes the pull request and ends the row, and Finalize then lands the run locally under its own gate and trailer.

package loomcli

import (
	"errors"
	"io"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/pullrequest"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/spf13/cobra"
)

// prContinueNoRecord refuses a run that has opened no pull request yet.
const prContinueNoRecord = "loom: pr-continue: this run has no pull request record, so there is no review to end; way forward: wait until the PR-Review row has opened the pull request, then run lym loom pr-continue again"

// prContinueVerb sets Continue in the pull-request record at recordPath, guarded by the lock at lockPath, and reports a JSON envelope naming the pull request, returning the exit code.
// A record already carrying Continue or ClosedForContinue succeeds the same way and is not written again.
// A run with no record is refused, and an unreadable or unwritable record is an error envelope carrying the error.
func prContinueVerb(out io.Writer, recordPath, lockPath string) int {
	rec, found, err := pullrequest.ReadRecord(recordPath, lockPath)
	if err != nil {
		return output.Err(out, "loom: pr-continue: read the pull request record: "+err.Error()+"; way forward: run lym loom pr-continue again, and tell the operator when it keeps failing")
	}
	if !found {
		return output.Err(out, prContinueNoRecord)
	}
	if !rec.Continue && !rec.ClosedForContinue {
		err := pullrequest.UpdateRecord(recordPath, lockPath, func(rec *pullrequest.Record) error {
			rec.Continue = true
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
		"note":     "the run will close pull request " + rec.URL + " and land through Finalize",
	})
}

// prContinueCmd builds the `pr-continue` subcommand.
func (c *loomCLI) prContinueCmd() *cobra.Command {
	return &cobra.Command{
		Use:         "pr-continue",
		Short:       "tell the run the operator is done with the pull request review, so it goes on",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceRole},
		Long: `pr-continue sets the run's continue flag in its pull request record. The
PR-Review agent runs it in its task worktree when the operator asks it to
continue. The PR-Review row then closes the pull request without merging it, and
the run goes on through Finalize, which lands it locally under its own gate. It
takes no arguments, and is refused until the PR-Review row has opened the pull
request.

Example:
  lym loom pr-continue`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			code := prContinueVerb(cmd.OutOrStdout(), shedrun.PullRequestRecord(c.location, shedrun.SelfRunID), shedrun.PullRequestRecordLock(c.location, shedrun.SelfRunID))
			clihelp.SetExit(cmd.Context(), code)
			return nil
		},
	}
}
