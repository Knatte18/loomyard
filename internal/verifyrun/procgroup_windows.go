//go:build windows

// procgroup_windows.go kills only the verify shell on cancel or expiry, since Windows has no process group to signal here.

package verifyrun

import (
	"os/exec"

	"github.com/Knatte18/loomyard/internal/logger"
)

// configureProcessKill makes a context cancel or expiry kill the verify shell alone.
// It cannot tell whether a descendant exists,
// so every kill warns that descendants may outlive the shell.
func configureProcessKill(cmd *exec.Cmd, command string) {
	cmd.Cancel = func() error {
		logger.Warn("verifyrun: killed only the verify shell; its descendants may outlive it", "pid", cmd.Process.Pid, "command", command)
		return cmd.Process.Kill()
	}
}
