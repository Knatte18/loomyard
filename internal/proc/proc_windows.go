//go:build windows

// Package proc provides cross-OS primitives for controlling child-process window visibility and detachment.
//
// On Windows, HideWindow suppresses the console window using CREATE_NO_WINDOW, and Detach
// additionally creates a new process group (CREATE_NEW_PROCESS_GROUP) so the child survives
// parent exit and is unaffected by the parent's Ctrl-C signal.

package proc

import (
	"os"
	"os/exec"
	"syscall"
)

const createNoWindow uint32 = 0x08000000
const createNewProcessGroup uint32 = 0x00000200
const createBreakawayFromJob uint32 = 0x01000000

// HideWindow configures the command to run without a console window.
func HideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: createNoWindow,
	}
}

// Detach configures the command to run detached in a new process group and without a console
// window.
func Detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: createNoWindow | createNewProcessGroup,
	}
}

// DetachBreakaway configures the command like Detach, additionally setting
// CREATE_BREAKAWAY_FROM_JOB.
func DetachBreakaway(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: createNoWindow | createNewProcessGroup | createBreakawayFromJob,
	}
}

// IsAlive reports whether the process identified by pid is currently alive.
func IsAlive(pid int) bool {
	_, err := os.FindProcess(pid)
	return err == nil
}

// KillPID force-kills the process identified by pid.
func KillPID(pid int) error {
	process, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return process.Kill()
}
