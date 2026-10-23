//go:build windows

// proc_windows.go — Windows process control primitives.

package proc

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
)

const createNoWindow uint32 = 0x08000000
const createNewProcessGroup uint32 = 0x00000200
const createBreakawayFromJob uint32 = 0x01000000

// HideWindow configures the command to run without a console window.
func HideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: createNoWindow,
	}
}

// Detach configures the command to run detached in a new process group and without a console
// window.
func Detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: createNoWindow | createNewProcessGroup,
	}
}

// DetachBreakaway configures the command like Detach, additionally setting
// CREATE_BREAKAWAY_FROM_JOB.
func DetachBreakaway(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: createNoWindow | createNewProcessGroup | createBreakawayFromJob,
	}
}

// IsAlive reports whether the process identified by pid is currently alive.
func IsAlive(pid int) bool {
	_, err := os.FindProcess(pid)
	return err == nil
}

// StartTime reports the start time as unreadable on Windows, which keeps the resume check failing closed there.
func StartTime(pid int) (string, bool) {
	return "", false
}

// CreationTime reads the creation time of the process identified by pid and reports false when the process cannot be opened or its times read.
// It lets a caller tell a live process from a reused pid, since StartTime is unreadable on Windows.
func CreationTime(pid int) (time.Time, bool) {
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return time.Time{}, false
	}
	defer windows.CloseHandle(handle)
	var creation, exit, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(handle, &creation, &exit, &kernel, &user); err != nil {
		return time.Time{}, false
	}
	return time.Unix(0, creation.Nanoseconds()), true
}

// ParentPID reports no parent on Windows, where creel resolves identity from process ancestry on Linux only.
func ParentPID(pid int) (int, bool) {
	return 0, false
}

// TreeCPU reports errors.ErrUnsupported on Windows, which a CPU watchdog reads as no watchdog applying.
func TreeCPU(root int) ([]ProcessCPU, error) {
	return nil, errors.ErrUnsupported
}

// ListProcesses reports errors.ErrUnsupported on Windows, where the process links it reads do not exist.
func ListProcesses() ([]Process, error) {
	return nil, errors.ErrUnsupported
}

// ListStats reports errors.ErrUnsupported on Windows, where the stat files it reads do not exist.
func ListStats() ([]StatEntry, error) {
	return nil, errors.ErrUnsupported
}

// SignalProcess force-kills the process identified by pid when kill is set and reports errors.ErrUnsupported otherwise, since Windows has no SIGTERM.
func SignalProcess(pid int, kill bool) error {
	if kill {
		return KillPID(pid)
	}
	return errors.ErrUnsupported
}

// SignalGroup always reports ErrNoProcessGroup on Windows, where a process group cannot be signalled.
func SignalGroup(pgid int, kill bool) error {
	return ErrNoProcessGroup
}

// IsAncestor is always false on Windows, where the ancestry walk is not implemented.
func IsAncestor(pid int) bool {
	return false
}

// KillPID force-kills the process identified by pid.
func KillPID(pid int) error {
	process, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return process.Kill()
}
