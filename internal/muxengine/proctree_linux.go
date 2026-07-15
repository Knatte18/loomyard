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

package muxengine

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// descendantClosurePIDs expands roots to roots-plus-their-transitive-
// descendant pids by reading every /proc/<pid>/stat entry to build a
// pid->ppid map, then delegating to descendantClosure for the actual
// fixed-point walk — the /proc-backed counterpart to the Windows seam's
// single Win32_Process pass. It mirrors that seam's degradation: if /proc
// cannot be enumerated, or every stat read fails so the map comes out empty,
// it returns the bare roots slice rather than erroring, since a caller
// reaping a pane's process subtree must still be able to fall back to
// killing the roots it already knows about.
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

// serverProcessesOnSocket returns the OS pids of every process on this
// engine's -L socket, discovered by scanning /proc/*/cmdline for argv
// containing both the configured multiplexer binary (e.cfg.Tmux — tmux on
// Linux via the config-swap the platform seam relies on) and an adjacent
// "-L <socket>" pair. This scan is a stray-process backstop only — the
// authoritative liveness signal on Linux is tmux's own honest CLI absence
// exit code (has-session/list-sessions), which this probe does not touch.
// Returns nil on any /proc read failure, mirroring the Windows probe's
// degrade-rather-than-error contract.
//
// Deferred follow-up (verbatim from the discussion): the /proc/*/cmdline
// match assumes the tmux server process retains -L <socket> in its argv.
// Real tmux often rewrites its process title (e.g. to "tmux: server") and
// may not keep the -L token, so the stray-process backstop could miss the
// real server. This is a deferred real-Linux validation item; liveness does
// not depend on it (the CLI absence signal is authoritative) — the backstop
// is a belt-and-suspenders guarantee whose match shape must be verified
// against a live tmux in the follow-up.
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
