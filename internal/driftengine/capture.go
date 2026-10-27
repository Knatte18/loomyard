// capture.go — the main capture and the side capture: the body of the detached capture child and the one an agent runs for a spare baseline.
//
// Both render the declared capture command through the shared runner; only the main capture holds the capture lock and writes the capture record.

package driftengine

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/lock"
)

// CaptureHaltPrefix opens every Drift-Baseline Stuck reason on a failed or refused capture.
const CaptureHaltPrefix = "drift capture halt: "

// CaptureRequest is the told input of a main or side capture.
type CaptureRequest struct {
	// Paths are the drift paths of the task's anchor root.
	Paths Paths
	// WorktreeRoot is the task worktree the command's relative dir is joined onto.
	WorktreeRoot string
	// Step is the declared capture command.
	Step CommandStep
	// Pool is the gate-slot pool the capture takes one slot of.
	Pool *gateslot.Pool
	// WaitDir is where the capture's wait record is written while it waits for a slot.
	WaitDir string
	// Skip is the scope's skipped tests, written to the skip file.
	Skip []string
	// Check is the precondition a main capture runs under its lock; its error is returned untouched.
	// It returns the commit the capture runs on, which the capture record names.
	Check func() (head string, err error)
	// Timeout bounds the command from its start; production passes the step's timeout_min.
	Timeout time.Duration
	// Now is the clock the records' times come from.
	Now func() time.Time
	// LockRetry bounds how long a main capture retries a held capture lock before it refuses.
	LockRetry time.Duration
}

// lockHeldRefusal is the refusal of a capture or a spawn that finds the capture lock held.
func lockHeldRefusal(lockPath string) error {
	return fmt.Errorf("drift: a capture already holds %s; way forward: wait for it to end (lym drift status shows it), then lym loom resume", lockPath)
}

// RunCapture runs the main capture under the capture lock and returns its final record.
// A refusal of the lock or of Check is returned with no record written.
// A command that fails, times out or writes no file under the baseline directory is a record in state failed with its reason and no error.
// An existing non-empty baseline directory is first moved to the first free numbered backup beside it, so no baseline is deleted or overwritten.
func RunCapture(ctx context.Context, req CaptureRequest) (CaptureRecord, error) {
	p := req.Paths
	if err := os.MkdirAll(filepath.Dir(p.CaptureLock), 0o755); err != nil {
		return CaptureRecord{}, fmt.Errorf("drift: create %s: %w", filepath.Dir(p.CaptureLock), err)
	}
	held, taken, err := lock.AcquireWriteLockWithin(p.CaptureLock, req.LockRetry)
	if err != nil {
		return CaptureRecord{}, err
	}
	if !taken {
		return CaptureRecord{}, lockHeldRefusal(p.CaptureLock)
	}
	defer held.Release()

	head, err := req.Check()
	if err != nil {
		return CaptureRecord{}, err
	}

	rec := CaptureRecord{State: CaptureRunning, Pid: os.Getpid(), Head: head}
	if err := WriteCapture(p, rec); err != nil {
		return CaptureRecord{}, err
	}
	if err := backUpBaseline(p.BaselineDir); err != nil {
		return failCapture(p, rec, req.Now, err)
	}
	if err := writeSkipFile(p.SkipFile, req.Skip); err != nil {
		return failCapture(p, rec, req.Now, err)
	}

	rec.Log = logPathFor(p.LogsDir, "capture", req.Now())
	outcome, err := runStep(ctx, stepRun{
		WorktreeRoot: req.WorktreeRoot,
		Step:         req.Step,
		Pool:         req.Pool,
		WaitDir:      req.WaitDir,
		Site:         "drift capture",
		BaselineDir:  p.BaselineDir,
		SkipFile:     p.SkipFile,
		LogPath:      rec.Log,
		Timeout:      req.Timeout,
		Now:          req.Now,
		OnSlot: func(at time.Time) {
			rec.SlotAcquiredAt = stamp(at)
			_ = WriteCapture(p, rec)
		},
		OnStart: func(_ int, at time.Time) {
			rec.CommandStartedAt = stamp(at)
			_ = WriteCapture(p, rec)
		},
	})
	if err != nil {
		return failCapture(p, rec, req.Now, err)
	}
	defer outcome.release()

	rec.ExitCode = outcome.ExitCode
	rec.EndedAt = stamp(req.Now())
	switch {
	case outcome.TimedOut:
		rec.State, rec.Reason = CaptureFailed, "timed out after "+timeoutMinutes(req.Timeout)+"m"
	case outcome.ExitCode != 0:
		rec.State, rec.Reason = CaptureFailed, "exit "+strconv.Itoa(outcome.ExitCode)
	default:
		wrote, err := holdsFile(p.BaselineDir)
		if err != nil {
			return failCapture(p, rec, req.Now, err)
		}
		if wrote {
			rec.State = CapturePassed
		} else {
			rec.State, rec.Reason = CaptureFailed, "exit 0 but wrote no file to "+p.BaselineDir
		}
	}
	return rec, WriteCapture(p, rec)
}

// failCapture records rec as failed because err stopped the capture, and returns both.
func failCapture(p Paths, rec CaptureRecord, now func() time.Time, cause error) (CaptureRecord, error) {
	rec.State, rec.Reason, rec.EndedAt = CaptureFailed, cause.Error(), stamp(now())
	return rec, errors.Join(cause, WriteCapture(p, rec))
}

// stamp formats t as the RFC 3339 UTC text the records hold.
func stamp(t time.Time) string {
	return t.UTC().Format(time.RFC3339)
}

// timeoutMinutes renders a timeout as minutes, with a decimal for a fraction of one.
func timeoutMinutes(timeout time.Duration) string {
	return strconv.FormatFloat(timeout.Minutes(), 'f', -1, 64)
}

// backUpBaseline moves a non-empty baseline directory to the first free numbered backup beside it and leaves an empty baseline directory.
func backUpBaseline(baselineDir string) error {
	entries, err := os.ReadDir(baselineDir)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("drift: read the baseline directory: %w", err)
	}
	if len(entries) > 0 {
		backup := nextFree(filepath.Dir(baselineDir), filepath.Base(baselineDir)+".")
		if err := os.Rename(baselineDir, backup); err != nil {
			return fmt.Errorf("drift: move the existing baseline to %s: %w", backup, err)
		}
	}
	if err := os.MkdirAll(baselineDir, 0o755); err != nil {
		return fmt.Errorf("drift: create the baseline directory: %w", err)
	}
	return nil
}

// nextFree returns the path in dir named prefix and the lowest positive number that does not exist yet.
func nextFree(dir, prefix string) string {
	for n := 1; ; n++ {
		candidate := filepath.Join(dir, prefix+strconv.Itoa(n))
		if _, err := os.Lstat(candidate); errors.Is(err, os.ErrNotExist) {
			return candidate
		}
	}
}

// holdsFile reports whether dir holds at least one file at any depth.
func holdsFile(dir string) (bool, error) {
	found := false
	err := filepath.WalkDir(dir, func(_ string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			found = true
			return fs.SkipAll
		}
		return nil
	})
	if err != nil {
		return false, fmt.Errorf("drift: scan the baseline directory: %w", err)
	}
	return found, nil
}

// SideResult is how a side capture ended.
type SideResult struct {
	// Dir is the numbered directory under the side directory that the command wrote to.
	Dir string
	// Log is the command's log file.
	Log string
	// ExitCode is the command's exit code, -1 when a signal ended it.
	ExitCode int
	// TimedOut is true when the timeout killed the command.
	TimedOut bool
}

// RunSideCapture runs the capture command into the next free numbered directory under the side directory.
// It takes no capture lock, writes no capture record and needs no clean tree or branch point, so an agent can take a second profile beside the baseline.
func RunSideCapture(ctx context.Context, req CaptureRequest) (SideResult, error) {
	p := req.Paths
	if err := os.MkdirAll(p.SideDir, 0o755); err != nil {
		return SideResult{}, fmt.Errorf("drift: create the side capture directory: %w", err)
	}
	dir := nextFree(p.SideDir, "")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return SideResult{}, fmt.Errorf("drift: create %s: %w", dir, err)
	}
	if err := writeSkipFile(p.SkipFile, req.Skip); err != nil {
		return SideResult{}, err
	}

	result := SideResult{Dir: dir, Log: logPathFor(p.LogsDir, "side-capture", req.Now())}
	outcome, err := runStep(ctx, stepRun{
		WorktreeRoot: req.WorktreeRoot,
		Step:         req.Step,
		Pool:         req.Pool,
		WaitDir:      req.WaitDir,
		Site:         "drift side capture",
		BaselineDir:  dir,
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
	result.ExitCode, result.TimedOut = outcome.ExitCode, outcome.TimedOut
	return result, nil
}
