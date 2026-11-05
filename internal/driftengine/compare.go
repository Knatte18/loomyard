// compare.go — the compare run: the mechanical comparison Drift-Verify and `lym drift check` run against the baseline directory under `diff: command`.
//
// RunCompare shares the capture's runner and writes no record.

package driftengine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/lock"
)

// CompareRequest is the told input of a compare run.
type CompareRequest struct {
	// Paths are the drift paths of the task's anchor root.
	Paths Paths
	// WorktreeDir is the absolute worktree root the command's relative dir is joined onto.
	WorktreeDir string
	// AnchorDir is the absolute anchored directory a command with no dir runs in.
	AnchorDir string
	// Step is the declared compare command.
	Step CommandStep
	// Pool is the gate-slot pool the compare takes one slot of.
	Pool *gateslot.Pool
	// WaitDir is where the compare's wait record is written while it waits for a slot.
	WaitDir string
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
	// Reason says why a compare did not pass, where its command ran and what it printed last, and names its log; empty when it passed.
	Reason string
}

// RunCompare runs the declared compare command against the baseline directory and returns how it ended.
// It takes the compare marker, an OS lock, before it waits for its gate slot and holds it until it ends, so a compare queued for a slot reads as activity and one a dead compare left reads as none.
// A marker still held once LockRetry runs out is refused.
// A command still holding the retired {skip_file} placeholder is refused first, before the marker and the slot.
// A non-zero exit and a timeout are results that did not pass, not errors.
func RunCompare(ctx context.Context, req CompareRequest) (CompareResult, error) {
	if err := refuseSkipFilePlaceholder(siteCompare, stepCompare, req.Step.Command); err != nil {
		return CompareResult{}, err
	}
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

	result := CompareResult{Log: logPathFor(p.LogsDir, "compare", req.Now())}
	run := stepRun{
		WorktreeDir: req.WorktreeDir,
		AnchorDir:   req.AnchorDir,
		Name:        stepCompare,
		Step:        req.Step,
		Pool:        req.Pool,
		WaitDir:     req.WaitDir,
		Site:        siteCompare,
		BaselineDir: p.BaselineDir,
		LogPath:     result.Log,
		Timeout:     req.Timeout,
		Now:         req.Now,
		OnSlot:      func(time.Time) {},
		OnStart:     func(int, time.Time) {},
	}
	slot, err := acquireSlot(ctx, run)
	if err != nil {
		return result, err
	}
	defer slot.release()
	run.Slot = slot
	outcome, err := runStep(ctx, run)
	if err != nil {
		return result, err
	}

	result.ExitCode = outcome.ExitCode
	switch {
	case outcome.TimedOut:
		result.Reason = "timed out after " + timeoutMinutes(req.Timeout) + "m" + commandReport(outcome.Dir, result.Log) + "; log " + result.Log
	case outcome.ExitCode != 0:
		result.Reason = "exit " + strconv.Itoa(outcome.ExitCode) + commandReport(outcome.Dir, result.Log) + "; log " + result.Log
	default:
		result.Passed = true
	}
	return result, nil
}
