// spawnwatchdog.go implements ensureWatchdogSpawned, the best-effort spawn attempt up, resume and
// attach each make after their own engine op returns without error: it re-execs this same binary
// as `lyx reed watchdog --hub-path <hub> --tmux <tmux>`, detached, so the spawned process outlives
// this one and hosts the per-hub resize self-heal daemon for every worktree on the hub.

package reedcli

import (
	"os"
	"os/exec"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/proc"
)

// ensureWatchdogSpawned attempts to spawn the per-hub watchdog daemon detached, best-effort.
//
// It returns immediately when suppressWatchdogSpawn is set (a test binary, where re-exec'ing
// os.Executable() would run the whole suite recursively) or when hubPath is empty (this reedCLI
// was never given a hub, e.g. the watchdog verb's own PersistentPreRunE early return).
//
// A spawn failure is never fatal to the caller's own operation: up, resume and attach each already
// succeeded at their own engine op by the time this runs, and the daemon is a convenience the
// operator can always start by hand (running `lyx reed watchdog` in the foreground) if this
// best-effort spawn does not land.
func (c *reedCLI) ensureWatchdogSpawned() {
	if c.suppressWatchdogSpawn || c.hubPath == "" {
		return
	}

	// lock.TryAcquireWriteLock does not create the lock file's parent directory, and a hub that has
	// never booted a reed server may not have HubScratchDir yet.
	scratchDir := fabricengine.HubScratchDir(c.hubPath)
	if err := os.MkdirAll(scratchDir, 0o755); err != nil {
		logger.Warn("reed: could not create hub scratch dir, skipping watchdog spawn", "hub", c.hubPath, "path", scratchDir, "err", err)
		return
	}

	exe, err := os.Executable()
	if err != nil {
		logger.Warn("reed: could not resolve this binary, skipping watchdog spawn", "hub", c.hubPath, "err", err)
		return
	}

	cmd := exec.Command(exe, "reed", "watchdog", "--hub-path", c.hubPath, "--tmux", c.eng.TmuxPath())
	// The daemon is per-hub and outlives the worktree that spawned it. On Windows a held cwd handle
	// on a worktree directory blocks that directory's deletion, which would break fabric teardown —
	// pinning cmd.Dir to the hub instead avoids that entirely.
	cmd.Dir = c.hubPath
	// Leave stdin/stdout/stderr nil so no parent handles are inherited.
	proc.Detach(cmd)

	logger.Info("reed: spawning detached per-hub watchdog", "exe", exe, "hub", c.hubPath, "tmux", c.eng.TmuxPath())
	if err := cmd.Start(); err != nil { // intentionally not Wait()ed: a detached Start with no Wait
		// logs the spawn alone, since there is no teardown to log.
		logger.Warn("reed: watchdog spawn failed", "exe", exe, "hub", c.hubPath, "err", err)
	}
}
