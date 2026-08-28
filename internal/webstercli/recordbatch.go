// recordbatch.go implements the `record-batch` webster verb: Master's own bracket call immediately
// after a batch's fork returns.
// It runs websterengine.RecordBatch under the state-mutation lease (load, mutate, save, release)
// with a real, time.Sleep-backed Sleeper for the incremental fork audit's bounded settle retry,
// then performs the second of webster's four fabric-commit points (see the discussion's
// fabric-ownership decision): state.json and the batch report, once RecordBatch either lands a
// terminal digest or advances transcript attribution on a no_report retry -- both mutate
// deps.State, so both are durable before Master's next tool call.
package webstercli

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/summaryparser"
	"github.com/Knatte18/loomyard/internal/websterengine"
	"github.com/spf13/cobra"
)

// realSleeper is webstercli's production websterengine.Sleeper using time.Sleep.
type realSleeper struct{}

func (realSleeper) Sleep(d time.Duration) { time.Sleep(d) }

var _ websterengine.Sleeper = realSleeper{}

// digestFields converts a websterengine.Digest into the map output.Ok expects via json marshaling.
func digestFields(d websterengine.Digest) map[string]any {
	data, _ := json.Marshal(d)
	var fields map[string]any
	_ = json.Unmarshal(data, &fields)
	return fields
}

// recordBatchCmd builds the `record-batch <NN>` subcommand.
func (c *websterCLI) recordBatchCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "record-batch <NN>",
		Short: "Master's bracket call immediately after one batch's fork returns",
		Long: `record-batch <NN> refuses loud if no begin-batch record exists for this
batch (the bracket-discipline check), runs the incremental fork audit
(with a bounded settle retry against a zero-new-transcript miss), enforces
webster's own fork-audit policy, and -- once the batch's own report file
has landed -- distills and persists its digest. The envelope is the digest
verbatim (the pinned terse field set Master reads) plus any non-fatal
warnings. If the report has not landed yet, record-batch returns
{"no_report": true, "batch": "NN-<slug>"} and exits 0 -- a ladder signal,
not an error; Master re-forks once and calls record-batch again.

Example:
  lyx webster record-batch 3`,
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

			deps := websterengine.RecordDeps{
				Batches:     batches,
				State:       st,
				Config:      c.cfg,
				Engine:      c.engine,
				Geom:        c.geom,
				RefMatcher:  c.refMatcher,
				OutcomePath: websterengine.OutcomePath(c.geom.WebsterDir),
				SummaryPath: summaryparser.Path(c.geom.WebsterDir),
				Sleeper:     realSleeper{},
			}

			result, err := websterengine.RecordBatch(deps, batchNumber)
			if err != nil {
				_ = mutateLock.Release()
				mutateHeld = false
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}

			batchName := fmt.Sprintf("%02d-%s", batchNumber, st.Batches[batchNumber].Slug)

			if err := websterengine.SaveState(c.geom.WebsterDir, c.geom.ScratchDir, st); err != nil {
				_ = mutateLock.Release()
				mutateHeld = false
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}
			_ = mutateLock.Release()
			mutateHeld = false

			label := "no-report"
			if result.Digest != nil {
				label = result.Digest.Status
			}
			if _, syncErr := fabricSync(c.openFabric, c.anchorRel, fmt.Sprintf("record-batch %s %s", batchName, label)); syncErr != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, fmt.Sprintf("webster: batch %s recorded but the fabric sync failed: %v", batchName, syncErr)))
				return nil
			}

			warnings := ownerlessRunWarnings(c.geom.ScratchDir, result.Warnings)

			if result.NoReport {
				clihelp.SetExit(cmd.Context(), output.Ok(out, map[string]any{
					"no_report": true,
					"batch":     batchName,
					"warnings":  warnings,
				}))
				return nil
			}

			fields := digestFields(*result.Digest)
			fields["warnings"] = warnings
			clihelp.SetExit(cmd.Context(), output.Ok(out, fields))
			return nil
		},
	}

	return cmd
}
