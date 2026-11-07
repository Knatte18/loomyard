// smoke_proctree_test.go provides the process-tree probes the smoke harness uses on Linux.
// The probes read the process table only through tmuxkit,
// so this file opens nothing under /proc itself and shells out to nothing.
// It copies the process-tree logic rather than importing it from reedengine.
// Its equivalents there are unexported and bound to an *Engine value.
// It is a _test.go file, not a _linux.go one:
// its caller functions in smoke_test.go compile on every GOOS, gated only by the `tmux || llm` tag.
// They branch on runtime.GOOS into this file, so this file must compile everywhere too.
// It references hubHolder, which smoke_test.go declares, so both exist only inside the test binary.
// The tmuxkit probes are portable Go and report nothing on a non-Linux GOOS.

package reedcli

import (
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/testkit/tmuxkit"
)

// linuxDescendantClosure expands roots to roots-plus-their-transitive-descendants.
// It builds a pid-to-ppid map from the process table the tmuxkit probes report.
// It then repeatedly absorbs any pid whose parent is already accepted.
// It degrades to the bare roots if the process table cannot be read,
// so a caller reaping a pane's process subtree can still kill the roots it knows about.
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

// linuxArgvHasBinary reports whether argv contains binary's base name as one of its elements.
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

// linuxTmuxSocketPids returns the pids of every process whose argv names binary and a "-L socket" pair.
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

// linuxHubHolders returns every process whose current working directory is inside dir.
// Linux reports each cwd directly, so every holder it finds is a real leak.
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
