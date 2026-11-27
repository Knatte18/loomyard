// finalize_owner.go implements the owner checks of Finalize.
// The checks let a run land only on the machine that owns it and its parent.
// The file also words the repair note that a failed parent push carries after an adopter-first landing.

package landingshed

import (
	"context"
	"errors"
	"fmt"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// OwnerHaltPrefix opens the reason of every owner halt,
// so a reader of a halt note or a test tells an owner halt from any other stop.
const OwnerHaltPrefix = "owner check: "

// ownerRerun is the way forward of an owner check that could not be judged: the run's re-entry once the cause is gone.
const ownerRerun = `re-run "lym loom resume" in the task worktree`

// ownerChecks halts a landing on a machine that does not own the run and its parent.
// It returns nil when the landing may go on.
// A run whose pair has no batten run record reads as owned here, since the reconcile judges only recorded pairs,
// but its parent's owner is still read.
//
// held reports that the parent already holds the task's work, which marks a Finalize past its parent-side merge.
// Such a Finalize is judged by the reconcile's ended and stopped verdicts only.
// Neither the owner checks nor the unreachable halt keeps it from finishing.
// Otherwise the run-scope reconcile runs.
// An error is returned, an unreachable origin halts, and an ended or stopped entry stops as the reconcile's verdict.
// The run's owner is the owner of its `owned_elsewhere` entry,
// and the parent's owner comes from `Deps.ParentOwner`.
// A run another machine owns is handed over through Deps.HandOver, when it is wired, and halts naming the new owner.
// Otherwise it halts through ownerHaltReason, as does a parent another machine owns.
// A failed read of the parent's owner halts too.
func (fz *Finalize) ownerChecks(ctx context.Context, held bool) *ending {
	if held {
		// The work already reached the parent while the batten record still reads before Land;
		// a follow-up naming the cancel would release a landed entry.
		return reconcileVerdict(ctx, finalizeName, fz.deps.Reconcile, nil)
	}

	var report fabriclayout.OriginReport
	if fz.deps.Reconcile != nil {
		var err error
		if report, err = reconcileReport(ctx, finalizeName, fz.deps.Reconcile); err != nil {
			return &ending{err: err}
		}
		if report.Unreachable {
			return halt(ctx, finalizeName, OwnerHaltPrefix+fmt.Sprintf("origin was unreachable, so who owns this run could not be judged; %s once origin answers", ownerRerun))
		}
		if end := reportVerdict(ctx, finalizeName, report, fz.deps.RemoveFollowUp); end != nil {
			return end
		}
	}

	var runOwner string
	for _, entry := range report.Entries {
		if entry.State == fabriclayout.EntryOwnedElsewhere {
			runOwner = entry.Owner
			break
		}
	}

	if runOwner != "" && fz.deps.HandOver != nil {
		subject := fz.adoptSubject()
		if subject == "" {
			subject = fz.deps.Slug
		}
		return fz.handOver(ctx, runOwner, subject)
	}

	parent := ParentOwnership{Here: true}
	if fz.deps.ParentOwner != nil {
		var err error
		if parent, err = fz.deps.ParentOwner(); err != nil {
			return halt(ctx, finalizeName, OwnerHaltPrefix+fmt.Sprintf("could not read who owns the parent: %v; %s", err, ownerRerun))
		}
	}

	if runOwner == "" && parent.Here {
		return nil
	}
	return halt(ctx, finalizeName, OwnerHaltPrefix+ownerHaltReason(fz.deps.Slug, fz.adoptSubject(), runOwner, parent))
}

// adoptSubject reads what `lym adopt` takes to move the run.
// It returns the empty string when no seam is wired or the read fails, so the halt names the slug.
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

// handOver hands the run over to owner through Deps.HandOver, under a takeover entry naming owner and subject.
// It returns the halt that ends the landing; a failed hand-over is an error.
func (fz *Finalize) handOver(ctx context.Context, owner, subject string) *ending {
	err := fz.deps.HandOver(owner, subject)
	if !errors.Is(err, fabricengine.ErrHandedOver) {
		return &ending{err: fmt.Errorf("landingshed: %s: hand the run over to machine %q: %w", finalizeName, owner, err)}
	}
	return fz.handedOverHalt(ctx, err)
}

// handedOverStop reads whether another machine owns the run now and, when one does, hands the run over and halts.
// It returns nil when the landing may go on.
// An unreadable owner is returned as a transient failure, so the driver re-steps the landing once origin answers,
// and the Finalize never goes on with the owner unread.
// Nothing is read under Deps.PushSkipped, which has no origin to read it from.
func (fz *Finalize) handedOverStop(ctx context.Context) *ending {
	if fz.deps.OwnedElsewhere == nil || fz.deps.PushSkipped {
		return nil
	}
	owner, subject, elsewhere, err := fz.deps.OwnedElsewhere()
	if err != nil {
		return &ending{err: shedengine.MarkTransient(shedengine.TransientGitTransport,
			fmt.Errorf("landingshed: %s: read who owns the run: %w; the landing's re-step reads it again once origin answers", finalizeName, err))}
	}
	if !elsewhere {
		return nil
	}
	return fz.handOver(ctx, owner, subject)
}

// handedOverHalt returns the Stuck that halts a landing whose run was handed over, naming the hand-over's error.
func (fz *Finalize) handedOverHalt(ctx context.Context, handedOver error) *ending {
	return halt(ctx, finalizeName, OwnerHaltPrefix+fmt.Sprintf("%v; the landing finishes on that machine, and nothing more is merged or pushed here", handedOver))
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
	adoptRunWithParent := fmt.Sprintf("lym adopt %s %s", parentSubject, subject)
	if parentSubject == subject {
		adoptRunWithParent = "lym adopt " + subject
	}
	adoptOnParentOwner := fmt.Sprintf(`run "lym adopt %s" on machine %q`, subject, parent.Owner)

	switch {
	case runOwner == "":
		return fmt.Sprintf(`%s is owned by machine %q, so this machine does not land on it; way forward: adopt it here with "%s", which takes the run over and runs its check, after which the run goes on by itself once the check is green, or %s`,
			parentName, parent.Owner, adoptParentHere, adoptOnParentOwner)
	case parent.Here:
		return fmt.Sprintf(`this run is owned by machine %q, so this machine does not land it; way forward: run "lym adopt %s" here`, runOwner, subject)
	default:
		return fmt.Sprintf(`this run is owned by machine %q and %s by machine %q, so this machine lands neither; way forward: %s, or run "%s" here`,
			runOwner, parentName, parent.Owner, adoptOnParentOwner, adoptRunWithParent)
	}
}

// adopterFirstRepair words the repair note a failed parent push carries after an adopter-first landing.
// It returns the empty string when no note applies.
// The landing is adopter-first when origin's parent branch already holds the task's work,
// and this machine's parent branch carries the task's squash as a commit origin lacks.
// The note names the squash by the parent's head SHA, then says how to undo it.
// The undo is a reset when the squash is the only commit origin lacks.
// A rebase drops it when another commit is listed, and the note ends in removing the pair.
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
	return fmt.Sprintf(`; origin's parent branch already holds this task's work, so this machine's squash %s duplicates it; way forward: %s, then run "lym fabric remove %s"`, squash, repair, fz.deps.Slug)
}
