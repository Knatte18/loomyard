// cleanup.go implements the `cleanup` orch verb: the dry-run-by-default removal of old handoff files.

package orchcli

import (
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/orchengine"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/spf13/cobra"
)

// cleanupCmd builds the `cleanup` subcommand.
func (c *orchCLI) cleanupCmd() *cobra.Command {
	var apply bool
	cmd := &cobra.Command{
		Use:         "cleanup",
		Short:       "remove every handoff but the last one the orchestrator cycled from",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `Without --apply, cleanup is a read-only dry run: it lists the handoff files it
would remove and changes nothing. With --apply it removes every file under the
orch's handoffs directory except the one the persisted state names as the last
completed handoff, so a restart still resumes from it. It removes nothing, and
says why in "reason", while a cycle is in progress, when no handoff is recorded
or when the recorded one is gone.

Output: {"applied", "kept", "removed", "reason"}.

Example:
  lyx orch cleanup
  lyx orch cleanup --apply`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			out := cmd.OutOrStdout()
			result, err := orchengine.CleanupHandoffs(c.paths, apply)
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}
			clihelp.SetExit(cmd.Context(), output.Ok(out, map[string]any{
				"applied": apply,
				"kept":    result.Kept,
				"removed": result.Removed,
				"reason":  result.Reason,
			}))
			return nil
		},
	}
	cmd.Flags().BoolVar(&apply, "apply", false, "remove the old handoffs (default is dry-run/report)")
	return cmd
}
