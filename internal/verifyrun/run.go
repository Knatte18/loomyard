// Package verifyrun runs one shell command in-process and reports its exit code.
//
// It is the single shell runner shared by plan-verify (through internal/verifytree) and the per-card `**Verify:**` rerun,
// so shell selection lives in one place.
// It cannot live in internal/shell: that package is stdlib-only under the Shell Mechanics Seam, and this one logs through internal/logger.
// It imports only the standard library, internal/logger and internal/proc.
package verifyrun

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"runtime"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/proc"
)

// waitDelay bounds Wait's read after the shell exits or its context is cancelled,
// so a child still holding the output pipe cannot hang the call.
// It mirrors internal/treadleengine's gateWaitDelay.
const waitDelay = 10 * time.Second

// Run runs command through the platform shell (sh -c, or cmd /C on Windows) with dir as its working directory.
// Stdout and stderr both go to out.
// An out of io.Discard is left as nil so the child writes to the null device directly, with no copy pipe a lingering child could hold open.
//
// A nil env runs the shell with the parent's environment; a non-nil env is the shell's whole environment.
//
// A zero exit returns (0, nil), and a non-zero exit returns (exitCode, nil).
// A cancelled ctx returns (-1, err) with err wrapping ctx.Err(), after killing the shell:
// on Unix the shell's whole process group, elsewhere the shell alone.
// A shell that could not start returns (-1, err) naming the command.
func Run(ctx context.Context, command, dir string, env []string, out io.Writer) (int, error) {
	return run(ctx, command, dir, env, out, waitDelay)
}

// run is Run with the wait delay as a parameter, so a test can shorten the wait for a lingering pipe holder.
func run(ctx context.Context, command, dir string, env []string, out io.Writer, delay time.Duration) (int, error) {
	shellName, flag := "sh", "-c"
	if runtime.GOOS == "windows" {
		shellName, flag = "cmd", "/C"
	}

	cmd := exec.CommandContext(ctx, shellName, flag, command)
	cmd.Dir = dir
	cmd.Env = env
	cmd.WaitDelay = delay
	proc.ConfigureGroupKill(cmd, func(pid int, groupErr error) {
		switch {
		case groupErr == nil:
			logger.Info("verifyrun: killing verify process group", "pid", pid, "command", command)
		case errors.Is(groupErr, proc.ErrNoProcessGroup):
			logger.Warn("verifyrun: killed only the verify shell; its descendants may outlive it", "pid", pid, "command", command)
		default:
			logger.Warn("verifyrun: process group kill failed, killing the shell only", "pid", pid, "command", command, "cause", groupErr)
		}
	})
	if out != io.Discard {
		cmd.Stdout = out
		cmd.Stderr = out
	}

	logger.Info("verifyrun: spawning verify command", "shell", shellName, "command", command, "dir", dir)
	err := cmd.Run()
	if err == nil {
		logger.Info("verifyrun: verify command exited", "command", command, "dir", dir, "exitCode", 0)
		return 0, nil
	}

	if ctxErr := ctx.Err(); ctxErr != nil {
		logger.Info("verifyrun: verify command cancelled", "command", command, "dir", dir, "cause", ctxErr)
		return -1, fmt.Errorf("verifyrun: run verify command %q: %w", command, ctxErr)
	}

	if errors.Is(err, exec.ErrWaitDelay) && cmd.ProcessState != nil {
		code := cmd.ProcessState.ExitCode()
		logger.Info("verifyrun: verify command exited", "command", command, "dir", dir, "exitCode", code)
		return code, nil
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		logger.Info("verifyrun: verify command exited", "command", command, "dir", dir, "exitCode", exitErr.ExitCode())
		return exitErr.ExitCode(), nil
	}

	logger.Warn("verifyrun: verify command failed to spawn", "command", command, "dir", dir, "cause", err)
	return -1, fmt.Errorf("verifyrun: run verify command %q: %w", command, err)
}
