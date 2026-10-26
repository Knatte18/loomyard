// proctree.go holds the pure, build-tag-free process-tree logic the Linux and Windows probe seams
// (proctree_linux.go, proctree_windows.go) delegate to: /proc/<pid>/stat PPID and session parsing,
// descendant-closure computation over a pid->ppid map, and socket-cmdline matching.
// None of these functions touch the OS — they transform strings/maps/structs the platform files
// read off disk or a process-table query — which is what makes them unit-testable on the Windows
// host even though the Linux seam that calls them is only compile-checked here (see
// proctree_test.go).

package reedengine

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// ProcCmdline is one process's pid and parsed argv.
type ProcCmdline struct {
	PID  int
	Argv []string
}

// parseStat extracts the parent pid (field 4) and the session id (field 6) from /proc/<pid>/stat.
// Anchors on the last ')' to handle comm with embedded parens/spaces.
func parseStat(stat string) (ppid, session int, err error) {
	idx := strings.LastIndex(stat, ")")
	if idx == -1 {
		return 0, 0, fmt.Errorf("parse stat line: no closing paren found: %q", stat)
	}
	fields := strings.Fields(stat[idx+1:])
	if len(fields) < 4 {
		return 0, 0, fmt.Errorf("parse stat line: expected state, ppid, pgrp and session after comm, got %d fields: %q", len(fields), stat)
	}
	ppid, err = strconv.Atoi(fields[1])
	if err != nil {
		return 0, 0, fmt.Errorf("parse stat line: non-numeric ppid %q: %w", fields[1], err)
	}
	session, err = strconv.Atoi(fields[3])
	if err != nil {
		return 0, 0, fmt.Errorf("parse stat line: non-numeric session %q: %w", fields[3], err)
	}
	return ppid, session, nil
}

// processAge returns how long a process has run, from the system uptime and the process's /proc/<pid>/stat line.
// uptime is the content of /proc/uptime, whose first field is the seconds since boot;
// the process start time is field 22 of stat, in clock ticks since boot, and ticksPerSecond converts it.
// Anchors on the last ')' to handle comm with embedded parens/spaces.
// It errors on input it cannot read, a non-positive ticksPerSecond and a start later than the uptime.
func processAge(uptime, stat string, ticksPerSecond int64) (time.Duration, error) {
	if ticksPerSecond <= 0 {
		return 0, fmt.Errorf("process age: non-positive clock tick rate %d", ticksPerSecond)
	}
	uptimeFields := strings.Fields(uptime)
	if len(uptimeFields) == 0 {
		return 0, fmt.Errorf("process age: empty uptime")
	}
	uptimeSeconds, err := strconv.ParseFloat(uptimeFields[0], 64)
	if err != nil {
		return 0, fmt.Errorf("process age: non-numeric uptime %q: %w", uptimeFields[0], err)
	}
	idx := strings.LastIndex(stat, ")")
	if idx == -1 {
		return 0, fmt.Errorf("process age: no closing paren found in stat: %q", stat)
	}
	// Field 3 (state) is the first after comm, so field 22 is index 19.
	fields := strings.Fields(stat[idx+1:])
	if len(fields) < 20 {
		return 0, fmt.Errorf("process age: expected 20 fields after comm, got %d: %q", len(fields), stat)
	}
	startTicks, err := strconv.ParseInt(fields[19], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("process age: non-numeric start time %q: %w", fields[19], err)
	}
	startSeconds := float64(startTicks) / float64(ticksPerSecond)
	if startSeconds > uptimeSeconds {
		return 0, fmt.Errorf("process age: start %.2fs is later than uptime %.2fs", startSeconds, uptimeSeconds)
	}
	return time.Duration((uptimeSeconds - startSeconds) * float64(time.Second)), nil
}

// auxvClockTicksKey is the ELF auxiliary vector key AT_CLKTCK, the kernel's clock tick rate.
const auxvClockTicksKey = 17

// defaultClockTicksPerSecond is the rate a C library reports when the auxiliary vector holds no AT_CLKTCK entry.
const defaultClockTicksPerSecond = 100

// clockTicksFromAuxv returns the clock tick rate the auxiliary vector auxv declares, the unit of a /proc/<pid>/stat start time.
// It reads the rate without cgo, so the package builds no cgo file whose cached output names the checkout it was built in,
// and it returns defaultClockTicksPerSecond when auxv holds no positive AT_CLKTCK entry, as sysconf(_SC_CLK_TCK) does.
func clockTicksFromAuxv(auxv [][2]uintptr) int64 {
	for _, entry := range auxv {
		if entry[0] == auxvClockTicksKey && entry[1] > 0 {
			return int64(entry[1])
		}
	}
	return defaultClockTicksPerSecond
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
