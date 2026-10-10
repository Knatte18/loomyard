// distill.go implements the `distill` orch verb: an operator's request that the watcher write a note and compact the session, whatever cycle_mode says.

package orchcli

import (
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/orchengine"
	"github.com/spf13/cobra"
)

// distillCmd builds the `distill` subcommand.
func (c *orchCLI) distillCmd() *cobra.Command {
	return &cobra.Command{
		Use:         "distill",
		Short:       "ask the watcher to compact the orchestrator session now",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `distill writes a compact-cycle request the watcher picks up at the session's
next idle moment, whatever cycle_mode says. The session writes a note first;
only then does the watcher compact it in place. The envelope's watcher_live
tells you at once when no watcher is running to act on it; the request is then
withdrawn. A request the watcher has not finished within the handoff timeout
is abandoned.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return c.requestCmd(cmd, orchengine.CycleCompact)
		},
	}
}
