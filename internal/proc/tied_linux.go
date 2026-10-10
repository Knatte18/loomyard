// tied_linux.go ties a child to its parent with a parent-death signal and kills its tree through the process group.

package proc

import (
	"os"
	"runtime"
	"strconv"
	"syscall"
)

// startPlatform starts the child in its own process group with a parent-death SIGKILL.
// The start and the wait run on one goroutine locked to its OS thread, because the parent-death signal follows the thread that started the child.
func (t *Tied) startPlatform() error {
	if t.cmd.SysProcAttr == nil {
		t.cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	t.cmd.SysProcAttr.Setpgid = true
	t.cmd.SysProcAttr.Pdeathsig = syscall.SIGKILL

	started := make(chan error, 1)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		if err := t.cmd.Start(); err != nil {
			started <- err
			return
		}
		// The record is set before Wait runs, since a context cancel inside Wait calls Kill, which reads it.
		pid := t.cmd.Process.Pid
		startTime, _ := StartTime(pid)
		t.record = TreeRecord{PID: pid, PGID: pid, StartTime: startTime}
		started <- nil
		t.done <- t.cmd.Wait()
	}()
	return <-started
}

// releasePlatform has nothing to release on Linux.
func (t *Tied) releasePlatform() {}

// quitPlatform sends SIGQUIT to the child alone.
func (t *Tied) quitPlatform() error {
	return syscall.Kill(t.record.PID, syscall.SIGQUIT)
}

// killPlatform sends SIGKILL to the child's process group, killing the child alone when the group kill fails.
func (t *Tied) killPlatform() error {
	if err := syscall.Kill(-t.record.PGID, syscall.SIGKILL); err != nil {
		return t.cmd.Process.Kill()
	}
	return nil
}

// killTreePlatform kills rec's process group while some live process reports that group id with a start time at or after the recorded one.
// Linux keeps a group id unassigned while any member lives, so a match is the recorded tree, and the check holds after the group leader is dead.
func killTreePlatform(rec TreeRecord) (bool, error) {
	if rec.PGID <= 1 {
		return false, nil
	}
	recordedStart, err := strconv.ParseUint(rec.StartTime, 10, 64)
	if err != nil {
		return false, nil
	}
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return false, err
	}
	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}
		fields, ok := statFieldsAfterName(pid)
		if !ok {
			continue
		}
		// Field 5 is the process group, the third entry after the name.
		pgid, err := strconv.Atoi(fields[2])
		if err != nil || pgid != rec.PGID {
			continue
		}
		start, err := strconv.ParseUint(fields[19], 10, 64)
		if err != nil || start < recordedStart {
			continue
		}
		if err := syscall.Kill(-rec.PGID, syscall.SIGKILL); err != nil {
			if err == syscall.ESRCH {
				return false, nil
			}
			return false, err
		}
		return true, nil
	}
	return false, nil
}

// isBreakawayRefusedPlatform is always false: only Windows has a job that forbids breakaway.
func isBreakawayRefusedPlatform(err error) bool {
	return false
}
