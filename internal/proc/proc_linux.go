// proc_linux.go — Linux process control primitives.
//
// On Linux, HideWindow is a no-op (there are no console windows).
// Detach places the process in a new session using Setsid so it survives parent exit and is
// unaffected by the parent's signal handling.

package proc

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
)

// HideWindow is a no-op on Linux (no console windows to suppress).
func HideWindow(cmd *exec.Cmd) {}

// IsAlive reports whether the process identified by pid is currently alive.
func IsAlive(pid int) bool {
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return process.Signal(syscall.Signal(0)) == nil
}

// StartTime returns the start time of the process identified by pid, in the form Claude's session registry records as procStart:
// field 22 of /proc/<pid>/stat (clock ticks since boot).
// It reports false when the process does not exist or its stat cannot be parsed.
func StartTime(pid int) (string, bool) {
	fields, ok := statFieldsAfterName(pid)
	if !ok {
		return "", false
	}
	// The remainder starts at field 3, so field 22 is the 20th entry.
	return fields[19], true
}

// statFieldsAfterName returns the fields of /proc/<pid>/stat that follow the command name, starting at field 3.
// It reports false when the process does not exist or its stat cannot be parsed.
func statFieldsAfterName(pid int) ([]string, bool) {
	data, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return nil, false
	}
	// The command name in field 2 may hold spaces or parentheses, so count fields after the last ')'.
	stat := string(data)
	end := strings.LastIndexByte(stat, ')')
	if end < 0 {
		return nil, false
	}
	fields := strings.Fields(stat[end+1:])
	if len(fields) < 20 {
		return nil, false
	}
	return fields, true
}

// KillPID force-kills the process identified by pid.
func KillPID(pid int) error {
	process, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return process.Kill()
}

// Detach configures the command to run in a new session and survive parent exit.
func Detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}

// DetachBreakaway configures the command like Detach, additionally surviving a Windows Job Object.
func DetachBreakaway(cmd *exec.Cmd) {
	Detach(cmd)
}
