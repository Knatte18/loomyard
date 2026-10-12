// proc_linux.go — Linux process control primitives.
//
// On Linux, HideWindow is a no-op (there are no console windows).
// Detach places the process in a new session using Setsid so it survives parent exit and is
// unaffected by the parent's signal handling.

package proc

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
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

// userHZ is the kernel's fixed tick rate for the CPU times in /proc/<pid>/stat, in ticks per second.
const userHZ = 100

// TreeCPU returns root and every descendant of it, each with its executable path and the user plus system CPU time it has used.
// A process that ends between the /proc listing and its reads is skipped; an unreadable /proc is an error.
func TreeCPU(root int) ([]ProcessCPU, error) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil, fmt.Errorf("proc: list /proc: %w", err)
	}

	children := map[int][]int{}
	cpu := map[int]time.Duration{}
	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}
		fields, ok := statFieldsAfterName(pid)
		if !ok {
			continue
		}
		// The remainder starts at field 3, so field 4 (parent pid) is entry 1, and fields 14 and 15 (user and system ticks) are entries 11 and 12.
		parent, parentErr := strconv.Atoi(fields[1])
		userTicks, userErr := strconv.ParseInt(fields[11], 10, 64)
		systemTicks, systemErr := strconv.ParseInt(fields[12], 10, 64)
		if parentErr != nil || userErr != nil || systemErr != nil {
			continue
		}
		children[parent] = append(children[parent], pid)
		cpu[pid] = time.Duration(userTicks+systemTicks) * time.Second / userHZ
	}

	var tree []ProcessCPU
	pending := []int{root}
	for len(pending) > 0 {
		pid := pending[0]
		pending = pending[1:]
		used, found := cpu[pid]
		if !found {
			continue
		}
		tree = append(tree, ProcessCPU{PID: pid, Path: executablePath(pid), CPU: used})
		pending = append(pending, children[pid]...)
	}
	return tree, nil
}

// executablePath returns the executable of pid from its /proc exe link, falling back to the first field of its command line.
// The 15-character command name in stat is never used, so a long test-binary name keeps its suffix.
func executablePath(pid int) string {
	base := "/proc/" + strconv.Itoa(pid)
	if path, err := os.Readlink(base + "/exe"); err == nil {
		return path
	}
	cmdline, err := os.ReadFile(base + "/cmdline")
	if err != nil {
		return ""
	}
	first, _, _ := strings.Cut(string(cmdline), "\x00")
	return first
}

// IsAncestor reports whether pid is a parent, grandparent or further ancestor of the calling process.
// The process itself, pid 0 and a pid that is not alive are never ancestors.
func IsAncestor(pid int) bool {
	if pid <= 0 {
		return false
	}
	for current := os.Getppid(); current > 0; {
		if current == pid {
			return true
		}
		fields, ok := statFieldsAfterName(current)
		if !ok {
			return false
		}
		parent, err := strconv.Atoi(fields[1])
		if err != nil {
			return false
		}
		current = parent
	}
	return false
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
