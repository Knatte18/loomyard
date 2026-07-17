// awaitbatch.go implements AwaitBatch, the bounded long-poll Master calls
// between forking a batch's implementer and recording it. On Claude Code
// 2.1.205 the Agent-tool fork is a BACKGROUNDED agent — it returns
// immediately instead of synchronously inside Master's turn — so Master
// needs a blocking tool call to stay inside its turn until the fork's
// batch-report lands; a Master that simply ends its turn "waiting" is
// classified asking by the shuttle file contract and kills the whole run
// (found live in round fable-r1). AwaitBatch is that call: a pure, bounded
// watch on the batch's report path — no state read, no state mutation, no
// weft — mirroring recover-batch's re-entrant long-poll idiom (each call
// blocks at most one wait window; the caller re-calls until the report is
// present or its fork has finished without one).

package websterengine

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Knatte18/loomyard/internal/builderengine"
)

// awaitTick is the fixed re-check cadence AwaitBatch polls the report path
// on: frequent enough that a fork's just-written report is seen within a
// second, cheap enough that a full poll_wait_s window costs nothing but
// stat calls.
const awaitTick = time.Second

// AwaitResult is what one AwaitBatch call hands back to its caller
// (internal/webstercli's await-batch verb): BatchName is the batch's
// "NN-<batch-slug>" identifier, ReportPresent reports whether the batch's
// report file existed by the time the call returned (true the instant it
// appears — AwaitBatch never sleeps out the rest of its window once the
// report is on disk), and ElapsedS is how many seconds this call actually
// blocked.
type AwaitResult struct {
	BatchName     string
	ReportPresent bool
	ElapsedS      int
}

// AwaitBatch blocks until batchNumber's batch-report file exists in
// reportsDir or wait elapses, re-checking on a fixed one-second tick via
// clk. It reads and mutates NOTHING but the report path's existence — no
// state.json, no lease, no weft — so a zombie or manual caller can never
// corrupt a run through it, and the caller (Master, per the master
// template) simply re-calls it while its fork is still running. A missing
// batch number is an error (the same findBatch refusal every other verb
// applies); a report that never appears within wait is NOT an error — it
// returns ReportPresent: false and the caller decides (re-call, or
// record-batch's no_report ladder once the fork has finished).
func AwaitBatch(plan *builderengine.Plan, reportsDir string, batchNumber int, wait time.Duration, clk Clock) (*AwaitResult, error) {
	batch, err := findBatch(plan, batchNumber)
	if err != nil {
		return nil, err
	}

	batchName := fmt.Sprintf("%02d-%s", batch.Number, batch.Slug)
	reportPath := filepath.Join(reportsDir, builderengine.BatchReportFileName(batch.Number, batch.Slug))

	start := clk.Now()
	for {
		if _, statErr := os.Stat(reportPath); statErr == nil {
			return &AwaitResult{
				BatchName:     batchName,
				ReportPresent: true,
				ElapsedS:      int(clk.Now().Sub(start).Seconds()),
			}, nil
		} else if !os.IsNotExist(statErr) {
			return nil, fmt.Errorf("webster: stat batch report %s: %w", reportPath, statErr)
		}

		elapsed := clk.Now().Sub(start)
		if elapsed >= wait {
			return &AwaitResult{
				BatchName:     batchName,
				ReportPresent: false,
				ElapsedS:      int(elapsed.Seconds()),
			}, nil
		}
		clk.Sleep(awaitTick)
	}
}
