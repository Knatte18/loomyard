// Package desktophook runs the machine's configured window command after lym opens a desktop window, so an operator's own script, such as one that snaps the new window into the active desktop's tiling grid, can place it.
// It knows no window manager or desktop: it waits desktop.yaml's window_wait_ms for the new window to appear, then runs window_command through the platform shell in the window's directory.
// With window_command empty, the template default, it waits for nothing and runs nothing.
// Both keys are machine keys, read from the hub's never-tracked machine override, so each machine names its own command.
package desktophook

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/verifyrun"
)

// timing is the waiting and bounding AfterWindow applies, held as a value so a test can shorten it.
type timing struct {
	// sleep waits the configured window_wait_ms before the command runs.
	sleep func(time.Duration)
	// commandTimeout bounds one run of the window command, so a script that hangs cannot hold up the verb that opened the window.
	commandTimeout time.Duration
	// commandWaitDelay bounds the read of the command's output after it exits or is killed, so a child still holding the pipe cannot hang the run.
	commandWaitDelay time.Duration
}

// productionTiming is the timing AfterWindow runs with.
var productionTiming = timing{sleep: time.Sleep, commandTimeout: 30 * time.Second, commandWaitDelay: 5 * time.Second}

// AfterWindow runs the window command the stack under dirs configures, once a window on dir has been opened.
// It waits window_wait_ms first, so the command sees the new window, then runs window_command through the platform shell with dir as its working directory, bounded by a timeout.
// With window_command empty or blank it returns nil at once.
// A config that fails to load, and a command that fails to start, exits nonzero or times out, return an error carrying the command's output;
// the window is already open by then, so a caller only reports it.
func AfterWindow(dirs configengine.Dirs, dir string) error {
	return afterWindow(dirs, dir, productionTiming)
}

// afterWindow is AfterWindow under the given timing.
func afterWindow(dirs configengine.Dirs, dir string, t timing) error {
	cfg, err := LoadConfig(dirs)
	if err != nil {
		return fmt.Errorf("desktophook: load desktop config: %w", err)
	}
	if strings.TrimSpace(cfg.WindowCommand) == "" {
		return nil
	}
	t.sleep(time.Duration(cfg.WindowWaitMs) * time.Millisecond)
	return runWindowCommand(cfg.WindowCommand, dir, t)
}

// runWindowCommand runs command through the platform shell in dir and waits for it, within t.commandTimeout.
func runWindowCommand(command, dir string, t timing) error {
	ctx, cancel := context.WithTimeout(context.Background(), t.commandTimeout)
	defer cancel()

	shellName, flag := verifyrun.ShellFor(runtime.GOOS)
	cmd := exec.CommandContext(ctx, shellName, flag, command)
	cmd.Dir = dir
	cmd.WaitDelay = t.commandWaitDelay
	proc.HideWindow(cmd)

	logger.Info("desktophook: spawning the window command", "command", command, "dir", dir)
	output, err := cmd.CombinedOutput()
	logger.Info("desktophook: the window command ended", "command", command, "dir", dir, "err", err)
	if err != nil {
		return fmt.Errorf("desktophook: window command %q: %w; output: %s", command, err, strings.TrimSpace(string(output)))
	}
	return nil
}
