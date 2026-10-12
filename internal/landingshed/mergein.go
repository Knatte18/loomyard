// mergein.go implements the Merge-In producer: under the parent lock it brings the local parent branch level with its upstream, merges that tip into the task worktree, verifies the merged tree and writes the merge record the merge review and the guarded landing rows read.

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

// parentPuller is the narrow seam Merge-In holds the opened parent pair's handle behind.
// Production has exactly one way to obtain it, deps.OpenParentFabric, and the seam exists so in-package tests substitute a fake.
type parentPuller interface {
	// FastForwardPair moves the parent pair to its upstream as a fast-forward only. See fabricengine.Fabric.FastForwardPair.
	FastForwardPair(opts fabricengine.SyncOptions) (fabricengine.FastForwardResult, error)
}

// The compile-time assertion that *fabricengine.Fabric satisfies parentPuller.
var _ parentPuller = (*fabricengine.Fabric)(nil)

// mergeBefore is what Merge-In reads before the merge, which the merge record carries.
type mergeBefore struct {
	taskBase    string
	taskHead    string
	reviewedTip string
}

// MergeIn is the ShedProducer that merges the parent branch into the task worktree under the parent lock and records the merge.
type MergeIn struct {
	deps     Deps
	resolver resolver
	gate     verifyGate
	premerge preMerge
	guard    parentGuard
	// parentOpener adapts deps.OpenParentFabric's concrete return type onto the parentPuller seam.
	// It is called only when the parent pair has to be fast-forwarded, never at construction.
	parentOpener func() (parentPuller, error)
	// before is set by Call after the parent is synced and read by writeRecord.
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
		Fabric:       fabricHandle,
		Shuttle:      deps.Shuttle,
		WorktreeRoot: deps.WorktreeRoot,
		ScratchDir:   deps.ScratchDir,
		StencilsDir:  deps.StencilsDir,
		ParentName:   deps.ParentName,
		ConflictSpec: deps.Config.Conflict,
		Registry:     deps.Registry,
		Timeout:      time.Duration(deps.Config.ConflictTimeoutMin) * time.Minute,
	})
	if err != nil {
		return nil, fmt.Errorf("landingshed: NewMergeIn: build resolver: %w", err)
	}

	gate := newVerifyGate(deps)
	gate.recorder = &failureRecorder{failingTests: deps.FailingTests, head: deps.TaskHead}
	return &MergeIn{
		deps:     deps,
		resolver: res,
		gate:     gate,
		premerge: newPreMerge(deps),
		guard:    newParentGuard(deps),
		parentOpener: func() (parentPuller, error) {
			h, err := deps.OpenParentFabric()
			if err != nil {
				return nil, err
			}
			return h, nil
		},
	}, nil
}

// Call runs one Merge-In iteration.
//
// It takes the parent lock before it touches the parent and keeps it on every way out: a later row or the loop's exit frees it.
// A paused wait and an unreadable lock record are Stuck with Halt, so the run stops at this row.
// A parent branch that is ahead of or diverged from its upstream, a parent pair that cannot be fast-forwarded and a failed plan verify are Stuck for a human;
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

	reason, err := m.guard.ensure(ctx, mergeInName)
	if err != nil {
		if cerr := cancelErr(ctx, mergeInName); cerr != nil {
			return "", shedengine.OutputPointer{}, cerr
		}
		return "", shedengine.OutputPointer{}, err
	}
	if reason != "" {
		return stuckVerdict(ctx, mergeInName, reason, true)
	}

	tip, reason, err := m.syncParent(ctx)
	if outcome, out, err, stop := gateStop(ctx, mergeInName, reason, err); stop {
		return outcome, out, err
	}

	if err := m.readBefore(); err != nil {
		return "", shedengine.OutputPointer{}, err
	}

	reason, err = m.premerge.clear(mergeInName, m.deps.ParentBranch)
	if outcome, out, err, stop := gateStop(ctx, mergeInName, reason, err); stop {
		return outcome, out, err
	}
	reason, err = m.gate.clean(mergeInName, "before the merge-in")
	if outcome, out, err, stop := gateStop(ctx, mergeInName, reason, err); stop {
		return outcome, out, err
	}

	merge, err := m.resolver.Resolve(ctx, m.deps.ParentBranch)
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

// syncParent brings the local parent branch level with its upstream and returns the tip to merge, or a Stuck reason.
// Under the told push skip it neither fetches nor fast-forwards and returns the local tip.
// A parent branch that is ahead of or diverged from its upstream is Stuck before any pull runs, since Merge-In never pushes or rewrites the parent.
// A fast-forward runs only from the behind state and only moves the parent pair to its upstream tip, so it never touches local commits;
// the lock the caller holds keeps another run's landing from merging into the same pair meanwhile.
// After a fast-forward the returned tip is the one the pair was moved to, which can be newer than the upstream the tips read saw.
func (m *MergeIn) syncParent(ctx context.Context) (string, string, error) {
	branch := m.deps.ParentBranch
	if m.deps.PushSkipped {
		tips, err := m.deps.ReadParentTips(false)
		if err != nil {
			return "", "", fmt.Errorf("landingshed: %s: read the tips of parent branch %q: %w", mergeInName, branch, err)
		}
		return tips.Local, "", nil
	}

	tips, err := m.deps.ReadParentTips(true)
	if err != nil {
		return "", "", fmt.Errorf("landingshed: %s: read the tips of parent branch %q: %w", mergeInName, branch, err)
	}
	const resume = `then run "lyx loom resume"`
	switch tips.Relation {
	case fabricengine.ParentNoUpstream, fabricengine.ParentEqual:
		return tips.Local, "", nil
	case fabricengine.ParentAhead:
		return "", fmt.Sprintf("the local parent branch %q is ahead of its upstream; way forward: push or reconcile %q in its own pair, %s", branch, branch, resume), nil
	case fabricengine.ParentDiverged:
		return "", fmt.Sprintf("the local parent branch %q has diverged from its upstream; way forward: push or reconcile %q in its own pair, %s", branch, branch, resume), nil
	case fabricengine.ParentBehind:
	default:
		return "", "", fmt.Errorf("landingshed: %s: unknown relation %q of parent branch %q to its upstream", mergeInName, tips.Relation, branch)
	}

	parent, err := m.parentOpener()
	if err != nil {
		return "", fmt.Sprintf("no live pair for parent branch %q: %v; way forward: re-create the parent pair, %s", branch, err, resume), nil
	}
	result, err := parent.FastForwardPair(fabricengine.SyncOptions{SkipPush: m.deps.PushSkipped})
	if err != nil {
		return "", "", fmt.Errorf("landingshed: %s: fast-forward the parent pair of %q: %w", mergeInName, branch, err)
	}
	switch result.Refusal {
	case "":
		return result.NewHead, "", nil
	case fabricengine.FastForwardDirty:
		return "", fmt.Sprintf("the parent pair of %q has uncommitted changes; way forward: commit or stash them in the parent pair, %s", branch, resume), nil
	case fabricengine.FastForwardMergeInProgress:
		return "", fmt.Sprintf("a merge is in progress in the parent pair of %q; way forward: finish or abort that merge in the parent pair, %s", branch, resume), nil
	case fabricengine.FastForwardNotFastForward:
		return "", fmt.Sprintf("the upstream of %q is no longer a fast-forward of the parent pair; way forward: reconcile %q with its upstream in its own pair, %s", branch, branch, resume), nil
	default:
		return "", "", fmt.Errorf("landingshed: %s: unknown fast-forward refusal %q for parent branch %q", mergeInName, result.Refusal, branch)
	}
}

// readBefore reads what the merge record carries from before the merge:
// the task head, the parent tip last merged into the task, and the parent tip the last finished merge review reached.
// That last tip is the reviewed-tip file's, else the earlier merge record's reviewed tip, else the merge base, the fork point.
// An unreadable reviewed-tip file or earlier record is logged and skipped, so a damaged record cannot block every later merge.
func (m *MergeIn) readBefore() error {
	head, err := m.deps.TaskHead()
	if err != nil {
		return fmt.Errorf("landingshed: %s: read the task head: %w", mergeInName, err)
	}
	base, err := m.deps.MergeBase(m.deps.ParentBranch)
	if err != nil {
		return fmt.Errorf("landingshed: %s: find the parent tip last merged into the task: %w", mergeInName, err)
	}

	reviewed := base
	if tip, found, err := ReadReviewedTip(m.deps.MergeReviewedPath); err != nil {
		logger.Warn("landingshed: reviewed tip unreadable, falling back", "producer", mergeInName, "cause", err)
	} else if found && tip != "" {
		reviewed = tip
	} else if earlier, found, err := ReadMergeRecord(m.deps.MergeRecordPath); err != nil {
		logger.Warn("landingshed: earlier merge record unreadable, falling back", "producer", mergeInName, "cause", err)
	} else if found && earlier.ReviewedTip != "" {
		reviewed = earlier.ReviewedTip
	}

	m.before = mergeBefore{taskBase: base, taskHead: head, reviewedTip: reviewed}
	return nil
}

// writeRecord writes the merge record: tip is the parent tip merged, mergeCommit the merge commit (empty for an up-to-date merge) and merge the resolver's answer.
// The overlap is the paths the parent gained since the last reviewed tip against the paths the task changed since its last merge base;
// an up-to-date merge records an empty overlap.
func (m *MergeIn) writeRecord(tip, mergeCommit string, merge mergeresolve.Result) error {
	record := MergeRecord{
		ParentBranch: m.deps.ParentBranch,
		MergedTip:    tip,
		ReviewedTip:  m.before.reviewedTip,
		TaskBase:     m.before.taskBase,
		TaskHead:     m.before.taskHead,
		MergeCommit:  mergeCommit,
		UpToDate:     merge.AlreadyUpToDate,
	}
	if !merge.AlreadyUpToDate {
		parentPaths, err := m.deps.ChangedPaths(m.before.reviewedTip, tip)
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
