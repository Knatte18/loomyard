// drive.go implements the `drive` loom verb: the no-tmux escape hatch that runs the phase machine in
// the foreground, for debugging and CI.

package loomcli

import (
	"os"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/spf13/cobra"
)

// driveCmd builds the `drive` subcommand.
func (c *loomCLI) driveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "drive",
		Short: "run loom's phase machine in the foreground, with no tmux and no strand",
		Long: `drive runs loom's phase machine in the foreground: no tmux, no strand,
and no terminal handover. It is the no-tmux escape hatch for debugging and
CI. drive never seeds a status file and never commits anything -- only
"lyx loom run" seeds, because only it owns the commit-before-precondition
ordering the bootstrap needs.

Example:
  lyx loom drive`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			out := cmd.OutOrStdout()

			// Pre-flight: refuse on the envelope when the status file does not exist yet, naming
			// the two-word bootstrap verb as the remedy. This refusal exists so the operator is
			// told on the envelope rather than discovering it as the phase machine's own
			// seed-missing precondition failure buried in the detached driver's log -- only
			// "lyx loom run" may seed, because only it owns the commit-before-precondition
			// ordering.
			if _, err := os.Stat(c.deps.StatusPath); err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, "loom: no status file at "+c.deps.StatusPath+"; run \"lyx loom run\" first to bootstrap this task"))
				return nil
			}

			shed, err := loomshed.New(c.deps)
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}

			// Run's already-running sentinel (shedengine.ErrShedBusy) is treated as an ordinary
			// error envelope rather than a special case here: a second driver against the same
			// status file is a real refusal, not a race to tolerate.
			result, err := shed.Run(cmd.Context())
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}

			clihelp.SetExit(cmd.Context(), output.Ok(out, map[string]any{
				"outcome":         string(result.Outcome),
				"halted_producer": result.HaltedProducer,
				"reason":          result.Reason,
				"history_length":  len(result.History),
			}))
			return nil
		},
	}
}
