// recoverbatch.go implements the `recover-batch` webster verb: the
// re-entrant, bounded long-poll escalation call Master's own prompt makes
// when a fork reports stuck or never reports at all. It drives
// websterengine's three lease-scoped phases with a real, wall-clock Clock:
// RecoverSpawnOrAttach under the state-mutation lease (saved and
// weft-committed "... spawn" when this call performed the spawn),
// RecoverAwait with the lease RELEASED (a single wait blocks up to
// poll_wait_s -- holding the lease across it would stall every concurrent
// verb and run entry for minutes, the exact hold AcquireStateMutation's
// contract forbids), and, on a terminal digest, PersistRecoveryTerminal
// into a FRESHLY reloaded state under a re-acquired lease, followed by the
// "... <status>" terminal weft commit -- webster's third and fourth
// weft-commit points, each now carrying exactly the mutation its label
// names.
package webstercli

import (
	"fmt"
	"strconv"
	"time"

	"github.com/Knatte18/loomyard/internal/builderengine"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/websterengine"
	"github.com/spf13/cobra"
)

// recoverRealClock is webstercli's own production websterengine.Clock: a
// genuine time.Now/time.Sleep, mirroring buildercli's own pollRealClock
// pattern (internal/buildercli/poll.go).
type recoverRealClock struct{}

func (recoverRealClock) Now() time.Time        { return time.Now() }
func (recoverRealClock) Sleep(d time.Duration) { time.Sleep(d) }

var _ websterengine.Clock = recoverRealClock{}

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
call weft-commits the batch report and state.json and returns the digest
envelope, exactly like record-batch's own terminal envelope. If --wait
elapses first it returns {"batch": "NN-<slug>", "status": "running",
"elapsed_s": N} instead, touching neither git nor weft -- Master re-calls
recover-batch again. A call that performs the spawn itself weft-commits
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

			plan, err := builderengine.ParsePlan(c.planDir)
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}

			var slug string
			for _, b := range plan.Batches {
				if b.Number == batchNumber {
					slug = b.Slug
					break
				}
			}
			batchName := fmt.Sprintf("%02d-%s", batchNumber, slug)

			mutateLock, err := websterengine.AcquireStateMutation(c.websterDir)
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

			st, err := websterengine.LoadState(c.websterDir)
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
				Starter:      c.starter,
				Plan:         plan,
				State:        st,
				Roles:        c.roles,
				Config:       c.cfg,
				Engine:       c.engine,
				Mux:          c.mux,
				ShuttleCfg:   c.shuttleCfg,
				Layout:       c.layout,
				WorktreeRoot: c.layout.Cwd,
				WebsterDir:   c.websterDir,
				ReportsDir:   c.reportsDir,
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
				if err := websterengine.SaveState(c.websterDir, st); err != nil {
					_ = mutateLock.Release()
					mutateHeld = false
					clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
					return nil
				}
			}
			// The state phase is over; the bounded wait below runs with the
			// lease RELEASED, per AcquireStateMutation's contract.
			_ = mutateLock.Release()
			mutateHeld = false

			if spawned {
				if _, weftErr := weftCommit(c.layout, fmt.Sprintf("recover-batch %s spawn", batchName)); weftErr != nil {
					clihelp.SetExit(cmd.Context(), output.Err(out, fmt.Sprintf("webster: batch %s recovery spawned but the weft sync failed: %v", batchName, weftErr)))
					return nil
				}
			}

			result, err := websterengine.RecoverAwait(deps, batchNumber, bs, waitBudget, recoverRealClock{})
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}

			if result.Digest != nil {
				// Terminal: re-acquire the lease and merge the digest into a
				// FRESHLY loaded state — the pre-wait copy may be minutes
				// stale, and saving it would erase a concurrent mutation.
				terminalLock, err := websterengine.AcquireStateMutation(c.websterDir)
				if err != nil {
					clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
					return nil
				}
				fresh, err := websterengine.LoadState(c.websterDir)
				if err == nil && fresh == nil {
					err = fmt.Errorf("webster: state.json disappeared during the recovery wait for batch %s", batchName)
				}
				if err == nil {
					err = websterengine.PersistRecoveryTerminal(fresh, batchNumber, result.Digest)
				}
				if err == nil {
					err = websterengine.SaveState(c.websterDir, fresh)
				}
				_ = terminalLock.Release()
				if err != nil {
					clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
					return nil
				}

				if _, weftErr := weftCommit(c.layout, fmt.Sprintf("recover-batch %s %s", batchName, result.Digest.Status)); weftErr != nil {
					clihelp.SetExit(cmd.Context(), output.Err(out, fmt.Sprintf("webster: batch %s recovery classified %s but the weft sync failed: %v", batchName, result.Digest.Status, weftErr)))
					return nil
				}

				fields := digestFields(*result.Digest)
				fields["warnings"] = result.Warnings
				clihelp.SetExit(cmd.Context(), output.Ok(out, fields))
				return nil
			}

			clihelp.SetExit(cmd.Context(), output.Ok(out, map[string]any{
				"batch":     batchName,
				"status":    "running",
				"elapsed_s": result.ElapsedS,
			}))
			return nil
		},
	}

	cmd.Flags().DurationVar(&wait, "wait", 0, "long-poll wait budget before returning a running snapshot; 0 defers to webster.yaml's poll_wait_s")

	return cmd
}
