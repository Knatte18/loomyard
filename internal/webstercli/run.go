// run.go implements the `run` webster verb: it maps websterengine.Run's outcome onto the run-level
// backstop fabric commit (the fourth and last of webster's four fabric-commit points, see the
// discussion's fabric-ownership decision) and the CLI envelope.
// ErrRunBusy skips the fabric sync entirely:
// the losing call touched nothing, so syncing would commit the winner's in-flight partial state
// under a misleading label;
// every other exit -- success OR error, including ErrFingerprintMismatch and the distinct
// MasterDied/MasterTimeout errors -- runs the backstop fabric commit before its
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

// runDeps builds the websterengine.RunDeps for this CLI's current wiring.
func (c *websterCLI) runDeps() websterengine.RunDeps {
	return websterengine.RunDeps{
		Starter:      c.masterStarter,
		Stopper:      c.runner,
		Engine:       c.engine,
		ShuttleCfg:   c.shuttleCfg,
		Roles:        c.roles,
		Config:       c.cfg,
		Batcher:      c.batcher,
		Geom:         c.geom,
		RefMatcher:   c.refMatcher,
		FrictionDir:  c.frictionDir,
		ParentBranch: c.parentBranch,
	}
}

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
state and reports and starts over on that mismatch, or while audit findings
are pending: it discards them, with a warning each, once their suspect paths
match the run's start commit, and refuses while any differs, while HEAD is
not the run's start commit, or while a pending plan path differs from the
plan the run recorded (restore it with "lyx webster restore-plan"); with an
unchanged plan and nothing pending --fresh is a no-op and the run RESUMES
from state.json, so a fully-completed plan re-reports done without
re-driving anything -- force a from-scratch re-run of an unchanged plan by
editing the plan or archiving _lyx/webster/state.json aside by hand), clears
any leftover pause flag once those refusal gates pass, archives any stale
outcome.yaml/summary.md,
spawns a fresh Master session via shuttle (fork-authorized, never resumed),
and blocks until Master writes its own outcome.yaml and summary.md
(done/stuck) or the shuttle spawn itself ends died/timed-out. Every
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

			// Master types its verbs flagless from the stencil in BOTH modes, so it resolves the
			// DEFAULT plan directory — a run over a --plan-dir override would boot a real session
			// whose every in-pane verb then wire-refuses against a plan it cannot see (found live
			// in crucible round fable5-high-r3, F-A3; the hub half of the same failure went
			// unrefused until crucible round opus-medium-r6, R6-8). Refused here, before any
			// substrate boots; the recourse is the default location. Every other verb keeps
			// honoring the override.
			if c.planDirOverridden {
				clihelp.SetExit(cmd.Context(), output.Err(out, fmt.Sprintf("webster: run cannot spawn Master over a --plan-dir override: Master's own in-pane verbs are flagless and resolve the default plan directory; place the plan at %s and re-run without --plan-dir", c.planDirDefault)))
				return nil
			}

			// Standalone mode boots its own reed session here, idempotently, because nothing else
			// can: `lyx reed up` is hub-only. AddStrand now self-heals a cold worktree on its own,
			// so this call is no longer the only thing standing between a standalone spawn and a
			// dead end (pre-fix the spawn died on "no reed session" with an impossible recourse,
			// found live in crucible round fable5-high-r3, F-A1) — it stays as a deliberate early,
			// explicit boot, chosen so a boot failure surfaces here with its own
			// envelope-reportable error and so the boot happens at a controlled point rather than
			// wherever AddStrand is first called. Nil in hub mode, where the session is the
			// operator's or loom's own to manage.
			if c.reedUp != nil {
				if err := c.reedUp(cmd.Context(), true); err != nil {
					clihelp.SetExit(cmd.Context(), output.Err(out, fmt.Sprintf("webster: bring up the standalone reed session: %v; way forward: transient, re-run `lyx webster run`", err)))
					return nil
				}
			}

			result, runErr := websterengine.Run(c.runDeps(), websterengine.RunOptions{Fresh: fresh})

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
					msg = fmt.Sprintf("%s (additionally, the fabric sync failed: %v; %s)", msg, syncErr, fabricSyncWayForward)
				}
				clihelp.SetExit(cmd.Context(), output.Err(out, msg))
				return nil
			}

			if syncErr != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, fmt.Sprintf("webster: run finished (%s) but the fabric sync failed: %v; %s", result.Outcome, syncErr, fabricSyncWayForward)))
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

	cmd.Flags().BoolVar(&fresh, "fresh", false, "archive the stale state.json and reports dir and start a fresh run on a plan-fingerprint mismatch; also discards pending audit findings once their suspect paths match the run's start commit, and refuses while any differs, while HEAD is not the run's start commit, or while a pending plan path differs from the plan the run recorded")

	return cmd
}
