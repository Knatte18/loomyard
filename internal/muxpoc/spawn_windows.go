//go:build windows

// spawn_windows.go — windowless process launching on Windows.
//
// spawnServer launches the psmux server as a detached, windowless process with
// its own process group. spawnAttach launches Windows Terminal maximized and
// attached to the session.

package muxpoc

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

const (
	createNewProcessGroup = 0x00000200
	createNoWindow        = 0x08000000
)

// spawnServer sets cmd.SysProcAttr to launch windowless and detached on Windows.
// Called before cmd.Start() to launch the psmux server invisible.
func spawnServer(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: createNoWindow | createNewProcessGroup,
	}
}

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
