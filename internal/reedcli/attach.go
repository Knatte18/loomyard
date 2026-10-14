// attach.go implements the `attach` reed verb: an in-place terminal handover into the tmux session
// — no new window is spawned.
// attach is the one registered JSON-envelope exception in this package: every fallible step runs
// pre-flight on the envelope,
// but the terminal-handover tail (once stdio belongs to tmux) emits no JSON
// on success.
// The handover itself is handOver, declared once per platform.
// It execs tmux over this process on Unix and runs it as a child process on Windows.
// The handover argv itself comes from reedengine.Engine.AttachArgv, not a package-level builder here:
// this file's only remaining pre-flight addition is reading the operator's own terminal size via
// golang.org/x/term and handing it to the engine's builder. That size read and the builder call are
// both pre-flight steps that nonetheless never write to the envelope — each degrades to today's bare
// argv and logs a warning instead, so c.eng.EnsureSession() and c.eng.Status() remain the only two
// pre-flight steps that can abort with an envelope error.

package reedcli

import (
	"errors"
	"os"
	"os/exec"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// attachCmd builds the `attach` subcommand, handing the operator's terminal to tmux attach-session.
func (c *reedCLI) attachCmd() *cobra.Command {
	return &cobra.Command{
		Use:         "attach",
		Short:       "attach the operator's terminal to the reed session in place",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `attach hands the operator's own terminal over to tmux attach-session, in
place — no new window is spawned (never wt.exe). On Unix attach replaces
itself with tmux, so no lyx process outlives the handover; on Windows it runs
tmux as a child with the terminal inherited. attach boots
this worktree's session when none is up, rather than refusing. Every
fallible step (booting the session if it is cold, then checking that the
server/session is up) runs pre-flight and reports through the normal JSON
envelope; once the terminal handover begins, stdio belongs to tmux and
nothing further is written to it, even on success.
The handover also asks tmux to apply a layout computed for this terminal's
own size, chained onto the attach; when no terminal size is readable (a
piped stdout, no controlling terminal), the attach proceeds exactly as
before, with no chained layout.

Example:
  lyx reed attach`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			out := cmd.OutOrStdout()

			// Pre-flight, in a load-bearing order: EnsureSession first, Status
			// second. EnsureSession self-heals a cold worktree by booting the
			// session; Status calls requireSessionLocked, which refuses when no
			// session is up, so running Status first would refuse on a cold
			// worktree before anything booted, reinstating the exact bug this
			// task removes. Status is kept rather than replaced because it also
			// reaches loadOrInitStateLocked, which is what makes a warm attach
			// refuse on an unreadable .lyx/reed.json and on a live foreign
			// session — refusals EnsureSession's early return cannot produce,
			// since it reads no state at all.
			booted, err := c.eng.EnsureSession()
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}
			if booted {
				logger.Info("reedcli: attach booted a cold worktree's session", "socket", c.eng.Socket(), "session", c.eng.SessionName())
			}

			// Surface the friendly no-session error (see
			// reedengine.requireSessionLocked/noSessionMessage), or any other
			// Status failure, on the envelope before ever touching stdio, since
			// after the handover below no JSON can reach the caller.
			if _, err := c.eng.Status(); err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}

			// Attempted after the Status pre-flight and BEFORE attach.Run() hands the operator's
			// stdio over: a spawn placed after the handover would fire only once the operator
			// detaches, which is the one moment it is useless.
			c.ensureWatchdogSpawned()

			// Read the operator's own terminal size against stdout. On error (piped
			// output, no controlling terminal) this does not report on the envelope
			// and does not abort: AttachArgv answers a non-positive cols/rows with
			// the bare argv, exactly today's behaviour, so nothing regresses on a
			// non-TTY.
			cols, rows, err := term.GetSize(int(os.Stdout.Fd()))
			if err != nil {
				logger.Warn("reed: no terminal size available, attaching without a chained layout", "err", err)
				cols, rows = 0, 0
			}

			// Terminal-handover tail: stdio now belongs to tmux, so this is the one documented exception to "every RunE ends in a JSON envelope".
			// No JSON is written here even on failure, since tmux's own stderr already reached the operator's terminal.
			// The exit code still propagates, so a failed attach is not reported as success.
			exitCode, err := handOver(c.eng.TmuxPath(), c.eng.AttachArgv(cols, rows))
			if err != nil {
				logger.Warn("reedcli: tmux attach handover failed", "tmux", c.eng.TmuxPath(), "err", err)
			}
			if exitCode != 0 {
				clihelp.SetExit(cmd.Context(), exitCode)
			}
			return nil
		},
	}
}

// runAttachChild runs tmux with args as a child that inherits the operator's stdio and returns the child's exit code.
// A child that ran and exited non-zero returns its code and a nil error; one that could not start returns 1 and the error.
func runAttachChild(tmux string, args []string) (exitCode int, err error) {
	attach := exec.Command(tmux, args...)
	attach.Stdin = os.Stdin
	attach.Stdout = os.Stdout
	attach.Stderr = os.Stderr

	logger.Info("reedcli: spawning tmux attach", "tmux", tmux, "args", args)
	runErr := attach.Run()
	var exitErr *exec.ExitError
	switch {
	case runErr == nil:
	case errors.As(runErr, &exitErr):
		exitCode = exitErr.ExitCode()
	default:
		exitCode, err = 1, runErr
	}
	logger.Info("reedcli: tmux attach exited", "tmux", tmux, "exitCode", exitCode)
	return exitCode, err
}
