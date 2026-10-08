// reset.go implements the `reset` webster verb: the guarded way to move the task branch back to a commit the run recorded.
// It plans the reset with websterengine.PlanReset, performs it through fabricengine's pair-checkout reset, clears the persisted pre-fix head and fabric-syncs state.json.
// A reset to start ends by archiving the run record, or archives alone when the start cannot be moved to.
// The verb runs no git of its own, so an agent that is denied `git reset --hard` still has a way to recover.
package webstercli

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/websterengine"
	"github.com/spf13/cobra"
)

// resetCmd builds the `reset` subcommand.
func (c *websterCLI) resetCmd() *cobra.Command {
	var to string
	var batch int
	cmd := &cobra.Command{
		Use:   "reset --to start|pre-fix|report-head|last-batch-head|batch-start [--batch NN]",
		Short: "move the task branch back to a commit the run recorded",
		Long: `reset moves the task worktree's HEAD, index and tracked files back to a commit
the run recorded, so a recovery needs no git reset of its own.
--to start is the run's start commit: the oldest recorded batch start, or the
octopus merge-base of the starts when none is the oldest.
--to pre-fix is the HEAD the verify gate started its fixes from.
--to report-head --batch NN is the head_sha of batch NN's report: the batch is
begun and not terminal, its report parses, the head descends from the batch's
start, no later batch is begun and no recovery strand of the batch is live.
--to last-batch-head is the last batch head the run recorded.
--to batch-start --batch NN is batch NN's recorded start commit, refused when a
later batch recorded a start.
--batch is required for report-head and batch-start and refused for the others.
--to start removes the run's live recovery strands first, so no recovery agent keeps writing into the tree the reset moves; the other targets remove none.
It refuses, changing nothing, while a run holds the run lock (except --to
report-head, which Master runs inside its run), during a merge, off
the task branch, with no recorded target, when the target commit is missing or
is not an ancestor of HEAD (--to start archives without moving instead, in all
three cases and when the starts share no common ancestor), while a tracked path the run did not write
itself is dirty, when the remote task branch holds commits the checkout lacks
(the refusal lists them and names the git merge --strategy ours step for the run's
own abandoned commits), and when the remote cannot be read or updated.
It discards commits above the target on the task branch and uncommitted changes
to tracked paths the run wrote, and moves the remote task branch back to the
target so a later push is not rejected; it leaves untracked files, the records side and every
other branch alone, takes no raw SHA and has no force flag.
FABRIC_SKIP_PUSH=1 leaves the remote task branch alone.
It clears the persisted pre-fix head and changes no other webster state, except
that --to start also archives the run record (state.json and the reports dir
renamed with a stamp, the rendered prompts cleared) behind a pending-findings
guard judged against HEAD, so a following "lyx webster run" starts a new run.
The guard refuses, with the move already made, only on a contract file a fork
wrote last, a plan path that differs from the recorded plan, or a suspect path
that differs from HEAD, each naming its clearing step; re-running the reset
then converges. Every other pending finding is dropped with a warning.
Otherwise run what the refusal that sent you here says.
In standalone mode it refuses and names the git reset --keep command to run.
On success the envelope carries target, sha, mutations (the worktree_reset
entry, and a remote_branch_updated entry when the remote moved) and partial
(false). --to start also carries moved (false when the start could not be moved
to and the record was only archived, with the reason key naming why), uncommitted
(the worktree paths left uncommitted outside the run's own state, which the next
run starts over) and warnings (the findings the archive dropped).
When the checkout rewrite fails after the remote moved, the error
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
			if !slices.Contains(websterengine.ResetTargets, target) {
				names := make([]string, len(websterengine.ResetTargets))
				for i, known := range websterengine.ResetTargets {
					names[i] = string(known)
				}
				return fail(fmt.Sprintf("webster: reset --to %q is not one of %s; way forward: re-run `lyx webster reset --to start`, or name another target, adding --batch NN for %s or %s",
					to, strings.Join(names, ", "), websterengine.ResetToReportHead, websterengine.ResetToBatchStart))
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
				Reed:         c.reed,
				ParentBranch: c.parentBranch,
				Branch:       branch,
			}, target, batch)
			if err != nil {
				return fail(err.Error())
			}
			if fab == nil && !plan.ArchiveOnly {
				return fail(fmt.Sprintf("webster: reset --to %s refused: standalone mode has no task pair for the reset to guard; way forward: run `git reset --keep %s` in the task worktree, which keeps uncommitted changes",
					target, plan.SHA))
			}

			var parent string
			if !plan.ArchiveOnly {
				if parent, err = c.parentBranch(); err != nil {
					return fail(fmt.Sprintf("webster: reset --to %s refused: the parent branch is unknown (%v); way forward: run `lyx fabric reconcile` to repair the pair, then re-run `lyx webster reset --to %s`", target, err, target))
				}
			}
			if target == websterengine.ResetToStart {
				var removeErr *websterengine.RecoveryStrandRemoveError
				if errors.As(websterengine.RemoveRecoveryStrands(c.reed, st), &removeErr) {
					return fail(fmt.Sprintf("webster: reset --to %s refused: %v; way forward: run `lyx reed remove %s`, then re-run `lyx webster reset --to %s`", target, removeErr, removeErr.GUID, target))
				}
			}
			rec := fabricengine.NewMutations("")
			if !plan.ArchiveOnly {
				if err := fab.ResetPairCode(rec, plan.SHA, parent, plan.OwnPaths, fabricengine.EnvSyncOptions()); err != nil {
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
			}

			warnings := []string{}
			if target == websterengine.ResetToStart {
				dropped, err := websterengine.ArchiveRunAfterReset(c.engine, c.geom, st)
				if err != nil {
					if plan.ArchiveOnly {
						return fail(err.Error())
					}
					return fail(fmt.Sprintf("webster: the branch was reset to %s but the run record was not archived: %v", plan.SHA, err))
				}
				warnings = append(warnings, dropped...)
			}
			_ = mutateLock.Release()
			mutateHeld = false

			if _, syncErr := fabricSync(c.openFabric, c.anchorRel, "reset"); syncErr != nil {
				return fail(fmt.Sprintf("webster: the branch was reset but the fabric sync failed: %v; %s", syncErr, fabricSyncWayForward))
			}

			fields := map[string]any{
				"target":    string(plan.Target),
				"mutations": rec.Entries(),
				"partial":   false,
			}
			if !plan.ArchiveOnly {
				fields["sha"] = plan.SHA
			}
			if target == websterengine.ResetToStart {
				uncommitted, err := websterengine.UncommittedPaths(c.geom)
				if err != nil {
					return fail(fmt.Sprintf("webster: the run record was archived but the uncommitted paths could not be read: %v", err))
				}
				fields["moved"] = !plan.ArchiveOnly
				fields["uncommitted"] = append([]string{}, uncommitted...)
				fields["warnings"] = warnings
				if plan.ArchiveOnly {
					fields["reason"] = plan.Reason
				}
			}
			clihelp.SetExit(cmd.Context(), output.Ok(out, fields))
			return nil
		},
	}
	cmd.Flags().StringVar(&to, "to", "", "the recorded commit to reset to: start, pre-fix, report-head, last-batch-head or batch-start (required); report-head and batch-start take --batch")
	cmd.Flags().IntVar(&batch, "batch", 0, "the batch number report-head and batch-start resolve against (refused for the other targets)")
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
