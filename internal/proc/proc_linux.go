// proc_linux.go — Linux process control primitives.
//
// On Linux, HideWindow is a no-op (there are no console windows).
// Detach places the process in a new session using Setsid so it survives parent exit and is
// unaffected by the parent's signal handling.

package proc

import (
	"os"
	"os/exec"
	"syscall"
)

// HideWindow is a no-op on Linux (no console windows to suppress).
func HideWindow(cmd *exec.Cmd) {}

// IsAlive reports whether the process identified by pid is currently alive.
func IsAlive(pid int) bool {
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return process.Signal(syscall.Signal(0)) == nil
}

// KillPID force-kills the process identified by pid.
func KillPID(pid int) error {
	process, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return process.Kill()
}

// Detach configures the command to run in a new session and survive parent exit.
func Detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}

// DetachBreakaway configures the command like Detach, additionally surviving a Windows Job Object.
func DetachBreakaway(cmd *exec.Cmd) {
	Detach(cmd)
}
