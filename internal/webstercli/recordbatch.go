// recordbatch.go implements the `record-batch` webster verb: Master's own
// bracket call immediately after a batch's fork returns. It runs
// websterengine.RecordBatch under the state-mutation lease (load, mutate,
// save, release) with a real, time.Sleep-backed Sleeper for the incremental
// fork audit's bounded settle retry, then performs the second of webster's
// four weft-commit points (see the discussion's weft-ownership decision):
// state.json and the batch report, once RecordBatch either lands a terminal
// digest or advances transcript attribution on a no_report retry -- both
// mutate deps.State, so both are durable before Master's next tool call.
package webstercli

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/Knatte18/loomyard/internal/builderengine"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/websterengine"
	"github.com/spf13/cobra"
)

// realSleeper is webstercli's own production websterengine.Sleeper: a
// genuine time.Sleep, mirroring buildercli's own pollRealClock pattern
// (internal/buildercli/poll.go) of a package-local, production-only clock
// seam satisfied structurally rather than by declared type identity.
type realSleeper struct{}

func (realSleeper) Sleep(d time.Duration) { time.Sleep(d) }

var _ websterengine.Sleeper = realSleeper{}

// digestFields converts a Digest into the map output.Ok expects: Digest's
// own json tags already spell the pinned terse field set the Master reads,
// so a marshal/unmarshal round trip through map[string]any reuses them
// exactly rather than re-listing every field by hand here. Mirrors
// buildercli's own digestFields, minus the running-snapshot field-stripping
// buildercli's poll needs: RecordBatch only ever returns a terminal digest
// (never a running one -- that classification belongs to recover-batch),
// so files_changed/dirty are always populated here.
func digestFields(d builderengine.Digest) map[string]any {
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

			plan, err := builderengine.ParsePlan(c.planDir)
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}

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

			deps := websterengine.RecordDeps{
				Plan:         plan,
				State:        st,
				Config:       c.cfg,
				Engine:       c.engine,
				Layout:       c.layout,
				WorktreeRoot: c.layout.Cwd,
				ReportsDir:   c.reportsDir,
				OutcomePath:  websterengine.OutcomePath(c.websterDir),
				SummaryPath:  websterengine.SummaryPath(c.websterDir),
				Sleeper:      realSleeper{},
			}

			result, err := websterengine.RecordBatch(deps, batchNumber)
			if err != nil {
				// ErrNoBeginRecord and every audit-policy violation are
				// returned before RecordBatch ever mutates deps.State, so
				// there is nothing to persist on this path; both are loud
				// errors, never a distinct operational envelope.
				_ = mutateLock.Release()
				mutateHeld = false
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}

			// RecordBatch's own bracket-discipline check (ErrNoBeginRecord)
			// already guarantees st.Batches[batchNumber] is present on this
			// success path.
			batchName := fmt.Sprintf("%02d-%s", batchNumber, st.Batches[batchNumber].Slug)

			if err := websterengine.SaveState(c.websterDir, st); err != nil {
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
			if _, weftErr := weftCommit(c.layout, fmt.Sprintf("record-batch %s %s", batchName, label)); weftErr != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, fmt.Sprintf("webster: batch %s recorded but the weft sync failed: %v", batchName, weftErr)))
				return nil
			}

			warnings := ownerlessRunWarnings(c.websterDir, result.Warnings)

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
