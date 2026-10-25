// spawn.go — the detached capture child's spawn, and the lock probe every reader of capture activity uses.

package driftengine

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/runproc"
)

// ErrBreakawayRefused marks a capture start Windows refused for breakaway when the caller admits no detached fallback.
var ErrBreakawayRefused = errors.New("drift: the capture start was refused breakaway")

// childSpawn describes one start of the capture child, told to the spawn seam.
type childSpawn struct {
	argv []string
	dir  string
	log  *os.File
	// breakaway is true for a start that leaves the caller's job object, false for a plain detached start.
	breakaway bool
}

// spawnedChild is a started capture child.
type spawnedChild struct {
	pid int
	// exited is closed when the child process has ended.
	exited <-chan struct{}
}

// SpawnRequest is the told input of StartCapture.
type SpawnRequest struct {
	// Executable is the lyx binary the child runs.
	Executable string
	// WorktreeRoot is the directory the child runs in.
	WorktreeRoot string
	// Paths are the drift paths of the task's anchor root.
	Paths Paths
	// RunProcDir and Owner locate the run's process records, where the child is registered.
	RunProcDir string
	Owner      string
	// FallbackDetach retries a start Windows refused for breakaway as a plain detached start; the caller then lives as long as its shell's job.
	// Unset, a refused start returns ErrBreakawayRefused.
	FallbackDetach bool
	// RecordWait bounds how long StartCapture waits for the child's running record.
	RecordWait time.Duration
	// Poll is the interval at which StartCapture reads the capture record.
	Poll time.Duration

	// spawn starts the child; nil starts the real command.
	spawn func(childSpawn) (spawnedChild, error)
	// breakawayRefused classifies a start error; nil is proc.IsBreakawayRefused.
	breakawayRefused func(error) bool
}

// StartCapture starts `<executable> drift capture-child` detached in the worktree root and returns its pid once the capture record carries that pid, in any state.
// The child writes that record after its precondition held under the capture lock, so the caller moves the run on only after the precondition held.
// It refuses while the capture lock reads held, and returns an error naming the child's log when the child ends, or the bound runs out, before the record.
func StartCapture(req SpawnRequest) (int, error) {
	held, err := ProbeLock(req.Paths.CaptureLock)
	if err != nil {
		return 0, err
	}
	if held {
		return 0, lockHeldRefusal(req.Paths.CaptureLock)
	}

	if err := os.MkdirAll(req.Paths.LogsDir, 0o755); err != nil {
		return 0, fmt.Errorf("drift: create the log directory: %w", err)
	}
	childLog := logPathFor(req.Paths.LogsDir, "capture-child", time.Now())
	logFile, err := os.OpenFile(childLog, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return 0, fmt.Errorf("drift: open the capture child log: %w", err)
	}
	defer logFile.Close()

	argv := []string{req.Executable, "drift", "capture-child"}
	child, err := req.startChild(childSpawn{argv: argv, dir: req.WorktreeRoot, log: logFile, breakaway: true})
	if err != nil && req.refusedBreakaway(err) {
		if !req.FallbackDetach {
			return 0, fmt.Errorf("%w: %v", ErrBreakawayRefused, err)
		}
		logger.Warn("drift: the capture child was refused breakaway, so it lives only as long as the shell's job", "error", err.Error())
		child, err = req.startChild(childSpawn{argv: argv, dir: req.WorktreeRoot, log: logFile})
	}
	if err != nil {
		return 0, fmt.Errorf("drift: start the capture child: %w", err)
	}
	logger.Info("drift: capture child spawned", "pid", child.pid, "executable", req.Executable, "log", childLog)
	if err := runproc.Register(req.RunProcDir, req.Owner, runproc.ChildRecord(child.pid, runproc.RoleCapture, argv)); err != nil {
		logger.Warn("drift: registering the capture child failed", "pid", child.pid, "error", err.Error())
	}

	return child.pid, awaitRunningRecord(req, child, childLog)
}

// awaitRunningRecord polls the capture record until it carries the child's pid.
// A capture that failed between two polls still carries it, so any state counts as the start.
// It fails when the child ends first or RecordWait runs out, naming the child's log and the caller's way forward.
func awaitRunningRecord(req SpawnRequest, child spawnedChild, childLog string) error {
	reads := func() bool {
		rec, found, err := ReadCapture(req.Paths)
		return err == nil && found && rec.Pid == child.pid
	}
	deadline := time.NewTimer(req.RecordWait)
	defer deadline.Stop()

	for {
		if reads() {
			return nil
		}
		select {
		case <-time.After(req.Poll):
		case <-child.exited:
			if reads() {
				return nil
			}
			return startFailure(req, childLog, "ended before it recorded its start")
		case <-deadline.C:
			if reads() {
				return nil
			}
			return startFailure(req, childLog, "did not record its start within "+req.RecordWait.String())
		}
	}
}

// startFailure builds the error of a capture child that did not record its start, with its caller's way forward.
func startFailure(req SpawnRequest, childLog, what string) error {
	wayForward := "read " + childLog + ", then lyx loom resume, whose entry captures again"
	if req.FallbackDetach {
		wayForward = "read " + childLog + ", then, once lyx drift status shows the capture lock free, run lyx drift capture again, or lyx drift defer followed by lyx loom resume"
	}
	return fmt.Errorf("drift: the capture child %s; way forward: %s", what, wayForward)
}

// startChild starts the child through the spawn seam, or as the real command when none is set.
func (r SpawnRequest) startChild(spec childSpawn) (spawnedChild, error) {
	if r.spawn != nil {
		return r.spawn(spec)
	}
	cmd := exec.Command(spec.argv[0], spec.argv[1:]...)
	cmd.Dir = spec.dir
	cmd.Stdout, cmd.Stderr = spec.log, spec.log
	if spec.breakaway {
		proc.DetachBreakaway(cmd)
	} else {
		proc.Detach(cmd)
	}
	if err := cmd.Start(); err != nil {
		return spawnedChild{}, err
	}
	pid := cmd.Process.Pid
	exited := make(chan struct{})
	go func() {
		waitErr := cmd.Wait()
		logger.Info("drift: capture child exited", "pid", pid, "error", errorText(waitErr))
		close(exited)
	}()
	return spawnedChild{pid: pid, exited: exited}, nil
}

// refusedBreakaway reports whether err is a start refused for breakaway.
func (r SpawnRequest) refusedBreakaway(err error) bool {
	if r.breakawayRefused != nil {
		return r.breakawayRefused(err)
	}
	return proc.IsBreakawayRefused(err)
}

// CaptureJobName returns the Windows job-object name of a worktree's capture: "lyx-drift-capture-" followed by a hex hash of the worktree root.
func CaptureJobName(worktreeRoot string) string {
	sum := sha256.Sum256([]byte(worktreeRoot))
	return "lyx-drift-capture-" + hex.EncodeToString(sum[:8])
}

// ProbeLock reports whether the OS lock at path is held, by taking it without blocking and releasing it at once.
// A missing lock file or directory reads as not held and creates nothing.
func ProbeLock(path string) (bool, error) {
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("drift: probe %s: %w", filepath.Base(path), err)
	}
	probe, taken, err := lock.TryAcquireWriteLock(path)
	if err != nil {
		return false, err
	}
	if !taken {
		return true, nil
	}
	return false, probe.Release()
}

// Active reports whether a capture or a compare is running or queued for a slot.
func Active(p Paths) (bool, error) {
	for _, path := range []string{p.CaptureLock, p.CompareMarker} {
		held, err := ProbeLock(path)
		if err != nil || held {
			return held, err
		}
	}
	return false, nil
}
