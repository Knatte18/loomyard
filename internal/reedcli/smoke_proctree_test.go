//go:build tmux || llm

// smoke_proctree_test.go provides the process-tree probes the smoke harness uses on Linux.
// They are the analogue of the Windows helpers' Get-CimInstance Win32_Process scripts (paneProcessTree, panePaneSubtree, tmuxSocketPids, pidClosure, hubHolders, serverProcCountForSession in smoke_test.go).
// Every read of the kernel's process table goes through the tmuxkit probes for the pid list, parent pid, argv, cwd and comm, so this file opens nothing under /proc itself and shells out to nothing.
// It is a small, self-contained test-harness copy rather than an import from reedengine, since that package's equivalents (parseStat, descendantClosure, matchSocketCmdlines) are unexported and only meaningful bound to an *Engine value.
// It is deliberately a _test.go file, not a _linux.go one:
// its caller functions in smoke_test.go compile on every GOOS (gated only by the `tmux || llm` tag) and runtime.GOOS-branch into this file's functions, so this file must compile everywhere too, and it references hubHolder, which is itself declared in smoke_test.go and so only exists inside the test binary.
// The tmuxkit probes are portable Go and simply report nothing on a non-Linux GOOS.

package reedcli

import (
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/testkit/tmuxkit"
)

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
	for _, pid := range tmuxkit.Pids() {
		if ppid, ok := tmuxkit.ProcPPID(pid); ok {
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
// reedengine's serverProcessesOnSocket.
func linuxTmuxSocketPids(binary, socket string) []int {
	var out []int
	for _, pid := range tmuxkit.Pids() {
		argv, ok := tmuxkit.ProcArgv(pid)
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
	for _, pid := range tmuxkit.Pids() {
		cwd, ok := tmuxkit.ProcCwd(pid)
		if !ok {
			continue
		}
		if cwd != dir && !strings.HasPrefix(cwd, dir+string(filepath.Separator)) {
			continue
		}
		name, _ := tmuxkit.ProcComm(pid)
		holders = append(holders, hubHolder{pid: pid, name: name})
	}
	return holders
}
