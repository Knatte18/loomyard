// stop.go implements the `stop` orch verb: remove the orchestrator strand and let the watcher notice and exit on its own.

package orchcli

import (
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/orchengine"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/spf13/cobra"
)

// stopCmd builds the `stop` subcommand.
func (c *orchCLI) stopCmd() *cobra.Command {
	return &cobra.Command{
		Use:         "stop",
		Short:       "remove the orchestrator strand",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `stop removes the recorded orchestrator strand when reed tracks it. An untracked
or unrecorded strand reports removed: false rather than an error. The watcher
exits on its own once the strand is gone, and the next start resets any
persisted cycle phase.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			out := cmd.OutOrStdout()

			st, err := orchengine.LoadState(c.paths)
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}
			strands, err := c.strands.Strands()
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}

			_, tracked := trackedStrand(strands, st.Strand)
			if tracked {
				if err := c.strands.RemoveStrand(st.Strand); err != nil {
					clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
					return nil
				}
			}
			clihelp.SetExit(cmd.Context(), output.Ok(out, map[string]any{
				"removed": tracked,
				"strand":  st.Strand,
			}))
			return nil
		},
	}
}
