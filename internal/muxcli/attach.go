// attach.go implements the `attach` mux verb: an in-place terminal handover
// into the psmux session — no new window is spawned. attach is the one
// registered JSON-envelope exception in this package: every fallible step
// runs pre-flight on the envelope, but the terminal-handover tail (once
// stdio is inherited by the child psmux process) emits no JSON on success.

package muxcli

import (
	"errors"
	"os"
	"os/exec"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/spf13/cobra"
)

// attachArgv builds the psmux argv for an in-place attach: "-L <socket>
// attach-session -t <session>". It is kept as a pure function, separate
// from the exec.Command call it feeds, so cli_test.go can assert the built
// invocation without spawning psmux or needing a live session.
func attachArgv(socket, session string) []string {
	return []string{"-L", socket, "attach-session", "-t", session}
}

// attachCmd builds the `attach` subcommand: a session-level verb (no strand
// argument) that runs Status() pre-flight — Status takes the op lock and
// returns a non-nil error when the server/session is absent (it is read-only:
// it reports live/dead without reconciling) — then hands the operator's own
// stdin/stdout/stderr to a psmux
// attach-session child, in place. Only that terminal-handover tail is
// exempt from the JSON-envelope contract; every step before it still
// reports through output.Err/output.Ok.
func (c *muxCLI) attachCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "attach",
		Short: "attach the operator's terminal to the mux session in place",
		Long: `attach hands the operator's own stdio over to a psmux attach-session
child, in place — no new window is spawned (never wt.exe). Every fallible
step (checking that the server/session is up) runs pre-flight and reports
through the normal JSON envelope; once the terminal handover begins, stdio
belongs to psmux and nothing further is written to it, even on success.

Example:
  lyx mux attach`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			out := cmd.OutOrStdout()

			// Pre-flight: surface the friendly no-session error (see
			// muxengine.requireSessionLocked/noSessionMessage), or any other
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
