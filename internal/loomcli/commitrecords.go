// commitrecords.go implements the `commit-records` loom verb: it commits and pushes the run's records from Go when the driver stops.
// The driver writes its stop report after its last `lyx shed step`, so no transition commit can capture it,
// and the Fabric Git Invariant forbids the agent committing it itself: the agent triggers this verb, and the verb commits through fabricengine.
//
// The cobra shell is thin over commitRecordsVerb, which takes the three fabric calls as commitStatusDeps so the disposition table is testable without git.

package loomcli

import (
	"io"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/spf13/cobra"
)

// commitRecordsMessage is the commit subject the verb lands under.
const commitRecordsMessage = "loom: commit run records"

// commitRecordsVerb commits and pushes the run's records over d and reports a JSON envelope, returning the exit code.
// A mid-merge sibling is skipped exactly as the status seam skips it, as a success with committed false;
// a probe error, a commit error and a push error are each an error envelope.
// A clean tree is not an error: committing an already-clean pathspec is a no-op.
func commitRecordsVerb(out io.Writer, d commitStatusDeps) int {
	active, err := d.MergeActive()
	if err != nil {
		return output.Err(out, "loom: commit-records: probe merge state: "+err.Error())
	}
	if active {
		return output.Ok(out, map[string]any{
			"committed": false,
			"skipped":   "the task's run records sibling is mid-merge; conclude the merge and run commit-records again",
		})
	}
	if err := d.Commit(commitRecordsMessage); err != nil {
		return output.Err(out, "loom: commit-records: commit failed: "+err.Error())
	}
	if err := d.Push(); err != nil {
		return output.Err(out, "loom: commit-records: the commit landed locally but was not pushed: "+err.Error())
	}
	return output.Ok(out, map[string]any{"committed": true})
}

// commitRecordsCmd builds the `commit-records` subcommand.
func (c *loomCLI) commitRecordsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "commit-records",
		Short: "commit and push the run's records: status, reviews, friction notes and drive reports",
		Long: `commit-records commits and pushes the run's records: the status file, the review
round record, the friction notes under _lyx/loom/friction/ and the drive reports under
_lyx/shed/<slug>/drive-reports/. The ly-drive end-of-session command runs it after the
driver writes its stop report. It is safe to run by hand in a task worktree before
resuming a batten run whose teardown refused uncommitted run records. A tree with nothing
to commit succeeds without adding a commit.

Example:
  lyx loom commit-records`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			clihelp.SetExit(cmd.Context(), commitRecordsVerb(cmd.OutOrStdout(), loomCommitStatusDeps(c.location, c.runID)))
			return nil
		},
	}
}
