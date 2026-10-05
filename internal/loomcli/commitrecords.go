// commitrecords.go implements the `commit-records` loom verb: it commits and pushes the run's records from Go when the driver stops.
// The driver writes its stop report after its last `lyx shed step`, so no transition commit can capture it,
// and the Fabric Git Invariant forbids the agent committing it itself: the agent triggers this verb, and the verb commits through fabricengine.
//
// The cobra shell is thin over commitRecordsVerb, which takes the three fabric calls as commitStatusDeps so the disposition table is testable without git.

package loomcli

import (
	"io"
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/shedrun"
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
		return output.Err(out, "loom: commit-records: probe merge state: "+err.Error()+"; way forward: transient, re-run lyx loom commit-records")
	}
	if active {
		return output.Ok(out, map[string]any{
			"committed": false,
			"skipped":   "the task's run records sibling is mid-merge; conclude the merge and run commit-records again",
		})
	}
	if err := d.Commit(commitRecordsMessage); err != nil {
		return output.Err(out, "loom: commit-records: commit failed: "+err.Error()+"; way forward: transient, re-run lyx loom commit-records")
	}
	if err := d.Push(); err != nil {
		return output.Err(out, "loom: commit-records: the commit landed locally but was not pushed: "+err.Error()+"; way forward: lyx fabric push pushes the landed commit, or re-run lyx loom commit-records")
	}
	return output.Ok(out, map[string]any{"committed": true})
}

// writeParkMarker writes the driver park marker at markerPath, holding reportPath, creating its directory.
// The marker is what `lyx loom start` reads to resume a parked driver by typing into its pane,
// so writing it from Go, not leaving it to the driving agent, keeps a park from silently stalling the run.
func writeParkMarker(markerPath, reportPath string) error {
	if err := os.MkdirAll(filepath.Dir(markerPath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(markerPath, []byte(reportPath), 0o644)
}

// commitRecordsCmd builds the `commit-records` subcommand.
func (c *loomCLI) commitRecordsCmd() *cobra.Command {
	var park string
	cmd := &cobra.Command{
		Use:   "commit-records",
		Short: "commit and push the run's records: status, reviews, friction notes and drive reports",
		Long: `commit-records commits and pushes the run's records: the status file, the review
round record, the friction notes under _lyx/loom/friction/ and the drive reports under
_lyx/shed/<slug>/drive-reports/. The loom driver's end-of-session command runs it after the
driver writes its stop report. It is safe to run by hand in a task worktree before
resuming a batten run whose teardown refused uncommitted run records. A tree with nothing
to commit succeeds without adding a commit.

With --park <report>, a parking driver's command: after the commit, whatever its
outcome, it writes the driver park marker holding the stop report's path, which
lyx loom start needs to resume the parked session.

Example:
  lyx loom commit-records
  lyx loom commit-records --park _lyx/shed/<slug>/drive-reports/<report>.md`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			code := commitRecordsVerb(cmd.OutOrStdout(), loomCommitStatusDeps(c.location, c.runID))
			if park != "" {
				// The session parks whatever the commit's outcome, so the marker is written on every path;
				// a failed write is the one thing that would stall the run, so it fails the verb.
				marker := shedrun.ParkMarker(c.location, shedrun.ResolveRunID(c.location, c.runID))
				if err := writeParkMarker(marker, park); err != nil {
					code = output.Err(cmd.OutOrStdout(), "loom: commit-records: write park marker "+marker+": "+err.Error()+"; way forward: transient, re-run lyx loom commit-records --park "+park)
				}
			}
			clihelp.SetExit(cmd.Context(), code)
			return nil
		},
	}
	cmd.Flags().StringVar(&park, "park", "", "after committing, write the driver park marker holding this stop report path")
	return cmd
}
