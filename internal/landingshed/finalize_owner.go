// finalize_owner.go implements Finalize's owner checks, which let a run land only on the machine that owns it and its parent, and the repair note a failed parent push carries after an adopter-first landing.

package landingshed

import (
	"context"
	"fmt"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// OwnerHaltPrefix opens the reason of every halt Finalize's owner checks make, so a reader of a halt note or a test tells an owner halt from any other stop.
const OwnerHaltPrefix = "owner check: "

// ownerRerun is the way forward of an owner check that could not be judged: the run's re-entry once the cause is gone.
const ownerRerun = `re-run "lym loom resume" in the task worktree`

// ownerChecks halts a landing on a machine that does not own the run and its parent.
// stop is false when the landing may go on, including a run whose pair has no batten run record, which the reconcile judges not at all and so reads as owned here.
// Otherwise the other values are what Call returns at once.
//
// A parent already holding the task's work marks a Finalize past its parent-side merge.
// It is judged by the reconcile's ended and stopped verdicts only, so neither owner check nor the unreachable halt keeps it from finishing.
// An error reading that mark halts, so a doubt never skips the checks.
// Otherwise the run-scope reconcile runs: an error is returned, an unreachable origin halts, and an ended or stopped entry stops as the reconcile's verdict.
// The run's owner is the owner of its owned_elsewhere entry, and the parent's comes from Deps.ParentOwner;
// a parent or run owned by another machine halts through ownerHaltReason, and so does a failed read of the parent's owner.
func (fz *Finalize) ownerChecks(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error, bool) {
	if fz.deps.ParentHoldsTaskWork != nil {
		held, err := fz.deps.ParentHoldsTaskWork()
		if err != nil {
			return fz.ownerHalt(ctx, fmt.Sprintf("could not read whether the parent branch already holds the task's work: %v; %s", err, ownerRerun))
		}
		if held {
			return reconcileVerdict(ctx, finalizeName, fz.deps.Reconcile)
		}
	}

	var report fabricengine.OriginReport
	if fz.deps.Reconcile != nil {
		var err error
		if report, err = reconcileReport(ctx, finalizeName, fz.deps.Reconcile); err != nil {
			return "", shedengine.OutputPointer{}, err, true
		}
		if report.Unreachable {
			return fz.ownerHalt(ctx, fmt.Sprintf("origin was unreachable, so who owns this run could not be judged; %s once origin answers", ownerRerun))
		}
		if outcome, out, err, stop := reportVerdict(ctx, finalizeName, report); stop {
			return outcome, out, err, true
		}
	}

	var runOwner string
	for _, entry := range report.Entries {
		if entry.State == fabricengine.EntryOwnedElsewhere {
			runOwner = entry.Owner
			break
		}
	}

	parent := ParentOwnership{Here: true}
	if fz.deps.ParentOwner != nil {
		var err error
		if parent, err = fz.deps.ParentOwner(); err != nil {
			return fz.ownerHalt(ctx, fmt.Sprintf("could not read who owns the parent: %v; %s", err, ownerRerun))
		}
	}

	if runOwner == "" && parent.Here {
		return "", shedengine.OutputPointer{}, nil, false
	}
	return fz.ownerHalt(ctx, ownerHaltReason(fz.deps.Slug, fz.adoptSubject(), runOwner, parent))
}

// adoptSubject reads what `lym adopt` takes to move the run, "" when no seam is wired or the read fails, which ownerHaltReason reads as the slug.
func (fz *Finalize) adoptSubject() string {
	if fz.deps.AdoptSubject == nil {
		return ""
	}
	subject, err := fz.deps.AdoptSubject()
	if err != nil {
		logger.Warn("landingshed: read what adopt takes for the run failed; the halt names the slug", "producer", finalizeName, "cause", err)
		return ""
	}
	return subject
}

// ownerHalt returns the halting Stuck of an owner check with reason behind OwnerHaltPrefix, which writes the loom halt note.
func (fz *Finalize) ownerHalt(ctx context.Context, reason string) (shedengine.Outcome, shedengine.OutputPointer, error, bool) {
	outcome, out, err := stuckVerdict(ctx, finalizeName, OwnerHaltPrefix+reason, true)
	return outcome, out, err, true
}

// ownerHaltReason words the reason of an owner halt for the run slug.
// subject is what `lym adopt` takes to move the run, the beam or `root`, and reads as slug when empty.
// runOwner is the machine owning the run, empty when this machine does; the parent is judged by parent.Here.
// Each wording names the owners at fault and the adopt commands that move them, with `root` for the prime.
func ownerHaltReason(slug, subject, runOwner string, parent ParentOwnership) string {
	if subject == "" {
		subject = slug
	}
	parentSubject, parentName := "root", "the prime"
	if parent.Slug != "" {
		parentSubject, parentName = parent.Slug, fmt.Sprintf("the parent pair %q", parent.Slug)
	}
	adoptParentHere := "lym adopt " + parentSubject
	adoptRunWithParent := fmt.Sprintf("lym adopt %s %s --keep-local", parentSubject, subject)
	if parentSubject == subject {
		adoptRunWithParent = fmt.Sprintf("lym adopt %s --keep-local", subject)
	}
	adoptOnParentOwner := fmt.Sprintf(`run "lym adopt %s" on machine %q`, subject, parent.Owner)

	switch {
	case runOwner == "":
		return fmt.Sprintf(`%s is owned by machine %q, so this machine does not land on it; way forward: adopt it here with "%s", which resumes the halted run, or %s, which stops at its first step and names "--keep-local" and "--drop-local" when that machine drove the run before and holds local commits or a live pair for it`,
			parentName, parent.Owner, adoptParentHere, adoptOnParentOwner)
	case parent.Here:
		return fmt.Sprintf(`this run is owned by machine %q, so this machine does not land it; way forward: run "lym adopt %s --keep-local" here`, runOwner, subject)
	default:
		return fmt.Sprintf(`this run is owned by machine %q and %s by machine %q, so this machine lands neither; way forward: %s, or run "%s" here`,
			runOwner, parentName, parent.Owner, adoptOnParentOwner, adoptRunWithParent)
	}
}

// adopterFirstRepair words the repair note a failed parent push carries after an adopter-first landing, or "" when none applies.
// The landing is adopter-first when origin's parent branch already holds the task's work while this machine's parent branch carries the task's squash as a commit origin lacks.
// The note names the squash by the parent's head SHA, then undoes it with a reset when it is the only commit origin lacks, or a rebase dropping it when another commit is listed, and ends in removing the pair.
// An unreadable seam, origin's parent lacking the work and no commit to repair all add nothing.
func (fz *Finalize) adopterFirstRepair(parentHandle parentMerger) string {
	if fz.deps.OriginParentHoldsWork == nil || fz.deps.ParentCommitsOriginLacks == nil {
		return ""
	}
	held, err := fz.deps.OriginParentHoldsWork()
	if err != nil {
		logger.Warn("landingshed: read whether origin's parent holds the task's work failed", "producer", finalizeName, "cause", err)
		return ""
	}
	if !held {
		return ""
	}
	commits, err := fz.deps.ParentCommitsOriginLacks()
	if err != nil {
		logger.Warn("landingshed: list the parent commits origin lacks failed", "producer", finalizeName, "cause", err)
		return ""
	}
	if len(commits) == 0 {
		return ""
	}
	squash, err := parentHandle.HeadSHA()
	if err != nil {
		logger.Warn("landingshed: read the parent head failed", "producer", finalizeName, "cause", err)
		return ""
	}
	repair := fmt.Sprintf(`run "git reset --keep origin/%s" in the parent's worktree`, fz.deps.ParentBranch)
	if len(commits) > 1 {
		repair = fmt.Sprintf(`run "git rebase --onto %s~1 %s %s" in the parent's worktree, and on a conflict run "git rebase --abort" and drop the squash by hand`, squash, squash, fz.deps.ParentBranch)
	}
	return fmt.Sprintf(`; origin's parent branch already holds this task's work, so this machine's squash %s duplicates it; way forward: %s, then run "lym fabric remove %s", leaving the owner pause standing`, squash, repair, fz.deps.Slug)
}
