// parentalign.go implements the Parent-Align producer.
// It merges the parent's tip into the task worktree with no parent lock and no write to the parent pair.
// It runs the `landing` gate over the merged tree and writes the merge record the guarded landing rows read.

package landingshed

import (
	"context"
	"fmt"

	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/mergeresolve"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/verifytree"
)

// parentAlignName is the producer name that Parent-Align's log lines and error text carry.
// It is also the row name its failure record carries.
const parentAlignName = "Parent-Align"

// mergeBefore is what a landing row reads before it merges the parent into the task, which the merge record carries.
type mergeBefore struct {
	// taskBase is the parent tip last merged into the task, empty when no merge-base read is wired.
	taskBase string
	// taskHead is the task head before the merge.
	taskHead string
	// earlier is the merge record the last merge wrote, zero when none is readable.
	// An absent aligned tip in it reads as the task's fork point from the parent.
	earlier MergeRecord
}

// readMergeBefore reads, for producer, the task's head and merge base and the merge record the last merge wrote.
// An unreadable earlier record is logged and read as absent, so a damaged record cannot block every later merge.
func readMergeBefore(deps Deps, producer string) (mergeBefore, error) {
	var before mergeBefore
	var err error
	if before.taskHead, err = deps.TaskHead(); err != nil {
		return before, fmt.Errorf("landingshed: %s: read the task head: %w", producer, err)
	}
	if deps.MergeBase != nil {
		if before.taskBase, err = deps.MergeBase(deps.ParentBranch); err != nil {
			return before, fmt.Errorf("landingshed: %s: find the parent tip last merged into the task: %w", producer, err)
		}
	}
	if earlier, found, err := ReadMergeRecord(deps.MergeRecordPath); err != nil {
		logger.Warn("landingshed: earlier merge record unreadable, reading it as absent", "producer", producer, "cause", err)
	} else if found {
		before.earlier = earlier
	}
	return before, nil
}

// ParentAlign is the ShedProducer that merges the parent branch's tip into the task worktree and records the merge.
type ParentAlign struct {
	deps     Deps
	resolver resolver
	gate     verifyGate
	premerge preMerge
	guard    parentGuard
	// before is set by Call before the merge and read by writeRecord.
	before mergeBefore
	// attempts is the entry's fixer attempt record, set by Call before the merge and kept current as the fixer runs.
	attempts FixerAttempts
}

// The values of FixerAttempts.Halt, which notes the halt that last stopped an entry:
// spent means the entry has no attempts left;
// over-cap marks a change over the cap at Parent-Align;
// review-capped marks an over-cap change at Finalize whose fix reviews ran out of rounds without approving it.
const (
	fixerHaltSpent        = "spent"
	fixerHaltOverCap      = "over-cap"
	fixerHaltReviewCapped = "review-capped"
)

var _ shedengine.ShedProducer = (*ParentAlign)(nil)

// NewParentAlign constructs a ParentAlign from deps.
// It returns an error when a required field of deps is nil or empty, and the error names that field.
// It builds the resolver at construction time.
func NewParentAlign(deps Deps) (*ParentAlign, error) {
	switch {
	case deps.OpenFabric == nil:
		return nil, fmt.Errorf("landingshed: NewParentAlign: Deps.OpenFabric must not be nil")
	case deps.OpenParentFabric == nil:
		return nil, fmt.Errorf("landingshed: NewParentAlign: Deps.OpenParentFabric must not be nil")
	case deps.ReadParentTips == nil:
		return nil, fmt.Errorf("landingshed: NewParentAlign: Deps.ReadParentTips must not be nil")
	case deps.ChangedPaths == nil:
		return nil, fmt.Errorf("landingshed: NewParentAlign: Deps.ChangedPaths must not be nil")
	case deps.MergeBase == nil:
		return nil, fmt.Errorf("landingshed: NewParentAlign: Deps.MergeBase must not be nil")
	case deps.TaskHead == nil:
		return nil, fmt.Errorf("landingshed: NewParentAlign: Deps.TaskHead must not be nil")
	case deps.Lock.Store == nil:
		return nil, fmt.Errorf("landingshed: NewParentAlign: Deps.Lock.Store must not be nil")
	case deps.MergeRecordPath == "":
		return nil, fmt.Errorf("landingshed: NewParentAlign: Deps.MergeRecordPath must not be empty")
	}

	res, err := newResolver(deps, "NewParentAlign")
	if err != nil {
		return nil, err
	}
	return &ParentAlign{
		deps:     deps,
		resolver: res,
		gate:     newLandingGate(deps),
		premerge: newPreMerge(deps),
		guard:    newParentGuard(deps),
	}, nil
}

// Call runs one Parent-Align iteration.
//
// It runs the run-scope origin reconcile after the status commit:
// a run origin ended or stopped is Stuck, and an unreachable origin proceeds.
// It writes nothing in the parent pair and holds no parent lock across the merge.
// A paused lock wait and an unreadable lock record for the local-tip read are Stuck with Halt,
// so the run stops at this row.
// After the merge and the `landing` gate it aligns:
// it runs the merge fixer when the parent brought in commits or the gate fails,
// and every halt of that alignment is a Stuck with Halt whose reason opens with LandingStopPrefix.
// A gate whose result is a runner crash or a memory pressure stop judged no code:
// it halts the same way before any alignment, with no fixer.
// A fixer's escalate on a tapestry, darn or sampler run (Deps.EscalateJudgedFirst) opens with EscalateStopPrefix.
// A failed record write and every infrastructure fault are errors.
func (m *ParentAlign) Call(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
	if err := entryErr(ctx, parentAlignName); err != nil {
		return "", shedengine.OutputPointer{}, err
	}

	// The status file is rewritten on every transition and committed only at bootstrap,
	// and the merge guard refuses any tracked modification, so it is committed here.
	if m.deps.CommitStatus != nil {
		if err := m.deps.CommitStatus(); err != nil {
			return "", shedengine.OutputPointer{}, fmt.Errorf("landingshed: %s: commit status file: %w", parentAlignName, err)
		}
	}

	// The run-scope reconcile judges and never moves the parent.
	if end := reconcileVerdict(ctx, parentAlignName, m.deps.Reconcile, m.deps.RemoveFollowUp); end != nil {
		return end.result()
	}

	tip, lockReason, err := parentMergeTip(ctx, m.deps, m.guard, parentAlignName, false, false)
	if err != nil {
		return fail(ctx, parentAlignName, err).result()
	}
	if lockReason != "" {
		return halt(ctx, parentAlignName, lockReason).result()
	}

	if m.before, err = readMergeBefore(m.deps, parentAlignName); err != nil {
		return "", shedengine.OutputPointer{}, err
	}
	attemptsReason, err := m.readAttempts()
	if err != nil {
		return "", shedengine.OutputPointer{}, err
	}
	if attemptsReason != "" {
		return halt(ctx, parentAlignName, attemptsReason).result()
	}

	reason, err := m.premerge.clear(parentAlignName, m.deps.ParentBranch)
	if end := gateEnding(ctx, parentAlignName, reason, err); end != nil {
		return end.result()
	}
	reason, err = m.gate.clean(parentAlignName, "before the merge-in")
	if end := gateEnding(ctx, parentAlignName, reason, err); end != nil {
		return end.result()
	}

	merge, err := m.resolver.Resolve(ctx, m.deps.ParentBranch, tip)
	if err != nil {
		return fail(ctx, parentAlignName, fmt.Errorf("landingshed: %s: merge-in against parent branch %q: %w", parentAlignName, m.deps.ParentBranch, err)).result()
	}
	if merge.Outcome == mergeresolve.OutcomeStuck {
		return stuck(ctx, parentAlignName, merge.Reason).result()
	}
	mergeCommit := ""
	if !merge.AlreadyUpToDate {
		if mergeCommit, err = m.deps.TaskHead(); err != nil {
			return "", shedengine.OutputPointer{}, fmt.Errorf("landingshed: %s: read the merge commit: %w", parentAlignName, err)
		}
	}
	m.gate.recorder.mergeCommit = mergeCommit

	// The verify runs on committed content,
	// and a verify that dirties the tree is a non-hermetic test that halts rather than ships.
	reason, err = m.gate.clean(parentAlignName, "after the merge-in")
	if end := gateEnding(ctx, parentAlignName, reason, err); end != nil {
		return end.result()
	}
	gateReason, unjudged, err := m.gate.check(ctx, parentAlignName, m.deps.ParentBranch)
	if err != nil {
		return fail(ctx, parentAlignName, err).result()
	}
	if unjudged {
		return halt(ctx, parentAlignName, gateReason).result()
	}
	if gateReason == "" {
		reason, err = m.gate.clean(parentAlignName, "after the verify")
		if end := gateEnding(ctx, parentAlignName, reason, err); end != nil {
			return end.result()
		}
	}

	if end := m.align(ctx, tip, mergeCommit, gateReason); end != nil {
		return end.result()
	}
	m.gate.clearFailure()

	if err := m.writeRecord(tip, mergeCommit, tip, merge); err != nil {
		return "", shedengine.OutputPointer{}, err
	}
	if err := m.removeAttempts(); err != nil {
		return "", shedengine.OutputPointer{}, err
	}
	if cerr := cancelErr(ctx, parentAlignName); cerr != nil {
		return "", shedengine.OutputPointer{}, cerr
	}
	return shedengine.Done, shedengine.OutputPointer{}, nil
}

// parentMergeTip picks the parent tip producer merges with no parent lock held across the merge.
// It returns that tip, or a Stuck reason of the parent lock wait.
// With an upstream and no push skip, it fetches and takes the upstream tip,
// so a commit on the local parent that origin lacks is not merged in, except the owed landing tips below.
// With the push skipped (Deps.PushSkipped) or no upstream, it takes the local tip.
// When takeOwedLocal is set, it also takes the local tip of a parent ahead of its upstream by owed landing tips alone.
// Deps.ParentAheadByOwed reports that state.
// A landing lands on top of those tips, so the local tip is the head it compares with.
// It holds the parent lock through guard only to read the local tip, so the merge itself runs unlocked.
// Under that lock it asks again about an owed-ahead parent.
// It takes the upstream tip when a commit that is no owed landing has arrived since.
// When front is set, the read waits for the lock at the front of the queue.
// A paused lock wait and an unreadable lock record come back as the reason, for a halting Stuck.
func parentMergeTip(ctx context.Context, deps Deps, guard parentGuard, producer string, front, takeOwedLocal bool) (string, string, error) {
	branch := deps.ParentBranch
	readAheadByOwed := func() (bool, error) {
		owed, err := deps.ParentAheadByOwed()
		if err != nil {
			return false, fmt.Errorf("landingshed: %s: read whether parent branch %q is ahead by owed landings alone: %w", producer, branch, err)
		}
		return owed, nil
	}
	// owedUpstream is the upstream tip of a parent first read as ahead by owed landing tips alone, else empty.
	owedUpstream := ""
	if !deps.PushSkipped {
		tips, err := deps.ReadParentTips(true)
		if err != nil {
			return "", "", fmt.Errorf("landingshed: %s: read the tips of parent branch %q: %w", producer, branch, err)
		}
		switch tips.Relation {
		case fabriclayout.ParentNoUpstream:
		case fabriclayout.ParentAhead:
			if !takeOwedLocal || deps.ParentAheadByOwed == nil {
				return tips.Upstream, "", nil
			}
			owed, err := readAheadByOwed()
			if err != nil {
				return "", "", err
			}
			if !owed {
				return tips.Upstream, "", nil
			}
			owedUpstream = tips.Upstream
		case fabriclayout.ParentEqual, fabriclayout.ParentBehind, fabriclayout.ParentDiverged:
			return tips.Upstream, "", nil
		default:
			return "", "", fmt.Errorf("landingshed: %s: unknown relation %q of parent branch %q to its upstream", producer, tips.Relation, branch)
		}
	}

	reason, err := guard.ensure(ctx, producer, front)
	if err != nil || reason != "" {
		return "", reason, err
	}
	defer guard.release(producer)
	tips, err := deps.ReadParentTips(false)
	if err != nil {
		return "", "", fmt.Errorf("landingshed: %s: read the tips of parent branch %q: %w", producer, branch, err)
	}
	// The first answer was read with no lock, and a commit may have reached the parent before the tip just read.
	if owedUpstream != "" {
		owed, err := readAheadByOwed()
		if err != nil {
			return "", "", err
		}
		if !owed {
			return owedUpstream, "", nil
		}
	}
	return tips.Local, "", nil
}

// removeAttempts removes the row's fixer attempt record.
func (m *ParentAlign) removeAttempts() error {
	return RemoveFixerAttempts(FixerAttemptsPath(m.deps.RecordsDir, parentAlignName))
}

// readAttempts reads the entry's fixer attempt record into `m.attempts` and fixes the entry's fork point.
// readAttempts writes `AlignFrom` only when the merge record has no aligned tip and the attempt record no fork point.
// The value is the task's fork point from the parent, read before the merge,
// so a resumed entry whose merge is already up to date still reads the parent commits the first merge brought in.
// An attempt record that does not decode comes back as a reason, for a halting Stuck.
func (m *ParentAlign) readAttempts() (string, error) {
	entry, err := m.deps.RowEntry(parentAlignName)
	if err != nil {
		return "", fmt.Errorf("landingshed: %s: read the row entry: %w", parentAlignName, err)
	}
	ownPath := FixerAttemptsPath(m.deps.RecordsDir, parentAlignName)
	rec, reason, err := readRowAttempts(parentAlignName, ownPath, entry)
	if err != nil || reason != "" {
		return reason, err
	}
	if m.before.earlier.AlignedTip == "" && rec.AlignFrom == "" {
		rec.AlignFrom = m.before.taskBase
		if err := WriteFixerAttempts(ownPath, rec); err != nil {
			return "", err
		}
	}
	m.attempts = rec
	return "", nil
}

// align decides what the parent's merge needs, given the `landing` gate's failure `gateReason`, or "" for a pass.
// The parent brought in commits when its merged tip adds something over the aligned tip.
// With no aligned tip in the merge record, the entry's fork point stands in for it.
// A merge that brought none over a passing gate is aligned with no fixer.
// Parent commits brought in, or a failing gate, run the merge fixer on the attempts the entry has left.
// A passing gate on a resume after an over-cap halt reads the operator's work as the alignment and spawns nothing.
// An entry with no attempts left spawns no fixer.
// It returns nil when Parent-Align may proceed to record the merge and advance the aligned tip.
// Otherwise it returns how the call ends.
func (m *ParentAlign) align(ctx context.Context, tip, mergeCommit string, gateReason string) *ending {
	attemptsPath := FixerAttemptsPath(m.deps.RecordsDir, parentAlignName)
	rec := m.attempts
	passed := gateReason == ""
	capacity := m.deps.Config.ParentAlignFixAttempts
	alignFrom := m.before.earlier.AlignedTip
	if alignFrom == "" {
		alignFrom = rec.AlignFrom
	}

	overCapResume := rec.Halt == fixerHaltOverCap
	if overCapResume {
		head, err := m.deps.TaskHead()
		if err != nil {
			return fail(ctx, parentAlignName, fmt.Errorf("landingshed: %s: read the task head: %w", parentAlignName, err))
		}
		rec.Baseline, rec.Halt = head, ""
		if err := WriteFixerAttempts(attemptsPath, rec); err != nil {
			return fail(ctx, parentAlignName, err)
		}
		m.attempts = rec
	}
	attemptsLeft := rec.Spent < capacity

	switch {
	case passed && overCapResume:
		return nil
	case passed && !attemptsLeft:
		return m.judgeFixerChange(ctx, rec)
	case !passed && !attemptsLeft:
		return m.haltSpent(ctx, gateReason)
	case passed:
		holds, err := m.deps.HoldsWork(alignFrom, tip)
		if err != nil {
			return fail(ctx, parentAlignName, fmt.Errorf("landingshed: %s: test whether %s adds anything over %s: %w", parentAlignName, tip, alignFrom, err))
		}
		if holds {
			return m.judgeFixerChange(ctx, rec)
		}
	}

	if rec.Baseline == "" {
		head, err := m.deps.TaskHead()
		if err != nil {
			return fail(ctx, parentAlignName, fmt.Errorf("landingshed: %s: read the task head: %w", parentAlignName, err))
		}
		rec.Baseline = head
	}
	outcome, updated, err := newFixerRun(m.deps, parentAlignName, m.gate).run(ctx, FixerModeParentAlign, rec, capacity, [2]string{alignFrom, tip})
	m.attempts = updated
	if err != nil {
		return fail(ctx, parentAlignName, err)
	}
	switch outcome.end {
	case fixerEndFixed:
		return m.judgeFixerChange(ctx, updated)
	case fixerEndSpent:
		updated.Halt = fixerHaltSpent
		if err := WriteFixerAttempts(attemptsPath, updated); err != nil {
			return fail(ctx, parentAlignName, err)
		}
		m.attempts = updated
	}
	return halt(ctx, parentAlignName, outcome.haltReason(parentAlignName, m.deps.EscalateJudgedFirst))
}

// haltSpent halts the entry whose attempts are spent over a gate that still fails, naming the failed gate's log.
func (m *ParentAlign) haltSpent(ctx context.Context, gateReason string) *ending {
	log := singleLine(gateReason)
	failure, found, err := verifytree.ReadLandingFailure(m.gate.paths)
	if err != nil {
		return fail(ctx, parentAlignName, err)
	}
	if found {
		log = failure.LogPath
	}
	return halt(ctx, parentAlignName, fixerOutcome{end: fixerEndSpent, failureLog: log}.haltReason(parentAlignName, m.deps.EscalateJudgedFirst))
}

// judgeFixerChange judges the entry's fixer change against the caps when no review follows Parent-Align.
// The change runs from the fixer's baseline to HEAD.
// An entry whose fixer never ran has no baseline and no change.
// A change over the caps halts with the fixer's commits named and notes over-cap in the attempt record;
// a change within them, or one a following review will see, returns nil so Parent-Align proceeds.
func (m *ParentAlign) judgeFixerChange(ctx context.Context, rec FixerAttempts) *ending {
	if rec.Baseline == "" {
		return nil
	}
	reviewOff := true
	if m.deps.CodeReviewOff != nil {
		off, err := m.deps.CodeReviewOff()
		if err != nil {
			return fail(ctx, parentAlignName, fmt.Errorf("landingshed: %s: read the loom config for the Code-Review budget: %w", parentAlignName, err))
		}
		reviewOff = off
	}
	if !reviewOff {
		return nil
	}
	head, err := m.deps.TaskHead()
	if err != nil {
		return fail(ctx, parentAlignName, fmt.Errorf("landingshed: %s: read the task head: %w", parentAlignName, err))
	}
	change, err := m.deps.LineChange(rec.Baseline, head)
	if err != nil {
		return fail(ctx, parentAlignName, fmt.Errorf("landingshed: %s: read the fixer's change: %w", parentAlignName, err))
	}
	reason := FixCap{MaxLines: m.deps.Config.FinalizeFixMaxLines, IsTestFile: m.deps.IsTestFile}.Exceeded(change)
	if reason == "" {
		return nil
	}
	rec.Halt = fixerHaltOverCap
	if err := WriteFixerAttempts(FixerAttemptsPath(m.deps.RecordsDir, parentAlignName), rec); err != nil {
		return fail(ctx, parentAlignName, err)
	}
	m.attempts = rec
	return halt(ctx, parentAlignName, fmt.Sprintf("%sthe fixer's change in %s is over the caps: %s; fixer commits: %s..%s; way forward: review and fix on the task branch, then `lym loom resume`", LandingStopPrefix, parentAlignName, reason, rec.Baseline, head))
}

// writeRecord writes the merge record of one alignment:
// the parent tip merged, the merge commit and the aligned tip it records.
// Unless the merge was up to date, the record also carries the overlap:
// where the parent's changed paths meet the task's changed paths.
// The parent's paths run from the earlier aligned tip to the tip merged.
// Without an earlier aligned tip, they run from the task's fork point.
// The task's paths run from its last merge base to its head.
// An up-to-date merge has no merge commit and records an empty overlap.
func (m *ParentAlign) writeRecord(tip, mergeCommit, alignedTip string, merge mergeresolve.Result) error {
	record := MergeRecord{
		ParentBranch: m.deps.ParentBranch,
		MergedTip:    tip,
		AlignedTip:   alignedTip,
		TaskBase:     m.before.taskBase,
		TaskHead:     m.before.taskHead,
		MergeCommit:  mergeCommit,
		UpToDate:     merge.AlreadyUpToDate,
	}
	if !merge.AlreadyUpToDate {
		parentFrom := m.before.earlier.AlignedTip
		if parentFrom == "" {
			parentFrom = m.before.taskBase
		}
		parentPaths, err := m.deps.ChangedPaths(parentFrom, tip)
		if err != nil {
			return fmt.Errorf("landingshed: %s: list the paths the parent changed: %w", parentAlignName, err)
		}
		taskPaths, err := m.deps.ChangedPaths(m.before.taskBase, m.before.taskHead)
		if err != nil {
			return fmt.Errorf("landingshed: %s: list the paths the task changed: %w", parentAlignName, err)
		}
		record.Overlap = computeOverlap(parentPaths, taskPaths)
	}
	if err := WriteMergeRecord(m.deps.MergeRecordPath, record); err != nil {
		return fmt.Errorf("landingshed: %s: %w", parentAlignName, err)
	}
	return nil
}
