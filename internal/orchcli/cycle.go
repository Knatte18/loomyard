// cycle.go implements the `cycle` orch verb: an operator's request that the watcher run one
// handoff cycle at its next idle moment.

package orchcli

import (
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/orchengine"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/spf13/cobra"
)

// cycleCmd builds the `cycle` subcommand.
func (c *orchCLI) cycleCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "cycle",
		Short: "ask the watcher to cycle the orchestrator session now",
		Long: `cycle writes a cycle request the watcher picks up at the session's next idle
moment. The envelope's watcher_live tells you at once when no watcher is running
to act on it.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			out := cmd.OutOrStdout()

			if err := orchengine.RequestCycle(c.paths); err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}
			watcherLive, err := orchengine.WatcherLive(c.paths)
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}
			clihelp.SetExit(cmd.Context(), output.Ok(out, map[string]any{
				"requested":    true,
				"watcher_live": watcherLive,
			}))
			return nil
		},
	}
}
