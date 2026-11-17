// awaitbatch.go implements AwaitBatch, a bounded wait for one batch's report file, behind the `await-batch` verb.
// The wait only watches the report path: it reads and mutates no state and calls no fabric.
// Each call blocks at most one wait window, and the caller re-calls until the report is present.

package websterengine

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Knatte18/loomyard/internal/batcher"
)

// awaitTick is the interval between checks of the report path while waiting.
const awaitTick = time.Second

// DefaultAwaitWaitS is the default block of one await-batch call, in seconds, when --wait is not given.
// It is short so that an agent's foreground call stays under the ~2-minute auto-background threshold of Claude Code.
const DefaultAwaitWaitS = 30

// AwaitResult reports the outcome of one bounded wait for a batch report.
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
