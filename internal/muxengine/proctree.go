// proctree.go holds the pure, build-tag-free process-tree logic the Linux
// and Windows probe seams (proctree_linux.go, proctree_windows.go) delegate
// to: /proc/<pid>/stat PPID parsing, descendant-closure computation over a
// pid->ppid map, and socket-cmdline matching. None of these functions touch
// the OS — they transform strings/maps/structs the platform files read off
// disk or a process-table query — which is what makes them unit-testable on
// the Windows host even though the Linux seam that calls them is only
// compile-checked here (see proctree_test.go).

package muxengine

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
)

// ProcCmdline is one process's pid and parsed argv, the shape
// matchSocketCmdlines consumes. The Linux seam builds these from
// /proc/<pid>/cmdline's NUL-separated bytes; tests build them directly from
// literal slices.
type ProcCmdline struct {
	PID  int
	Argv []string
}

// parseStatPPID extracts the parent pid (field 4) from the contents of a
// /proc/<pid>/stat line. Field 2 (comm, the executable name in parentheses)
// is the one field in the line that is not whitespace-delimited: the kernel
// writes it verbatim between parentheses, so a process named e.g. "a) b" (a
// space and an embedded close-paren are both valid in a comm) would corrupt a
// naive split-on-whitespace parse. Parsing instead anchors on the LAST ')' in
// the line — comm's own parens can only appear before it — and reads the
// state and PPID as the first two whitespace-separated fields after that
// point, matching the stat(5) layout: "pid (comm) state ppid ...".
func parseStatPPID(stat string) (int, error) {
	idx := strings.LastIndex(stat, ")")
	if idx == -1 {
		return 0, fmt.Errorf("parse stat line: no closing paren found: %q", stat)
	}
	fields := strings.Fields(stat[idx+1:])
	if len(fields) < 2 {
		return 0, fmt.Errorf("parse stat line: expected state and ppid after comm, got %d fields: %q", len(fields), stat)
	}
	ppid, err := strconv.Atoi(fields[1])
	if err != nil {
		return 0, fmt.Errorf("parse stat line: non-numeric ppid %q: %w", fields[1], err)
	}
	return ppid, nil
}

// descendantClosure returns roots plus every transitive descendant reachable
// through pidToPPID — the pure analog of the existing Windows WMI parent-walk
// that descendantClosurePIDs (proctree_windows.go / proctree_linux.go) seeds
// from a live process-table snapshot. It repeatedly absorbs any pid whose
// parent is already accepted until a pass absorbs nothing, tolerating a pid
// whose ppid is absent from the map entirely (that pid is simply never
// reachable, not a fatal condition — a snapshot race can legitimately miss a
// parent that exited between reads). The pass count is capped at
// len(pidToPPID)+1: since each pass can only ever add a previously-unaccepted
// pid and there are at most len(pidToPPID) distinct pids to add, that bound
// is always enough to reach the fixed point, and it also guarantees a pid
// re-parented into its own subtree (a cycle) can never wedge the loop.
func descendantClosure(pidToPPID map[int]int, roots []int) []int {
	accepted := make(map[int]bool, len(roots))
	for _, r := range roots {
		accepted[r] = true
	}
	maxPasses := len(pidToPPID) + 1
	for pass := 0; pass < maxPasses; pass++ {
		changed := false
		for pid, ppid := range pidToPPID {
			if accepted[pid] {
				continue
			}
			if accepted[ppid] {
				accepted[pid] = true
				changed = true
			}
		}
		if !changed {
			break
		}
	}
	out := make([]int, 0, len(accepted))
	for pid := range accepted {
		out = append(out, pid)
	}
	return out
}

// matchSocketCmdlines returns the pids of every proc whose argv names both
// binary (matched on the argv element's base name, so an absolute path like
// /usr/bin/tmux matches the configured "tmux") and an adjacent "-L" <socket>
// pair — the pure analog of serverProcessesOnSocket's Windows CommandLine
// regex match, applied to /proc/*/cmdline's already-split argv instead of a
// single command-line string. A proc naming the binary without -L, or naming
// a different socket after -L, is deliberately not a match: either shape
// describes an unrelated process (or the wrong server), not this engine's
// socket-holder.
func matchSocketCmdlines(procs []ProcCmdline, binary, socket string) []int {
	var out []int
	for _, p := range procs {
		if argvNamesBinaryAndSocket(p.Argv, binary, socket) {
			out = append(out, p.PID)
		}
	}
	return out
}

// argvNamesBinaryAndSocket reports whether argv contains both binary's base
// name as one of its elements and an "-L" element immediately followed by an
// element equal to socket.
func argvNamesBinaryAndSocket(argv []string, binary, socket string) bool {
	wantBase := filepath.Base(binary)
	hasBinary := false
	hasSocket := false
	for i, arg := range argv {
		if filepath.Base(arg) == wantBase {
			hasBinary = true
		}
		if arg == "-L" && i+1 < len(argv) && argv[i+1] == socket {
			hasSocket = true
		}
	}
	return hasBinary && hasSocket
}
