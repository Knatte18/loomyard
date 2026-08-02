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

	"github.com/Knatte18/loomyard/internal/batcher"
)

// awaitTick is the fixed re-check cadence AwaitBatch polls the report path on.
const awaitTick = time.Second

// DefaultAwaitWaitS is await-batch's default per-call block when --wait is not given.
// Deliberately SHORT to keep Master's foreground turn alive (Claude Code backgrounds commands after ~2 minutes).
const DefaultAwaitWaitS = 30

// AwaitResult is what one AwaitBatch call returns to its caller.
type AwaitResult struct {
	BatchName     string
	ReportPresent bool
	ElapsedS      int
}

// AwaitBatch blocks until batchNumber's batch-report file exists in reportsDir or wait elapses.
// It reads and mutates nothing but the report path's existence.
func AwaitBatch(batches []batcher.Batch, reportsDir string, batchNumber int, wait time.Duration, clk Clock) (*AwaitResult, error) {
	batch, err := findBatch(batches, batchNumber)
	if err != nil {
		return nil, err
	}
	number, slug := batchIdentity(batch)

	batchName := fmt.Sprintf("%02d-%s", number, slug)
	reportPath := filepath.Join(reportsDir, ReportFileName(number, slug))

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
