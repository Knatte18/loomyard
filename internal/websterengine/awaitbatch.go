// awaitbatch.go implements AwaitBatch, a bounded wait for one batch's report file behind the
// `await-batch` verb an operator can call while a run is in flight.
// It is a pure watch on the batch's report path — no state read, no state mutation, no fabric —
// mirroring recover-batch's re-entrant long-poll idiom: each call blocks at most one wait window,
// and the caller re-calls until the report is present.

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
// Short so an agent caller's foreground call stays under Claude Code's ~2-minute auto-background
// threshold.
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
