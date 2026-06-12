//go:build !windows

// spawn_other.go — process launching on non-Windows.
//
// spawnServer starts a process in its own session (Setsid) so it survives
// the parent's exit. spawnAttach runs psmux interactively with inherited stdio.

package muxpoc

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

// spawnServer sets cmd.SysProcAttr to launch in a new session on non-Windows.
// Called before cmd.Start() to launch the psmux server detached.
func spawnServer(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}

// spawnAttach runs psmux attached to the session with inherited stdin/stdout/stderr.
// Blocks until the user detaches (normal for non-Windows interactive use).
func spawnAttach(psmuxPath, socket, session string) error {
	cmd := exec.Command(psmuxPath, "-L", socket, "attach-session", "-t", session)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("run psmux: %w", err)
	}
	return nil
}
