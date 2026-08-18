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

			plan, err := planparser.ParsePlan(c.planDir)
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}
			batches := c.batcher.Batch(plan.Cards)
			batchName := fmt.Sprintf("%02d-%s", batchNumber, batchSlugFor(batches, batchNumber))

			mutateLock, err := websterengine.AcquireStateMutation(c.websterScratchDir)
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

			st, err := websterengine.LoadState(c.websterDir, c.websterScratchDir)
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
				if err := websterengine.SaveState(c.websterDir, c.websterScratchDir, st); err != nil {
					_ = mutateLock.Release()
					mutateHeld = false
					clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
					return nil
				}
			}

			_ = mutateLock.Release()
			mutateHeld = false

			if spawned {
				if _, syncErr := fabricSync(c.layout, fmt.Sprintf("recover-batch %s spawn", batchName)); syncErr != nil {
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
				terminalLock, err := websterengine.AcquireStateMutation(c.websterScratchDir)
				if err != nil {
					clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
					return nil
				}
				fresh, err := websterengine.LoadState(c.websterDir, c.websterScratchDir)
				if err == nil && fresh == nil {
					err = fmt.Errorf("webster: state.json disappeared during the recovery wait for batch %s", batchName)
				}
				if err == nil {
					err = websterengine.PersistRecoveryTerminal(fresh, batchNumber, result.Digest)
				}
				if err == nil {
					err = websterengine.SaveState(c.websterDir, c.websterScratchDir, fresh)
				}
				_ = terminalLock.Release()
				if err != nil {
					clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
					return nil
				}

				if _, syncErr := fabricSync(c.layout, fmt.Sprintf("recover-batch %s %s", batchName, result.Digest.Status)); syncErr != nil {
					clihelp.SetExit(cmd.Context(), output.Err(out, fmt.Sprintf("webster: batch %s recovery classified %s but the fabric sync failed: %v", batchName, result.Digest.Status, syncErr)))
					return nil
				}

				fields := digestFields(*result.Digest)
				fields["warnings"] = ownerlessRunWarnings(c.websterScratchDir, result.Warnings)
				clihelp.SetExit(cmd.Context(), output.Ok(out, fields))
				return nil
			}

			clihelp.SetExit(cmd.Context(), output.Ok(out, map[string]any{
				"batch":     batchName,
				"status":    "running",
				"elapsed_s": result.ElapsedS,
				"warnings":  ownerlessRunWarnings(c.websterScratchDir, result.Warnings),
			}))
			return nil
		},
	}

	cmd.Flags().DurationVar(&wait, "wait", 0, "long-poll wait budget before returning a running snapshot; 0 defers to webster.yaml's poll_wait_s")

	return cmd
}
