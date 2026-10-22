// procgroup_windows.go kills only the started process on cancel or expiry, since Windows has no process group to signal here.

package proc

import (
	"os"
	"os/exec"
	"time"
)

// configurePlatformGroupKill makes a cancel kill cmd's process alone and report ErrNoProcessGroup, since it cannot tell whether a descendant exists.
func configurePlatformGroupKill(cmd *exec.Cmd, report func(pid int, groupErr error)) {
	cmd.Cancel = func() error {
		pid := cmd.Process.Pid
		report(pid, ErrNoProcessGroup)
		return cmd.Process.Kill()
	}
}

// configurePlatformGroupTerminate makes a cancel kill cmd's process alone and report ErrNoProcessGroup, since there is no group to signal; killAfter is unused and awaitGone returns at once.
func configurePlatformGroupTerminate(cmd *exec.Cmd, _ time.Duration, report func(pid int, sig os.Signal, groupErr error)) (awaitGone func()) {
	cmd.Cancel = func() error {
		pid := cmd.Process.Pid
		report(pid, os.Kill, ErrNoProcessGroup)
		return cmd.Process.Kill()
	}
	return func() {}
}
