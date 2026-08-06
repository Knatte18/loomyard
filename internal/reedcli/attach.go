// attach.go implements the `attach` reed verb: an in-place terminal handover into the tmux session — no new window is spawned.
// attach is the one registered JSON-envelope exception in this package: every fallible step runs pre-flight on the envelope,
// but the terminal-handover tail (once stdio is inherited by the child tmux process) emits no JSON on success.

package reedcli

import (
	"errors"
	"os"
	"os/exec"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/spf13/cobra"
)

// attachArgv reports the tmux argv for an in-place attach.
func attachArgv(socket, session string) []string {
	return []string{"-L", socket, "attach-session", "-t", "=" + session}
}

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

			attach := exec.Command(c.eng.TmuxPath(), attachArgv(c.eng.Socket(), c.eng.SessionName())...)
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
