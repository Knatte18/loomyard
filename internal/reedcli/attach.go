// attach.go implements the `attach` reed verb, which hands the operator's terminal to the tmux session in place.
// Once the handover begins, stdio belongs to tmux and the verb writes no JSON, even on failure;
// it is the one registered JSON-envelope exception in this package,
// and `handOver` is declared per platform.

package reedcli

import (
	"errors"
	"os"
	"os/exec"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/reedengine"
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
itself with tmux, so no lym process outlives the handover; on Windows it runs
tmux as a child with the terminal inherited. attach boots
this worktree's session when none is up, rather than refusing. Every
fallible step (checking that tmux is at or above lym's minimum version, then
booting the session if it is cold, then checking that the server/session is
up) runs pre-flight and reports through the normal JSON
envelope; once the terminal handover begins, stdio belongs to tmux and
nothing further is written to it, even on success.
The handover also asks tmux to apply a layout computed for this terminal's
own size, chained onto the attach; when no terminal size is readable (a
piped stdout, no controlling terminal), the attach proceeds exactly as
before, with no chained layout.

Example:
  lym reed attach`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			out := cmd.OutOrStdout()

			// The floor check comes first.
			// A below-floor tmux is refused with the floor error before `EnsureSession` or `Status` runs tmux;
			// the refusal still reaches the envelope, because stdio has not been handed to tmux yet.
			if err := reedengine.CheckTmuxFloor(c.eng.TmuxPath()); err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}

			// The order is load-bearing: `EnsureSession` runs before `Status`.
			// `EnsureSession` boots the session of a cold worktree.
			// `Status` calls `requireSessionLocked`, which refuses when no session is up.
			// Running `Status` first would refuse a cold worktree before anything booted.
			// `Status` also reaches `loadOrInitStateLocked`, which refuses an unreadable `.lym/reed.json`.
			// The same read refuses a live foreign session.
			// `EnsureSession`'s early return cannot produce those refusals, since it reads no state.
			booted, err := c.eng.EnsureSession()
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}
			if booted {
				logger.Info("reedcli: attach booted a cold worktree's session", "socket", c.eng.Socket(), "session", c.eng.SessionName())
			}

			// Status failures, including the friendly no-session error, go on the envelope before stdio is touched.
			// After the handover no JSON can reach the caller.
			if _, err := c.eng.Status(); err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}

			// The watchdog spawn runs after the `Status` pre-flight and before `handOver` hands the operator's stdio to tmux.
			// A spawn placed after the handover would fire only once the operator detaches, which is the one moment it is useless.
			c.ensureWatchdogSpawned()

			// Read the operator's terminal size from stdout.
			// On error (piped output, no controlling terminal) the read neither reports on the envelope nor aborts;
			// `cols` and `rows` stay zero, which `AttachArgv` reads as no known size.
			cols, rows, err := term.GetSize(int(os.Stdout.Fd()))
			if err != nil {
				logger.Warn("reed: no terminal size available, attaching without a chained layout", "err", err)
				cols, rows = 0, 0
			}

			// Terminal-handover tail: once the handover begins, stdio belongs to tmux, so no JSON is written here, even on failure.
			// tmux's own stderr already reached the operator's terminal.
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
// A child that ran and exited non-zero returns its code and a nil error;
// one that could not start returns 1 and the error.
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
