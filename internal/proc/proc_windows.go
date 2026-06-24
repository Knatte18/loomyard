//go:build windows

// Package proc provides cross-OS primitives for controlling child-process window visibility and detachment.
//
// On Windows, HideWindow suppresses the console window using CREATE_NO_WINDOW, and Detach
// additionally creates a new process group (CREATE_NEW_PROCESS_GROUP) so the child survives
// parent exit and is unaffected by the parent's Ctrl-C signal.

package proc

import (
	"os/exec"
	"syscall"
)

const createNoWindow uint32 = 0x08000000
const createNewProcessGroup uint32 = 0x00000200

// HideWindow configures the command to run without a console window.
// On Windows, it sets CREATE_NO_WINDOW via SysProcAttr.
func HideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: createNoWindow,
	}
}

// Detach configures the command to run detached in a new process group and without a console window.
// On Windows, this sets both CREATE_NO_WINDOW and CREATE_NEW_PROCESS_GROUP so the child
// survives parent exit and is not affected by parent Ctrl-C signals.
func Detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: createNoWindow | createNewProcessGroup,
	}
}
