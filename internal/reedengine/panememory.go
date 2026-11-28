// panememory.go moves a fresh strand pane into a memory-capped scope of the systemd user manager.

package reedengine

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/Knatte18/loomyard/internal/logger"
)

// defaultPaneMemoryMaxMiB is the pane memory cap, in MiB, a Config without a positive reed.pane_memory_max_mib takes.
const defaultPaneMemoryMaxMiB = 8192

// PaneScopeUnitPrefix starts the name of the scope a strand pane runs in; the pane's shell pid and ".scope" end it.
const PaneScopeUnitPrefix = "lym-pane-"

// paneMemoryMax returns cfg's pane memory cap in bytes, floored to the default for a zero or negative value.
func paneMemoryMax(cfg Config) int64 {
	mib := cfg.PaneMemoryMaxMiB
	if mib <= 0 {
		mib = defaultPaneMemoryMaxMiB
	}
	return int64(mib) << 20
}

// capPaneMemoryLocked moves the shell of paneID, the fresh pane of s, into a scope capped at the configured bytes.
// A runaway process the pane starts is then killed inside the scope, before the host runs out of memory,
// and the pane and its other processes live on.
// A pane the cap cannot reach still launches: the cause is logged at Warn and the launch goes on uncapped.
func (e *Engine) capPaneMemoryLocked(paneID string, s *Strand) {
	out, err := e.tmux.output("display-message", "-p", "-t", paneID, "#{pane_pid}")
	if err != nil {
		logger.Warn("reed: the pane runs with no memory cap, since its shell's pid could not be read", "strand", s.GUID, "pane", paneID, "err", err)
		return
	}
	pid, err := strconv.Atoi(strings.TrimSpace(out))
	if err != nil || pid <= 0 {
		logger.Warn("reed: the pane runs with no memory cap, since tmux named no shell pid for it", "strand", s.GUID, "pane", paneID, "answer", strings.TrimSpace(out))
		return
	}
	limit := paneMemoryMax(e.cfg)
	unit := fmt.Sprintf("%s%d.scope", PaneScopeUnitPrefix, pid)
	logger.Info("reed: moving the pane's shell into a memory-capped scope through busctl", "strand", s.GUID, "pane", paneID, "pid", pid, "scope", unit, "bytes", limit)
	cgroup, err := e.capMemory(pid, unit, limit)
	if err != nil {
		logger.Warn("reed: the pane runs with no memory cap", "strand", s.GUID, "name", s.Name, "pane", paneID, "pid", pid, "cause", err)
		return
	}
	logger.Info("reed: the pane runs in a memory-capped scope", "strand", s.GUID, "pane", paneID, "cgroup", cgroup, "bytes", limit)
}
