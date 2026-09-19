// spawnwatchdog.go owns the per-hub watchdog daemon's detached spawn for every caller: reedcli's
// up/attach/resume, and (once internal/loomcli wires it in) loom's own start path. The daemon is
// per-hub and deliberately outlives any one worktree's Engine — that is why cmd.Dir is pinned to
// the hub below, and why SpawnWatchdog is a package-level function rather than an Engine method.
// suppress is passed in rather than computed here so the "never re-exec os.Executable() under
// go test" guard stays visible at each call site, instead of being buried inside this file.
//
// internal/reedengine is also the standalone engine's own package, and this function is inert
// unless called with a non-empty hubPath, which standalone never does.

package reedengine

import (
	"os"
	"os/exec"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/proc"
)

// SpawnWatchdog attempts to spawn the per-hub watchdog daemon detached, best-effort.
//
// It returns immediately when suppress is set (a test binary, where re-exec'ing os.Executable()
// would run the whole suite recursively) or when hubPath is empty (this caller was never given a
// hub, e.g. the watchdog verb's own PersistentPreRunE early return).
//
// A spawn failure is never fatal to the caller's own operation: up, resume and attach each already
// succeeded at their own engine op by the time this runs, and the daemon is a convenience the
// operator can always start by hand (running `lyx reed watchdog` in the foreground) if this
// best-effort spawn does not land.
func SpawnWatchdog(hubPath, tmuxPath string, suppress bool) {
	if suppress || hubPath == "" {
		return
	}

	// lock.TryAcquireWriteLock does not create the lock file's parent directory, and a hub that has
	// never booted a reed server may not have HubScratchDir yet.
	scratchDir := fabricengine.HubScratchDir(hubPath)
	if err := os.MkdirAll(scratchDir, 0o755); err != nil {
		logger.Warn("reed: could not create hub scratch dir, skipping watchdog spawn", "hub", hubPath, "path", scratchDir, "err", err)
		return
	}

	exe, err := os.Executable()
	if err != nil {
		logger.Warn("reed: could not resolve this binary, skipping watchdog spawn", "hub", hubPath, "err", err)
		return
	}

	cmd := exec.Command(exe, "reed", "watchdog", "--hub-path", hubPath, "--tmux", tmuxPath)
	// The daemon is per-hub and outlives the worktree that spawned it. On Windows a held cwd handle
	// on a worktree directory blocks that directory's deletion, which would break fabric teardown —
	// pinning cmd.Dir to the hub instead avoids that entirely.
	cmd.Dir = hubPath
	// Leave stdin/stdout/stderr nil so no parent handles are inherited.
	proc.Detach(cmd)

	logger.Info("reed: spawning detached per-hub watchdog", "exe", exe, "hub", hubPath, "tmux", tmuxPath)
	if err := cmd.Start(); err != nil { // intentionally not Wait()ed: a detached Start with no Wait
		// logs the spawn alone, since there is no teardown to log.
		logger.Warn("reed: watchdog spawn failed", "exe", exe, "hub", hubPath, "err", err)
	}
}
