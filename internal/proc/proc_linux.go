// proc_linux.go — Linux process control primitives.
//
// On Linux, HideWindow is a no-op (there are no console windows).
// Detach places the process in a new session using Setsid so it survives parent exit
// and is unaffected by the parent's signal handling.

package proc

import (
	"os/exec"
	"syscall"
)

// HideWindow is a no-op on Linux (no console windows to suppress).
func HideWindow(cmd *exec.Cmd) {}

// Detach configures the command to run in a new session and survive parent exit.
// On Linux, Setsid is the equivalent of Windows CREATE_NEW_PROCESS_GROUP | CREATE_NO_WINDOW:
// it places the child in a new session with its own process group and no controlling terminal,
// so the parent's Ctrl-C signal and exit do not reach it.
func Detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
