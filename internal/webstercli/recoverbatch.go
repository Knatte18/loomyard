// recoverbatch.go implements the `recover-batch` webster verb: the re-entrant, bounded long-poll
// escalation call Master's own prompt makes when a fork reports stuck or never reports at all.
// It drives websterengine's three lease-scoped phases with a real, wall-clock Clock:
// RecoverSpawnOrAttach under the state-mutation lease (saved and fabric-committed "...
// spawn" when this call performed the spawn), RecoverAwait with the lease RELEASED (a single wait
// blocks up to poll_wait_s -- holding the lease across it would stall every concurrent verb and run
// entry for minutes, the exact hold AcquireStateMutation's contract forbids), and, on a terminal
// digest, PersistRecoveryTerminal into a FRESHLY reloaded state under a re-acquired lease, followed
// by the "... <status>" terminal fabric commit -- webster's third and fourth fabric-commit points, each
// now carrying exactly the mutation its label names.
package webstercli

import (
	"fmt"
	"strconv"
	"time"

	"github.com/Knatte18/loomyard/internal/batcher"
	"github.com/Knatte18/loomyard/internal/clihelp"
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

// recoverBatchCmd builds the `recover-batch <NN>` subcommand.
func (c *websterCLI) recoverBatchCmd() *cobra.Command {
	var wait time.Duration

	cmd := &cobra.Command{
		Use:   "recover-batch <NN>",
		Short: "escalate one batch to a cold recovery strand and long-poll it for a terminal digest",
		Long: `recover-batch <NN> spawns a cold, fresh recovery strand for a batch a fork
reported stuck (or never reported at all) -- or, on a re-entrant call,
attaches to the recovery strand a prior call already spawned -- then blocks
for up to --wait watching it for a terminal classification. A terminal
call fabric-commits the batch report and state.json and returns the digest
envelope, exactly like record-batch's own terminal envelope. If --wait
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
			// Every batch-computation site sequences, so all five agree on
			// one order by construction rather than by comment.
			batches, _ := websterengine.SequenceBatches(c.batcher.Batch(plan.Cards))
			batchName := fmt.Sprintf("%02d-%s", batchNumber, batchSlugFor(batches, batchNumber))

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
				waitBudget = time.Duration(c.cfg.PollWaitS) * time.Second
			}

			// Standalone mode boots its own reed session here, idempotently, because nothing else
			// can: `lyx reed up` is hub-only, so the advice reed's own "no reed session" error gives
			// cannot reach standalone geometry at all. recover-batch is not a read-only verb — it
			// SPAWNS a cold recovery strand through reed.AddStrand, which requires a live session —
			// and after a run ends its session is gone, so `lyx webster recover-batch N` was the same
			// impossible-recourse dead end `run` already fixed. Nil in hub mode, where the session is
			// the operator's or loom's own to manage.
			//
			// It runs HERE rather than at the top of this RunE so a call that refuses on a bad batch
			// number, an unparseable plan, or an absent run boots no substrate at all; the
			// state-mutation lease is already held across the spawn RecoverSpawnOrAttach itself
			// performs, so bringing the session up under it adds no new hold.
			if c.reedUp != nil {
				if err := c.reedUp(); err != nil {
					clihelp.SetExit(cmd.Context(), output.Err(out, fmt.Sprintf("webster: bring up the standalone reed session: %v", err)))
					return nil
				}
			}

			deps := websterengine.RecoverDeps{
				Starter:    c.starter,
				Plan:       plan,
				Batches:    batches,
				State:      st,
				Roles:      c.roles,
				Config:     c.cfg,
				Engine:     c.engine,
				Reed:       c.reed,
				ShuttleCfg: c.shuttleCfg,
				Geom:       c.geom,
			}

			bs, spawned, err := websterengine.RecoverSpawnOrAttach(deps, batchNumber, recoverRealClock{})
			if err != nil {
				_ = mutateLock.Release()
				mutateHeld = false
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
					clihelp.SetExit(cmd.Context(), output.Err(out, fmt.Sprintf("webster: batch %s recovery spawned but the fabric sync failed: %v", batchName, syncErr)))
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
					err = fmt.Errorf("webster: state.json disappeared during the recovery wait for batch %s", batchName)
				}
				var fingerprintBefore string
				var postWarnings []string
				if err == nil {
					fingerprintBefore = fresh.PlanFingerprint
					postWarnings, err = websterengine.PersistRecoveryTerminal(deps, fresh, batchNumber, result.Digest)
					result.Warnings = append(result.Warnings, postWarnings...)
				}
				if err == nil {
					err = websterengine.SaveState(c.geom.WebsterDir, c.geom.ScratchDir, fresh)
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
				if err != nil {
					clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
					return nil
				}

				if _, syncErr := fabricSync(c.openFabric, c.anchorRel, fmt.Sprintf("recover-batch %s %s", batchName, result.Digest.Status)); syncErr != nil {
					clihelp.SetExit(cmd.Context(), output.Err(out, fmt.Sprintf("webster: batch %s recovery classified %s but the fabric sync failed: %v", batchName, result.Digest.Status, syncErr)))
					return nil
				}

				fields := digestFields(*result.Digest)
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

	cmd.Flags().DurationVar(&wait, "wait", 0, "long-poll wait budget before returning a running snapshot; 0 defers to webster.yaml's poll_wait_s")

	return cmd
}
