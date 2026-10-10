//go:build windows

// child_windows.go runs go test inside a kill-on-close job object, so any exit of `lyx gate test`, a hard kill included, ends the child's whole tree.

package gatecli

import (
	"fmt"
	"os"
	"os/exec"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/Knatte18/loomyard/internal/logger"
)

// terminatingSignals are the signals `lyx gate test` catches to kill the child's tree and release its slot before it exits; Windows delivers only an interrupt.
var terminatingSignals = []os.Signal{os.Interrupt}

// runChild starts cmd, assigns it to a job object created with kill-on-job-close right after it starts, waits for it and returns Wait's error.
// A cancelled context terminates the job, and closing the job handle, which the operating system does when `lyx gate test` dies, ends every process in it.
func runChild(cmd *exec.Cmd) error {
	job, err := newKillOnCloseJob()
	if err != nil {
		return err
	}
	defer windows.CloseHandle(job)

	cmd.Cancel = func() error {
		logger.Info("gate test: terminating go test job object", "pid", cmd.Process.Pid)
		return windows.TerminateJobObject(job, 1)
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	if err := assignToJob(job, cmd.Process.Pid); err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return err
	}
	return cmd.Wait()
}

// newKillOnCloseJob creates a job object whose processes all end when its last handle closes.
func newKillOnCloseJob() (windows.Handle, error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return 0, fmt.Errorf("create job object: %w", err)
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{
		BasicLimitInformation: windows.JOBOBJECT_BASIC_LIMIT_INFORMATION{LimitFlags: windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE},
	}
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
		_ = windows.CloseHandle(job)
		return 0, fmt.Errorf("set job object kill-on-close: %w", err)
	}
	return job, nil
}

// assignToJob puts the process pid into job.
func assignToJob(job windows.Handle, pid int) error {
	process, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(pid))
	if err != nil {
		return fmt.Errorf("open go test process %d: %w", pid, err)
	}
	defer windows.CloseHandle(process)
	if err := windows.AssignProcessToJobObject(job, process); err != nil {
		return fmt.Errorf("assign go test process %d to its job object: %w", pid, err)
	}
	return nil
}
