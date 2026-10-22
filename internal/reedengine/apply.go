// apply.go holds the pane-id sets the engine's reconcile and resume paths test membership against:
// the panes present in the window, the panes not dead, and the panes whose strand's engine still runs.

package reedengine

import (
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// launchGrace is how long a pane's first process may show only its start shell before the strand reads dead.
// A pane just launched shows the shell until the engine takes the foreground.
const launchGrace = 30 * time.Second

// liveIDSet builds the set of pane ids present in live, alive or dead-but-remain-on-exit.
// That is "present in the window", not "still running".
func liveIDSet(live []LivePane) map[string]bool {
	ids := make(map[string]bool, len(live))
	for _, p := range live {
		ids[p.ID] = true
	}
	return ids
}

// aliveIDSet builds the set of pane ids that are present AND not dead.
// That is the pane-level "still running" set, as distinct from liveIDSet's "present in the window" set.
// The pane-level checks use it (Selvage health, pane options, title repair), since a pane running only its shell is a healthy pane there.
// A strand's liveness is engineAliveIDs' judgment instead.
func aliveIDSet(live []LivePane) map[string]bool {
	ids := make(map[string]bool, len(live))
	for _, p := range live {
		if !p.Dead {
			ids[p.ID] = true
		}
	}
	return ids
}

// engineAliveIDs builds the set of pane ids whose strand's engine still runs: the panes of present that are not dead, less every shell-only pane past the launch grace.
// A pane is shell-only when checkShell is true, its start and foreground commands are both known and name the same shell, and age reports its first process at least launchGrace old.
// An empty command skips that rule, and an age that cannot be read counts as inside the grace, so a failed read never reads a pane dead.
// age is called only for a pane whose commands match.
func engineAliveIDs(present []LivePane, age func(pid int) (time.Duration, bool), checkShell bool) map[string]bool {
	ids := make(map[string]bool, len(present))
	for _, p := range present {
		if p.Dead {
			continue
		}
		if checkShell && p.StartCommand != "" && p.CurrentCommand != "" && shellBaseName(p.StartCommand) == shellBaseName(p.CurrentCommand) {
			if paneAge, ok := age(p.PID); ok && paneAge >= launchGrace {
				continue
			}
		}
		ids[p.ID] = true
	}
	return ids
}

// shellBaseName returns the executable name of a pane command: spaces and one pair of surrounding quotes trimmed, the first word's base name, and one leading '-' dropped.
// So the login shell "-bash" and the pin "/bin/bash" (or "\"/usr/bin/zsh -l\"") name the same shell.
func shellBaseName(command string) string {
	command = strings.TrimSpace(command)
	if len(command) >= 2 && (command[0] == '"' || command[0] == '\'') && command[len(command)-1] == command[0] {
		command = command[1 : len(command)-1]
	}
	fields := strings.Fields(command)
	if len(fields) == 0 {
		return ""
	}
	return strings.TrimPrefix(filepath.Base(fields[0]), "-")
}

// engineAliveIDSet is engineAliveIDs over this engine's pane-age probe.
// The shell-only rule is skipped on Windows, where a PowerShell cmdlet runs inside the shell.
func (e *Engine) engineAliveIDSet(present []LivePane) map[string]bool {
	return engineAliveIDs(present, e.paneAge, runtime.GOOS != "windows")
}
