// status.go implements the `status` mux verb: reports this worktree's
// tracked strands and their live/dead state after reconciling against the
// live pane set. v1 reports only the current session — enumerating stray
// servers across the hub is deferred.

package muxcli

import (
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/spf13/cobra"
)

// statusCmd builds the `status` subcommand: calls c.eng.Status() and emits
// every tracked strand's guid, name, pane id, and live/dead state.
func (c *muxCLI) statusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "show this worktree's tracked strands and their live/dead state",
		Long: `status reconciles against the live pane set and reports every strand
tracked for this worktree's session: guid, name, pane id, and whether its
pane is currently alive.

v1 reports only the current worktree's session — enumerating stray servers
across the hub is deferred.

Example:
  lyx mux status`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			out := cmd.OutOrStdout()

			result, err := c.eng.Status()
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}

			strands := make([]map[string]any, len(result.Strands))
			for i, s := range result.Strands {
				strands[i] = map[string]any{
					"guid":   s.GUID,
					"name":   s.Name,
					"paneId": s.PaneID,
					"live":   s.Live,
				}
			}

			clihelp.SetExit(cmd.Context(), output.Ok(out, map[string]any{
				"session": result.Session,
				"socket":  result.Socket,
				"strands": strands,
			}))
			return nil
		},
	}
}
