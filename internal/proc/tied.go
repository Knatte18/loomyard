// tied.go starts a child tied to the calling process's life and kills a child's whole process tree, from the live child or from a record read back from disk.

package proc

import (
	"context"
	"os"
	"os/exec"
	"reflect"
	"sync"
)

// TreeRecord identifies a child's process tree well enough for a later process to kill it.
// StartTime is empty where the platform cannot read a start time.
type TreeRecord struct {
	PID       int    `json:"pid"`
	PGID      int    `json:"pgid"`
	StartTime string `json:"start_time"`
}

// Tied is a child started by StartTied.
type Tied struct {
	cmd    *exec.Cmd
	record TreeRecord
	// job is the per-child kill-on-close job object on Windows, zero elsewhere.
	job  uintptr
	done chan error

	waitOnce sync.Once
	waitErr  error
}

// StartTied starts cmd tied to the calling process's life, in its own process group whose id is the child's pid.
// On Linux the kernel kills the child when the calling process dies; on Windows a kill-on-close job object does.
// When the caller set no cmd.Cancel on an exec.CommandContext child, so it still holds the default that kills the child alone, it installs one that calls Kill and the child dies with its whole tree on cancel.
// A Cancel the caller set is kept, and a plain exec.Command has none.
func StartTied(cmd *exec.Cmd) (*Tied, error) {
	t := &Tied{cmd: cmd, done: make(chan error, 1)}
	if hasDefaultCancel(cmd) {
		cmd.Cancel = t.Kill
	}
	if err := t.startPlatform(); err != nil {
		return nil, err
	}
	return t, nil
}

// hasDefaultCancel reports whether cmd's Cancel is the one exec.CommandContext installs.
// All closures made by that one function literal share a code pointer, so a throwaway command's Cancel identifies it.
func hasDefaultCancel(cmd *exec.Cmd) bool {
	if cmd.Cancel == nil {
		return false
	}
	reference := exec.CommandContext(context.Background(), "").Cancel
	return reflect.ValueOf(cmd.Cancel).Pointer() == reflect.ValueOf(reference).Pointer()
}

// Record returns the child's pid, process-group id and start time.
func (t *Tied) Record() TreeRecord {
	return t.record
}

// Wait waits for the child to exit and returns the error exec.Cmd.Wait reports.
// It may be called more than once.
func (t *Tied) Wait() error {
	t.waitOnce.Do(func() {
		t.waitErr = <-t.done
		t.releasePlatform()
	})
	return t.waitErr
}

// Quit asks the child alone to dump its goroutines with SIGQUIT, so a Go child's dump lands on its stderr.
// It does nothing on Windows.
func (t *Tied) Quit() error {
	return t.quitPlatform()
}

// Kill kills the child's whole tree, falling back to the child alone when the group or job kill fails.
func (t *Tied) Kill() error {
	return t.killPlatform()
}

// KillTree kills the process tree a record names and reports whether it killed anything.
// On Linux it kills the process group only while a live process still reports the recorded group id with a start time at or after the recorded one, so a reused group id is left alone.
// On Windows it kills nothing and reports false; the tree dies through TerminateNamedJob on its loop's job.
func KillTree(rec TreeRecord) (bool, error) {
	return killTreePlatform(rec)
}

// SelfRecord returns the calling process's own pid and start time, the shape a pid file records.
func SelfRecord() TreeRecord {
	pid := os.Getpid()
	start, _ := StartTime(pid)
	return TreeRecord{PID: pid, StartTime: start}
}

// HoldNamedJob puts the calling process into a kill-on-close Windows job object called name and returns the function that releases its handle.
// The name exists only while a handle is open, so TerminateNamedJob reaches the live holder and its whole tree and never a reused pid.
// On Linux it does nothing and returns a no-op release.
func HoldNamedJob(name string) (func() error, error) {
	return holdNamedJobPlatform(name)
}

// TerminateNamedJob terminates the Windows job object called name and reports false when no such job exists.
// On Linux it always reports false.
func TerminateNamedJob(name string) (bool, error) {
	return terminateNamedJobPlatform(name)
}

// IsBreakawayRefused reports whether err is Windows refusing a CREATE_BREAKAWAY_FROM_JOB start because the enclosing job forbids breakaway.
// It is always false on Linux.
func IsBreakawayRefused(err error) bool {
	return isBreakawayRefusedPlatform(err)
}
