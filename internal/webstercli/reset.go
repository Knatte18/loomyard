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
		Use:         "reset",
		Short:       "move the task branch back to a commit the run recorded",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `reset moves the task worktree's HEAD, index and tracked files back to a commit
the run recorded, so a recovery needs no git reset of its own. It discards
commits above the target and uncommitted changes to tracked paths the run
wrote, and moves the remote task branch back too; untracked files, the records
side and every other branch are left alone.

Reach for it when a refusal names it as the way forward, or to start a run
over from its start commit.

--to names the target, one of:

  start            the run's start commit; also removes the run's live
                   recovery strands and archives the run record, so the
                   next "lyx webster run" starts a new run
  pre-fix          the HEAD the verify gate started its fixes from
  report-head      the head_sha of batch --batch's report
  last-batch-head  the last batch head the run recorded
  batch-start      batch --batch's recorded start commit

--batch is required for report-head and batch-start and refused for the
others. reset takes no raw SHA and has no force flag; FABRIC_SKIP_PUSH=1
leaves the remote task branch alone. A refusal names its way forward.

The envelope carries target, sha, mutations and partial; --to start and a
standalone reset add uncommitted and warnings, and --to start adds moved.

Example:
  lyx webster reset --to start
  lyx webster reset --to report-head --batch 3`,
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
