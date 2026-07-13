// run.go implements the `run` builder verb: it maps builderengine.Run's
// outcome onto the run-level backstop weft commit and the CLI envelope.
// ErrRunBusy skips the weft sync entirely (perchcli's ErrBlockBusy
// exemption applies verbatim: the losing call touched nothing, so syncing
// would commit the winner's in-flight partial state under a misleading
// label); every other exit -- success OR error, including
// ErrFingerprintMismatch and the distinct asking/died/timeout orchestrator
// errors -- runs the backstop weft commit before its envelope, since
// completed batches' artifacts must not strand uncommitted.

package buildercli

import (
	"errors"
	"fmt"

	"github.com/Knatte18/loomyard/internal/builderengine"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/spf13/cobra"
)

// runCmd builds the `run` subcommand.
func (c *builderCLI) runCmd() *cobra.Command {
	var fresh bool

	cmd := &cobra.Command{
		Use:   "run",
		Short: "spawn or resume the orchestrator and block until the plan reaches a terminal outcome",
		Long: `run takes the builder run-level lock, runs the automatic plan-validation
gate, checks the on-disk plan's fingerprint against state.json's recorded
one (refusing with a message naming "run --fresh" on a mismatch -- --fresh
archives the stale state and reports and starts over), clears any leftover
pause flag once those refusal gates pass (a run that refuses leaves a
pending pause intact), archives any stale outcome.yaml, spawns a fresh
orchestrator session via shuttle (never "claude --resume" -- the
orchestrator is always spawned fresh, hydrated from on-disk state), and
blocks until the orchestrator writes its own outcome.yaml (done/stuck) or
the shuttle spawn itself ends asking/died/timed-out. Every exit except a
"run is already in progress" refusal (another "lyx builder run" already
owns the run -- this call touched nothing) runs a backstop weft commit
before printing its envelope, so a run that ends in error still leaves its
completed batches' artifacts committed.

Example:
  lyx builder run
  lyx builder run --fresh`,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()

			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}

			deps := builderengine.RunDeps{
				Runner:       c.orchestratorStarter,
				Mux:          c.mux,
				PlanDir:      c.planDir,
				BuilderDir:   c.builderDir,
				ReportsDir:   c.reportsDir,
				WorktreeRoot: c.layout.Cwd,
				Config:       c.cfg,
				Roles:        c.roles,
			}

			result, runErr := builderengine.Run(deps, builderengine.RunOptions{Fresh: fresh})

			// ErrRunBusy: another invocation owns the run right now; this
			// call touched NOTHING on disk. Running the weft sync below
			// would commit (and push) the winner's in-flight partial state
			// under a misleading "run ERROR" label -- perchcli's
			// ErrBlockBusy exemption applies verbatim.
			if errors.Is(runErr, builderengine.ErrRunBusy) {
				clihelp.SetExit(cmd.Context(), output.Err(out, runErr.Error()))
				return nil
			}

			outcomeLabel := "ERROR"
			if runErr == nil {
				outcomeLabel = result.Outcome
			}
			committed, weftErr := weftCommit(c.layout, fmt.Sprintf("run %s", outcomeLabel))

			if runErr != nil {
				msg := runErr.Error()
				if weftErr != nil {
					msg = fmt.Sprintf("%s (additionally, the weft sync failed: %v)", msg, weftErr)
				}
				clihelp.SetExit(cmd.Context(), output.Err(out, msg))
				return nil
			}

			if weftErr != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, fmt.Sprintf("builder: run finished (%s) but the weft sync failed: %v", result.Outcome, weftErr)))
				return nil
			}

			clihelp.SetExit(cmd.Context(), output.Ok(out, map[string]any{
				"outcome":       result.Outcome,
				"stuck_reason":  result.StuckReason,
				"batches_done":  result.BatchesDone,
				"run_dir":       result.RunDir,
				"session_id":    result.SessionID,
				"weftCommitted": committed,
			}))
			return nil
		},
	}

	cmd.Flags().BoolVar(&fresh, "fresh", false, "archive the stale state.json and reports dir and start a fresh run on a plan-fingerprint mismatch")

	return cmd
}
