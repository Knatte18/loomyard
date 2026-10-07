//go:build !windows

// procgroup_unix.go puts the verify shell in its own process group and kills the whole group on cancel or expiry.

package verifyrun

import (
	"os/exec"
	"syscall"

	"github.com/Knatte18/loomyard/internal/logger"
)

// configureProcessKill starts cmd in its own process group and makes a context cancel or expiry SIGKILL the whole group,
// so a descendant of the verify shell cannot outlive it.
// When the group kill fails it kills the shell alone.
func configureProcessKill(cmd *exec.Cmd, command string) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		pid := cmd.Process.Pid
		logger.Info("verifyrun: killing verify process group", "pid", pid, "command", command)
		if err := syscall.Kill(-pid, syscall.SIGKILL); err != nil {
			logger.Warn("verifyrun: process group kill failed, killing the shell only", "pid", pid, "command", command, "cause", err)
			return cmd.Process.Kill()
		}
		return nil
	}
}
