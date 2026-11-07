// Package verifyrun runs one shell command in-process and reports its exit code.
//
// It is the one shell runner for plan-verify (through internal/verifytree) and the per-card `**Verify:**` rerun.
// So shell selection lives in one place.
// The quoting of one literal argument for that shell, QuoteArg, sits beside the shell choice so the two cannot differ.
// It cannot live in internal/shell: that package is stdlib-only under the Shell Mechanics Seam,
// and this one logs through internal/logger.
// RunTied runs a command like Run for a caller that records the shell's process group.
// That caller wants the shell to die with the calling process.
// It imports only the standard library, internal/logger and internal/proc.
package verifyrun

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/proc"
)

// waitDelay bounds Wait's read after the shell exits or its context is cancelled,
// so a child still holding the output pipe cannot hang the call.
// Its value matches the gate wait in internal/treadleengine.
const waitDelay = 10 * time.Second

// Run runs command through the platform shell (sh -c, or cmd /C on Windows) with dir as its working directory.
// Stdout and stderr both go to out.
// An out equal to io.Discard is left as nil, so the child writes to the null device directly.
// That leaves no copy pipe a lingering child could hold open.
//
// A nil env runs the shell with the parent's environment; a non-nil env is the shell's whole environment.
//
// A zero exit returns (0, nil), and a non-zero exit returns (exitCode, nil).
// A cancelled ctx returns (-1, err) with err wrapping ctx.Err().
// On Unix the cancel sends SIGTERM to the shell's whole process group, then SIGKILL to it after the wait delay.
// Run returns only after the group is gone or that SIGKILL is sent.
// Elsewhere the cancel kills the shell alone.
// A shell that could not start returns (-1, err) naming the command.
func Run(ctx context.Context, command, dir string, env []string, out io.Writer) (int, error) {
	return run(ctx, command, dir, env, out, waitDelay)
}

// RunTied runs a command tied to the calling process's life.
// On Linux the shell gets a parent-death SIGKILL, so it dies when the calling process does;
// elsewhere the caller's own job ends the tree.
// started is called with the shell's pid, process-group id and start time once the shell runs and before the wait;
// so the caller can record the group.
func RunTied(ctx context.Context, command, dir string, env []string, out io.Writer, started func(proc.TreeRecord)) (int, error) {
	return runTied(ctx, command, dir, env, out, waitDelay, started)
}

// ErrPercentUnderCmd is returned by QuoteArg for a Windows value holding a percent sign:
// cmd expands %NAME% even inside double quotes, so no quoting makes such a value one literal argument.
var ErrPercentUnderCmd = errors.New("verifyrun: cmd expands % inside quotes; a value holding % cannot be passed as one literal argument")

// ShellFor returns the shell and its command flag for goos: sh -c everywhere but Windows, which gets cmd /C.
// A caller that spawns a command itself starts the same shell this way.
func ShellFor(goos string) (name, flag string) {
	if goos == "windows" {
		return "cmd", "/C"
	}
	return "sh", "-c"
}

// QuoteArg quotes value as one literal argument for the shell that goos selects,
// so a caller can splice it into a shell command.
// For sh it wraps the value in single quotes, writing each embedded single quote as '\”.
// For cmd it wraps the value in double quotes, doubling each embedded double quote;
// a value holding a percent sign returns ErrPercentUnderCmd and no quoted text.
func QuoteArg(goos, value string) (string, error) {
	if goos == "windows" {
		if strings.Contains(value, "%") {
			return "", ErrPercentUnderCmd
		}
		return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`, nil
	}
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'", nil
}

// run runs command through the platform shell with delay as the wait delay,
// so a test can shorten the wait for a lingering pipe holder.
func run(ctx context.Context, command, dir string, env []string, out io.Writer, delay time.Duration) (int, error) {
	return runTied(ctx, command, dir, env, out, delay, nil)
}

// startAndWaitTied starts cmd and waits for it on one goroutine locked to its OS thread.
// The parent-death signal follows the thread that started the child.
// started is called with the child's record once it runs, before the wait.
func startAndWaitTied(cmd *exec.Cmd, started func(proc.TreeRecord)) error {
	result := make(chan error, 1)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		if err := cmd.Start(); err != nil {
			result <- err
			return
		}
		pid := cmd.Process.Pid
		startTime, _ := proc.StartTime(pid)
		started(proc.TreeRecord{PID: pid, PGID: pid, StartTime: startTime})
		result <- cmd.Wait()
	}()
	return <-result
}

// runTied runs command through the platform shell with delay as the wait delay.
// A non-nil started ties the shell to the calling process and is called with its record before the wait.
// A nil started leaves the shell untied.
func runTied(ctx context.Context, command, dir string, env []string, out io.Writer, delay time.Duration, started func(proc.TreeRecord)) (int, error) {
	shellName, flag := ShellFor(runtime.GOOS)

	cmd := exec.CommandContext(ctx, shellName, flag, command)
	cmd.Dir = dir
	cmd.Env = env
	cmd.WaitDelay = delay
	awaitGone := proc.ConfigureGroupTerminate(cmd, delay, func(pid int, sig os.Signal, groupErr error) {
		switch {
		case groupErr == nil && sig == syscall.SIGTERM:
			logger.Info("verifyrun: sending SIGTERM to verify process group", "pid", pid, "command", command)
		case groupErr == nil:
			logger.Info("verifyrun: sending SIGKILL after the wait delay to verify process group", "pid", pid, "command", command)
		case errors.Is(groupErr, proc.ErrNoProcessGroup):
			logger.Warn("verifyrun: killed only the verify shell; its descendants may outlive it", "pid", pid, "command", command)
		default:
			logger.Warn("verifyrun: signalling the verify process group failed", "pid", pid, "signal", sig, "command", command, "cause", groupErr)
		}
	})
	if out != io.Discard {
		cmd.Stdout = out
		cmd.Stderr = out
	}

	logger.Info("verifyrun: spawning verify command", "shell", shellName, "command", command, "dir", dir)
	var err error
	if started == nil {
		err = cmd.Run()
	} else {
		tieToParent(cmd)
		err = startAndWaitTied(cmd, started)
	}
	awaitGone()
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
