// finalize_queue.go implements Finalize's part in the landing queue of the parent branch:
// it joins the queue, acts on the run's entry under the parent lock and lands a run whose work the parent already holds.

package landingshed

import (
	"context"
	"fmt"
	"time"

	"github.com/Knatte18/loomyard/internal/beamsetting"
	"github.com/Knatte18/loomyard/internal/landingqueue"
	"github.com/Knatte18/loomyard/internal/summaryparser"
)

// queueAction is what the run does under the parent lock with its landing queue entry.
type queueAction int

const (
	// queueOff is a run with no entry in the queue, which lands alone.
	queueOff queueAction = iota
	// queueLead is an entry with no outcome.
	queueLead
	// queueAlone is an entry whose run merges, gates and lands by itself under the lock.
	queueAlone
	// queueMergeFirst is an entry whose run merges with the lock freed before it lands by itself.
	queueMergeFirst
	// queueHeld is a run whose work the parent branch already holds.
	queueHeld
)

// joinQueue writes the run's entry in the parent branch's landing queue, with what a landing of the run takes.
// A run whose landing needs GitHub writes no entry, and so does one whose need cannot be read.
// It returns nil when the call goes on, with an entry written or none, and how the call ends otherwise.
func (fz *Finalize) joinQueue(ctx context.Context, summary summaryparser.Summary, credit string) *ending {
	if fz.deps.Queue == nil {
		return nil
	}
	if needsGitHub, err := fz.needsGitHubOnDisk(); err != nil || needsGitHub {
		return nil
	}
	if end := fz.pushRecords(); end != nil {
		return end
	}
	failed := func(what string, err error) *ending {
		return fail(ctx, finalizeName, fmt.Errorf("landingshed: %s: %s: %w", finalizeName, what, err))
	}
	trueMerge := !fz.deps.Config.Squash
	if readMode := fz.deps.PullRequest.Mode; readMode != nil {
		mode, err := readMode()
		if err != nil {
			return failed("read the pull request mode", err)
		}
		trueMerge = trueMerge || mode == beamsetting.PullRequestMergeIn
	}
	owner, err := fz.deps.Lock.Owner()
	if err != nil {
		return failed("resolve the parent lock owner", err)
	}
	taskHead, err := fz.deps.TaskHead()
	if err != nil {
		return failed("read the task head", err)
	}
	recordsHead, err := fz.deps.RecordsHead()
	if err != nil {
		return failed("read the records head", err)
	}
	_, _, message := fz.landingTexts(summary, credit)
	err = fz.deps.Queue.Join(landingqueue.Entry{
		Slug:        fz.deps.Slug,
		Pair:        fz.deps.WorktreeRoot,
		RunID:       owner.RunID,
		PID:         owner.PID,
		StartTime:   owner.StartTime,
		TaskHead:    taskHead,
		RecordsHead: recordsHead,
		Message:     message,
		TrueMerge:   trueMerge,
		JoinedAt:    time.Now().UTC(),
	})
	if err != nil {
		return failed("write the queue entry", err)
	}
	return nil
}

// actOnOutcome reads whether the parent holds the run's work and the run's queue entry, and picks what the run does.
// It is called under the parent lock.
// A landed outcome whose commit the parent no longer holds is cleared from the entry.
func (fz *Finalize) actOnOutcome(ctx context.Context) (queueAction, *ending) {
	if fz.deps.Queue == nil {
		return queueOff, nil
	}
	failed := func(what string, err error) (queueAction, *ending) {
		return queueOff, fail(ctx, finalizeName, fmt.Errorf("landingshed: %s: %s: %w", finalizeName, what, err))
	}
	entry, found, err := fz.deps.Queue.Read(fz.deps.Slug)
	if err != nil {
		return failed("read the queue entry", err)
	}
	if !found {
		return queueOff, nil
	}
	held, err := fz.parentHoldsWork()
	if err != nil {
		return queueOff, halt(ctx, finalizeName, ownerHoldsReadReason(err))
	}
	if held {
		return queueHeld, nil
	}
	switch entry.Outcome {
	case landingqueue.OutcomeFailed:
		return queueAlone, nil
	case landingqueue.OutcomeConflict:
		return queueMergeFirst, nil
	case landingqueue.OutcomeLanded:
		onParent, err := fz.deps.IsAncestor(entry.Commit, "refs/heads/"+fz.deps.ParentBranch)
		if err != nil {
			return failed("test whether the parent holds the landing commit", err)
		}
		if onParent {
			return queueHeld, nil
		}
		if err := fz.deps.Queue.SetOutcome(fz.deps.Slug, "", ""); err != nil {
			return failed("clear the queue outcome", err)
		}
	}
	return queueLead, nil
}

// landHeld finishes the landing of a run whose work the parent branch already holds, under the parent lock.
// It merges nothing, runs no gate and writes no landing commit.
// notice is queued at Done.
func (fz *Finalize) landHeld(ctx context.Context, notice string) *ending {
	if end := fz.ownerChecks(ctx, true); end != nil {
		return end
	}
	if end := fz.handedOverStop(ctx); end != nil {
		return end
	}
	commit, own, err := fz.ownLandingCommit()
	if err != nil {
		return fail(ctx, finalizeName, err)
	}
	// The landed diff of the run's own landing commit starts at its first parent.
	// A head that is not that commit has no known base.
	base := ""
	if own {
		base = commit + "^1"
	}
	fz.recordLanding(commit, base, nil)
	return fz.pushParent(ctx, commit, nil, notice)
}

// ownLandingCommit returns the commit that landed the run on the parent branch, and true.
// It is the queue entry's landed commit while the parent holds it.
// Otherwise it is the newest parent commit naming the run.
// With neither it returns the parent's head and false.
func (fz *Finalize) ownLandingCommit() (string, bool, error) {
	if fz.deps.Queue != nil {
		entry, found, err := fz.deps.Queue.Read(fz.deps.Slug)
		if err != nil {
			return "", false, fmt.Errorf("landingshed: %s: read the queue entry: %w", finalizeName, err)
		}
		if found && entry.Outcome == landingqueue.OutcomeLanded {
			onParent, err := fz.deps.IsAncestor(entry.Commit, "refs/heads/"+fz.deps.ParentBranch)
			if err != nil {
				return "", false, fmt.Errorf("landingshed: %s: test whether the parent holds the landing commit: %w", finalizeName, err)
			}
			if onParent {
				return entry.Commit, true, nil
			}
		}
	}
	if fz.deps.NewestLanding != nil {
		commit, found, err := fz.deps.NewestLanding()
		if err != nil {
			return "", false, fmt.Errorf("landingshed: %s: find the run's landing on the parent branch: %w", finalizeName, err)
		}
		if found {
			return commit, true, nil
		}
	}
	parent, err := fz.parentOpener()
	if err != nil {
		return "", false, fmt.Errorf("landingshed: %s: open the parent pair: %w", finalizeName, err)
	}
	head, err := parent.HeadSHA()
	if err != nil {
		return "", false, fmt.Errorf("landingshed: %s: read the head of parent branch %q: %w", finalizeName, fz.deps.ParentBranch, err)
	}
	return head, false, nil
}

// holdsFixerChange reports whether the run's attempt record holds a fixer change in the current row entry.
// A record that does not decode counts as holding one.
func (fz *Finalize) holdsFixerChange() (bool, error) {
	rec, reason, err := fz.readAttempts(FixerAttemptsPath(fz.deps.RecordsDir, finalizeName))
	if err != nil {
		return false, err
	}
	return reason != "" || rec.Baseline != "", nil
}

// keepOutOfBatches gives the run's queue entry the failed outcome at the task head when it has none,
// so no leader takes the run while a session works on its branch with the parent lock freed.
// It returns nil when the call goes on.
func (fz *Finalize) keepOutOfBatches(ctx context.Context) *ending {
	if fz.deps.Queue == nil {
		return nil
	}
	failed := func(what string, err error) *ending {
		return fail(ctx, finalizeName, fmt.Errorf("landingshed: %s: %s: %w", finalizeName, what, err))
	}
	entry, found, err := fz.deps.Queue.Read(fz.deps.Slug)
	if err != nil {
		return failed("read the queue entry", err)
	}
	if !found || entry.Outcome != "" {
		return nil
	}
	head, err := fz.deps.TaskHead()
	if err != nil {
		return failed("read the task head", err)
	}
	if err := fz.deps.Queue.SetOutcome(fz.deps.Slug, landingqueue.OutcomeFailed, head); err != nil {
		return failed("set the failed outcome", err)
	}
	return nil
}

// mergeNeedsSession reports whether merging the parent branch into the task branch would stop on a conflict.
func (fz *Finalize) mergeNeedsSession() (bool, error) {
	if fz.deps.MergesClean == nil {
		return false, nil
	}
	clean, err := fz.deps.MergesClean(fz.deps.ParentBranch)
	if err != nil {
		return false, fmt.Errorf("landingshed: %s: test whether parent branch %q merges cleanly: %w", finalizeName, fz.deps.ParentBranch, err)
	}
	return !clean, nil
}
