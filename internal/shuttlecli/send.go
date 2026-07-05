// send.go implements the `send` shuttle verb: a thin guid/text-to-call
// mapper over c.runner.Send, letting an operator or another process type a
// one-line update into a shuttle run's pane as its next turn.

package shuttlecli

import (
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/spf13/cobra"
)

// sendCmd builds the `send <guid> <text>` subcommand: plays the engine's
// compose-and-submit choreography into the strand identified by <guid>,
// typing <text> as its next turn. text must be a single line — the file
// contract carries multiline updates (write a file and send a one-line
// pointer to it), and c.runner.Send enforces this. c.runner.Send also
// confirms guid names a shuttle run before touching the pane; a miss
// reports "not a shuttle strand" through output.Err.
func (c *shuttleCLI) sendCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "send <guid> <text>",
		Short: "type a one-line update into a shuttle run's pane as its next turn",
		Long: `send plays the engine's compose-and-submit choreography into the strand
identified by <guid>, typing <text> as its next turn. <text> must be a
single line: the file contract carries multiline updates — write a file and
send a one-line pointer to it (e.g. "read updated-task.md and continue").

Example:
  lyx shuttle send 3fae21ac9b1d4c0e "read updated-task.md and continue"`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			out := cmd.OutOrStdout()
			guid, text := args[0], args[1]

			if err := c.runner.Send(guid, text); err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}

			clihelp.SetExit(cmd.Context(), output.Ok(out, map[string]any{
				"guid":   guid,
				"action": "send",
			}))
			return nil
		},
	}
}
