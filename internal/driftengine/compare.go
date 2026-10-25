// compare.go — the compare run and the skip list it is told: the mechanical comparison Drift-Verify runs itself and the one an agent asks for.
//
// RunCompare shares the capture's runner and writes no record; counting a run against the agent session's budget is the caller's.

package driftengine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"time"

	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/lock"
)

// CompareRequest is the told input of a compare run.
type CompareRequest struct {
	// Paths are the drift paths of the task's anchor root.
	Paths Paths
	// WorktreeRoot is the task worktree the command's relative dir is joined onto.
	WorktreeRoot string
	// Step is the declared compare command.
	Step CommandStep
	// Pool is the gate-slot pool the compare takes one slot of.
	Pool *gateslot.Pool
	// WaitDir is where the compare's wait record is written while it waits for a slot.
	WaitDir string
	// Skip is the tests the compare leaves out, written to the skip file.
	Skip []string
	// Timeout bounds the command from its start; production passes the step's timeout_min.
	Timeout time.Duration
	// Now is the clock the log's name comes from.
	Now func() time.Time
	// LockRetry bounds how long a compare retries a compare marker a reader's probe holds before it refuses.
	// It must be positive: a zero bound refuses even a free marker.
	LockRetry time.Duration
}

// CompareResult is how a compare run ended.
type CompareResult struct {
	// ExitCode is the command's exit code, -1 when a signal ended it.
	ExitCode int
	// Log is the command's log file.
	Log string
	// Passed is true when the command exited 0 within its timeout.
	Passed bool
	// Reason says why a compare did not pass and names its log; empty when it passed.
	Reason string
}

// RunCompare runs the declared compare command against the baseline directory and returns how it ended.
// It takes the compare marker, an OS lock, before it waits for its gate slot and holds it until it ends, so a compare queued for a slot reads as activity and one a dead compare left reads as none.
// A marker still held once LockRetry runs out is refused.
// A non-zero exit and a timeout are results that did not pass, not errors.
func RunCompare(ctx context.Context, req CompareRequest) (CompareResult, error) {
	p := req.Paths
	if err := os.MkdirAll(filepath.Dir(p.CompareMarker), 0o755); err != nil {
		return CompareResult{}, fmt.Errorf("drift: create %s: %w", filepath.Dir(p.CompareMarker), err)
	}
	marker, taken, err := lock.AcquireWriteLockWithin(p.CompareMarker, req.LockRetry)
	if err != nil {
		return CompareResult{}, err
	}
	if !taken {
		return CompareResult{}, fmt.Errorf("drift: a compare is already running; way forward: wait for it, then run it again")
	}
	defer marker.Release()

	if err := writeSkipFile(p.SkipFile, req.Skip); err != nil {
		return CompareResult{}, err
	}
	result := CompareResult{Log: logPathFor(p.LogsDir, "compare", req.Now())}
	outcome, err := runStep(ctx, stepRun{
		WorktreeRoot: req.WorktreeRoot,
		Step:         req.Step,
		Pool:         req.Pool,
		WaitDir:      req.WaitDir,
		Site:         "drift compare",
		BaselineDir:  p.BaselineDir,
		SkipFile:     p.SkipFile,
		LogPath:      result.Log,
		Timeout:      req.Timeout,
		Now:          req.Now,
		OnSlot:       func(time.Time) {},
		OnStart:      func(int, time.Time) {},
	})
	if err != nil {
		return result, err
	}
	outcome.release()

	result.ExitCode = outcome.ExitCode
	switch {
	case outcome.TimedOut:
		result.Reason = "timed out after " + timeoutMinutes(req.Timeout) + "m; log " + result.Log
	case outcome.ExitCode != 0:
		result.Reason = "exit " + strconv.Itoa(outcome.ExitCode) + "; log " + result.Log
	default:
		result.Passed = true
	}
	return result, nil
}

// SkipList returns the tests a compare leaves out: the scope's skipped tests, then every accepted test, without duplicates.
// A withdrawn test is off the accepted list, so it is run again.
func SkipList(scope ScopeRecord) []string {
	var skip []string
	add := func(test string) {
		if !slices.Contains(skip, test) {
			skip = append(skip, test)
		}
	}
	for _, test := range scope.Skip {
		add(test)
	}
	for _, acceptance := range scope.Accepted {
		add(acceptance.Test)
	}
	return skip
}
