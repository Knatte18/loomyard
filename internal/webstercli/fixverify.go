// fixverify.go implements the `fix-verify` webster verb: the re-entrant, blocking call Master makes, backgrounded, when a verify-gate failure's way forward names the cold fixer.
// It drives websterengine's cold-fixer phases with a real, wall-clock Clock.
// FixVerifySpawnOrAttach runs under the state-mutation lease, and a spawn is saved and fabric-committed "fix-verify spawn".
// FixVerifyAwait runs with the lease released.
// A terminal result is recorded by RecordVerifyFixerTerminal into a freshly reloaded state under a re-acquired lease, saved and committed "fix-verify <status>".
package webstercli

import (
	"fmt"
	"time"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/websterengine"
	"github.com/spf13/cobra"
)

// fixVerifyDeps builds the websterengine.FixVerifyDeps for this CLI's current wiring.
// It refuses a gate config that cannot be loaded, before the caller mutates anything.
func (c *websterCLI) fixVerifyDeps() (websterengine.FixVerifyDeps, error) {
	gate, err := c.loadGateConfig()
	if err != nil {
		return websterengine.FixVerifyDeps{}, err
	}
	return websterengine.FixVerifyDeps{
		Starter:        c.starter,
		Roles:          c.roles,
		Config:         c.cfg,
		Engines:        c.engines,
		Reed:           c.reed,
		Stopper:        c.runner,
		ShuttleCfg:     c.shuttleCfg,
		Geom:           c.geom,
		FrictionDir:    c.frictionDir,
		LanguageSkills: c.languageSkills,
		Gate:           gate,
	}, nil
}

// fixVerifyNotDoneMessage is the error of a fix-verify call whose fixer ended dead for reason.
func fixVerifyNotDoneMessage(reason string) string {
	return fmt.Sprintf("webster: the cold verify-gate fixer ended dead (%s); way forward: rewrite your two contract files and end your turn, so the gate verifies again", reason)
}

// fixVerifyCmd builds the `fix-verify` subcommand.
func (c *websterCLI) fixVerifyCmd() *cobra.Command {
	var wait time.Duration

	cmd := &cobra.Command{
		Use:         "fix-verify",
		Short:       "escalate a verify-gate failure to a cold fixer strand and long-poll it until it ends",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceRole},
		Long: `fix-verify spawns a cold, fresh fixer strand at the recovery role for the
latest verify-gate failure the run's verify-gate report records -- or, on a
re-entrant call, attaches to the fixer a prior call spawned and has not yet
seen end. The fixer reads the report, fixes the cause in source, commits each
fix as "fix: <summary>" and writes a reply naming what it fixed and what it
could not. A call that spawns first waits for its provider to come up, and
every call then blocks for up to --wait watching it.

A terminal call fabric-commits state.json and returns {"status": "done",
"reply": "..."} once the fixer wrote its reply; a fixer whose turn ended
without a reply, whose strand is gone or that outlived recovery_timeout_min
returns {"status": "dead", "dead_reason": "..."} and exits non-zero. Either
way the gate, not this verb, judges the fix: Master rewrites its contract
files and ends its turn, and the gate verifies again. If --wait elapses first
it returns {"status": "running", "elapsed_s": N}.

It refuses, spawning nothing, when no verify-gate report is on disk, when
the recovery role's engine cannot audit forks and when the gate config has
no "round" list. A call that spawns fabric-commits state.json at once, so
the strand survives a crash. Under a pause that covers the pair it refuses
first, claiming Master into it.

Example:
  lyx webster fix-verify
  lyx webster fix-verify --wait 8m`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()

			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}

			if refusal := c.refuseCovered("fix-verify", "fix-verify spawns no cold fixer", ""); refusal != "" {
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
				clihelp.SetExit(cmd.Context(), output.Err(out, `webster: no run in progress; run "lyx webster run" first`))
				return nil
			}

			waitBudget := wait
			if waitBudget == 0 {
				waitBudget = websterengine.RecoveryWaitBudget(c.cfg)
			}

			// Standalone mode boots its own reed session here, as recover-batch does, since nothing else can reach standalone geometry.
			if c.reedUp != nil {
				if err := c.reedUp(); err != nil {
					clihelp.SetExit(cmd.Context(), output.Err(out, fmt.Sprintf("webster: bring up the standalone reed session: %v; way forward: transient, re-run `lyx webster fix-verify`", err)))
					return nil
				}
			}

			deps, err := c.fixVerifyDeps()
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}

			rec, spawned, err := websterengine.FixVerifySpawnOrAttach(deps, st, recoverRealClock{})
			if err == nil && spawned {
				err = websterengine.SaveState(c.geom.WebsterDir, c.geom.ScratchDir, st)
			}
			_ = mutateLock.Release()
			mutateHeld = false
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}

			if spawned {
				if _, syncErr := fabricSync(c.openFabric, c.anchorRel, "fix-verify spawn"); syncErr != nil {
					clihelp.SetExit(cmd.Context(), output.Err(out, fmt.Sprintf("webster: the cold verify-gate fixer spawned but the fabric sync failed: %v; %s", syncErr, fabricSyncWayForward)))
					return nil
				}
			}

			result, err := websterengine.FixVerifyAwait(deps, rec, waitBudget, recoverRealClock{})
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

			if err := c.persistFixVerifyTerminal(rec.StrandGUID, result); err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}
			if _, syncErr := fabricSync(c.openFabric, c.anchorRel, "fix-verify "+result.Status); syncErr != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, fmt.Sprintf("webster: the cold verify-gate fixer ended %s but the fabric sync failed: %v; %s", result.Status, syncErr, fabricSyncWayForward)))
				return nil
			}

			fields["warnings"] = ownerlessRunWarnings(c.geom.ScratchDir, result.Warnings)
			if result.Status != websterengine.DigestStatusDone {
				fields["dead_reason"] = result.DeadReason
				clihelp.SetExit(cmd.Context(), output.ErrFields(out, fixVerifyNotDoneMessage(result.DeadReason), fields))
				return nil
			}
			fields["reply"] = result.Reply
			clihelp.SetExit(cmd.Context(), output.Ok(out, fields))
			return nil
		},
	}

	cmd.Flags().DurationVar(&wait, "wait", 0, "operator override of the wait budget; a shorter wait can return a running snapshot; 0 blocks until the fixer ends or recovery_timeout_min")

	return cmd
}

// persistFixVerifyTerminal records result as the terminal status of the fixer strandGUID names, in a state reloaded under a fresh state-mutation lease.
// A state that no longer records the fixer, which a reset leaves, is left unsaved.
func (c *websterCLI) persistFixVerifyTerminal(strandGUID string, result *websterengine.FixVerifyResult) error {
	lease, err := websterengine.AcquireStateMutation(c.geom.ScratchDir)
	if err != nil {
		return err
	}
	defer func() { _ = lease.Release() }()
	st, err := websterengine.LoadState(c.geom.WebsterDir, c.geom.ScratchDir)
	if err != nil {
		return err
	}
	if st == nil || !websterengine.RecordVerifyFixerTerminal(st, strandGUID, result) {
		return nil
	}
	return websterengine.SaveState(c.geom.WebsterDir, c.geom.ScratchDir, st)
}
