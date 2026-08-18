// run.go implements the `run` webster verb: it maps websterengine.Run's outcome onto the run-level
// backstop fabric commit (the fourth and last of webster's four fabric-commit points, see the
// discussion's fabric-ownership decision) and the CLI envelope.
// ErrRunBusy skips the fabric sync entirely:
// the losing call touched nothing, so syncing would commit the winner's in-flight partial state
// under a misleading label;
// every other exit -- success OR error, including ErrFingerprintMismatch and the distinct
// MasterAsking/MasterDied/MasterTimeout errors -- runs the backstop fabric commit before its
// envelope, since completed batches' artifacts must not strand uncommitted.
package webstercli

import (
	"errors"
	"fmt"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/websterengine"
	"github.com/spf13/cobra"
)

// runCmd builds the `run` subcommand.
func (c *websterCLI) runCmd() *cobra.Command {
	var fresh bool

	cmd := &cobra.Command{
		Use:   "run",
		Short: "spawn or resume Master and block until the plan reaches a terminal outcome",
		Long: `run takes the webster run-level lock, runs the automatic plan-validation
gate (including the zero-batch pre-flight refusal), checks the on-disk
plan's fingerprint against state.json's recorded one (refusing with a
message naming "run --fresh" on a mismatch -- --fresh archives the stale
state and reports and starts over ONLY on that mismatch; with an
unchanged plan --fresh is a no-op and the run RESUMES from state.json, so
a fully-completed plan re-reports done without re-driving anything --
force a from-scratch re-run of an unchanged plan by editing the plan or
archiving _lyx/webster/state.json aside by hand), clears any leftover pause flag once
those refusal gates pass, archives any stale outcome.yaml/summary.md,
spawns a fresh Master session via shuttle (fork-authorized, never resumed),
and blocks until Master writes its own outcome.yaml and summary.md
(done/stuck) or the shuttle spawn itself ends asking/died/timed-out. Every
exit except a "run is already in progress" refusal (another
"lyx webster run" already owns the run -- this call touched nothing) runs
a backstop fabric commit before printing its envelope, so a run that ends in
error still leaves its completed batches' artifacts committed.

Example:
  lyx webster run
  lyx webster run --fresh`,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()

			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}

			deps := websterengine.RunDeps{
				Starter:    c.masterStarter,
				Reed:       c.reed,
				Engine:     c.engine,
				ShuttleCfg: c.shuttleCfg,
				Roles:      c.roles,
				Config:     c.cfg,
				Batcher:    c.batcher,
				Geom:       c.geom,
				RefMatcher: c.refMatcher,
				OpenBisector: func() (websterengine.FabricBisector, error) {
					return c.openFabric()
				},
			}

			result, runErr := websterengine.Run(deps, websterengine.RunOptions{Fresh: fresh})

			if errors.Is(runErr, websterengine.ErrRunBusy) {
				clihelp.SetExit(cmd.Context(), output.Err(out, runErr.Error()))
				return nil
			}

			outcomeLabel := "ERROR"
			if runErr == nil {
				outcomeLabel = result.Outcome
			}
			committed, syncErr := fabricSync(c.openFabric, c.anchorRel, fmt.Sprintf("run %s", outcomeLabel))

			if runErr != nil {
				msg := runErr.Error()
				if syncErr != nil {
					msg = fmt.Sprintf("%s (additionally, the fabric sync failed: %v)", msg, syncErr)
				}
				clihelp.SetExit(cmd.Context(), output.Err(out, msg))
				return nil
			}

			if syncErr != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, fmt.Sprintf("webster: run finished (%s) but the fabric sync failed: %v", result.Outcome, syncErr)))
				return nil
			}

			clihelp.SetExit(cmd.Context(), output.Ok(out, map[string]any{
				"outcome":         result.Outcome,
				"stuck_reason":    result.StuckReason,
				"batches_done":    result.BatchesDone,
				"summary_title":   result.SummaryTitle,
				"fabricCommitted": committed,
				"warnings":        result.Warnings,
			}))
			return nil
		},
	}

	cmd.Flags().BoolVar(&fresh, "fresh", false, "archive the stale state.json and reports dir and start a fresh run on a plan-fingerprint mismatch")

	return cmd
}
