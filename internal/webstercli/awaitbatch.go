// awaitbatch.go implements the `await-batch` webster verb: the bounded
// long-poll Master calls between forking a batch's implementer and recording
// it. Forks are backgrounded agents on current Claude Code (they return
// immediately, not synchronously inside Master's turn — round fable-r1's F1
// finding), so Master must stay inside its turn by blocking on this verb
// until the fork's report lands; ending the turn instead classifies the run
// asking and kills it. The verb is deliberately stateless: no state.json
// read, no lease, no weft — a pure bounded watch on the report path, so it
// can never corrupt a run no matter who calls it or when.
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

// awaitBatchCmd builds the `await-batch <NN>` subcommand.
func (c *websterCLI) awaitBatchCmd() *cobra.Command {
	var wait time.Duration

	cmd := &cobra.Command{
		Use:   "await-batch <NN>",
		Short: "block until one batch's report file lands (or the wait window elapses)",
		Long: `await-batch <NN> blocks for up to --wait watching for batch NN's report
file to appear, returning {"batch": "NN-<slug>", "report": true} the moment
it lands or {"report": false} when the window elapses first. It reads and
mutates nothing else -- no state.json, no weft -- so it is safe to call at
any time. Master calls it immediately after spawning a batch's fork (forks
are backgrounded agents and return before the batch is done): re-call it
while the fork is still running, then call record-batch once it returns
{"report": true} -- or once the fork has finished without a report
(record-batch then classifies no_report).

Example:
  lyx webster await-batch 3
  lyx webster await-batch 3 --wait 8m`,
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

			waitBudget := wait
			if waitBudget == 0 {
				waitBudget = time.Duration(c.cfg.PollWaitS) * time.Second
			}

			result, err := websterengine.AwaitBatch(plan, c.reportsDir, batchNumber, waitBudget, recoverRealClock{})
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}

			clihelp.SetExit(cmd.Context(), output.Ok(out, map[string]any{
				"batch":     result.BatchName,
				"report":    result.ReportPresent,
				"elapsed_s": result.ElapsedS,
			}))
			return nil
		},
	}

	cmd.Flags().DurationVar(&wait, "wait", 0, "long-poll wait budget before returning report:false; 0 defers to webster.yaml's poll_wait_s")

	return cmd
}
