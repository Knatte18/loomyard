// reset.go implements the `reset` weaver verb and its envelope.

package weavercli

import (
	"io"

	"github.com/spf13/cobra"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/weaverengine"
)

// resetCmd builds the `reset` subcommand.
func (c *weaverCLI) resetCmd() *cobra.Command {
	var stopStrands bool
	cmd := &cobra.Command{
		Use:         "reset",
		Short:       "start the weaver run over from its start commit and the plan it started with",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `reset starts a stopped Weaver run over. It moves the task branch back to the
run's start commit, discarding the commits above it and the uncommitted changes
the run left. It restores the plan directory to its content when the run began,
renames the run record under a UTC-stamped name inside _lym/weaver/, clears the
run's scratch files but its locks, and commits the plan restore and the
archived record together. Webster's run directory is left as it was.

It refuses, changing nothing, while the row's run lock is held (wait for the row
to stop, or pause it with "lym loom pause --after" the implementer row), while a
merge is in progress, and while a tracked path the run did not leave is
uncommitted (commit it, or restore it with "git checkout -- <path>").

A strand of the run that is still live refuses the reset. --stop-strand stops
the strands the reset finds by the output files of this run's recorded attempts
and fixers, and nothing else. Once it has stopped one, every tracked path left
uncommitted counts as the run's own and is discarded with the reset. A reset
whose commit failed is finished by running it again.

The envelope carries start_sha, plan_commit and archived, and finished when the
call only completed an earlier reset's commit.

Example:
  lym weaver reset
  lym weaver reset --stop-strand`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			if clihelp.ShouldAbort(ctx) {
				return nil
			}
			out := cmd.OutOrStdout()
			deps, err := c.resetDepsFor(c.loc)
			if err != nil {
				clihelp.SetExit(ctx, output.Err(out, err.Error()))
				return nil
			}
			clihelp.SetExit(ctx, c.runReset(out, deps, stopStrands))
			return nil
		},
	}
	cmd.Flags().BoolVar(&stopStrands, "stop-strand", false, "stop the live strands of this run's recorded attempts and fixers instead of refusing over them")
	return cmd
}

// resetVerb runs weaverengine.Reset and prints one envelope.
// A reset that went through is ok with the start commit, the restored plan commit and the archived names.
// A refusal or a failure is an error envelope carrying the text, which names its way forward.
func resetVerb(out io.Writer, deps weaverengine.ResetDeps, stopStrands bool) int {
	result, err := weaverengine.Reset(deps, stopStrands)
	if err != nil {
		return output.Err(out, err.Error())
	}
	archived := result.Archived
	if archived == nil {
		archived = []string{}
	}
	return output.Ok(out, map[string]any{
		"start_sha":   result.StartSHA,
		"plan_commit": result.PlanCommit,
		"archived":    archived,
		"finished":    result.Finished,
	})
}
