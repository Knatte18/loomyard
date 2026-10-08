// reset.go implements the `reset` webster verb: the guarded way to move the task branch back to a commit the run recorded.
// It plans the reset with websterengine.PlanReset, performs it through fabricengine's pair-checkout reset (in standalone mode, with gitrepo's keep-reset), clears the persisted pre-fix head and fabric-syncs state.json.
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
	"github.com/Knatte18/loomyard/internal/gitrepo"
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
In standalone mode there is no task pair, so it moves HEAD with git's keep form
instead, touching no remote: an uncommitted change is carried across, and when
the move would overwrite one the reset refuses, changing nothing, and names each
such path with the step that clears it; a tracked change outside the run's own
writes does not refuse there, since keep guards it.
On success the envelope carries target, sha, mutations (the worktree_reset
entry, and a remote_branch_updated entry when the remote moved) and partial
(false). --to start, and every standalone reset, also carries uncommitted (the
worktree paths left uncommitted outside the run's own state; absent when git
status fails) and warnings (the findings the
archive dropped, and the git status failure). --to start also carries moved
(false when the start could not be moved to and the record was only archived,
with the reason key naming why).
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
				Standalone:   fab == nil,
			}, target, batch)
			if err != nil {
				return fail(err.Error())
			}

			var parent string
			if !plan.ArchiveOnly && fab != nil {
				if parent, err = c.parentBranch(); err != nil {
					return fail(fmt.Sprintf("webster: reset --to %s refused: the parent branch is unknown (%v); way forward: run `lyx fabric reconcile` to repair the pair, then re-run `lyx webster reset --to %s`", target, err, target))
				}
			}
			if target == websterengine.ResetToStart {
				var removeErr *websterengine.RecoveryStrandRemoveError
				if errors.As(websterengine.RemoveRecoveryStrands(c.runner, st), &removeErr) {
					return fail(fmt.Sprintf("webster: reset --to %s refused: %v; way forward: run `lyx reed remove %s`, then re-run `lyx webster reset --to %s`", target, removeErr, removeErr.GUID, target))
				}
			}
			rec := fabricengine.NewMutations("")
			if !plan.ArchiveOnly {
				if fab == nil {
					if err := c.standaloneKeepReset(plan); err != nil {
						return fail(err.Error())
					}
				} else if err := fab.ResetPairCode(rec, plan.SHA, parent, plan.OwnPaths, fabricengine.EnvSyncOptions()); err != nil {
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
			// A standalone reset keeps every change git carries across, so it lists what it left whatever the target.
			if target == websterengine.ResetToStart || fab == nil {
				// The reset is done by now, so a failed listing is reported beside it rather than as a refusal.
				if uncommitted, err := websterengine.UncommittedPaths(c.geom); err != nil {
					warnings = append(warnings, fmt.Sprintf("the uncommitted paths could not be read: %v; `git status` in the task worktree lists them", err))
				} else {
					fields["uncommitted"] = append([]string{}, uncommitted...)
				}
				fields["warnings"] = warnings
			}
			if target == websterengine.ResetToStart {
				fields["moved"] = !plan.ArchiveOnly
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

// standaloneKeepReset performs the planned branch move in standalone mode with git's keep form, touching no remote.
// Keep carries every uncommitted change across and refuses, changing nothing, over one the move would overwrite;
// that refusal comes back as websterengine.KeepResetRefusal, naming each such path and its way forward.
func (c *websterCLI) standaloneKeepReset(plan websterengine.ResetPlan) error {
	repo := gitrepo.New(c.geom.WorktreeRoot)
	cause := repo.ResetKeep(plan.SHA)
	if cause == nil {
		return nil
	}
	var overwritten []string
	changed, changedErr := repo.WorktreeChangedFiles()
	moved, movedErr := repo.ChangedFilesSince(plan.SHA)
	if changedErr == nil && movedErr == nil {
		for _, path := range changed {
			if slices.Contains(moved, path) {
				overwritten = append(overwritten, path)
			}
		}
		slices.Sort(overwritten)
	}
	return websterengine.KeepResetRefusal(plan, cause, overwritten)
}

// standaloneBranch is the branch probe PlanReset gets in standalone mode, where no fabric handle names the task branch.
// Standalone has no pair and no recorded parent branch, so there is no foreign branch to refuse.
func standaloneBranch() (string, error) {
	return "", nil
}
