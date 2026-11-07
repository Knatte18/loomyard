// proctree_windows.go implements the process-tree probes for Windows through the configured shell's Win32_Process table.
// That table is the only reliable tmux-server liveness and parent-walk signal here.
// Every tmux CLI probe exits the same with or without a server, so the CLI cannot serve as that signal.

package reedengine

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
)

// descendantClosurePIDs expands roots to their transitive descendants via Win32_Process.
// Pane teardown must reap the full subtree, because the shell nests below the launcher.
// Falls back to bare roots on probe failure.
func (e *Engine) descendantClosurePIDs(roots []int) []int {
	if len(roots) == 0 {
		return nil
	}
	rootLiterals := make([]string, len(roots))
	for i, pid := range roots {
		rootLiterals[i] = strconv.Itoa(pid)
	}
	// Seed a set with the root pids, then repeatedly absorb any process whose parent is already in the set:
	// the transitive descendant closure.
	script := fmt.Sprintf(`$roots=@(%s)
$all=Get-CimInstance Win32_Process | Select-Object ProcessId,ParentProcessId
$acc=New-Object System.Collections.Generic.HashSet[int]
foreach($r in $roots){[void]$acc.Add([int]$r)}
$changed=$true
while($changed){$changed=$false;foreach($p in $all){if($acc.Contains([int]$p.ParentProcessId) -and -not $acc.Contains([int]$p.ProcessId)){[void]$acc.Add([int]$p.ProcessId);$changed=$true}}}
$acc`, strings.Join(rootLiterals, ","))
	logger.Debug("reedengine: spawning process-tree probe", "shell", e.cfg.Shell, "roots", roots)
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

// paneProcessAge answers 0 and false on Windows, since the liveness rule that reads a pane's age is skipped there.
func paneProcessAge(pid int) (time.Duration, bool) {
	return 0, false
}

// sessionMemberPIDs returns nil: Windows has no POSIX sessions,
// so a pane's processes are reaped through the descendant snapshot alone.
func sessionMemberPIDs(sessions []int) []int {
	return nil
}

// serverProcessesOnSocket returns OS pids on this engine's socket, read through Get-CimInstance Win32_Process.
// That query is the only reliable liveness signal on Windows.
// Returns nil on query failure; callers degrade to best-effort behavior.
func (e *Engine) serverProcessesOnSocket() []int {
	// The Name filter must match the configured binary: any other name finds no server,
	// so the probe reports the socket as clear and the server leaks.
	script := fmt.Sprintf(
		`(Get-CimInstance Win32_Process -Filter "Name='%s'" | Where-Object { $_.CommandLine -match [regex]::Escape('-L %s') }).ProcessId`,
		tmuxProcessName(e.cfg.Tmux),
		e.Socket(),
	)
	logger.Debug("reedengine: spawning process-tree probe", "shell", e.cfg.Shell, "socket", e.Socket())
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
