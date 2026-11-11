// awaitforks.go implements the read-only `await-forks` shuttle verb: the capped wait a role agent runs after spawning forks,
// so it takes every fork's result in one turn instead of waking once per fork.
// It reads the calling session's own transcript through the engine and writes nothing.

package shuttlecli

import (
	"fmt"
	"os"
	"time"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/spf13/cobra"
)

// awaitForksVerb is the cobra name of the await-forks verb.
const awaitForksVerb = "await-forks"

// awaitForksCmd builds `lym shuttle await-forks [--cap <duration>]`.
// It resolves the calling session's run from the strand name reed gave its pane.
// It prints ok with ready true once none of the session's forks is still running.
// When --cap elapses first it prints ok with ready false and the forks still running, telling the caller to run the command again.
// A caller outside a shuttle strand, an unknown strand and a transcript read error are errors with exit 1.
func (c *shuttleCLI) awaitForksCmd() *cobra.Command {
	var awaitCap time.Duration
	cmd := &cobra.Command{
		Use:         awaitForksVerb,
		Short:       "wait, capped, until every fork the calling session spawned has finished",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceRole},
		Long: `await-forks waits until every fork the calling agent session spawned has
finished, printing ok with ready true. Every fork result that arrived during
the wait reaches the session together when the command returns, so the
session takes all of them in one turn instead of waking once per fork.
When --cap elapses first it prints ok with ready false and the forks still
running, and exits 0, telling the caller to run the command again;
a fork that never reports keeps it answering ready false, so the caller bounds its re-runs.

Run it from the session that spawned the forks, in a message after the one
that spawned them, with a Bash tool timeout above --cap (300000 ms), and
re-run it in a turn that does nothing else, so the prompt cache stays warm
between calls. It runs from the anchored directory, any directory below it,
or anywhere in the worktree's records sibling. It is read-only.

Example:
  lym shuttle await-forks`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			if clihelp.ShouldAbort(ctx) {
				return nil
			}
			out := cmd.OutOrStdout()

			name := os.Getenv(agentname.StrandNameEnv)
			if name == "" {
				clihelp.SetExit(ctx, output.Err(out, "await-forks: the calling session runs in no shuttle strand, so it has no forks to wait on; way forward: run it from the agent session that spawned the forks"))
				return nil
			}
			guid, err := c.reed.ResolveStrandGUID(name)
			if err != nil {
				clihelp.SetExit(ctx, output.Err(out, fmt.Sprintf("await-forks: resolve the calling strand %q: %v", name, err)))
				return nil
			}

			outstanding, ready, err := c.runner.AwaitForks(ctx, guid, awaitCap)
			if err != nil {
				clihelp.SetExit(ctx, output.Err(out, err.Error()))
				return nil
			}
			if !ready {
				output.Ok(out, map[string]any{"ready": false, "outstanding": taskFields(outstanding), "message": "forks are still running; call this command again"})
				return nil
			}
			output.Ok(out, map[string]any{"ready": true})
			return nil
		},
	}
	cmd.Flags().DurationVar(&awaitCap, "cap", shuttleengine.DefaultAwaitCap, "longest time one call waits before answering ready false")
	return cmd
}
