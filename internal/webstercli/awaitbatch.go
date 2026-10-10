// awaitbatch.go implements the `await-batch` webster verb: a bounded wait for one batch's report
// file, for an operator watching a run.
// Master does not call it: it ends its turn while a backgrounded fork works and records the batch
// on the fork's completion notification.
// The verb is deliberately stateless: no state.json read, no lease, no fabric commit — a pure bounded watch
// on the report path, so it can never corrupt a run no matter who calls it or when.
package webstercli

import (
	"fmt"
	"strconv"
	"time"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/websterengine"
	"github.com/spf13/cobra"
)

// awaitBatchCmd builds the `await-batch <nn>` subcommand.
func (c *websterCLI) awaitBatchCmd() *cobra.Command {
	var wait time.Duration

	cmd := &cobra.Command{
		Use:         "await-batch <nn>",
		Short:       "block until one batch's report file lands (or the wait window elapses)",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceRole},
		Long: `await-batch <nn> blocks for up to --wait watching for batch NN's report
file to appear, returning {"batch": "NN-<slug>", "report": true} the moment
it lands or {"report": false} when the window elapses first. It reads and
mutates nothing else -- no state.json, no fabric commit -- so it is safe to call at
any time, for example by an operator watching a run; Master itself waits on
its fork's completion notification instead.

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

			plan, err := planparser.ParsePlan(c.geom.PlanDir)
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}
			st, err := websterengine.LoadState(c.geom.WebsterDir, c.geom.ScratchDir)
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}
			batches, err := c.executionBatches(plan, st)
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}

			// Default to a short block so an agent caller's foreground call
			// stays under Claude Code's auto-background threshold.
			waitBudget := wait
			if waitBudget == 0 {
				waitBudget = time.Duration(websterengine.DefaultAwaitWaitS) * time.Second
			}

			result, err := websterengine.AwaitBatch(batches, c.geom.ReportsDir, batchNumber, waitBudget, recoverRealClock{})
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}

			clihelp.SetExit(cmd.Context(), output.Ok(out, map[string]any{
				"batch":     result.BatchName,
				"report":    result.ReportPresent,
				"elapsed_s": result.ElapsedS,
				"warnings":  ownerlessRunWarnings(c.geom.ScratchDir, nil),
			}))
			return nil
		},
	}

	cmd.Flags().DurationVar(&wait, "wait", 0, "poll window before returning report:false; 0 uses the short foreground default (~30s), deliberately NOT poll_wait_s")

	return cmd
}
