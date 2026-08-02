// proctree_linux.go implements the two process-tree probes
// (descendantClosurePIDs, serverProcessesOnSocket) directly against /proc —
// Linux has no Win32_Process analog, so both probes read the kernel's own
// process table instead of shelling out to a helper. Each enumerates the
// numeric entries under /proc, reads the per-pid file it needs
// (/proc/<pid>/stat or /proc/<pid>/cmdline), and delegates the actual
// decision to the pure helpers in proctree.go (parseStatPPID,
// descendantClosure, matchSocketCmdlines). Real-Linux execution of this file
// is a deferred follow-up (see serverProcessesOnSocket's doc comment); here
// it is compile-checked only, by the batch's `GOOS=linux go build` gate.

package reedengine

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// descendantClosurePIDs expands roots to transitive descendants by reading
// /proc/<pid>/stat entries. Degrades to bare roots on read failure.
func (e *Engine) descendantClosurePIDs(roots []int) []int {
	if len(roots) == 0 {
		return nil
	}
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return roots
	}
	pidToPPID := make(map[int]int, len(entries))
	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			// /proc holds many non-numeric entries (self, cpuinfo, ...);
			// skip them rather than treating them as a read failure.
			continue
		}
		stat, err := os.ReadFile(filepath.Join("/proc", entry.Name(), "stat"))
		if err != nil {
			// A pid can exit between the ReadDir snapshot and this read —
			// a benign race, not a fatal condition for the whole probe.
			continue
		}
		ppid, err := parseStatPPID(string(stat))
		if err != nil {
			continue
		}
		pidToPPID[pid] = ppid
	}
	if len(pidToPPID) == 0 {
		return roots
	}
	return descendantClosure(pidToPPID, roots)
}

// serverProcessesOnSocket returns OS pids of processes on this engine's socket,
// scanned from /proc/*/cmdline. Returns nil on read failure.
// Note: This is a backstop only; liveness is tmux's CLI absence signal.
func (e *Engine) serverProcessesOnSocket() []int {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}
	var procs []ProcCmdline
	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}
		raw, err := os.ReadFile(filepath.Join("/proc", entry.Name(), "cmdline"))
		if err != nil {
			continue
		}
		// /proc/<pid>/cmdline is NUL-separated argv with a trailing NUL;
		// trim it before splitting so the split does not yield a spurious
		// empty trailing element.
		argv := strings.Split(strings.TrimSuffix(string(raw), "\x00"), "\x00")
		procs = append(procs, ProcCmdline{PID: pid, Argv: argv})
	}
	return matchSocketCmdlines(procs, e.cfg.Tmux, e.Socket())
}
