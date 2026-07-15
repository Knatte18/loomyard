//go:build smoke

// smoke_proctree_test.go provides the /proc-native process-tree probes the
// smoke harness uses on Linux — the direct analogue of the Windows helpers'
// Get-CimInstance Win32_Process scripts (paneProcessTree, panePaneSubtree,
// tmuxSocketPids, pidClosure, hubHolders, serverProcCountForSession in
// smoke_test.go), and structurally identical to
// internal/muxengine/proctree_linux.go's approach: read /proc/<pid>/stat for
// ppid, /proc/<pid>/cmdline for argv, /proc/<pid>/cwd for cwd. Unlike
// Windows, Linux needs no external process to answer any of these
// questions — the kernel exposes them directly over /proc — so these
// probes shell out to nothing, not even a POSIX substitute for pwsh.
// Reimplemented here as a small, self-contained test-harness copy rather
// than imported from muxengine, since that package's equivalents
// (parseStatPPID, descendantClosure, matchSocketCmdlines) are unexported and
// only meaningful bound to an *Engine value. Deliberately a _test.go file
// (not a _linux.go one): its caller functions in smoke_test.go compile on
// every GOOS (gated only by the smoke tag) and runtime.GOOS-branch into this
// file's functions, so this file must compile everywhere too, and it
// references hubHolder, which is itself declared in smoke_test.go and so
// only exists inside the test binary — the os.ReadFile/ReadDir/Readlink
// calls here are portable Go and simply error (never get called) on a
// non-Linux GOOS.

package muxcli

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// linuxPids returns every numeric entry under /proc — the live pid set at
// this instant. Returns nil if /proc cannot be read at all.
func linuxPids() []int {
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

// linuxProcPPID reads pid's parent pid from /proc/<pid>/stat, anchored on
// the last ")" the same way muxengine's parseStatPPID is, since comm (field
// 2) can itself contain spaces or parens.
func linuxProcPPID(pid int) (int, bool) {
	raw, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "stat"))
	if err != nil {
		return 0, false
	}
	stat := string(raw)
	idx := strings.LastIndex(stat, ")")
	if idx == -1 {
		return 0, false
	}
	fields := strings.Fields(stat[idx+1:])
	if len(fields) < 2 {
		return 0, false
	}
	ppid, err := strconv.Atoi(fields[1])
	if err != nil {
		return 0, false
	}
	return ppid, true
}

// linuxProcArgv reads pid's argv from /proc/<pid>/cmdline (NUL-separated,
// trailing NUL trimmed before splitting so it never yields a spurious empty
// trailing element).
func linuxProcArgv(pid int) ([]string, bool) {
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

// linuxProcCwd reads pid's current working directory via the /proc/<pid>/cwd
// symlink — trivially available on Linux, unlike Windows where reaching a
// live process's cwd needs a PEB read via P/Invoke (see hubHolders' Windows
// script).
func linuxProcCwd(pid int) (string, bool) {
	cwd, err := os.Readlink(filepath.Join("/proc", strconv.Itoa(pid), "cwd"))
	if err != nil {
		return "", false
	}
	return cwd, true
}

// linuxProcComm reads pid's short executable name from /proc/<pid>/comm.
func linuxProcComm(pid int) (string, bool) {
	raw, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "comm"))
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(string(raw)), true
}

// linuxDescendantClosure expands roots to roots-plus-their-transitive-
// descendants by building a pid->ppid map from every /proc/<pid>/stat entry,
// then repeatedly absorbing any pid whose parent is already accepted — the
// /proc-native counterpart to the Windows helpers' Win32_Process closure
// loop (paneProcessTree, panePaneSubtree, pidClosure). Degrades to the bare
// roots if /proc cannot be read at all, so a caller reaping a pane's process
// subtree can still fall back to killing the roots it already knows about.
func linuxDescendantClosure(roots []int) []int {
	if len(roots) == 0 {
		return nil
	}
	pidToPPID := make(map[int]int)
	for _, pid := range linuxPids() {
		if ppid, ok := linuxProcPPID(pid); ok {
			pidToPPID[pid] = ppid
		}
	}
	if len(pidToPPID) == 0 {
		return roots
	}
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

// linuxArgvHasBinary reports whether argv contains binary's base name as one
// of its elements — the /proc-native counterpart of the Windows probes'
// CommandLine binary-name match.
func linuxArgvHasBinary(argv []string, binary string) bool {
	want := filepath.Base(binary)
	for _, a := range argv {
		if filepath.Base(a) == want {
			return true
		}
	}
	return false
}

// linuxArgvHasFlagValue reports whether argv contains flag immediately
// followed by an element equal to value.
func linuxArgvHasFlagValue(argv []string, flag, value string) bool {
	for i, a := range argv {
		if a == flag && i+1 < len(argv) && argv[i+1] == value {
			return true
		}
	}
	return false
}

// linuxTmuxSocketPids returns the pids of every process whose argv names
// both binary and an adjacent "-L socket" pair — the /proc-native analogue
// of tmuxSocketPids' Windows Win32_Process filter, structurally identical to
// muxengine's serverProcessesOnSocket.
func linuxTmuxSocketPids(binary, socket string) []int {
	var out []int
	for _, pid := range linuxPids() {
		argv, ok := linuxProcArgv(pid)
		if !ok {
			continue
		}
		if linuxArgvHasBinary(argv, binary) && linuxArgvHasFlagValue(argv, "-L", socket) {
			out = append(out, pid)
		}
	}
	return out
}

// linuxHubHolders returns every process whose current working directory is
// inside dir, read straight off /proc/<pid>/cwd — the /proc-native
// counterpart of hubHolders' Windows PEB-read script. Linux exposes cwd
// directly as a symlink, so unlike Windows there is no ConPTY-host artifact
// class to exempt: any Linux holder this finds is a genuine leak.
func linuxHubHolders(dir string) []hubHolder {
	var holders []hubHolder
	for _, pid := range linuxPids() {
		cwd, ok := linuxProcCwd(pid)
		if !ok {
			continue
		}
		if cwd != dir && !strings.HasPrefix(cwd, dir+string(filepath.Separator)) {
			continue
		}
		name, _ := linuxProcComm(pid)
		holders = append(holders, hubHolder{pid: pid, name: name})
	}
	return holders
}
