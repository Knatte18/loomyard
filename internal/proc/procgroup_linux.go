// procgroup_linux.go puts a command in its own process group and kills the whole group on cancel or expiry.

package proc

import (
	"os/exec"
	"syscall"
)

// configurePlatformGroupKill starts cmd in its own process group and makes a cancel SIGKILL the whole group, killing the command alone when the group kill fails.
func configurePlatformGroupKill(cmd *exec.Cmd, report func(pid int, groupErr error)) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		pid := cmd.Process.Pid
		groupErr := syscall.Kill(-pid, syscall.SIGKILL)
		report(pid, groupErr)
		if groupErr != nil {
			return cmd.Process.Kill()
		}
		return nil
	}
}
