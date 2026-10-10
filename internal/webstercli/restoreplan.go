// restoreplan.go implements the `restore-plan` webster verb: it writes every plan file that differs from the plan the run recorded back from webster's own stored copy.
// It runs websterengine.RestorePlan under the state-mutation lease (load, restore, release), then fabric-syncs.
package webstercli

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/websterengine"
	"github.com/spf13/cobra"
)

// restorePlanCmd builds the `restore-plan` subcommand.
func (c *websterCLI) restorePlanCmd() *cobra.Command {
	return &cobra.Command{
		Use:         "restore-plan",
		Short:       "restore every plan file that differs from the plan the run recorded",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `restore-plan writes back each plan file from the copy webster kept when it
recorded the plan's hashes, and removes a plan file the run never recorded.
It never changes state.json or the task worktree.
It refuses, changing nothing, when a copy is missing.
On success the envelope carries restored, the file names written back or
removed, empty when nothing differed.

Example:
  lyx webster restore-plan`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()

			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}

			mutateLock, err := websterengine.AcquireStateMutation(c.geom.ScratchDir)
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}
			mutateHeld := true
			defer func() {
				if mutateHeld {
					_ = mutateLock.Release()
				}
			}()

			st, err := websterengine.LoadState(c.geom.WebsterDir, c.geom.ScratchDir)
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}
			if st == nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, `webster: no run in progress; run "lyx webster run" first`))
				return nil
			}

			restored, err := websterengine.RestorePlan(st, c.geom)
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}
			_ = mutateLock.Release()
			mutateHeld = false

			if _, syncErr := fabricSync(c.openFabric, c.anchorRel, "restore-plan"); syncErr != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, fmt.Sprintf("webster: plan restored but the fabric sync failed: %v; %s", syncErr, fabricSyncWayForward)))
				return nil
			}

			if restored == nil {
				restored = []string{}
			}
			clihelp.SetExit(cmd.Context(), output.Ok(out, map[string]any{"restored": restored}))
			return nil
		},
	}
}
