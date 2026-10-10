// procgroup_windows.go kills only the started process on cancel or expiry, since Windows has no process group to signal here.

package proc

import "os/exec"

// ConfigureGroupKill makes a context cancel or expiry kill cmd's process alone.
// It cannot tell whether a descendant exists, so it calls report with the command's pid and ErrNoProcessGroup after every kill.
// proc cannot log, so the caller's report callback is where a kill is logged.
func ConfigureGroupKill(cmd *exec.Cmd, report func(pid int, groupErr error)) {
	cmd.Cancel = func() error {
		pid := cmd.Process.Pid
		report(pid, ErrNoProcessGroup)
		return cmd.Process.Kill()
	}
}
