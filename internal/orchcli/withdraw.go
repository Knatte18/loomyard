// withdraw.go implements the `withdraw` orch verb: an operator's removal of a pending cycle request.

package orchcli

import (
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/orchengine"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/spf13/cobra"
)

// withdrawCmd builds the `withdraw` subcommand.
func (c *orchCLI) withdrawCmd() *cobra.Command {
	return &cobra.Command{
		Use:         "withdraw",
		Short:       "withdraw the pending cycle request",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `withdraw removes the pending cycle request of either mode, in any phase, and
answers withdrawn true. It removes only the request marker, never a cycle that
already started: a request present during a cycle has not started, so a true
answer means no cycle started from it.
With no request pending it answers withdrawn false with phase, the persisted
cycle phase, and with cycle_trigger, the trigger of the running cycle, unless
the phase is idle or resuming, where an earlier cycle's trigger may remain.
It never refuses, so a caller whose request a cycle start consumed first learns
which cycle runs instead of reading a failed command.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			out := cmd.OutOrStdout()

			withdrawn, err := orchengine.WithdrawCycleRequest(c.paths)
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}
			if withdrawn {
				clihelp.SetExit(cmd.Context(), output.Ok(out, map[string]any{"withdrawn": true}))
				return nil
			}
			st, err := orchengine.LoadState(c.paths)
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}
			fields := map[string]any{"withdrawn": false, "phase": string(st.Phase)}
			if st.Phase != orchengine.PhaseIdle && st.Phase != orchengine.PhaseResuming && st.Phase != "" {
				fields["cycle_trigger"] = st.CycleTrigger
			}
			clihelp.SetExit(cmd.Context(), output.Ok(out, fields))
			return nil
		},
	}
}
