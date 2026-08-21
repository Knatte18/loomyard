// pause.go implements the `pause` loom verb: it sets the shared pause-requested flag the running
// phase machine consumes at its next producer boundary, and nothing else.

package loomcli

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/state"
	"github.com/spf13/cobra"
)

// pauseCmd builds the `pause` subcommand.
func (c *loomCLI) pauseCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "pause",
		Short: "request a pause at loom's next producer boundary",
		Long: `pause sets a request the running phase machine consumes at its next
producer boundary. It does not kill anything -- the machine itself clears
the flag in the persist that records the paused state.

Example:
  lyx loom pause`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			out := cmd.OutOrStdout()

			err := state.UpdateJSON(c.shedPaths.StatusPath, c.shedPaths.StatusLockPath, func(cur shedengine.Status, found bool) (shedengine.Status, error) {
				if !found {
					return shedengine.Status{}, fmt.Errorf("loom: no status file at %s; there is nothing running to pause -- run \"lyx loom run\" first to bootstrap this task", c.shedPaths.StatusPath)
				}
				cur.PauseRequested = true
				return cur, nil
			})
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}

			clihelp.SetExit(cmd.Context(), output.Ok(out, map[string]any{
				"status_file": c.shedPaths.StatusPath,
			}))
			return nil
		},
	}
}
