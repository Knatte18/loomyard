//go:build windows

// proc_windows.go — Windows process control primitives.

package proc

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
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
