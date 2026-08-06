// pause.go implements the `pause` builder verb: it writes the pause flag
// file spawn-batch's batch-boundary check refuses against once set, and
// documents the batch-boundary (not mid-batch) semantics the discussion
// pins.

package buildercli

import (
	"github.com/Knatte18/loomyard/internal/builderengine"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/spf13/cobra"
)

// pauseCmd builds the `pause` subcommand.
func (c *builderCLI) pauseCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pause",
		Short: "request a pause at the next batch boundary",
		Long: `pause writes a flag file "spawn-batch" checks at the batch boundary. Once
set, "lyx builder spawn-batch" refuses to start a new batch with a
"paused": true envelope; any batch already in flight finishes normally --
pause never interrupts a running implementer. Resume with
"lyx builder run", which clears the flag at its own entry.

Example:
  lyx builder pause`,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()

			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}

			if err := builderengine.RequestPause(c.builderDir); err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}

			clihelp.SetExit(cmd.Context(), output.Ok(out, map[string]any{
				"pause_requested": true,
			}))
			return nil
		},
	}

	return cmd
}
