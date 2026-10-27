// parentalign.go implements the Parent-Align producer: it merges the parent's tip into the task worktree with no parent lock and no write to the parent pair, runs the `landing` gate over the merged tree and writes the merge record the guarded landing rows read.

package landingshed

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"time"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/mergeresolve"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/verifytree"
)

// parentAlignName is the producer name Parent-Align's log lines and error text carry, and the row name its failure record carries.
const parentAlignName = "Parent-Align"

// retiredMergeInName is the row name Parent-Align carried before its rename, which a run parked on the old row still holds in its status file and attempt record.
const retiredMergeInName = "Merge-In"

// parentPuller is the narrow seam over the parent pair's fast-forward, which `*fabricengine.Fabric` satisfies.
type parentPuller interface {
	// FastForwardPair moves the parent pair to its upstream as a fast-forward only. See fabricengine.Fabric.FastForwardPair.
	FastForwardPair(opts fabricengine.SyncOptions) (fabricengine.FastForwardResult, error)
}

// The compile-time assertion that *fabricengine.Fabric satisfies parentPuller.
var _ parentPuller = (*fabricengine.Fabric)(nil)

// mergeBefore is what Parent-Align reads before the merge, which the merge record carries.
type mergeBefore struct {
	taskBase   string
	taskHead   string
	alignedTip string
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

// fixerHaltSpent and fixerHaltOverCap are the FixerAttempts.Halt notes a spent cap and an over-cap change leave.
const (
	fixerHaltSpent   = "spent"
	fixerHaltOverCap = "over-cap"
)

var _ shedengine.ShedProducer = (*ParentAlign)(nil)

// NewParentAlign constructs a ParentAlign from deps, refusing a nil OpenFabric, OpenParentFabric, ReadParentTips, ChangedPaths, MergeBase or TaskHead, a nil Lock.Store and an empty MergeRecordPath, each with its own error.
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

	fabricHandle, err := deps.OpenFabric()
	if err != nil {
		return nil, fmt.Errorf("landingshed: NewParentAlign: open pair: %w", err)
	}
	res, err := mergeresolve.New(mergeresolve.Deps{
		Fabric:         fabricHandle,
		Shuttle:        deps.Shuttle,
		WorktreeRoot:   deps.WorktreeRoot,
		ScratchDir:     deps.ScratchDir,
		StencilsDir:    deps.StencilsDir,
		ParentName:     deps.ParentName,
		LanguageSkills: deps.LanguageSkills,
		ConflictSpec:   deps.Config.Conflict,
		Registry:       deps.Registry,
		Timeout:        time.Duration(deps.Config.ConflictTimeoutMin) * time.Minute,
	})
	if err != nil {
		return nil, fmt.Errorf("landingshed: NewParentAlign: build resolver: %w", err)
	}

	gate := newVerifyGate(deps)
	gate.recorder = &failureRecorder{failingTests: deps.FailingTests, traceFailure: deps.TraceFailure, head: deps.TaskHead}
	return &ParentAlign{
		deps:     deps,
		resolver: res,
		gate:     gate,
		premerge: newPreMerge(deps),
		guard:    newParentGuard(deps),
	}, nil
}

// Call runs one Parent-Align iteration.
//
// It runs the run-scope origin reconcile after the status commit: a run origin ended or stopped is Stuck, and an unreachable origin proceeds.
// It writes nothing in the parent pair and holds no parent lock across the merge; a paused lock wait and an unreadable lock record for the local-tip read are Stuck with Halt, so the run stops at this row.
// After the merge and the `landing` gate it aligns: it runs the merge fixer when the parent brought in commits or the gate fails,
// and every halt of that alignment is a Stuck with Halt whose reason opens with LandingStopPrefix.
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
	if outcome, out, err, stop := reconcileVerdict(ctx, parentAlignName, m.deps.Reconcile); stop {
		return outcome, out, err
	}

	tip, lockReason, err := m.mergeTip(ctx)
	if err != nil {
		if cerr := cancelErr(ctx, parentAlignName); cerr != nil {
			return "", shedengine.OutputPointer{}, cerr
		}
		return "", shedengine.OutputPointer{}, err
	}
	if lockReason != "" {
		return stuckVerdict(ctx, parentAlignName, lockReason, true)
	}

	if err := m.readBefore(); err != nil {
		return "", shedengine.OutputPointer{}, err
	}
	attemptsReason, err := m.readAttempts()
	if err != nil {
		return "", shedengine.OutputPointer{}, err
	}
	if attemptsReason != "" {
		return stuckVerdict(ctx, parentAlignName, attemptsReason, true)
	}

	reason, err := m.premerge.clear(parentAlignName, m.deps.ParentBranch)
	if outcome, out, err, stop := gateStop(ctx, parentAlignName, reason, err); stop {
		return outcome, out, err
	}
	reason, err = m.gate.clean(parentAlignName, "before the merge-in")
	if outcome, out, err, stop := gateStop(ctx, parentAlignName, reason, err); stop {
		return outcome, out, err
	}

	merge, err := m.resolver.Resolve(ctx, m.deps.ParentBranch, tip)
	if err != nil {
		if cerr := cancelErr(ctx, parentAlignName); cerr != nil {
			return "", shedengine.OutputPointer{}, cerr
		}
		return "", shedengine.OutputPointer{}, fmt.Errorf("landingshed: %s: merge-in against parent branch %q: %w", parentAlignName, m.deps.ParentBranch, err)
	}
	if merge.Outcome == mergeresolve.OutcomeStuck {
		return stuckVerdict(ctx, parentAlignName, merge.Reason, false)
	}
	mergeCommit := ""
	if !merge.AlreadyUpToDate {
		if mergeCommit, err = m.deps.TaskHead(); err != nil {
			return "", shedengine.OutputPointer{}, fmt.Errorf("landingshed: %s: read the merge commit: %w", parentAlignName, err)
		}
	}
	m.gate.recorder.mergeCommit = mergeCommit

	// The verify runs on committed content, and a verify that dirties the tree is a non-hermetic test that halts rather than ships.
	reason, err = m.gate.clean(parentAlignName, "after the merge-in")
	if outcome, out, err, stop := gateStop(ctx, parentAlignName, reason, err); stop {
		return outcome, out, err
	}
	gateReason, err := m.gate.check(ctx, parentAlignName, m.deps.ParentBranch)
	if outcome, out, err, stop := gateStop(ctx, parentAlignName, "", err); stop {
		return outcome, out, err
	}
	if gateReason == "" {
		reason, err = m.gate.clean(parentAlignName, "after the verify")
		if outcome, out, err, stop := gateStop(ctx, parentAlignName, reason, err); stop {
			return outcome, out, err
		}
	}

	if outcome, out, err, stop := m.align(ctx, tip, mergeCommit, gateReason); stop {
		return outcome, out, err
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

// mergeTip picks the parent tip to merge and returns it, or a Stuck reason of the parent lock wait.
// With an upstream and no push skip it fetches and takes the upstream tip, never the local tip, so a landing another run made locally and has not pushed is never merged in.
// Under the told push skip, or with no upstream, it takes the parent lock only for the read of the local tip and releases it before returning, so the merge itself runs unlocked.
// A paused lock wait and an unreadable lock record come back as the reason, for a halting Stuck.
func (m *ParentAlign) mergeTip(ctx context.Context) (string, string, error) {
	branch := m.deps.ParentBranch
	if !m.deps.PushSkipped {
		tips, err := m.deps.ReadParentTips(true)
		if err != nil {
			return "", "", fmt.Errorf("landingshed: %s: read the tips of parent branch %q: %w", parentAlignName, branch, err)
		}
		switch tips.Relation {
		case fabricengine.ParentNoUpstream:
		case fabricengine.ParentEqual, fabricengine.ParentAhead, fabricengine.ParentBehind, fabricengine.ParentDiverged:
			return tips.Upstream, "", nil
		default:
			return "", "", fmt.Errorf("landingshed: %s: unknown relation %q of parent branch %q to its upstream", parentAlignName, tips.Relation, branch)
		}
	}

	reason, err := m.guard.ensure(ctx, parentAlignName)
	if err != nil || reason != "" {
		return "", reason, err
	}
	defer m.guard.release(parentAlignName)
	tips, err := m.deps.ReadParentTips(false)
	if err != nil {
		return "", "", fmt.Errorf("landingshed: %s: read the tips of parent branch %q: %w", parentAlignName, branch, err)
	}
	return tips.Local, "", nil
}

// readBefore reads what the merge record carries from before the merge:
// the task head, the parent tip last merged into the task, and the aligned tip of the earlier merge record.
// An absent aligned tip stays empty, which reads as the task's fork point from the parent.
// An unreadable earlier record is logged and read as an empty aligned tip, so a damaged record cannot block every later merge.
func (m *ParentAlign) readBefore() error {
	head, err := m.deps.TaskHead()
	if err != nil {
		return fmt.Errorf("landingshed: %s: read the task head: %w", parentAlignName, err)
	}
	base, err := m.deps.MergeBase(m.deps.ParentBranch)
	if err != nil {
		return fmt.Errorf("landingshed: %s: find the parent tip last merged into the task: %w", parentAlignName, err)
	}

	aligned := ""
	if earlier, found, err := ReadMergeRecord(m.deps.MergeRecordPath); err != nil {
		logger.Warn("landingshed: earlier merge record unreadable, falling back", "producer", parentAlignName, "cause", err)
	} else if found {
		aligned = earlier.AlignedTip
	}

	m.before = mergeBefore{taskBase: base, taskHead: head, alignedTip: aligned}
	return nil
}

// removeAttempts removes the row's fixer attempt record and the one it carried under its retired name.
func (m *ParentAlign) removeAttempts() error {
	for _, row := range []string{parentAlignName, retiredMergeInName} {
		if err := RemoveFixerAttempts(FixerAttemptsPath(m.deps.ScratchDir, row)); err != nil {
			return err
		}
	}
	return nil
}

// readAttempts reads the entry's fixer attempt record into m.attempts and fixes the entry's fork point.
// A run halted on the row under its retired name left its record there, so the retired name's record is read when the row's own is absent.
// When the merge record holds no aligned tip and the attempt record no fork point, the task's fork point from the parent, read before the merge, is written as the record's AlignFrom,
// so a resumed entry whose merge is already up to date still reads the parent commits the first merge brought in.
// An attempt record that does not decode comes back as a reason, for a halting Stuck.
func (m *ParentAlign) readAttempts() (string, error) {
	entry, err := m.deps.RowEntry(parentAlignName)
	if err != nil {
		return "", fmt.Errorf("landingshed: %s: read the row entry: %w", parentAlignName, err)
	}
	ownPath := FixerAttemptsPath(m.deps.ScratchDir, parentAlignName)
	path := ownPath
	if _, statErr := os.Stat(ownPath); errors.Is(statErr, fs.ErrNotExist) {
		retired := FixerAttemptsPath(m.deps.ScratchDir, retiredMergeInName)
		if _, retiredErr := os.Stat(retired); retiredErr == nil {
			path = retired
		}
	}
	rec, err := ReadFixerAttempts(path, entry)
	if err != nil {
		logger.Warn("landingshed: fixer attempt record unreadable", "producer", parentAlignName, "cause", err)
		return fmt.Sprintf("%sthe fixer attempt record of %s does not decode: %s; way forward: remove %s, which grants the entry a fresh cap, then `lym loom resume`", LandingStopPrefix, parentAlignName, path, path), nil
	}
	if m.before.alignedTip == "" && rec.AlignFrom == "" {
		rec.AlignFrom = m.before.taskBase
		if err := WriteFixerAttempts(ownPath, rec); err != nil {
			return "", err
		}
	}
	m.attempts = rec
	return "", nil
}

// align decides what the parent's merge needs after the `landing` gate ran with gateReason as its failure, "" for a pass.
// The parent brought in commits when its merged tip adds something over the aligned tip, or over the entry's fork point when the merge record holds none.
// A merge that brought none over a passing gate is aligned with no fixer.
// Parent commits brought in, or a failing gate, run the merge fixer on the attempts the entry has left.
// A resume after an over-cap halt, or of an entry with no attempts left, reads the operator's work as the alignment and spawns nothing.
// stop is false when Parent-Align may proceed to record the merge and advance the aligned tip; otherwise the other values are what Call returns at once.
func (m *ParentAlign) align(ctx context.Context, tip, mergeCommit string, gateReason string) (shedengine.Outcome, shedengine.OutputPointer, error, bool) {
	attemptsPath := FixerAttemptsPath(m.deps.ScratchDir, parentAlignName)
	rec := m.attempts
	passed := gateReason == ""
	capacity := m.deps.Config.ParentAlignFixAttempts
	alignFrom := m.before.alignedTip
	if alignFrom == "" {
		alignFrom = rec.AlignFrom
	}

	overCapResume := rec.Halt == fixerHaltOverCap
	if overCapResume {
		head, err := m.deps.TaskHead()
		if err != nil {
			return m.alignError(ctx, fmt.Errorf("landingshed: %s: read the task head: %w", parentAlignName, err))
		}
		rec.Baseline, rec.Halt = head, ""
		if err := WriteFixerAttempts(attemptsPath, rec); err != nil {
			return m.alignError(ctx, err)
		}
		m.attempts = rec
	}
	attemptsLeft := rec.Spent < capacity

	switch {
	case passed && overCapResume:
		return "", shedengine.OutputPointer{}, nil, false
	case passed && !attemptsLeft:
		return m.judgeFixerChange(ctx, rec)
	case !passed && !attemptsLeft:
		return m.haltSpent(ctx, gateReason)
	case passed:
		holds, err := m.deps.HoldsWork(alignFrom, tip)
		if err != nil {
			return m.alignError(ctx, fmt.Errorf("landingshed: %s: test whether %s adds anything over %s: %w", parentAlignName, tip, alignFrom, err))
		}
		if holds {
			return m.judgeFixerChange(ctx, rec)
		}
	}

	if rec.Baseline == "" {
		head, err := m.deps.TaskHead()
		if err != nil {
			return m.alignError(ctx, fmt.Errorf("landingshed: %s: read the task head: %w", parentAlignName, err))
		}
		rec.Baseline = head
	}
	outcome, updated, err := newFixerRun(m.deps, parentAlignName, m.gate).run(ctx, FixerModeParentAlign, rec, capacity, [2]string{alignFrom, tip})
	m.attempts = updated
	if err != nil {
		return m.alignError(ctx, err)
	}
	switch outcome.end {
	case fixerEndFixed:
		return m.judgeFixerChange(ctx, updated)
	case fixerEndSpent:
		updated.Halt = fixerHaltSpent
		if err := WriteFixerAttempts(attemptsPath, updated); err != nil {
			return m.alignError(ctx, err)
		}
		m.attempts = updated
	}
	return m.haltStop(ctx, outcome.haltReason(parentAlignName))
}

// alignError returns err from Call, or the cancellation error when ctx is cancelled.
func (m *ParentAlign) alignError(ctx context.Context, err error) (shedengine.Outcome, shedengine.OutputPointer, error, bool) {
	if cerr := cancelErr(ctx, parentAlignName); cerr != nil {
		return "", shedengine.OutputPointer{}, cerr, true
	}
	return "", shedengine.OutputPointer{}, err, true
}

// haltStop returns a halting Stuck with reason from Call.
func (m *ParentAlign) haltStop(ctx context.Context, reason string) (shedengine.Outcome, shedengine.OutputPointer, error, bool) {
	outcome, ptr, err := stuckVerdict(ctx, parentAlignName, reason, true)
	return outcome, ptr, err, true
}

// haltSpent halts the entry whose attempts are spent over a gate that still fails, naming the failed gate's log.
func (m *ParentAlign) haltSpent(ctx context.Context, gateReason string) (shedengine.Outcome, shedengine.OutputPointer, error, bool) {
	log := singleLine(gateReason)
	failure, found, err := verifytree.ReadLandingFailure(m.gate.paths)
	if err != nil {
		return m.alignError(ctx, err)
	}
	if found {
		log = failure.LogPath
	}
	return m.haltStop(ctx, fixerOutcome{end: fixerEndSpent, failureLog: log}.haltReason(parentAlignName))
}

// judgeFixerChange judges the entry's fixer change, from its baseline to HEAD, against the caps when no review follows Parent-Align.
// An entry whose fixer never ran has no baseline and no change.
// A change over the caps halts with the fixer's commits named and notes over-cap in the attempt record;
// a change within them, or one a following review will see, lets Parent-Align proceed.
func (m *ParentAlign) judgeFixerChange(ctx context.Context, rec FixerAttempts) (shedengine.Outcome, shedengine.OutputPointer, error, bool) {
	proceed := func() (shedengine.Outcome, shedengine.OutputPointer, error, bool) {
		return "", shedengine.OutputPointer{}, nil, false
	}
	if rec.Baseline == "" {
		return proceed()
	}
	reviewOff := true
	if m.deps.CodeReviewOff != nil {
		off, err := m.deps.CodeReviewOff()
		if err != nil {
			return m.alignError(ctx, fmt.Errorf("landingshed: %s: read the loom config for the Code-Review budget: %w", parentAlignName, err))
		}
		reviewOff = off
	}
	if !reviewOff {
		return proceed()
	}
	head, err := m.deps.TaskHead()
	if err != nil {
		return m.alignError(ctx, fmt.Errorf("landingshed: %s: read the task head: %w", parentAlignName, err))
	}
	change, err := m.deps.LineChange(rec.Baseline, head)
	if err != nil {
		return m.alignError(ctx, fmt.Errorf("landingshed: %s: read the fixer's change: %w", parentAlignName, err))
	}
	reason := FixCap{MaxLines: m.deps.Config.FinalizeFixMaxLines, IsTestFile: m.deps.IsTestFile}.Exceeded(change)
	if reason == "" {
		return proceed()
	}
	rec.Halt = fixerHaltOverCap
	if err := WriteFixerAttempts(FixerAttemptsPath(m.deps.ScratchDir, parentAlignName), rec); err != nil {
		return m.alignError(ctx, err)
	}
	m.attempts = rec
	return m.haltStop(ctx, fmt.Sprintf("%sthe fixer's change in %s is over the caps: %s; fixer commits: %s..%s; way forward: review and fix on the task branch, then `lym loom resume`", LandingStopPrefix, parentAlignName, reason, rec.Baseline, head))
}

// writeRecord writes the merge record: tip is the parent tip merged, mergeCommit the merge commit (empty for an up-to-date merge), alignedTip the aligned tip it records and merge the resolver's answer.
// The overlap is the paths the parent gained since the earlier aligned tip, or since the task's fork point when it is empty, against the paths the task changed since its last merge base;
// an up-to-date merge records an empty overlap.
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
		parentFrom := m.before.alignedTip
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
