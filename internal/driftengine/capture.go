// capture.go holds the main capture and the current capture.
// The main capture is the body of the detached capture child;
// the current capture runs into a scratch directory for a check.
//
// Both render the declared capture command through the shared runner and hold the capture lock while it runs;
// only the main capture writes the capture record.

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

// CaptureRequest is the input a main or current capture runs on, supplied by its caller.
type CaptureRequest struct {
	// Paths are the drift paths of the task's anchor root.
	Paths Paths
	// WorktreeDir is the absolute worktree root the command's relative dir is joined onto.
	WorktreeDir string
	// AnchorDir is the absolute anchored directory a command with no dir runs in.
	AnchorDir string
	// Step is the declared capture command.
	Step CommandStep
	// Pool is the gate-slot pool the capture takes one slot of.
	Pool *gateslot.Pool
	// WaitDir is where the capture's wait record is written while it waits for a slot.
	WaitDir string
	// InheritedSlot is the lock path of the slot a gate holds around the caller, empty for a caller outside a gate.
	// A capture given a slot that the pool reports as held runs in it and acquires none;
	// the wiring reads it from the process environment, since the engine never does.
	InheritedSlot string
	// Sleep is the wait of a current capture for the capture lock; nil sleeps for real.
	Sleep func(context.Context, time.Duration) error
	// Check is the precondition a main capture runs under its lock; its error is returned untouched.
	// It returns the commit the capture runs on, which the capture record names.
	Check func() (head string, err error)
	// Timeout bounds the command from its start; production passes the step's timeout_min.
	Timeout time.Duration
	// Now is the clock the records' times come from.
	Now func() time.Time
	// LockRetry bounds how long a main capture retries a held capture lock before it refuses.
	LockRetry time.Duration
	// OnEnd, when non-nil, receives the final record of a main capture whose command ran.
	// It is called once the record is written and while the capture lock is still held.
	OnEnd func(rec CaptureRecord)
}

// lockHeldRefusal is the refusal of a capture or a spawn that finds the capture lock held.
func lockHeldRefusal(lockPath string) error {
	return fmt.Errorf("drift: a capture already holds %s; way forward: wait for it to end (lym drift status shows it), then lym loom resume", lockPath)
}

// RunCapture runs the main capture under the capture lock and returns its final record.
// It takes its gate slot before the lock, so a capture waiting for a slot holds no lock and writes no record,
// and a lock holder always holds its slot.
// A refusal of the lock or of Check is returned with no record written.
// A command still holding the {skip_file} placeholder is refused before the slot, the lock and the baseline backup.
// No record is written for it.
// A command that fails, times out or writes no file under the baseline directory yields a failed record.
// That record carries no error;
// its reason names the directory the command ran in and the command's last output lines, or that it printed nothing.
// An existing non-empty baseline directory is first moved to a numbered backup beside it.
// The backup is the first number whose directory and metadata directory are both free.
// Its metadata directory moves with it, so no baseline is deleted or overwritten.
// A leftover metadata directory beside an empty baseline directory is removed.
// Once the command ran, the final record is handed to OnEnd under the capture lock.
func RunCapture(ctx context.Context, req CaptureRequest) (CaptureRecord, error) {
	if err := refuseSkipFilePlaceholder(siteCapture, stepCapture, req.Step.Command); err != nil {
		return CaptureRecord{}, err
	}
	p := req.Paths
	run := stepRun{
		WorktreeDir:   req.WorktreeDir,
		AnchorDir:     req.AnchorDir,
		Name:          stepCapture,
		Step:          req.Step,
		Pool:          req.Pool,
		WaitDir:       req.WaitDir,
		InheritedSlot: req.InheritedSlot,
		Site:          siteCapture,
		BaselineDir:   p.BaselineDir,
		MetaDir:       p.BaselineMetaDir,
		Timeout:       req.Timeout,
		Now:           req.Now,
	}
	slot, err := acquireSlot(ctx, run)
	if err != nil {
		return CaptureRecord{}, err
	}
	defer slot.release()

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

	rec.Log = logPathFor(p.LogsDir, "capture", req.Now())
	run.Slot, run.LogPath = slot, rec.Log
	run.OnSlot = func(at time.Time) {
		rec.SlotAcquiredAt = stamp(at)
		_ = WriteCapture(p, rec)
	}
	run.OnStart = func(_ int, at time.Time) {
		rec.CommandStartedAt = stamp(at)
		_ = WriteCapture(p, rec)
	}
	outcome, err := runStep(ctx, run)
	if err != nil {
		return failCapture(p, rec, req.Now, err)
	}

	rec.ExitCode = outcome.ExitCode
	rec.EndedAt = stamp(req.Now())
	report := func() string { return commandReport(outcome.Dir, rec.Log) }
	switch {
	case outcome.TimedOut:
		rec.State, rec.Reason = CaptureFailed, "timed out after "+timeoutMinutes(req.Timeout)+"m"+report()
	case outcome.ExitCode != 0:
		rec.State, rec.Reason = CaptureFailed, "exit "+strconv.Itoa(outcome.ExitCode)+report()
	default:
		wrote, err := holdsFile(p.BaselineDir)
		if err != nil {
			return failCapture(p, rec, req.Now, err)
		}
		if wrote {
			rec.State = CapturePassed
		} else {
			rec.State, rec.Reason, rec.WroteNoFile = CaptureFailed, noFileReason(p.BaselineDir)+report(), true
		}
	}
	if err := WriteCapture(p, rec); err != nil {
		return rec, err
	}
	if req.OnEnd != nil {
		req.OnEnd(rec)
	}
	return rec, nil
}

// FailedCaptureError is the error of a main capture whose record ended failed.
// It carries the record's reason and log, and it names the ways forward.
// The ways forward apply because HEAD is on the branch point, where every main capture runs.
func FailedCaptureError(rec CaptureRecord) error {
	return fmt.Errorf("drift: the baseline capture ended %s (%s)%s; way forward: %s", rec.State, rec.Reason, logClause(rec.Log), awaitWaysForward(true))
}

// noFileReason is the reason of a capture that exited 0 and wrote no file under baselineDir.
func noFileReason(baselineDir string) string {
	return "exit 0 but wrote no file to " + baselineDir
}

// failCapture records rec as failed because cause stopped the capture, and returns both.
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

// backUpBaseline moves a non-empty baseline directory to a numbered backup beside it.
// The backup is the first number whose directory and metadata directory are both free.
// The baseline's metadata directory, when it exists, moves to the backup's metadata directory;
// when that move fails, the baseline directory moves back so the pair stays together.
// When the move back fails too, both errors are returned and the pair stays split.
// It leaves an empty baseline directory in place.
// It removes the metadata directory beside an empty baseline, which describes no baseline.
func backUpBaseline(baselineDir string) error {
	entries, err := os.ReadDir(baselineDir)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("drift: read the baseline directory: %w", err)
	}
	if len(entries) == 0 {
		if err := os.RemoveAll(metaDirFor(baselineDir)); err != nil {
			return fmt.Errorf("drift: remove the metadata of the empty baseline: %w", err)
		}
	}
	if len(entries) > 0 {
		backup := nextFreeBackup(baselineDir)
		if err := os.Rename(baselineDir, backup); err != nil {
			return fmt.Errorf("drift: move the existing baseline to %s: %w", backup, err)
		}
		err := os.Rename(metaDirFor(baselineDir), metaDirFor(backup))
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			// The baseline moves back so the pair stays together:
			// an absent baseline would make the next capture delete the metadata left behind.
			moveErr := fmt.Errorf("drift: move the existing baseline's metadata to %s: %w", metaDirFor(backup), err)
			if rollbackErr := os.Rename(backup, baselineDir); rollbackErr != nil {
				return errors.Join(moveErr, fmt.Errorf("drift: move the backup %s back to %s: %w", backup, baselineDir, rollbackErr))
			}
			return moveErr
		}
	}
	if err := os.MkdirAll(baselineDir, 0o755); err != nil {
		return fmt.Errorf("drift: create the baseline directory: %w", err)
	}
	return nil
}

// nextFreeBackup returns the backup path of baselineDir with the lowest positive number.
// Neither the backup nor its metadata directory may exist yet.
func nextFreeBackup(baselineDir string) string {
	exists := func(path string) bool {
		_, err := os.Lstat(path)
		return !errors.Is(err, os.ErrNotExist)
	}
	for n := 1; ; n++ {
		candidate := baselineDir + "." + strconv.Itoa(n)
		if !exists(candidate) && !exists(metaDirFor(candidate)) {
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

// CurrentCapture is how a current capture ended.
type CurrentCapture struct {
	// Dir is the fresh directory under the current-capture directory that the command wrote to.
	Dir string
	// MetaDir is the sibling of Dir the command's {meta_dir} renders to, which no check compares.
	MetaDir string
	// Log is the command's log file, a sibling of Dir so the log never counts as a captured file.
	Log string
	// Passed is true when the command exited 0 within its timeout and wrote at least one file.
	Passed bool
	// Reason says why the capture did not pass: where its command ran, what it printed last, or the refusal of its step.
	// It is empty when the capture passed.
	Reason string
}

// CaptureCurrent runs the declared capture command into a fresh directory under the current-capture directory,
// so a check can diff the current code against the baseline.
// It takes its gate slot first, then waits for the capture lock without a bound of its own:
// a lock holder already holds its slot, and this capture holds the lock while the command runs.
// It writes no capture record and never touches the baseline directory or its backups.
// A step refused before its command starts is a result that did not pass.
// That covers a dir that is no directory and a command holding the {skip_file} placeholder.
// So is a command that fails, times out or writes no file.
// An error is an I/O failure.
func CaptureCurrent(ctx context.Context, req CaptureRequest) (CurrentCapture, error) {
	p := req.Paths
	dir, err := newCurrentDir(p.CurrentDir, req.Now())
	if err != nil {
		return CurrentCapture{}, err
	}
	result := CurrentCapture{Dir: dir, MetaDir: metaDirFor(dir), Log: dir + ".log"}
	if err := refuseSkipFilePlaceholder(siteCurrentCapture, stepCapture, req.Step.Command); err != nil {
		result.Reason = err.Error()
		return result, nil
	}
	run := stepRun{
		WorktreeDir:   req.WorktreeDir,
		AnchorDir:     req.AnchorDir,
		Name:          stepCapture,
		Step:          req.Step,
		Pool:          req.Pool,
		WaitDir:       req.WaitDir,
		InheritedSlot: req.InheritedSlot,
		Site:          siteCurrentCapture,
		BaselineDir:   dir,
		MetaDir:       result.MetaDir,
		LogPath:       result.Log,
		Timeout:       req.Timeout,
		Now:           req.Now,
		OnSlot:        func(time.Time) {},
		OnStart:       func(int, time.Time) {},
	}
	slot, err := acquireSlot(ctx, run)
	if err != nil {
		return result, err
	}
	defer slot.release()
	held, err := acquireCaptureLockWaiting(ctx, p.CaptureLock, req.Sleep)
	if err != nil {
		return result, err
	}
	defer held.Release()

	run.Slot = slot
	outcome, err := runStep(ctx, run)
	var refusal *stepRefusal
	if errors.As(err, &refusal) {
		result.Reason = refusal.Error()
		return result, nil
	}
	if err != nil {
		return result, err
	}
	report := commandReport(outcome.Dir, result.Log)
	switch {
	case outcome.TimedOut:
		result.Reason = "timed out after " + timeoutMinutes(req.Timeout) + "m" + report
	case outcome.ExitCode != 0:
		result.Reason = "exit " + strconv.Itoa(outcome.ExitCode) + report
	default:
		wrote, err := holdsFile(dir)
		if err != nil {
			return result, err
		}
		if wrote {
			result.Passed = true
		} else {
			result.Reason = noFileReason(dir) + report
		}
	}
	return result, nil
}

// newCurrentDir creates a fresh directory under currentDir, named by the UTC stamp of now and the pid.
// A concurrent capture of this process that took the same name gets a counter suffix.
func newCurrentDir(currentDir string, now time.Time) (string, error) {
	if err := os.MkdirAll(currentDir, 0o755); err != nil {
		return "", fmt.Errorf("drift: create %s: %w", currentDir, err)
	}
	name := now.UTC().Format(stampLayout) + "-" + strconv.Itoa(os.Getpid())
	for n := 1; ; n++ {
		candidate := filepath.Join(currentDir, name)
		if n > 1 {
			candidate += "-" + strconv.Itoa(n)
		}
		err := os.Mkdir(candidate, 0o755)
		if err == nil {
			return candidate, nil
		}
		if !errors.Is(err, fs.ErrExist) {
			return "", fmt.Errorf("drift: create %s: %w", candidate, err)
		}
	}
}

// acquireCaptureLockWaiting takes the capture lock, and it waits while another holder keeps it.
// Each wait goes through sleep, starting at one second and doubling up to thirty.
// A nil sleep waits for real.
// It has no bound of its own and returns when it holds the lock or ctx ends.
func acquireCaptureLockWaiting(ctx context.Context, lockPath string, sleep func(context.Context, time.Duration) error) (*lock.FileLock, error) {
	if sleep == nil {
		sleep = sleepFor
	}
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		return nil, fmt.Errorf("drift: create %s: %w", filepath.Dir(lockPath), err)
	}
	wait := captureWaitFloor
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		held, taken, err := lock.TryAcquireWriteLock(lockPath)
		if err != nil {
			return nil, err
		}
		if taken {
			return held, nil
		}
		if err := sleep(ctx, wait); err != nil {
			return nil, err
		}
		wait = min(wait*2, captureWaitCeiling)
	}
}

// sleepFor waits d, or returns the context's error once ctx ends first.
func sleepFor(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
