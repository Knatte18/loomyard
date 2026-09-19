// status.go implements the `status` lifecycle verb: a one-shot JSON envelope of a slug's persisted
// lifecycle status.

package lifecyclecli

import (
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/state"
	"github.com/spf13/cobra"
)

// statusCmd builds the `status <slug>` subcommand.
//
// It runs under the same prime-worktree refusal the parent's pre-run applies (cli.go), so it is
// refused from a task worktree exactly as run is -- even though it is read-only: the path it would
// read is derived from whichever worktree it is invoked in, so answering from a task worktree would
// report on a different file and quietly mislead, not merely fail to write.
func (c *lifecycleCLI) statusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status <slug>",
		Short: "report a task worktree's persisted lifecycle status",
		Long: `status reports a slug's persisted lifecycle status: the current producer, the
state, the error field, the activity, and the history, plus the resolved
status path so an operator can find the file. A slug that has never been
run on this machine is reported as a determined answer on the success
envelope, not as an error.

Example:
  lyx lifecycle status some-slug`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			ctx := cmd.Context()
			out := cmd.OutOrStdout()

			st, found, err := state.ReadJSONStrict[shedengine.Status](c.shedPaths.StatusPath, c.shedPaths.StatusLockPath)
			if err != nil {
				clihelp.SetExit(ctx, output.Err(out, "lifecyclecli: decode status file "+c.shedPaths.StatusPath+": "+err.Error()))
				return nil
			}
			if !found {
				// An absent status file is a determined answer -- the slug has not been run on this
				// machine -- not an error, since nothing failed.
				clihelp.SetExit(ctx, output.Ok(out, map[string]any{
					"found":       false,
					"status_path": c.shedPaths.StatusPath,
				}))
				return nil
			}

			clihelp.SetExit(ctx, output.Ok(out, map[string]any{
				"found":            true,
				"status_path":      c.shedPaths.StatusPath,
				"current_producer": st.CurrentProducer,
				"state":            string(st.State),
				"error":            st.Error,
				"activity":         st.Activity,
				"history":          st.History,
			}))
			return nil
		},
	}
	return cmd
}
