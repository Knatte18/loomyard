// interrupt.go implements the `interrupt` shuttle verb: a thin guid-to-call
// mapper over c.runner.Interrupt, letting an operator or another process
// stop a shuttle run's in-progress turn without killing its pane or session.

package shuttlecli

import (
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/spf13/cobra"
)

// interruptCmd builds the `interrupt <guid>` subcommand: plays the engine's
// interrupt choreography (e.g. a single Escape key press) into the strand
// identified by <guid>. c.runner.Interrupt confirms guid names a shuttle run
// before touching the pane; a miss reports "not a shuttle strand" through
// output.Err.
func (c *shuttleCLI) interruptCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "interrupt <guid>",
		Short: "stop a shuttle run's in-progress turn without killing its pane",
		Long: `interrupt plays the engine's interrupt choreography into the strand
identified by <guid>, stopping its in-progress turn. The pane and session
stay alive afterward — follow with "lyx shuttle send <guid> <text>" to give
the agent updated instructions and let it continue, or attach directly.

Example:
  lyx shuttle interrupt 3fae21ac9b1d4c0e`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			out := cmd.OutOrStdout()
			guid := args[0]

			if err := c.runner.Interrupt(guid); err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}

			clihelp.SetExit(cmd.Context(), output.Ok(out, map[string]any{
				"guid":   guid,
				"action": "interrupt",
			}))
			return nil
		},
	}
}
