//go:build !windows

// pluginhook_unix.go starts a plugin hook in its own process group, so a timeout ends everything the hook's shell forked.

package claudeengine

import (
	"os/exec"
	"syscall"
)

// setHookProcessGroup starts cmd in its own process group and makes its cancellation kill the whole group.
// A shell that forks a child leaves the child holding the output pipes, which only the group kill releases.
func setHookProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}
