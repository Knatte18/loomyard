// recoverbatch.go implements the `recover-batch` webster verb: the re-entrant, blocking escalation call Master's own prompt makes, backgrounded, when a fork reports stuck or never reports at all.
// It drives websterengine's three lease-scoped phases with a real, wall-clock Clock:
// RecoverSpawnOrAttach under the state-mutation lease (saved and fabric-committed "...
// spawn" when this call performed the spawn) -- the spawn phase under the lease now includes the
// provider's startup window (bounded by startup_timeout_s) -- RecoverAwait with the lease RELEASED
// (the wait phase that blocks up to recovery_timeout_min still runs with the lease released -- holding the
// lease across it would stall every concurrent verb and run entry for minutes, the exact hold
// AcquireStateMutation's contract forbids), and, on a terminal
// digest, PersistRecoveryTerminal into a FRESHLY reloaded state under a re-acquired lease, followed
// by the "... <status>" terminal fabric commit -- webster's third and fourth fabric-commit points, each
// now carrying exactly the mutation its label names.
// A *BatchFailedError from that persist step saves the reloaded state and commits it "... failed" before the batch_failed error envelope.
package webstercli

import (
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/Knatte18/loomyard/internal/batcher"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/websterengine"
	"github.com/spf13/cobra"
)

// recoverRealClock is webstercli's production websterengine.Clock using time.Now/time.Sleep.
type recoverRealClock struct{}

func (recoverRealClock) Now() time.Time        { return time.Now() }
func (recoverRealClock) Sleep(d time.Duration) { time.Sleep(d) }

var _ websterengine.Clock = recoverRealClock{}

// batchSlugFor returns the slug of the batcher.Batch whose identity matches batchNumber.
func batchSlugFor(batches []batcher.Batch, batchNumber int) string {
	for _, b := range batches {
		if len(b.Cards) > 0 && b.Cards[0].Number == batchNumber {
			return b.Cards[0].Slug
		}
	}
	return ""
}

// recoverBatchCmd builds the `recover-batch <nn>` subcommand.
func (c *websterCLI) recoverBatchCmd() *cobra.Command {
	var wait time.Duration

	cmd := &cobra.Command{
		Use:         "recover-batch <nn>",
		Short:       "escalate one batch to a cold recovery strand and long-poll it for a terminal digest",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceRole},
		Long: `recover-batch <nn> spawns a cold, fresh recovery strand for a batch a fork
reported stuck (or never reported at all) -- or, on a re-entrant call,
attaches to the recovery strand a prior call already spawned. A call that
spawns the recovery strand first waits for its provider to come up
(normally seconds), and every call then blocks for up to --wait watching it
for a terminal classification. A terminal
call fabric-commits the batch report and state.json and returns the digest
envelope, exactly like record-batch's own terminal envelope plus "recoveries",
the batch's counted recovery spawns, and "recovery_retry", true when the
recovery is dead, committed work of its own and the batch has a recovery left
to run. A call that would spawn a third counted recovery refuses with
{"recovery_exhausted": true}. A recovery
the post-batch checks reject takes the batch terminal failed: the failed
state and archived report are saved and committed, and the call exits
non-zero with {"batch_failed": true, "batch": "NN-<slug>", "warnings": [...]};
the envelope carries "card_amended": true when a card of the batch was amended
after the recovery spawned, and the error then names "lyx webster recover-batch NN"
again, which re-runs the batch on the amended card;
the error names "lyx webster recover-batch NN" unless a later card still
references a symbol the batch deletes: a call that finds that before spawning
refuses with {"batch_failed": true} and the plan edit as its way forward. If --wait
elapses first it returns {"batch": "NN-<slug>", "status": "running",
"elapsed_s": N} instead, touching neither git nor the repo -- Master re-calls
recover-batch again. A call that performs the spawn itself fabric-commits
state.json immediately, so a freshly-recorded recovery strand survives a
crash even if the bounded wait that follows never reaches terminal.

Example:
  lyx webster recover-batch 3
  lyx webster recover-batch 3 --wait 8m`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()

			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}

			batchNumber, err := strconv.Atoi(args[0])
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, fmt.Sprintf("webster: %q is not a valid batch number: %v", args[0], err)))
				return nil
			}

			plan, err := planparser.ParsePlan(c.geom.PlanDir)
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
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

			batches, err := c.executionBatches(plan, st)
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}
			batchName := fmt.Sprintf("%02d-%s", batchNumber, batchSlugFor(batches, batchNumber))

			waitBudget := wait
			if waitBudget == 0 {
				waitBudget = websterengine.RecoveryWaitBudget(c.cfg)
			}

			// Standalone mode boots its own reed session here, idempotently, because nothing else
			// can: `lyx reed up` is hub-only, so the advice reed's own "no reed session" error gives
			// cannot reach standalone geometry at all. recover-batch is not a read-only verb — it
			// spawns a cold recovery strand through reed.AddStrand, which now self-heals a cold
			// worktree on its own rather than requiring a live session — so this call is no longer
			// the only thing standing between a standalone spawn and a dead end. It stays as a
			// deliberate early, explicit boot: after a run ends its session is gone, and this keeps
			// `lyx webster recover-batch N` failing with its own envelope-reportable error rather
			// than whatever AddStrand's own self-heal would surface. Nil in hub mode, where the
			// session is the operator's or loom's own to manage.
			//
			// It runs HERE rather than at the top of this RunE so a call that refuses on a bad batch
			// number, an unparseable plan, or an absent run boots no substrate at all — an ordering
			// property the self-heal does not provide on its own, since AddStrand's boot now happens
			// wherever AddStrand is called, not at a controlled chokepoint like this one; the
			// state-mutation lease is already held across the spawn RecoverSpawnOrAttach itself
			// performs, so bringing the session up under it adds no new hold.
			//
			// watch is false here, unlike run's true: recover-batch is a short-lived verb that spawns
			// a cold recovery strand and returns, so a watcher bound to this call's context would be
			// dead before it observed anything, while one detached from that context would be an
			// unowned goroutine in an exiting process.
			if c.reedUp != nil {
				if err := c.reedUp(cmd.Context(), false); err != nil {
					clihelp.SetExit(cmd.Context(), output.Err(out, fmt.Sprintf("webster: bring up the standalone reed session: %v; way forward: transient, re-run `lyx webster recover-batch %02d`", err, batchNumber)))
					return nil
				}
			}

			deps := websterengine.RecoverDeps{
				Starter:      c.starter,
				Plan:         plan,
				Batches:      batches,
				State:        st,
				Roles:        c.roles,
				Config:       c.cfg,
				Engine:       c.engine,
				Reed:         c.reed,
				Stopper:      c.runner,
				ShuttleCfg:   c.shuttleCfg,
				Geom:         c.geom,
				ReadOnly:     fabricengine.IsReadOnlyCommand,
				FrictionDir:  c.frictionDir,
				ParentBranch: c.parentBranch,
			}

			bs, spawned, err := websterengine.RecoverSpawnOrAttach(deps, batchNumber, recoverRealClock{})
			if err != nil {
				_ = mutateLock.Release()
				mutateHeld = false
				if errors.Is(err, websterengine.ErrRecoveryNeedsFresh) {
					clihelp.SetExit(cmd.Context(), output.ErrFields(out, err.Error(), map[string]any{"needs_fresh": true}))
					return nil
				}
				if errors.Is(err, websterengine.ErrAuditNotAcceptable) {
					clihelp.SetExit(cmd.Context(), output.ErrFields(out, err.Error(), map[string]any{"audit_not_acceptable": true}))
					return nil
				}
				if errors.Is(err, websterengine.ErrRecoveryExhausted) {
					clihelp.SetExit(cmd.Context(), output.ErrFields(out, err.Error(), map[string]any{"recovery_exhausted": true}))
					return nil
				}
				if errors.Is(err, websterengine.ErrRecoveryDeleteReferenced) {
					clihelp.SetExit(cmd.Context(), output.ErrFields(out, err.Error(), map[string]any{"batch_failed": true}))
					return nil
				}
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}

			// A spawn mutated the state; persist the fresh strand record
			// before anything else so a crash mid-wait leaves it reclaimable.
			// A pure attach mutated nothing and needs no save.
			if spawned {
				if err := websterengine.SaveState(c.geom.WebsterDir, c.geom.ScratchDir, st); err != nil {
					_ = mutateLock.Release()
					mutateHeld = false
					clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
					return nil
				}
			}

			_ = mutateLock.Release()
			mutateHeld = false

			if spawned {
				if _, syncErr := fabricSync(c.openFabric, c.anchorRel, fmt.Sprintf("recover-batch %s spawn", batchName)); syncErr != nil {
					clihelp.SetExit(cmd.Context(), output.Err(out, fmt.Sprintf("webster: batch %s recovery spawned but the fabric sync failed: %v; %s", batchName, syncErr, fabricSyncWayForward)))
					return nil
				}
			}

			result, err := websterengine.RecoverAwait(deps, batchNumber, bs, waitBudget, recoverRealClock{})
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}

			if result.Digest != nil {
				terminalLock, err := websterengine.AcquireStateMutation(c.geom.ScratchDir)
				if err != nil {
					clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
					return nil
				}
				// Guarded by defer, the same shape every other lease in this package uses
				// (beginbatch.go, recordbatch.go, and this verb's own first lease). Nothing between
				// here and the explicit release below returns today, so this leaks nothing now — but
				// a future `return nil` added inside this block, which is the shape used everywhere
				// else in these RunEs, would hold mutate.lock for the process lifetime and block
				// every subsequent bracket verb (crucible round opus-medium-r6, R6-23).
				terminalHeld := true
				defer func() {
					if terminalHeld {
						_ = terminalLock.Release()
					}
				}()
				fresh, err := websterengine.LoadState(c.geom.WebsterDir, c.geom.ScratchDir)
				if err == nil && fresh == nil {
					err = fmt.Errorf("webster: state.json disappeared during the recovery wait for batch %s; way forward: re-run `lyx webster recover-batch %02d`", batchName, batchNumber)
				}
				var fingerprintBefore string
				var postWarnings []string
				if err == nil {
					fingerprintBefore = fresh.PlanFingerprint
					postWarnings, err = websterengine.PersistRecoveryTerminal(deps, fresh, batchNumber, result.Digest)
					result.Warnings = append(result.Warnings, postWarnings...)
				}
				batchFailed := errors.Is(err, websterengine.ErrBatchFailed)
				if err == nil {
					err = websterengine.SaveState(c.geom.WebsterDir, c.geom.ScratchDir, fresh)
				} else if batchFailed {
					// The batch went terminal failed: the whole reloaded state is the point,
					// so it is saved rather than only the fingerprint re-baseline.
					if saveErr := websterengine.SaveState(c.geom.WebsterDir, c.geom.ScratchDir, fresh); saveErr != nil {
						err = fmt.Errorf("%w; additionally, persisting the failed batch failed: %v", err, saveErr)
						batchFailed = false
					}
				} else if fresh != nil {
					// The post-batch pass re-baselines the plan fingerprint the moment handle
					// binding or the exact-tier drift repair rewrites the plan on disk, and it can
					// then block on a finding computed from the same delta. Persist that
					// re-baseline for the same reason both bracket verbs do.
					if saveErr := persistPlanFingerprintRebaseline(c.geom, fresh, fingerprintBefore); saveErr != nil {
						err = fmt.Errorf("%w; additionally, persisting the plan-fingerprint re-baseline this call had already earned failed: %v", err, saveErr)
					}
				}
				_ = terminalLock.Release()
				terminalHeld = false
				if batchFailed {
					msg := err.Error()
					if _, syncErr := fabricSync(c.openFabric, c.anchorRel, fmt.Sprintf("recover-batch %s failed", batchName)); syncErr != nil {
						msg = fmt.Sprintf("%s; additionally, the fabric sync failed: %v; %s", msg, syncErr, fabricSyncWayForward)
					}
					clihelp.SetExit(cmd.Context(), output.ErrFields(out, msg, c.batchFailedFields(batchName, result.Warnings, err)))
					return nil
				}
				if errors.Is(err, websterengine.ErrFingerprintMismatch) {
					clihelp.SetExit(cmd.Context(), output.ErrFields(out, err.Error(), map[string]any{"plan_drifted": true}))
					return nil
				}
				if err != nil {
					clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
					return nil
				}

				if _, syncErr := fabricSync(c.openFabric, c.anchorRel, fmt.Sprintf("recover-batch %s %s", batchName, result.Digest.Status)); syncErr != nil {
					clihelp.SetExit(cmd.Context(), output.Err(out, fmt.Sprintf("webster: batch %s recovery classified %s but the fabric sync failed: %v; %s", batchName, result.Digest.Status, syncErr, fabricSyncWayForward)))
					return nil
				}

				fields := digestFields(*result.Digest)
				fields["recovery_retry"], fields["recoveries"] = websterengine.RecoveryRetry(c.geom, fresh, batchNumber)
				fields["warnings"] = ownerlessRunWarnings(c.geom.ScratchDir, result.Warnings)
				clihelp.SetExit(cmd.Context(), output.Ok(out, fields))
				return nil
			}

			clihelp.SetExit(cmd.Context(), output.Ok(out, map[string]any{
				"batch":     batchName,
				"status":    "running",
				"elapsed_s": result.ElapsedS,
				"warnings":  ownerlessRunWarnings(c.geom.ScratchDir, result.Warnings),
			}))
			return nil
		},
	}

	cmd.Flags().DurationVar(&wait, "wait", 0, "operator override of the wait budget; a shorter wait can return a running snapshot; 0 blocks until a terminal digest or recovery_timeout_min")

	return cmd
}
