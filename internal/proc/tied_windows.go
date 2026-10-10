// tied_windows.go ties a child to its parent with a per-child kill-on-close job object, which the operating system closes when the holder dies.

package proc

import (
	"errors"
	"syscall"

	"golang.org/x/sys/windows"
)

// startPlatform creates the child's unnamed kill-on-close job, starts the child and assigns it to that job.
func (t *Tied) startPlatform() error {
	job, err := createKillOnCloseJob(nil)
	if err != nil {
		return err
	}
	if err := t.cmd.Start(); err != nil {
		_ = windows.CloseHandle(job)
		return err
	}
	pid := t.cmd.Process.Pid
	if err := assignProcessToJob(job, pid); err != nil {
		_ = t.cmd.Process.Kill()
		_ = t.cmd.Wait()
		_ = windows.CloseHandle(job)
		return err
	}
	// The job is unnamed, so no other process can reopen it and the record carries nothing of it.
	t.job = uintptr(job)
	t.record = TreeRecord{PID: pid, PGID: pid}
	go func() { t.done <- t.cmd.Wait() }()
	return nil
}

// releasePlatform closes the child's job handle once the child has exited.
func (t *Tied) releasePlatform() {
	if t.job != 0 {
		_ = windows.CloseHandle(windows.Handle(t.job))
	}
}

// quitPlatform does nothing: Windows has no SIGQUIT to deliver.
func (t *Tied) quitPlatform() error {
	return nil
}

// killPlatform terminates the child's job, killing the child alone when the job termination fails.
func (t *Tied) killPlatform() error {
	if err := windows.TerminateJobObject(windows.Handle(t.job), 1); err != nil {
		return t.cmd.Process.Kill()
	}
	return nil
}

// killTreePlatform kills nothing: the child's job sits inside its loop's named job and closed when the loop died.
func killTreePlatform(rec TreeRecord) (bool, error) {
	return false, nil
}

// isBreakawayRefusedPlatform reports ERROR_ACCESS_DENIED, which Windows returns for a CREATE_BREAKAWAY_FROM_JOB the enclosing job forbids.
func isBreakawayRefusedPlatform(err error) bool {
	var errno syscall.Errno
	return errors.As(err, &errno) && errno == windows.ERROR_ACCESS_DENIED
}
