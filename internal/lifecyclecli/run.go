// run.go implements the `run` lifecycle verb: it starts or resumes one slug's whole lifecycle run
// to its next halt.

package lifecyclecli

import (
	"errors"
	"fmt"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/lifecyclerecipe"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/state"
	"github.com/spf13/cobra"
)

// runCmd builds the `run <slug>` subcommand.
func (c *lifecycleCLI) runCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run <slug>",
		Short: "start or resume a task worktree's whole lifecycle run to its next halt",
		Long: `run drives the pair's create/run/teardown lifecycle to its next halt: done,
blocked, or paused. Invoked against a slug with no persisted status, it
starts a fresh run. Invoked against one already in progress, it resumes
silently from the persisted current producer -- there is no re-seed flag,
because every operator-fixable refusal this task raises lands as blocked,
which is the everyday resume path. A slug already done refuses on the
envelope, naming the per-slug directory to delete to run it again.

Example:
  lyx lifecycle run some-slug`,
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
			if found {
				switch st.State {
				case shedengine.StateDone:
					clihelp.SetExit(ctx, output.Err(out, fmt.Sprintf(
						"lifecyclecli: %q has already completed; delete %s to run it again",
						c.slug, LifecycleDir(c.location, c.slug),
					)))
					return nil
				case shedengine.StateRunning, shedengine.StateBlocked, shedengine.StateFailed, shedengine.StatePaused:
					// Each of these resumes silently from the persisted current producer, with no
					// re-seed, no prompt, and no flag: the engine itself already resumes from
					// blocked and failed, and StateBlocked is the everyday path, since every
					// operator-fixable refusal in this task lands there.
				default:
					clihelp.SetExit(ctx, output.Err(out, fmt.Sprintf("lifecyclecli: unrecognized status state %q", st.State)))
					return nil
				}
			} else {
				// An absent status file is a fresh start: shedengine.Shed.Run refuses to walk from a
				// status file that does not exist yet (it never seeds one itself), so this verb seeds
				// it here, at the entry row, before the Shed ever reads it. The mutate closure is
				// idempotent against a concurrently-seeded file: it leaves an already-present status
				// untouched rather than overwriting it.
				if err := state.UpdateJSON(c.shedPaths.StatusPath, c.shedPaths.StatusLockPath, func(cur shedengine.Status, found bool) (shedengine.Status, error) {
					if found {
						return cur, nil
					}
					return shedengine.Status{
						CurrentProducer: lifecyclerecipe.NameWorktreeCreate,
						State:           shedengine.StateRunning,
						History:         []shedengine.HistoryEntry{},
					}, nil
				}); err != nil {
					clihelp.SetExit(ctx, output.Err(out, err.Error()))
					return nil
				}
			}

			shed, err := lifecyclerecipe.New(c.env, c.shedPaths)
			if err != nil {
				clihelp.SetExit(ctx, output.Err(out, err.Error()))
				return nil
			}

			result, err := shed.Run(ctx)
			if err != nil {
				if errors.Is(err, shedengine.ErrShedBusy) {
					// The engine already takes the run lock non-blocking for the whole of one run,
					// so refusing here is its native behaviour; this verb's only job is to report
					// it legibly -- naming the lock path -- rather than surfacing a raw lock error.
					// There is no wait and no second driver spawned.
					clihelp.SetExit(ctx, output.Err(out, fmt.Sprintf(
						"lifecyclecli: another lifecycle run already holds the run lock %q", c.shedPaths.LockPath,
					)))
					return nil
				}
				clihelp.SetExit(ctx, output.Err(out, err.Error()))
				return nil
			}

			fields := map[string]any{
				"outcome":         string(result.Outcome),
				"halted_producer": result.HaltedProducer,
				"reason":          result.Reason,
			}
			// The envelope deliberately carries neither a mutations array nor a partial bool: a run
			// may perform zero, one, or two topology mutations at arbitrary points hours apart, so
			// there is no coherent single array at run scope, and partial has no referent here. The
			// mutation records are logged at Info by the wiring closures (wire.go) instead of
			// discarded.
			if c.abandonedSession != "" {
				fields["abandonedSession"] = c.abandonedSession
			}
			clihelp.SetExit(ctx, output.Ok(out, fields))
			return nil
		},
	}
	return cmd
}
