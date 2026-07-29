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

// DetachBreakaway configures the command like Detach, additionally setting
// CREATE_BREAKAWAY_FROM_JOB so the child survives a Windows Job Object with
// kill-on-close closing (lyx itself may run inside one; CREATE_NEW_PROCESS_GROUP
// alone does not save a child from that). This is a superset of Detach's flags,
// used by the supervised strategy's daemon spawn, which must outlive not just
// this process's exit but also any Job Object lyx itself is contained in.
// Detach itself is left untouched; every existing Detach caller's behavior is
// unaffected by this new function.
func DetachBreakaway(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: createNoWindow | createNewProcessGroup | createBreakawayFromJob,
	}
}

// IsAlive reports whether the process identified by pid is currently alive.
// On Windows, os.Process.Signal does not reliably support a signal-0
// liveness probe, but os.FindProcess itself calls OpenProcess and fails
// when pid does not exist — so the existence of a successful FindProcess
// call is itself the liveness signal here (no Signal call needed or
// reliable).
//
// A false positive is possible only in the narrow window of pid being
// reused by an unrelated process after the original one exited. This is
// acceptable for a staleness check that is not the sole gate: the
// protocol-version half of daemonStale, and the probe step downstream, both
// catch what a stale-but-PID-reused daemon would miss.
func IsAlive(pid int) bool {
	_, err := os.FindProcess(pid)
	return err == nil
}
