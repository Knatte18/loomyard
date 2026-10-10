// resume.go implements the `resume` reed verb: the only replayer,
// recreating not-live, non-hidden strands after a server restart or a single pane's death,
// dropping a not-live strand whose done-when paths all exist, and leaving already-live strands untouched.

package reedcli

import (
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/spf13/cobra"
)

// resumeCmd builds the `resume` subcommand.
func (c *reedCLI) resumeCmd() *cobra.Command {
	return &cobra.Command{
		Use:         "resume",
		Short:       "replay stored commands for not-live strands",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `resume is the only replayer: for every persisted, non-hidden strand that
is not currently live, it recreates the pane and runs the strand's stored
resumeCmd (or cmd, when resumeCmd is empty). Anchor:hidden strands are
skipped — they are pending first surface, not dead. Already-live strands are
left untouched (no double send-keys). A not-live strand whose done-when paths
all exist is finished: it is dropped from state instead of relaunched, and
counted in the envelope's dropped.

Example:
  lyx reed resume`,
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

			// Attempted after Resume returns without error, before the envelope write: same
			// reasoning as up's own spawn attempt.
			c.ensureWatchdogSpawned()

			clihelp.SetExit(cmd.Context(), output.Ok(out, map[string]any{
				"session": result.Session,
				"resumed": result.Resumed,
				"dropped": result.Dropped,
			}))
			return nil
		},
	}
}
