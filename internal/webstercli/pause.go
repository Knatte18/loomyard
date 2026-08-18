// pause.go implements the `pause` webster verb: it writes the pause flag file begin-batch's
// batch-boundary check refuses against once set.
// webster owns its own pause-flag mechanics (websterengine.RequestPause), deliberately
// module-local rather than shared.
package webstercli

import (
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/websterengine"
	"github.com/spf13/cobra"
)

// pauseCmd builds the `pause` subcommand.
func (c *websterCLI) pauseCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pause",
		Short: "request a pause at the next batch boundary",
		Long: `pause writes a flag file "begin-batch" checks at the batch boundary. Once
set, "lyx webster begin-batch" refuses to open a new batch with a
"paused": true envelope; any batch already forked finishes normally --
pause never interrupts a running fork. Resume with "lyx webster run", which
clears the flag at its own entry.

Example:
  lyx webster pause`,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()

			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}

			if err := websterengine.RequestPause(c.geom.ScratchDir); err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}

			clihelp.SetExit(cmd.Context(), output.Ok(out, map[string]any{
				"paused": true,
			}))
			return nil
		},
	}

	return cmd
}
