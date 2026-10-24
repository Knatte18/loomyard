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
withdrawn. While a hub pause holds the watcher the request waits for the
resume, and the envelope adds held_by_pause true with a note. A request the
watcher has not started within the handoff timeout is abandoned; once it
starts, the session gets the full handoff timeout for the note from when it
reads the note request. Neither timeout counts time a hub pause held the
request. "lyx orch withdraw" removes a
pending request, and "lyx orch status" shows it as cycle_request.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return c.requestCmd(cmd, orchengine.CycleCompact)
		},
	}
}
