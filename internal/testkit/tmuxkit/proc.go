// proc.go holds the kit's read-only /proc probes: the process list, a process's argv, cwd and executable, and the recognizer for a reed watchdog daemon.
// Each probe reports nothing where /proc does not exist.

package tmuxkit

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Pids returns every numeric entry under /proc.
func Pids() []int {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}
	pids := make([]int, 0, len(entries))
	for _, e := range entries {
		if pid, err := strconv.Atoi(e.Name()); err == nil {
			pids = append(pids, pid)
		}
	}
	return pids
}

// ProcArgv reads pid's argv from /proc/<pid>/cmdline.
// The bool is false when the process is gone or unreadable; a process with an empty command line answers a nil slice and true.
func ProcArgv(pid int) ([]string, bool) {
	raw, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "cmdline"))
	if err != nil {
		return nil, false
	}
	trimmed := strings.TrimSuffix(string(raw), "\x00")
	if trimmed == "" {
		return nil, true
	}
	return strings.Split(trimmed, "\x00"), true
}

// ProcCwd reads pid's current working directory from the /proc/<pid>/cwd link.
func ProcCwd(pid int) (string, bool) {
	return readProcLink(pid, "cwd")
}

// ProcExe reads pid's executable path from the /proc/<pid>/exe link.
func ProcExe(pid int) (string, bool) {
	return readProcLink(pid, "exe")
}

func readProcLink(pid int, name string) (string, bool) {
	target, err := os.Readlink(filepath.Join("/proc", strconv.Itoa(pid), name))
	if err != nil {
		return "", false
	}
	return target, true
}

// IsWatchdog reports whether argv is a `lyx reed watchdog` daemon: an adjacent `reed`, `watchdog` pair.
// The daemon detaches into its own session and idles out on its own production schedule, so it outlives the test that spawned it.
func IsWatchdog(argv []string) bool {
	for i := 0; i+1 < len(argv); i++ {
		if argv[i] == "reed" && argv[i+1] == "watchdog" {
			return true
		}
	}
	return false
}
