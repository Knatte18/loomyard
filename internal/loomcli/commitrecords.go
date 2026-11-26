// commitrecords.go implements the `commit-records` loom verb.
// The verb commits the run's records at driver stop and starts the pair's pusher.
// The driver writes its stop report after its last `lym shed step`, so no transition commit can capture it,
// and the Fabric Git Invariant forbids the agent committing it itself:
// the agent triggers this verb, and the verb commits through fabricengine.
//
// The cobra shell is thin over commitRecordsVerb, which takes its fabric calls through commitStatusDeps,
// so the disposition table is testable without git.

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

// commitRecordsVerb commits the run's records over d and starts the pair's pusher, which pushes them.
// It reports a JSON envelope and returns the exit code.
// A mid-merge sibling is a success with committed false and a skipped reason, and starts no pusher.
// A probe error and a commit error are each an error envelope with a way forward.
// Otherwise the envelope is ok.
// Its committed field says whether the commit made one, and pusher_started is the value StartPush returned.
// A clean tree is not an error: committing an already-clean pathspec is a no-op, and the pusher still starts.
func commitRecordsVerb(out io.Writer, d commitStatusDeps) int {
	active, err := d.MergeActive()
	if err != nil {
		return output.Err(out, "loom: commit-records: probe merge state: "+err.Error()+"; way forward: transient, re-run lym loom commit-records")
	}
	if active {
		return output.Ok(out, map[string]any{
			"committed": false,
			"skipped":   "the task's run records sibling is mid-merge; conclude the merge and run commit-records again",
		})
	}
	committed, err := d.Commit(commitRecordsMessage)
	if err != nil {
		return output.Err(out, "loom: commit-records: commit failed: "+err.Error()+"; way forward: transient, re-run lym loom commit-records")
	}
	return output.Ok(out, map[string]any{"committed": committed, "pusher_started": d.StartPush()})
}

// writeParkMarker writes the driver park marker at markerPath, holding reportPath, creating its directory.
// The marker is what `lym loom start` reads to resume a parked driver by typing into its pane,
// so writing it from Go, not leaving it to the driving agent, keeps a park from silently stalling the run.
func writeParkMarker(markerPath, reportPath string) error {
	if err := os.MkdirAll(filepath.Dir(markerPath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(markerPath, []byte(reportPath), 0o644)
}

// commitAndPark commits the run's records over d, then writes the park marker at markerPath holding reportPath.
// The marker is written whatever the commit's outcome, because the session parks either way.
// A failed write is the one thing that would stall the run, so it fails the call.
// It returns the exit code and prints the envelope to out.
func commitAndPark(out io.Writer, d commitStatusDeps, markerPath, reportPath string) int {
	code := commitRecordsVerb(out, d)
	if err := writeParkMarker(markerPath, reportPath); err != nil {
		code = output.Err(out, "loom: commit-records: write park marker "+markerPath+": "+err.Error()+"; way forward: transient, re-run lym loom commit-records --park "+reportPath)
	}
	return code
}

// commitRecordsCmd builds the `commit-records` subcommand.
func (c *loomCLI) commitRecordsCmd() *cobra.Command {
	var park string
	cmd := &cobra.Command{
		Use:         "commit-records",
		Short:       "commit the run's records and start the pair's pusher: status, reviews, friction notes and drive reports",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceRole},
		Long: `commit-records commits the run's records and starts the pair's pusher, which pushes them to
origin: the status file, the review round record, the friction notes under _lym/loom/friction/ and
the drive reports under _lym/shed/<slug>/drive-reports/. The loom driver's end-of-session command
runs it after the driver writes its stop report. It is safe to run by hand in a task worktree
before resuming a batten run whose teardown refused uncommitted run records. A tree with nothing
to commit succeeds without adding a commit. The envelope's pusher_started tells whether a pusher
started.

With --park <report>, a parking driver's command: after the commit, whatever its
outcome, it writes the driver park marker holding the stop report's path, which
lym loom start needs to resume the parked session.

Example:
  lym loom commit-records
  lym loom commit-records --park _lym/shed/<slug>/drive-reports/<report>.md`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			deps := loomCommitStatusDeps(c.location, c.runID)
			var code int
			if park != "" {
				code = commitAndPark(cmd.OutOrStdout(), deps, shedrun.ParkMarker(c.location, shedrun.ResolveRunID(c.location, c.runID)), park)
			} else {
				code = commitRecordsVerb(cmd.OutOrStdout(), deps)
			}
			clihelp.SetExit(cmd.Context(), code)
			return nil
		},
	}
	cmd.Flags().StringVar(&park, "park", "", "after committing, write the driver park marker holding this stop report path")
	return cmd
}
