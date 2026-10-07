// reset.go implements the `reset` webster verb: the guarded way to move the task branch back to a commit the run recorded.
// It plans the reset with websterengine.PlanReset, performs it through fabricengine's pair-checkout reset, clears the persisted pre-fix head and fabric-syncs state.json.
// The verb runs no git of its own, so an agent that is denied `git reset --hard` still has a way to recover.
package webstercli

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/websterengine"
	"github.com/spf13/cobra"
)

// resetCmd builds the `reset` subcommand.
func (c *websterCLI) resetCmd() *cobra.Command {
	var to string
	cmd := &cobra.Command{
		Use:   "reset --to start|pre-fix",
		Short: "move the task branch back to the run's start commit or the verify gate's pre-fix head",
		Long: `reset moves the task worktree's HEAD, index and tracked files back to a commit
the run recorded, so a recovery needs no git reset of its own.
--to start is the run's start commit: the oldest recorded batch start, or the
octopus merge-base of the starts when none is the oldest.
--to pre-fix is the HEAD the verify gate started its fixes from.
It refuses, changing nothing, while a run holds the run lock, during a merge, off
the task branch, with no recorded target, when the target commit is missing or
is not an ancestor of HEAD, while a tracked path the run did not write
itself is dirty, when the remote task branch holds commits the checkout lacks
(the refusal lists them and names the git merge --strategy ours step for the run's
own abandoned commits), and when the remote cannot be read or updated.
It discards commits above the target on the task branch and uncommitted changes
to tracked paths the run wrote, and moves the remote task branch back to the
target so a later push is not rejected; it leaves untracked files, the records side and every
other branch alone, takes no raw SHA and has no force flag.
WEFT_SKIP_PUSH=1 leaves the remote task branch alone.
It clears the persisted pre-fix head and changes no other webster state; run
"lyx webster run --fresh" or "lyx webster run" afterwards, as the refusal that
sent you here says.
In standalone mode it refuses and names the git reset --keep command to run.
On success the envelope carries target, sha, mutations (the worktree_reset
entry, and a remote_branch_updated entry when the remote moved) and partial
(false). When the checkout rewrite fails after the remote moved, the error
envelope carries mutations and partial true; re-running the reset converges.

Example:
  lyx webster reset --to start`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()

			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			fail := func(msg string) error {
				clihelp.SetExit(cmd.Context(), output.Err(out, msg))
				return nil
			}

			target := websterengine.ResetTarget(to)
			if target != websterengine.ResetToStart && target != websterengine.ResetToPreFix {
				return fail(fmt.Sprintf("webster: reset --to %q is not %q or %q; way forward: re-run `lyx webster reset --to %s` or `lyx webster reset --to %s`",
					to, websterengine.ResetToStart, websterengine.ResetToPreFix, websterengine.ResetToStart, websterengine.ResetToPreFix))
			}

			mutateLock, err := websterengine.AcquireStateMutation(c.geom.ScratchDir)
			if err != nil {
				return fail(err.Error())
			}
			mutateHeld := true
			defer func() {
				if mutateHeld {
					_ = mutateLock.Release()
				}
			}()

			st, err := websterengine.LoadState(c.geom.WebsterDir, c.geom.ScratchDir)
			if err != nil {
				return fail(err.Error())
			}

			var fab *fabricengine.Fabric
			branch := standaloneBranch
			if c.openFabric != nil {
				if fab, err = c.openFabric(); err != nil {
					return fail(err.Error())
				}
				branch = fab.CurrentBranch
			}
			plan, err := websterengine.PlanReset(websterengine.ResetDeps{
				Geom:         c.geom,
				State:        st,
				Engine:       c.engine,
				ParentBranch: c.parentBranch,
				Branch:       branch,
			}, target)
			if err != nil {
				return fail(err.Error())
			}
			if fab == nil {
				return fail(fmt.Sprintf("webster: reset --to %s refused: standalone mode has no task pair for the reset to guard; way forward: run `git reset --keep %s` in the task worktree, which keeps uncommitted changes",
					target, plan.SHA))
			}

			parent, err := c.parentBranch()
			if err != nil {
				return fail(fmt.Sprintf("webster: reset --to %s refused: the parent branch is unknown (%v); way forward: run `lyx fabric reconcile` to repair the pair, then re-run `lyx webster reset --to %s`", target, err, target))
			}
			rec := fabricengine.NewMutations("")
			if err := fab.ResetPairWarp(rec, plan.SHA, parent, plan.OwnPaths, fabricengine.EnvSyncOptions()); err != nil {
				if remoteBranchMoved(rec) {
					clihelp.SetExit(cmd.Context(), output.ErrFields(out, fmt.Sprintf("webster: reset --to %s moved the remote task branch but not the checkout: %v", target, err), map[string]any{
						"mutations": rec.Entries(),
						"partial":   true,
					}))
					return nil
				}
				return fail(fmt.Sprintf("webster: reset --to %s refused: %v", target, err))
			}

			st.PreFixHead = ""
			if err := websterengine.SaveState(c.geom.WebsterDir, c.geom.ScratchDir, st); err != nil {
				return fail(fmt.Sprintf("webster: the branch was reset to %s but state.json could not be saved: %v; way forward: re-run `lyx webster reset --to %s`", plan.SHA, err, target))
			}
			_ = mutateLock.Release()
			mutateHeld = false

			if _, syncErr := fabricSync(c.openFabric, c.anchorRel, "reset"); syncErr != nil {
				return fail(fmt.Sprintf("webster: the branch was reset but the fabric sync failed: %v; %s", syncErr, fabricSyncWayForward))
			}

			clihelp.SetExit(cmd.Context(), output.Ok(out, map[string]any{
				"target":    string(plan.Target),
				"sha":       plan.SHA,
				"mutations": rec.Entries(),
				"partial":   false,
			}))
			return nil
		},
	}
	cmd.Flags().StringVar(&to, "to", "", "the recorded commit to reset to: start or pre-fix (required)")
	return cmd
}

// remoteBranchMoved reports whether rec holds the entry of a remote task branch update, the state in which a failed reset is no longer a pre-flight refusal.
func remoteBranchMoved(rec *fabricengine.Mutations) bool {
	for _, entry := range rec.Entries() {
		if entry.Kind == fabricengine.KindRemoteBranchUpdated {
			return true
		}
	}
	return false
}

// standaloneBranch is the branch probe PlanReset gets in standalone mode, where no fabric handle names the task branch.
// Standalone has no pair and no recorded parent branch, so there is no foreign branch to refuse; the verb refuses the whole reset after planning it.
func standaloneBranch() (string, error) {
	return "", nil
}
