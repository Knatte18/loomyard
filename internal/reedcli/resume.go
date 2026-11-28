// resume.go implements the `resume` reed verb, the only replayer of stored commands.
// It recreates each persisted, non-hidden strand that is not live, after a server restart or a single pane's death.
// It drops a not-live strand whose done-when paths all exist, and it leaves already-live strands untouched.

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
counted in the envelope's dropped. While a pause covers this pair, its strands
are left down and named in the envelope's held, until "lym tender resume".

Example:
  lym reed resume`,
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

			// The spawn is attempted after Resume returns without error, before the envelope write.
			// The daemon needs a live session to discover, and resume recreates the panes that provide one.
			c.ensureWatchdogSpawned()

			clihelp.SetExit(cmd.Context(), output.Ok(out, map[string]any{
				"session": result.Session,
				"resumed": result.Resumed,
				"dropped": result.Dropped,
				"held":    result.Held,
			}))
			return nil
		},
	}
}
