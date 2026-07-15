// proctree_windows.go implements the two Windows-only process-tree probes
// (descendantClosurePIDs, serverProcessesOnSocket) via the configured shell's
// (pwsh on Windows) Get-CimInstance Win32_Process table — the only reliable
// tmux-server liveness and parent-walk signal on this platform, since every
// tmux CLI probe (list-sessions, kill-server, has-session) exits identically
// with and without a server on the socket. These bodies are moved here
// verbatim from lifecycle.go so the pure helpers in proctree.go stay
// platform-agnostic while the OS I/O they used to embed lives in one thin,
// filename-suffixed seam; see proctree_linux.go for the /proc-backed
// counterpart.

package muxengine

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// descendantClosurePIDs expands roots to roots-plus-their-transitive-
// descendant pids. #{pane_pid} names only a pane's immediate launcher
// process, but on Windows psmux nests the real shell (and the strand command
// it runs) below it, and it is that deeper descendant whose cwd is the
// worktree directory — so every pane-destroying op (Down's kill-session,
// RemoveStrand's kill-pane) must reap the whole subtree, not just the pane
// pid, or a leftover grandchild keeps the worktree dir busy. The closure is
// computed in one Win32_Process pass through the configured shell (pwsh on
// Windows; the same reliable Windows process-table probe
// serverProcessesOnSocket uses). It falls back to the bare roots if the
// shell probe fails (including non-Windows, where Win32_Process does not
// exist) and returns nil for no roots. Callers must pass only roots that are
// still running (a dead pane's recorded pid may already have been reused by
// an unrelated process, and a closure over a reused pid would mark innocent
// processes for force-kill).
func (e *Engine) descendantClosurePIDs(roots []int) []int {
	if len(roots) == 0 {
		return nil
	}
	rootLiterals := make([]string, len(roots))
	for i, pid := range roots {
		rootLiterals[i] = strconv.Itoa(pid)
	}
	// Seed a set with the root pids, then repeatedly absorb any process whose
	// parent is already in the set — the transitive descendant closure.
	script := fmt.Sprintf(`$roots=@(%s)
$all=Get-CimInstance Win32_Process | Select-Object ProcessId,ParentProcessId
$acc=New-Object System.Collections.Generic.HashSet[int]
foreach($r in $roots){[void]$acc.Add([int]$r)}
$changed=$true
while($changed){$changed=$false;foreach($p in $all){if($acc.Contains([int]$p.ParentProcessId) -and -not $acc.Contains([int]$p.ProcessId)){[void]$acc.Add([int]$p.ProcessId);$changed=$true}}}
$acc`, strings.Join(rootLiterals, ","))
	out, err := exec.Command(e.cfg.Shell, "-NoProfile", "-NonInteractive", "-Command", script).Output()
	if err != nil {
		return roots
	}
	var pids []int
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if pid, err := strconv.Atoi(strings.TrimSpace(line)); err == nil && pid > 0 {
			pids = append(pids, pid)
		}
	}
	if len(pids) == 0 {
		return roots
	}
	return pids
}

// serverProcessesOnSocket returns the OS pids of every psmux.exe process
// whose command line names this engine's -L socket — the main server AND
// psmux's internal "__warm__" helper. It queries the Windows process table
// through the configured shell (pwsh on Windows, via Get-CimInstance
// Win32_Process), because that table is the ONLY reliable tmux-server
// liveness signal: every tmux CLI probe (list-sessions, kill-server,
// has-session) exits 0 or 1 identically with and without a server on the
// socket. Returns nil on any query failure
// (including non-Windows, where Win32_Process does not exist) — callers
// degrade to best-effort behavior rather than failing the op.
func (e *Engine) serverProcessesOnSocket() []int {
	script := fmt.Sprintf(
		`(Get-CimInstance Win32_Process -Filter "Name='psmux.exe'" | Where-Object { $_.CommandLine -match [regex]::Escape('-L %s') }).ProcessId`,
		e.Socket(),
	)
	out, err := exec.Command(e.cfg.Shell, "-NoProfile", "-NonInteractive", "-Command", script).Output()
	if err != nil {
		return nil
	}
	var pids []int
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		pid, err := strconv.Atoi(strings.TrimSpace(line))
		if err == nil && pid > 0 {
			pids = append(pids, pid)
		}
	}
	return pids
}
