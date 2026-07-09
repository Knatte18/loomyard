// pause.go implements the `pause` perch verb: it writes the pause flag file
// a running block's PauseRequested seam polls between rounds, requesting the
// block honor a pause at the next round boundary rather than mid-round.

package perchcli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/perchengine"
	"github.com/spf13/cobra"
)

// pauseCmd builds the `pause` subcommand: validates that --run-id was
// supplied (manually, mirroring runCmd's --profile validation rather than
// cobra's MarkFlagRequired), then writes the pause flag file at
// perchengine.PauseFlagPath(<run dir>). The running block's round loop
// checks for this file only at the round boundary — never mid-round — and
// exits PAUSED when it finds it; re-running "lyx perch run" against the
// same profile resumes at the recorded round and clears the flag (Engine.Run
// clears it at entry, so a resumed block never instantly re-pauses on a
// flag left over from the run that requested this pause).
//
// pause never creates the run dir: if it does not already exist (the run-id
// names a block that never started, or a typo), that is reported as its own
// error rather than silently fabricating an empty run dir for a pause flag
// with nothing to pause. Writing the flag when it already exists is a no-op
// success (idempotent re-pause).
func (c *perchCLI) pauseCmd() *cobra.Command {
	var runID string

	cmd := &cobra.Command{
		Use:   "pause",
		Short: "request a running perch block pause at its next round boundary",
		Long: `pause writes a flag file that a running "lyx perch run" block checks between
rounds — never mid-round. Once the running block observes the flag it
finishes its current round, persists state, and exits PAUSED. Re-running
"lyx perch run" against the same profile resumes at the recorded round and
clears the flag automatically.

pause requires the run dir to already exist (the block must have started at
least once); it never creates one. Calling pause again while the flag is
already set is a no-op success.

Example:
  lyx perch pause --run-id my-plan-review-a1b2c3d4`,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()

			// Validate flag shape before ever touching c.runDirBase
			// (unpopulated when config resolution aborted), mirroring
			// runCmd's --profile validation.
			if runID == "" {
				clihelp.SetExit(cmd.Context(), output.Err(out, "perch: --run-id is required"))
				return nil
			}
			// --run-id is joined directly into a directory path under the
			// perch runs area; reject anything that is not the same safe
			// shape a derived id has, before it can resolve OUTSIDE that
			// directory (e.g. "../elsewhere") — pause writes a real file, so
			// an unvalidated id is a real path escape, not just a cosmetic
			// one.
			if !perchengine.ValidRunID(runID) {
				clihelp.SetExit(cmd.Context(), output.Err(out, fmt.Sprintf("perch: --run-id %q must be lowercase alphanumerics and dashes only (no path separators)", runID)))
				return nil
			}

			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}

			runDir := filepath.Join(c.runDirBase, runID)
			if _, err := os.Stat(runDir); err != nil {
				if os.IsNotExist(err) {
					clihelp.SetExit(cmd.Context(), output.Err(out, fmt.Sprintf("perch: no such run %q", runID)))
					return nil
				}
				clihelp.SetExit(cmd.Context(), output.Err(out, fmt.Sprintf("perch: stat run dir %q: %v", runDir, err)))
				return nil
			}

			pauseFile := perchengine.PauseFlagPath(runDir)
			if err := os.WriteFile(pauseFile, nil, 0o644); err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, fmt.Sprintf("perch: write pause flag %q: %v", pauseFile, err)))
				return nil
			}

			clihelp.SetExit(cmd.Context(), output.Ok(out, map[string]any{
				"runId":     runID,
				"pauseFile": pauseFile,
			}))
			return nil
		},
	}

	cmd.Flags().StringVar(&runID, "run-id", "", "run identity of the block to pause (required)")

	return cmd
}
