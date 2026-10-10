// procgroup_linux.go puts a command in its own process group and kills the whole group on cancel or expiry.

package proc

import (
	"os/exec"
	"syscall"
)

// ConfigureGroupKill starts cmd in its own process group and makes a context cancel or expiry SIGKILL the whole group,
// so a descendant of the command cannot outlive it.
// When the group kill fails it kills the command alone.
// It calls report with the command's pid after each kill: a nil groupErr when the group was killed, otherwise the group kill's error.
// proc cannot log, so the caller's report callback is where a kill is logged.
func ConfigureGroupKill(cmd *exec.Cmd, report func(pid int, groupErr error)) {
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
