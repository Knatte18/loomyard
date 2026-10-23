// mergein.go implements the Merge-In producer: it merges the parent's tip into the task worktree with no parent lock and no write to the parent pair, runs the `landing` gate over the merged tree and writes the merge record the guarded landing rows read.

package landingshed

import (
	"context"
	"fmt"
	"time"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/mergeresolve"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// mergeInName is the producer name Merge-In's log lines and error text carry, and the row name its failure record carries.
const mergeInName = "Merge-In"

// parentPuller is the narrow seam over the parent pair's fast-forward, which `*fabricengine.Fabric` satisfies.
type parentPuller interface {
	// FastForwardPair moves the parent pair to its upstream as a fast-forward only. See fabricengine.Fabric.FastForwardPair.
	FastForwardPair(opts fabricengine.SyncOptions) (fabricengine.FastForwardResult, error)
}

// The compile-time assertion that *fabricengine.Fabric satisfies parentPuller.
var _ parentPuller = (*fabricengine.Fabric)(nil)

// mergeBefore is what Merge-In reads before the merge, which the merge record carries.
type mergeBefore struct {
	taskBase   string
	taskHead   string
	alignedTip string
}

// MergeIn is the ShedProducer that merges the parent branch's tip into the task worktree and records the merge.
type MergeIn struct {
	deps     Deps
	resolver resolver
	gate     verifyGate
	premerge preMerge
	guard    parentGuard
	// before is set by Call before the merge and read by writeRecord.
	before mergeBefore
}

var _ shedengine.ShedProducer = (*MergeIn)(nil)

// NewMergeIn constructs a MergeIn from deps, refusing a nil OpenFabric, OpenParentFabric, ReadParentTips, ChangedPaths, MergeBase or TaskHead, a nil Lock.Store and an empty MergeRecordPath, each with its own error.
// It builds the resolver at construction time, as NewPublish does.
func NewMergeIn(deps Deps) (*MergeIn, error) {
	switch {
	case deps.OpenFabric == nil:
		return nil, fmt.Errorf("landingshed: NewMergeIn: Deps.OpenFabric must not be nil")
	case deps.OpenParentFabric == nil:
		return nil, fmt.Errorf("landingshed: NewMergeIn: Deps.OpenParentFabric must not be nil")
	case deps.ReadParentTips == nil:
		return nil, fmt.Errorf("landingshed: NewMergeIn: Deps.ReadParentTips must not be nil")
	case deps.ChangedPaths == nil:
		return nil, fmt.Errorf("landingshed: NewMergeIn: Deps.ChangedPaths must not be nil")
	case deps.MergeBase == nil:
		return nil, fmt.Errorf("landingshed: NewMergeIn: Deps.MergeBase must not be nil")
	case deps.TaskHead == nil:
		return nil, fmt.Errorf("landingshed: NewMergeIn: Deps.TaskHead must not be nil")
	case deps.Lock.Store == nil:
		return nil, fmt.Errorf("landingshed: NewMergeIn: Deps.Lock.Store must not be nil")
	case deps.MergeRecordPath == "":
		return nil, fmt.Errorf("landingshed: NewMergeIn: Deps.MergeRecordPath must not be empty")
	}

	fabricHandle, err := deps.OpenFabric()
	if err != nil {
		return nil, fmt.Errorf("landingshed: NewMergeIn: open pair: %w", err)
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
		return nil, fmt.Errorf("landingshed: NewMergeIn: build resolver: %w", err)
	}

	gate := newVerifyGate(deps)
	gate.recorder = &failureRecorder{failingTests: deps.FailingTests, traceFailure: deps.TraceFailure, head: deps.TaskHead}
	return &MergeIn{
		deps:     deps,
		resolver: res,
		gate:     gate,
		premerge: newPreMerge(deps),
		guard:    newParentGuard(deps),
	}, nil
}

// Call runs one Merge-In iteration.
//
// It runs the run-scope origin reconcile after the status commit: a run origin ended or stopped is Stuck, and an unreachable origin proceeds.
// It writes nothing in the parent pair and holds no parent lock across the merge; a paused lock wait and an unreadable lock record for the local-tip read are Stuck with Halt, so the run stops at this row.
// A failed `landing` gate is Stuck for a human;
// a failed record write and every infrastructure fault are errors.
func (m *MergeIn) Call(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
	if err := entryErr(ctx, mergeInName); err != nil {
		return "", shedengine.OutputPointer{}, err
	}

	// The status file is rewritten on every transition and committed only at bootstrap,
	// and the merge guard refuses any tracked modification, so it is committed here as Publish does.
	if m.deps.CommitStatus != nil {
		if err := m.deps.CommitStatus(); err != nil {
			return "", shedengine.OutputPointer{}, fmt.Errorf("landingshed: %s: commit status file: %w", mergeInName, err)
		}
	}

	// The run-scope reconcile judges and never moves the parent.
	if outcome, out, err, stop := reconcileVerdict(ctx, mergeInName, m.deps.Reconcile); stop {
		return outcome, out, err
	}

	tip, lockReason, err := m.mergeTip(ctx)
	if err != nil {
		if cerr := cancelErr(ctx, mergeInName); cerr != nil {
			return "", shedengine.OutputPointer{}, cerr
		}
		return "", shedengine.OutputPointer{}, err
	}
	if lockReason != "" {
		return stuckVerdict(ctx, mergeInName, lockReason, true)
	}

	if err := m.readBefore(); err != nil {
		return "", shedengine.OutputPointer{}, err
	}

	reason, err := m.premerge.clear(mergeInName, m.deps.ParentBranch)
	if outcome, out, err, stop := gateStop(ctx, mergeInName, reason, err); stop {
		return outcome, out, err
	}
	reason, err = m.gate.clean(mergeInName, "before the merge-in")
	if outcome, out, err, stop := gateStop(ctx, mergeInName, reason, err); stop {
		return outcome, out, err
	}

	merge, err := m.resolver.Resolve(ctx, m.deps.ParentBranch, tip)
	if err != nil {
		if cerr := cancelErr(ctx, mergeInName); cerr != nil {
			return "", shedengine.OutputPointer{}, cerr
		}
		return "", shedengine.OutputPointer{}, fmt.Errorf("landingshed: %s: merge-in against parent branch %q: %w", mergeInName, m.deps.ParentBranch, err)
	}
	if merge.Outcome == mergeresolve.OutcomeStuck {
		return stuckVerdict(ctx, mergeInName, merge.Reason, false)
	}
	mergeCommit := ""
	if !merge.AlreadyUpToDate {
		if mergeCommit, err = m.deps.TaskHead(); err != nil {
			return "", shedengine.OutputPointer{}, fmt.Errorf("landingshed: %s: read the merge commit: %w", mergeInName, err)
		}
	}
	m.gate.recorder.mergeCommit = mergeCommit

	// The verify runs on committed content, and a verify that dirties the tree is a non-hermetic test that halts rather than ships.
	reason, err = m.gate.clean(mergeInName, "after the merge-in")
	if outcome, out, err, stop := gateStop(ctx, mergeInName, reason, err); stop {
		return outcome, out, err
	}
	reason, err = m.gate.check(ctx, mergeInName, m.deps.ParentBranch)
	if outcome, out, err, stop := gateStop(ctx, mergeInName, reason, err); stop {
		return outcome, out, err
	}
	reason, err = m.gate.clean(mergeInName, "after the verify")
	if outcome, out, err, stop := gateStop(ctx, mergeInName, reason, err); stop {
		return outcome, out, err
	}
	m.gate.clearFailure()

	if err := m.writeRecord(tip, mergeCommit, merge); err != nil {
		return "", shedengine.OutputPointer{}, err
	}
	if cerr := cancelErr(ctx, mergeInName); cerr != nil {
		return "", shedengine.OutputPointer{}, cerr
	}
	return shedengine.Done, shedengine.OutputPointer{}, nil
}

// mergeTip picks the parent tip to merge and returns it, or a Stuck reason of the parent lock wait.
// With an upstream and no push skip it fetches and takes the upstream tip, never the local tip, so a landing another run made locally and has not pushed is never merged in.
// Under the told push skip, or with no upstream, it takes the parent lock only for the read of the local tip and releases it before returning, so the merge itself runs unlocked.
// A paused lock wait and an unreadable lock record come back as the reason, for a halting Stuck.
func (m *MergeIn) mergeTip(ctx context.Context) (string, string, error) {
	branch := m.deps.ParentBranch
	if !m.deps.PushSkipped {
		tips, err := m.deps.ReadParentTips(true)
		if err != nil {
			return "", "", fmt.Errorf("landingshed: %s: read the tips of parent branch %q: %w", mergeInName, branch, err)
		}
		switch tips.Relation {
		case fabricengine.ParentNoUpstream:
		case fabricengine.ParentEqual, fabricengine.ParentAhead, fabricengine.ParentBehind, fabricengine.ParentDiverged:
			return tips.Upstream, "", nil
		default:
			return "", "", fmt.Errorf("landingshed: %s: unknown relation %q of parent branch %q to its upstream", mergeInName, tips.Relation, branch)
		}
	}

	reason, err := m.guard.ensure(ctx, mergeInName)
	if err != nil || reason != "" {
		return "", reason, err
	}
	defer m.guard.release(mergeInName)
	tips, err := m.deps.ReadParentTips(false)
	if err != nil {
		return "", "", fmt.Errorf("landingshed: %s: read the tips of parent branch %q: %w", mergeInName, branch, err)
	}
	return tips.Local, "", nil
}

// readBefore reads what the merge record carries from before the merge:
// the task head, the parent tip last merged into the task, and the aligned tip of the earlier merge record.
// An absent aligned tip stays empty, which reads as the task's fork point from the parent.
// An unreadable earlier record is logged and read as an empty aligned tip, so a damaged record cannot block every later merge.
func (m *MergeIn) readBefore() error {
	head, err := m.deps.TaskHead()
	if err != nil {
		return fmt.Errorf("landingshed: %s: read the task head: %w", mergeInName, err)
	}
	base, err := m.deps.MergeBase(m.deps.ParentBranch)
	if err != nil {
		return fmt.Errorf("landingshed: %s: find the parent tip last merged into the task: %w", mergeInName, err)
	}

	aligned := ""
	if earlier, found, err := ReadMergeRecord(m.deps.MergeRecordPath); err != nil {
		logger.Warn("landingshed: earlier merge record unreadable, falling back", "producer", mergeInName, "cause", err)
	} else if found {
		aligned = earlier.AlignedTip
	}

	m.before = mergeBefore{taskBase: base, taskHead: head, alignedTip: aligned}
	return nil
}

// writeRecord writes the merge record: tip is the parent tip merged, mergeCommit the merge commit (empty for an up-to-date merge) and merge the resolver's answer.
// The aligned tip is carried forward from the earlier record unchanged.
// The overlap is the paths the parent gained since the aligned tip, or since the task's fork point when it is empty, against the paths the task changed since its last merge base;
// an up-to-date merge records an empty overlap.
func (m *MergeIn) writeRecord(tip, mergeCommit string, merge mergeresolve.Result) error {
	record := MergeRecord{
		ParentBranch: m.deps.ParentBranch,
		MergedTip:    tip,
		AlignedTip:   m.before.alignedTip,
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
			return fmt.Errorf("landingshed: %s: list the paths the parent changed: %w", mergeInName, err)
		}
		taskPaths, err := m.deps.ChangedPaths(m.before.taskBase, m.before.taskHead)
		if err != nil {
			return fmt.Errorf("landingshed: %s: list the paths the task changed: %w", mergeInName, err)
		}
		record.Overlap = computeOverlap(parentPaths, taskPaths)
	}
	if err := WriteMergeRecord(m.deps.MergeRecordPath, record); err != nil {
		return fmt.Errorf("landingshed: %s: %w", mergeInName, err)
	}
	return nil
}
