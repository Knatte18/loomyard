// spawnwatchdog.go owns the detached spawn of the per-hub watchdog daemon for every caller that starts it.
// The daemon is per-hub and outlives any one worktree's Engine.
// That is why cmd.Dir is pinned to the hub below, and why SpawnWatchdog is a package-level function.
// suppress is passed in by the caller,
// so the guard against re-executing os.Executable() under go test stays visible at each call site.
//
// internal/reedengine is also the standalone engine's own package.
// SpawnWatchdog does nothing unless hubPath is non-empty, which the standalone engine never passes.

package reedengine

import (
	"os"
	"os/exec"
	"strings"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/proc"
)

// SpawnWatchdog attempts to spawn the per-hub watchdog daemon detached, best-effort.
//
// It returns immediately when suppress is set, or when hubPath is empty.
// A test binary sets suppress, because re-executing os.Executable() there would run the whole suite recursively.
//
// shellPath is passed unconditionally, empty or not.
// The spawn is best-effort, and nobody reads the child's stderr.
// Refusing an empty shell here would silently cost the hub its whole watchdog daemon over one empty config value.
// An empty shell costs only the descendant half of a reap on Windows.
//
// The child's nil stdio gives it the null device,
// and its environment drops LYM_LOG_FILE so the operator's log file cannot reroute the daemon's stderr half.
// LYM_LOG_LEVEL passes on unchanged, so a spawn run at Debug verbosity starts a daemon at Debug.
//
// A spawn failure is never fatal to the caller's own operation, which has already succeeded by the time this runs.
// The operator can always start the daemon by hand with `lym reed watchdog` in the foreground.
func SpawnWatchdog(hubPath, tmuxPath, shellPath string, suppress bool) {
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

	cmd := exec.Command(exe, "reed", "watchdog", "--hub-path", hubPath, "--tmux", tmuxPath, "--shell", shellPath)
	// The daemon is per-hub and outlives the worktree that spawned it.
	// On Windows a held cwd handle on a worktree directory blocks that directory's deletion.
	// That would break fabric teardown, so pinning cmd.Dir to the hub avoids it entirely.
	cmd.Dir = hubPath
	cmd.Env = spawnWatchdogEnv(os.Environ())
	// Leave stdin/stdout/stderr nil so no parent handles are inherited.
	proc.Detach(cmd)

	logger.Info("reed: spawning detached per-hub watchdog", "exe", exe, "hub", hubPath, "tmux", tmuxPath, "shell", shellPath)
	// A detached spawn logs the spawn alone, since there is no teardown to log.
	if err := cmd.Start(); err != nil {
		logger.Warn("reed: watchdog spawn failed", "exe", exe, "hub", hubPath, "err", err)
		return
	}
	proc.Reap(cmd)
}

// spawnWatchdogEnv returns environ without any entry whose key is exactly LYM_LOG_FILE.
func spawnWatchdogEnv(environ []string) []string {
	out := make([]string, 0, len(environ))
	for _, entry := range environ {
		if strings.SplitN(entry, "=", 2)[0] == "LYM_LOG_FILE" {
			continue
		}
		out = append(out, entry)
	}
	return out
}
