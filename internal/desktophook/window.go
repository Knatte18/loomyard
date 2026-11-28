// Package desktophook runs the machine's configured window command after lym opens a desktop window.
// An operator's own script, such as one that snaps the new window into the active desktop's tiling grid, can place it.
// It knows no window manager or desktop.
// It waits desktop.window_wait_ms for the new window to appear.
// It then runs desktop.window_command through the platform shell in the window's directory.
// With desktop.window_command empty, the template default, it waits for nothing and runs nothing.
// Both keys are machine keys, read from the hub's never-tracked machine override, so each machine names its own command.
// Under placement, the placement child runs desktop.window_command, with no wait.
// The one exception is a child ended by SIGTERM, which runs no command.
package desktophook

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/verifyrun"
)

// timing is the waiting and bounding applied after a window opens, held as a value so a test can shorten it.
type timing struct {
	// sleep waits the configured desktop.window_wait_ms before the command runs.
	sleep func(time.Duration)
	// commandTimeout bounds one run of the window command, so a hung script cannot hold up the verb that opened the window.
	commandTimeout time.Duration
	// commandWaitDelay bounds the output read after the command exits or is killed, so a child holding the pipe cannot hang.
	commandWaitDelay time.Duration
	// groupKillAfter is how long the command's process group has after its SIGTERM before it gets SIGKILL.
	groupKillAfter time.Duration
}

// productionTiming is the timing the window command runs under in production.
var productionTiming = timing{sleep: time.Sleep, commandTimeout: 30 * time.Second, commandWaitDelay: 5 * time.Second, groupKillAfter: 5 * time.Second}

// AfterWindow runs the window command the stack under dirs configures, once a window on dir has been opened.
// It waits desktop.window_wait_ms first, so the command sees the new window.
// It then runs desktop.window_command through the platform shell with dir as its working directory.
// The run is bounded by a timeout.
// With desktop.window_command empty or blank it returns nil at once.
// A config that fails to load returns an error.
// So does a command that fails to start, exits nonzero or times out; that error carries the command's output.
// The window is already open by then, so a caller only reports it.
func AfterWindow(dirs configengine.Dirs, dir string) error {
	return afterWindow(dirs, dir, productionTiming)
}

// afterWindow runs the window command for dir under the given timing, so a test can shorten the waiting.
func afterWindow(dirs configengine.Dirs, dir string, t timing) error {
	cfg, err := LoadConfig(dirs)
	if err != nil {
		return fmt.Errorf("desktophook: load desktop config: %w", err)
	}
	if strings.TrimSpace(cfg.WindowCommand) == "" {
		return nil
	}
	t.sleep(time.Duration(cfg.WindowWaitMs) * time.Millisecond)
	return runWindowCommand(context.Background(), cfg.WindowCommand, dir, false, t)
}

// AfterPlacement runs the window command the stack under dirs configures, for a window on dir already known to be up.
// It waits for nothing.
// It runs desktop.window_command through the platform shell with dir as its working directory.
// The run is bounded by a timeout.
// With desktop.window_command empty or blank it returns nil at once.
// A cancel of ctx ends the command and every process it started, and the returned error wraps ctx's error.
// A config that fails to load returns an error, as does a command that fails to start, exits nonzero or times out.
func AfterPlacement(ctx context.Context, dirs configengine.Dirs, dir string) error {
	return afterPlacement(ctx, dirs, dir, productionTiming)
}

// afterPlacement runs the window command for dir at once under the given timing.
func afterPlacement(ctx context.Context, dirs configengine.Dirs, dir string, t timing) error {
	cfg, err := LoadConfig(dirs)
	if err != nil {
		return fmt.Errorf("desktophook: load desktop config: %w", err)
	}
	if strings.TrimSpace(cfg.WindowCommand) == "" {
		return nil
	}
	return runWindowCommand(ctx, cfg.WindowCommand, dir, true, t)
}

// runWindowCommand runs command through the platform shell in dir and waits for it, within t.commandTimeout.
// With ownGroup the command has its own process group, which a cancel of ctx or the timeout ends as a whole.
// Without it the command stays in the caller's group, so a signal sent to that group reaches it,
// and a cancel or the timeout kills the command alone.
// On a platform with no process group the command alone is killed either way.
func runWindowCommand(ctx context.Context, command, dir string, ownGroup bool, t timing) error {
	runCtx, cancel := context.WithTimeout(ctx, t.commandTimeout)
	defer cancel()

	shellName, flag := verifyrun.ShellFor(runtime.GOOS)
	cmd := exec.CommandContext(runCtx, shellName, flag, command)
	cmd.Dir = dir
	cmd.WaitDelay = t.commandWaitDelay
	proc.HideWindow(cmd)
	awaitGone := func() {}
	if ownGroup {
		awaitGone = proc.ConfigureGroupTerminate(cmd, t.groupKillAfter, func(pid int, sig os.Signal, groupErr error) {
			logger.Info("desktophook: signalling the window command's process group", "pid", pid, "signal", sig, "command", command, "err", groupErr)
		})
	}

	logger.Info("desktophook: spawning the window command", "command", command, "dir", dir)
	output, err := cmd.CombinedOutput()
	awaitGone()
	logger.Info("desktophook: the window command ended", "command", command, "dir", dir, "err", err)
	if err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("desktophook: window command %q canceled: %w; output: %s", command, ctx.Err(), strings.TrimSpace(string(output)))
		}
		return fmt.Errorf("desktophook: window command %q: %w; output: %s", command, err, strings.TrimSpace(string(output)))
	}
	return nil
}
