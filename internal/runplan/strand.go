// strand.go holds the strand seams a batch's attempt spawns and stops through, and the bounded wait for a batch report.

package runplan

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/Knatte18/loomyard/internal/batcher"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// Starter is the seam a batch's attempt or recovery strand spawns through.
// Start blocks until the spawned provider is past its startup gates, which shuttle guarantees.
// It returns an error when the provider never becomes ready, and it never waits for the run to finish.
// A *shuttleengine.Runner satisfies it.
type Starter interface {
	Start(shuttleengine.Spec) (*shuttleengine.Run, error)
}

// StrandStopper is the seam a leftover strand is stopped through, by guid.
// StopStrand records the stop on the strand's run.
// It ends a live strand, logging that kill at Warn, and does nothing for a strand that is not live.
type StrandStopper interface {
	StopStrand(guid string) error
}

// RecoveryStrandRemoveError is a recovery strand that could not be removed, with its guid and the cause.
type RecoveryStrandRemoveError struct {
	GUID string
	Err  error
}

func (e *RecoveryStrandRemoveError) Error() string {
	return fmt.Sprintf("could not remove recovery strand %s: %v", e.GUID, e.Err)
}

func (e *RecoveryStrandRemoveError) Unwrap() error { return e.Err }

// StopRecoveryStrands stops every recovery strand the batch records name, in batch-number order.
// Only a recovery batch with a StrandGUID has one.
// The first failure returns a *RecoveryStrandRemoveError naming the strand guid.
func StopRecoveryStrands(stopper StrandStopper, batches map[int]*BatchState) error {
	numbers := make([]int, 0, len(batches))
	for number := range batches {
		numbers = append(numbers, number)
	}
	sort.Ints(numbers)
	for _, number := range numbers {
		bs := batches[number]
		if bs == nil || bs.Kind != "recovery" || bs.StrandGUID == "" {
			continue
		}
		if err := stopper.StopStrand(bs.StrandGUID); err != nil {
			return &RecoveryStrandRemoveError{GUID: bs.StrandGUID, Err: err}
		}
	}
	return nil
}

// Clock abstracts time.Now and time.Sleep, so a bounded wait runs instantly under test.
// Any type with these two methods satisfies it, so a Clock goes straight into a polling loop without an adapter.
type Clock interface {
	Now() time.Time
	Sleep(time.Duration)
}

// awaitTick is the interval between checks of the report path while waiting.
const awaitTick = time.Second

// DefaultAwaitWaitS is the default block of one await call, in seconds.
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
// An unknown batch refuses with findWayForward, the caller's way to list the run's batches.
func AwaitBatch(batches []batcher.Batch, reportsDir string, batchNumber int, wait time.Duration, clk Clock, findWayForward string) (*AwaitResult, error) {
	batch, err := FindBatch(batches, batchNumber, findWayForward)
	if err != nil {
		return nil, err
	}
	number, slug := BatchIdentity(batch)

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
			return nil, fmt.Errorf("runplan: stat batch report %s: %w", reportPath, statErr)
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
