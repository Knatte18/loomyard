// proctree.go holds the pure, build-tag-free process-tree logic the Linux and Windows probe seams (proctree_linux.go, proctree_windows.go) delegate to: /proc/<pid>/stat PPID parsing, descendant-closure computation over a pid->ppid map, and socket-cmdline matching.
// None of these functions touch the OS — they transform strings/maps/structs the platform files read off disk or a process-table query — which is what makes them unit-testable on the Windows host even though the Linux seam that calls them is only compile-checked here (see proctree_test.go).

package reedengine

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
)

// ProcCmdline is one process's pid and parsed argv.
type ProcCmdline struct {
	PID  int
	Argv []string
}

// parseStatPPID extracts parent pid (field 4) from /proc/<pid>/stat.
// Anchors on the last ')' to handle comm with embedded parens/spaces.
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

// descendantClosure returns roots plus every transitive descendant.
// Repeatedly absorbs pids whose parent is already accepted (fixed-point walk).
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

// matchSocketCmdlines returns pids whose argv names both the binary
// (by base name) and an adjacent "-L" <socket> pair.
func matchSocketCmdlines(procs []ProcCmdline, binary, socket string) []int {
	var out []int
	for _, p := range procs {
		if argvNamesBinaryAndSocket(p.Argv, binary, socket) {
			out = append(out, p.PID)
		}
	}
	return out
}

// tmuxProcessName returns the Windows process-table Name for the configured
// binary (base name with ".exe" if not present).
func tmuxProcessName(binary string) string {
	// binary names a Windows path (this function derives a Windows
	// process-table Name), so the base-name split must recognize '\' even
	// when this code runs on a non-Windows test host: path/filepath.Base is
	// GOOS-native and would leave a "C:\...\tmux.exe"-shaped input untouched
	// on Linux, which is exactly what proctree.go's own package doc promises
	// never happens ("None of these functions touch the OS").
	name := binary
	if i := strings.LastIndexAny(name, `/\`); i >= 0 {
		name = name[i+1:]
	}
	if !strings.HasSuffix(strings.ToLower(name), ".exe") {
		name += ".exe"
	}
	return name
}

// argvNamesBinaryAndSocket reports whether argv contains binary (by base name)
// and an adjacent "-L" socket pair.
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
