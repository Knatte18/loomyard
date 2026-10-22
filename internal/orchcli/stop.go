// stop.go implements the `stop` orch verb: remove the orchestrator strand and let the watcher notice and exit on its own.

package orchcli

import (
	"fmt"
	"os"
	"time"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/orchengine"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/spf13/cobra"
)

// stopStartLockWait is how long `stop` waits for start.lock, which a `start` holds across its origin reconcile and session launch.
const stopStartLockWait = time.Minute

// stopCmd builds the `stop` subcommand.
func (c *orchCLI) stopCmd() *cobra.Command {
	return &cobra.Command{
		Use:         "stop",
		Short:       "remove the orchestrator strand",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `stop removes the recorded orchestrator strand when reed tracks it. An untracked
or unrecorded strand reports removed: false rather than an error. The watcher
exits on its own once the strand is gone, and the next start resets any
persisted cycle phase. stop holds start.lock around the removal, so a status
restart (watcher_restarted) never binds a watcher to a strand stop is removing;
it waits for a running start up to a minute, then refuses and names the lock.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			out := cmd.OutOrStdout()

			if err := os.MkdirAll(c.paths.Dir, 0o755); err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, fmt.Sprintf("orch: create %s: %v", c.paths.Dir, err)))
				return nil
			}
			startLock, acquired, err := lock.AcquireWriteLockWithin(c.paths.StartLockPath, c.startLockWait)
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}
			if !acquired {
				clihelp.SetExit(cmd.Context(), output.Err(out, fmt.Sprintf("orch: start.lock (%s) is still held by a running `lyx orch start`; way forward: rerun `lyx orch stop` once it ends", c.paths.StartLockPath)))
				return nil
			}
			defer func() { _ = startLock.Release() }()

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
