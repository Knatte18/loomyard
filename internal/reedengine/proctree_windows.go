// proctree_windows.go implements the two Windows-only process-tree probes (descendantClosurePIDs,
// serverProcessesOnSocket) via the configured shell's (pwsh on Windows) Get-CimInstance
// Win32_Process table — the only reliable tmux-server liveness and parent-walk signal on this
// platform, since every tmux CLI probe (list-sessions, kill-server, has-session) exits identically
// with and without a server on the socket.
// These bodies are moved here verbatim from lifecycle.go so the pure helpers in proctree.go stay
// platform-agnostic while the OS I/O they used to embed lives in one thin, filename-suffixed seam;
// see proctree_linux.go for the /proc-backed counterpart.

package reedengine

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/Knatte18/loomyard/internal/logger"
)

// descendantClosurePIDs expands roots to transitive descendants via Win32_Process.
// Pane-destroying ops must reap the full subtree (shell nests below launcher).
// Falls back to bare roots on probe failure.
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

// serverProcessesOnSocket returns OS pids on this engine's socket via
// Get-CimInstance Win32_Process (the only reliable liveness signal on Windows).
// Returns nil on query failure; callers degrade to best-effort behavior.
func (e *Engine) serverProcessesOnSocket() []int {
	// The Name filter must name the CONFIGURED binary: a hardcoded
	// 'psmux.exe' matched nothing on a machine whose config resolves tmux
	// to tmux.exe, so Down saw every socket as clear and leaked the server.
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
