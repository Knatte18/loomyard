// fixplan.go implements the `fix-plan` webster verb: the re-entrant, blocking call Master makes, backgrounded, after begin-batch refuses with a plan conflict.
// It drives websterengine's plan-fixer phases with a real, wall-clock Clock.
// FixPlanSpawnOrAttach runs under the state-mutation lease, and a spawn is saved and fabric-committed "fix-plan spawn".
// FixPlanAwait runs with the lease released, so the fixer's own rebaseline can take it.
// A terminal result is recorded by RecordPlanFixerTerminal into a freshly reloaded state under a re-acquired lease, saved and committed "fix-plan <status>".
package webstercli

import (
	"errors"
	"fmt"
	"time"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/websterengine"
	"github.com/spf13/cobra"
)

// fixPlanDeps builds the websterengine.FixPlanDeps for this CLI's current wiring.
func (c *websterCLI) fixPlanDeps() websterengine.FixPlanDeps {
	return websterengine.FixPlanDeps{
		Starter:     c.starter,
		Roles:       c.roles,
		Config:      c.cfg,
		Engines:     c.engines,
		Reed:        c.reed,
		Stopper:     c.runner,
		ShuttleCfg:  c.shuttleCfg,
		Geom:        c.geom,
		FrictionDir: c.frictionDir,
	}
}

// fixPlanNotDoneMessage is the error of a fix-plan call whose fixer ended dead for reason.
func fixPlanNotDoneMessage(reason string) string {
	return fmt.Sprintf("webster: the plan fixer ended dead (%s); way forward: write outcome: stuck to your outcome file, with a stuck_reason quoting begin-batch's plan_conflict refusal verbatim, so the conflict goes to the plan writer", reason)
}

// fixPlanCmd builds the `fix-plan` subcommand.
func (c *websterCLI) fixPlanCmd() *cobra.Command {
	var wait time.Duration

	cmd := &cobra.Command{
		Use:         "fix-plan",
		Short:       "hand a begin-batch plan conflict to a cold plan fixer strand and long-poll it until it ends",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceRole},
		Long: `fix-plan spawns a cold plan fixer strand at the master role for the
plan-conflict record the latest begin-batch refusal left -- or, on a
re-entrant call, attaches to the fixer a prior call spawned and has not yet
seen end. The fixer edits the cards the conflict names so they match the
tree, runs "lym webster rebaseline" naming them, and writes a reply whose
first line is its verdict: "fixed" when every finding was a stale name, path
or glyph, "plan_writer" when a fix would change what a card does, in which
case it leaves the plan as the run recorded it. A call that spawns first
waits for its provider to come up, and every call then blocks for up to
--wait watching it.

A terminal call fabric-commits state.json and returns {"status": "done",
"verdict": "...", "reply": "..."} once the fixer wrote its reply. A reply
without a known verdict reads as "plan_writer", and so does a "fixed" reply
from a fixer that moved HEAD or left a new uncommitted change, with a
warning.
On "fixed", Master calls begin-batch again; on "plan_writer" it ends its run
stuck, so the conflict goes to the plan writer. A fixer whose turn ended
without a reply, whose strand is gone or that outlived recovery_timeout_min
returns {"status": "dead", "dead_reason": "..."} and exits non-zero. If
--wait elapses first it returns {"status": "running", "elapsed_s": N}.

It refuses, spawning nothing, when no plan-conflict record names a finding,
when an earlier plan fixer of the run was given the same findings, when the
conflict's batch already had two plan fixers, and when the master role's
engine cannot audit forks. A call that spawns fabric-commits state.json at
once, so the strand survives a crash. Under a pause that covers the pair it
refuses first, claiming Master into it.

Example:
  lym webster fix-plan
  lym webster fix-plan --wait 8m`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()

			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}

			if refusal := c.refuseCovered("fix-plan", "fix-plan spawns no plan fixer", ""); refusal != "" {
				clihelp.SetExit(cmd.Context(), output.Err(out, refusal))
				return nil
			}

			mutateLock, err := websterengine.AcquireStateMutation(c.geom.ScratchDir)
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}
			mutateHeld := true
			defer func() {
				if mutateHeld {
					_ = mutateLock.Release()
				}
			}()

			st, err := websterengine.LoadState(c.geom.WebsterDir, c.geom.ScratchDir)
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}
			if st == nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, `webster: no run in progress; run "lym webster run" first`))
				return nil
			}

			waitBudget := wait
			if waitBudget == 0 {
				waitBudget = websterengine.RecoveryWaitBudget(c.cfg)
			}

			// Standalone mode boots its own reed session here, as fix-verify does, since nothing else can reach standalone geometry.
			if c.reedUp != nil {
				if err := c.reedUp(); err != nil {
					clihelp.SetExit(cmd.Context(), output.Err(out, fmt.Sprintf("webster: bring up the standalone reed session: %v; way forward: transient, re-run `lym webster fix-plan`", err)))
					return nil
				}
			}

			deps := c.fixPlanDeps()
			rec, spawned, err := websterengine.FixPlanSpawnOrAttach(deps, st, recoverRealClock{})
			if err == nil && spawned {
				if err = websterengine.SaveState(c.geom.WebsterDir, c.geom.ScratchDir, st); err != nil {
					// The strand is live but unrecorded, so nothing later could stop it.
					err = errors.Join(err, deps.Stopper.StopStrand(rec.StrandGUID))
				}
			}
			_ = mutateLock.Release()
			mutateHeld = false
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}

			if spawned {
				if _, syncErr := fabricSync(c.openFabric, c.anchorRel, "fix-plan spawn"); syncErr != nil {
					clihelp.SetExit(cmd.Context(), output.Err(out, fmt.Sprintf("webster: the plan fixer spawned but the fabric sync failed: %v; %s", syncErr, fabricSyncWayForward)))
					return nil
				}
			}

			result, err := websterengine.FixPlanAwait(deps, rec, waitBudget, recoverRealClock{})
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}

			fields := map[string]any{"status": result.Status, "elapsed_s": result.ElapsedS}
			if result.Status == websterengine.DigestStatusRunning {
				fields["warnings"] = ownerlessRunWarnings(c.geom.ScratchDir, result.Warnings)
				clihelp.SetExit(cmd.Context(), output.Ok(out, fields))
				return nil
			}

			if err := c.persistFixPlanTerminal(rec.StrandGUID, result); err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}
			if _, syncErr := fabricSync(c.openFabric, c.anchorRel, "fix-plan "+result.Status); syncErr != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, fmt.Sprintf("webster: the plan fixer ended %s but the fabric sync failed: %v; %s", result.Status, syncErr, fabricSyncWayForward)))
				return nil
			}

			fields["warnings"] = ownerlessRunWarnings(c.geom.ScratchDir, result.Warnings)
			if result.Status != websterengine.DigestStatusDone {
				fields["dead_reason"] = result.DeadReason
				clihelp.SetExit(cmd.Context(), output.ErrFields(out, fixPlanNotDoneMessage(result.DeadReason), fields))
				return nil
			}
			fields["verdict"] = result.Verdict
			fields["reply"] = result.Reply
			clihelp.SetExit(cmd.Context(), output.Ok(out, fields))
			return nil
		},
	}

	cmd.Flags().DurationVar(&wait, "wait", 0, "operator override of the wait budget; a shorter wait can return a running snapshot; 0 blocks until the fixer ends or recovery_timeout_min")

	return cmd
}

// persistFixPlanTerminal records result as the terminal status of the plan fixer strandGUID names, in a state reloaded under a fresh state-mutation lease.
// A state that no longer records the fixer, which a reset leaves, is left unsaved.
func (c *websterCLI) persistFixPlanTerminal(strandGUID string, result *websterengine.FixPlanResult) error {
	lease, err := websterengine.AcquireStateMutation(c.geom.ScratchDir)
	if err != nil {
		return err
	}
	defer func() { _ = lease.Release() }()
	st, err := websterengine.LoadState(c.geom.WebsterDir, c.geom.ScratchDir)
	if err != nil {
		return err
	}
	if st == nil || !websterengine.RecordPlanFixerTerminal(st, strandGUID, result) {
		return nil
	}
	return websterengine.SaveState(c.geom.WebsterDir, c.geom.ScratchDir, st)
}
