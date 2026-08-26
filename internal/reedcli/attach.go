// attach.go implements the `attach` reed verb: an in-place terminal handover into the tmux session
// — no new window is spawned.
// attach is the one registered JSON-envelope exception in this package: every fallible step runs
// pre-flight on the envelope,
// but the terminal-handover tail (once stdio is inherited by the child tmux process) emits no JSON
// on success.
// The handover argv itself comes from reedengine.Engine.AttachArgv, not a package-level builder here:
// this file's only remaining pre-flight addition is reading the operator's own terminal size via
// golang.org/x/term and handing it to the engine's builder. That size read and the builder call are
// both pre-flight steps that nonetheless never write to the envelope — each degrades to today's bare
// argv and logs a warning instead, so c.eng.Status() remains the only pre-flight step that can abort
// with an envelope error.

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
		Use:   "attach",
		Short: "attach the operator's terminal to the reed session in place",
		Long: `attach hands the operator's own stdio over to a tmux attach-session
child, in place — no new window is spawned (never wt.exe). Every fallible
step (checking that the server/session is up) runs pre-flight and reports
through the normal JSON envelope; once the terminal handover begins, stdio
belongs to tmux and nothing further is written to it, even on success.
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

			// Pre-flight: surface the friendly no-session error (see
			// reedengine.requireSessionLocked/noSessionMessage), or any other
			// Status failure, on the envelope before ever touching stdio, since
			// after the handover below no JSON can reach the caller.
			if _, err := c.eng.Status(); err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}

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

			attach := exec.Command(c.eng.TmuxPath(), c.eng.AttachArgv(cols, rows)...)
			attach.Stdin = os.Stdin
			attach.Stdout = os.Stdout
			attach.Stderr = os.Stderr

			// Terminal-handover tail: stdio is now inherited by the child
			// process, so this is the one documented exception to "every RunE
			// ends in a JSON envelope" — no JSON is written here even on
			// failure (tmux's own stderr already reached the operator's
			// terminal), but the child's exit code still propagates so a
			// failed attach is not reported as success.
			if err := attach.Run(); err != nil {
				exitCode := 1
				var exitErr *exec.ExitError
				if errors.As(err, &exitErr) {
					exitCode = exitErr.ExitCode()
				}
				clihelp.SetExit(cmd.Context(), exitCode)
			}
			return nil
		},
	}
}
