// resume.go implements the `resume` mux verb: the only replayer, recreating
// not-live, non-hidden strands after a server restart or a single pane's
// death, and leaving already-live strands untouched.

package muxcli

import (
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/spf13/cobra"
)

// resumeCmd builds the `resume` subcommand: calls c.eng.Resume() and emits
// the session name plus how many strands were relaunched.
func (c *muxCLI) resumeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "resume",
		Short: "replay stored commands for not-live strands",
		Long: `resume is the only replayer: for every persisted, non-hidden strand that
is not currently live, it recreates the pane and runs the strand's stored
resumeCmd (or cmd, when resumeCmd is empty). Anchor:hidden strands are
skipped — they are pending first surface, not dead. Already-live strands are
left untouched (no double send-keys).

Example:
  lyx mux resume`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			out := cmd.OutOrStdout()

			result, err := c.eng.Resume()
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}

			clihelp.SetExit(cmd.Context(), output.Ok(out, map[string]any{
				"session": result.Session,
				"resumed": result.Resumed,
			}))
			return nil
		},
	}
}
