//go:build windows

// spawnattach_windows.go — Windows Terminal attach for muxpoc.

package muxpoc

import (
	"fmt"
	"os/exec"
)

// spawnAttach launches Windows Terminal maximized and attached: wt.exe -w 0 -M
// -- <psmuxPath> -L <socket> attach-session -t <session>. If wt.exe is not
// found, falls back to a plain psmux command. Returns cmd.Start() error (not
// Wait — fire-and-forget).
func spawnAttach(psmuxPath, socket, session string) error {
	// Try to launch Windows Terminal
	cmd := exec.Command("wt.exe", "-w", "0", "-M", "--", psmuxPath, "-L", socket, "attach-session", "-t", session)
	if err := cmd.Start(); err == nil {
		return nil
	}

	// Fall back to plain psmux command if wt.exe not found
	cmd = exec.Command(psmuxPath, "-L", socket, "attach-session", "-t", session)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start psmux: %w", err)
	}
	return nil
}
