//go:build windows

// spawn_windows.go — windowless launching on Windows (psmux/claude are console apps;
// without this each would flash its own console window when launched from a
// console-less parent). Mirrors internal/board's spawn discipline.
package muxpoc

import (
	"os/exec"
	"syscall"
)

const (
	createNewProcessGroup = 0x00000200
	createNoWindow        = 0x08000000
)

// applyHidden makes a command run windowless. Used for every psmux invocation.
func applyHidden(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
}

// applyDetached makes a command run as a windowless, own-process-group child that
// survives the parent's exit (used to launch the muxpoc daemon in the background).
func applyDetached(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: createNoWindow | createNewProcessGroup,
	}
}
