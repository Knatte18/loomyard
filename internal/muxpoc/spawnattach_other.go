//go:build !windows

// spawnattach_other.go — psmux attach for non-Windows.

package muxpoc

import (
	"fmt"
	"os"
	"os/exec"
)

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
