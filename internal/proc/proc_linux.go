// proc_linux.go — Linux process control primitives.
//
// On Linux, HideWindow is a no-op (there are no console windows).
// Detach places the process in a new session using Setsid so it survives parent exit
// and is unaffected by the parent's signal handling.

package proc

import (
	"os"
	"os/exec"
	"syscall"
)

// HideWindow is a no-op on Linux (no console windows to suppress).
func HideWindow(cmd *exec.Cmd) {}

// IsAlive reports whether the process identified by pid is currently alive.
// On Unix, os.FindProcess always trivially succeeds regardless of whether
// pid actually exists, so process.Signal(syscall.Signal(0)) — which sends
// no actual signal, only checks deliverability/existence — is the real
// liveness probe here.
//
// A false positive is possible only in the narrow window of pid being
// reused by an unrelated process after the original one exited. This is
// acceptable for a staleness check that is not the sole gate: the
// protocol-version half of daemonStale, and the probe step downstream, both
// catch what a stale-but-PID-reused daemon would miss.
func IsAlive(pid int) bool {
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return process.Signal(syscall.Signal(0)) == nil
}

// Detach configures the command to run in a new session and survive parent exit.
// On Linux, Setsid is the equivalent of Windows CREATE_NEW_PROCESS_GROUP | CREATE_NO_WINDOW:
// it places the child in a new session with its own process group and no controlling terminal,
// so the parent's Ctrl-C signal and exit do not reach it.
func Detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
