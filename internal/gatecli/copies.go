// copies.go runs the concurrent copies of one tier that `lym gate test --copies` asks for.
// It sizes the slot lease a copies call takes, runs each copy as a tier run of its own, and merges their results.

package gatecli

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"slices"
	"sync"
	"time"

	"github.com/Knatte18/loomyard/internal/gateslot"
)

// copyLabel names copy number index, counted from 1, of the run labelled run.
func copyLabel(run string, index int) string {
	return fmt.Sprintf("%s copy %d", run, index)
}

// copyLease returns the slot count a call of asked copies of a tier holding tierSlots slots takes of the pool.
// It also returns how many copies the pool's poolSlots hold.
// A tier with a slot count of 0 needs every slot per copy, so its lease asks every slot and one copy runs.
func copyLease(asked, tierSlots, poolSlots int) (count, ran int) {
	if tierSlots == 0 {
		return 0, 1
	}
	count = asked * tierSlots
	return count, max(1, min(count, poolSlots)/tierSlots)
}

// copiesClampLine is the line a copies call prints when the pool's slots hold fewer copies of tier than were asked for.
func copiesClampLine(asked, ran int, tier string) string {
	return fmt.Sprintf("gate test: --copies asked for %d copies of %s and %d ran, as the pool's slots hold no more", asked, tier, ran)
}

// greenForCall returns the green record a call reads and writes.
// A copies call runs its copies live, so no green record carries them and it gets none.
func greenForCall(green *greenRun, request testRequest) *greenRun {
	if request.copies > 0 {
		return nil
	}
	return green
}

// lockedWriter serialises the writes of the copies that share one stderr.
type lockedWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (l *lockedWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.w.Write(p)
}

// runTierCopies runs request.copiesRun copies of run side by side, each a tier run of its own, and waits on all of them.
// Each copy runs under the tier's own counter, timeout, CPU watchdog and memory guard.
// A tier that asks for a prebuilt `lym` builds it once for all the copies, when a test they run may use it.
// A copy's stdout is kept in its log only, so the copies never interleave their output.
func runTierCopies(ctx context.Context, run tierRun, env []string, request testRequest) tierResult {
	if len(run.invocations) > 0 && needsPrebuiltLym(run, request.root) {
		prebuildCtx, cancel := withTimeoutMin(ctx, time.Duration(run.tier.TimeoutMin)*request.timeoutUnit, elapsedSinceNow(), time.After)
		bin, cleanup, err := prebuildLym(prebuildCtx, request.goBinary, request.root, run.tier.Parallel, request.prebuiltStore, request.pipeWaitDelay, nil)
		cancel()
		if err != nil {
			if ctx.Err() != nil {
				now := time.Now()
				return tierResult{record: gateslot.RunRecord{Tier: run.number, ScopeForm: run.scopeForm, Start: now, End: now, ExitCode: 1, Outcome: "failed"}}
			}
			return tierResult{refusal: fmt.Sprintf("gate test: cannot build lym for the tier: %v; way forward: fix the build, then re-run the same command", err)}
		}
		defer cleanup()
		if bin != "" {
			env = append(slices.Clone(env), gateslot.PrebuiltLymEnv+"="+bin)
		}
	}
	run.tier.PrebuiltLym = false

	copyRequest := request
	copyRequest.copiesRun = 0
	if request.stderr != nil {
		copyRequest.stderr = &lockedWriter{w: request.stderr}
	}
	results := make([]tierResult, request.copiesRun)
	var running sync.WaitGroup
	for i := range results {
		running.Add(1)
		go func() {
			defer running.Done()
			results[i] = runTierCommands(ctx, io.Discard, run, env, copyRequest)
		}()
	}
	running.Wait()
	return mergeCopies(results)
}

// mergeCopies folds the results of the copies of one tier run into the tier run's result.
// The merged record totals the copies' counts and launches and takes the first failing exit code.
// The merged result keeps the copies' own results, so the report can name each copy.
// It is a slice-pressure stop when one copy was and every other copy passed or was one too.
func mergeCopies(results []tierResult) tierResult {
	merged := tierResult{record: results[0].record, copies: results}
	merged.record.Launches = slices.Clone(merged.record.Launches)
	merged.record.Failures = slices.Clone(merged.record.Failures)
	var log bytes.Buffer
	pressured, failedOtherwise := false, false
	for i, result := range results {
		if result.slicePressure {
			pressured = true
		} else if result.record.ExitCode != 0 {
			failedOtherwise = true
		}
		merged.slicePressure = pressured && !failedOtherwise
		fmt.Fprintf(&log, "copy %d:\n", i+1)
		log.Write(result.log)
		if merged.refusal == "" {
			merged.refusal = result.refusal
		}
		if merged.trimpathNote == "" {
			merged.trimpathNote = result.trimpathNote
		}
		merged.processCapped = merged.processCapped || result.processCapped
		merged.memoryStopped = merged.memoryStopped || result.memoryStopped
		merged.failures = append(merged.failures, result.failures...)
		merged.causes = append(merged.causes, result.causes...)
		merged.unparsed = append(merged.unparsed, result.unparsed...)
		if i == 0 {
			continue
		}
		record, other := &merged.record, result.record
		if other.Start.Before(record.Start) {
			record.Start = other.Start
		}
		if other.End.After(record.End) {
			record.End = other.End
		}
		record.WallSec = max(record.WallSec, other.WallSec)
		record.Discovered += other.Discovered
		record.Passed += other.Passed
		record.Failed += other.Failed
		record.Skipped += other.Skipped
		record.TimedOut = record.TimedOut || other.TimedOut
		record.Launches = append(record.Launches, other.Launches...)
		record.Failures = append(record.Failures, other.Failures...)
		if record.ExitCode == 0 {
			record.ExitCode = other.ExitCode
		}
	}
	merged.log = log.Bytes()
	merged.record.Outcome = "passed"
	if merged.record.ExitCode != 0 {
		merged.record.Outcome = "failed"
	}
	return merged
}

// printStartedLines prints the started line of the run that ended as result: one line, or one per copy.
func (e *execution) printStartedLines(run tierRun, result tierResult) {
	if len(result.copies) == 0 {
		printLine(e.request.stderr, gateslot.FormatStartedLine(runLabel(run), len(result.record.Launches)))
		return
	}
	for i, one := range result.copies {
		printLine(e.request.stderr, gateslot.FormatStartedLine(copyLabel(runLabel(run), i+1), len(one.record.Launches)))
	}
}

// addFailedRuns adds the failed run of the tier run that ended red as result to the call's failed runs, one per red copy.
// The stray-file failure belongs to the copies together, so a copies run lists it as one more failed run.
func (e *execution) addFailedRuns(run tierRun, result tierResult, logPath string) {
	if len(result.copies) == 0 {
		e.failed = append(e.failed, tierFailedRun(run, result, logPath, e.request.root))
		return
	}
	for i, one := range result.copies {
		if one.record.ExitCode == 0 {
			continue
		}
		failed := tierFailedRun(run, one, logPath, e.request.root)
		failed.label = copyLabel(runLabel(run), i+1)
		e.failed = append(e.failed, failed)
	}
	if result.strayLine != "" {
		e.failed = append(e.failed, failedRun{label: runLabel(run), outcome: "exit 1", log: logPath, causes: []string{result.strayLine}})
	}
}
