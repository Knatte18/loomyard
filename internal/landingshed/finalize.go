// finalize.go implements the Finalize producer: it catches the task worktree up with the parent
// branch, then merges the task branch into the parent pair's own worktree -- reporting only what
// the shedengine.ShedProducer seam can carry: a bare verdict, an output pointer nobody persists, and
// an error.
//
// This producer's merge critical section is the parent-side merge call (step 4 below), and a future
// regeneration step folds into that same critical section rather than becoming a separate row -- no
// hook, no injectable interface, and no scaffolded span with an empty body is built for that today,
// because an interface with a permanently-nil implementation is exactly the hypothetical-requirement
// design this codebase avoids.
//
// A merged pull request does not make this producer redundant: the remote service only ever sees
// one of the two sides, so the other side's branch was never in the pull request at all, and after a
// remote-side merge the visible side reports already-up-to-date while the other genuinely merges.

package landingshed

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/mergeresolve"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// finalizeName is the producer name Finalize's log lines, error text, and stuck-reason filename
// carry.
const finalizeName = "Finalize"

// parentMerger is the narrow single-method seam Finalize holds the opened parent pair's handle
// behind, mirroring the resolver seam's own reasoning: production has exactly one way to obtain it
// -- deps.OpenParentFabric, adapted by NewFinalize's parentOpener closure -- and the seam exists so
// this package's own in-package tests can substitute a fake without a second public construction
// path anyone outside could reach for by mistake.
type parentMerger interface {
	// Merge merges source into the parent pair's own worktree. See fabricengine.Fabric.Merge.
	Merge(source string, opts fabricengine.MergeOptions) (fabricengine.MergeResult, error)
}

// The compile-time assertion that *fabricengine.Fabric satisfies parentMerger.
var _ parentMerger = (*fabricengine.Fabric)(nil)

// Finalize is the ShedProducer that merges the task branch into the parent pair's own worktree,
// after first catching the task worktree up with the parent branch.
type Finalize struct {
	deps     Deps
	resolver resolver
	// parentOpener adapts deps.OpenParentFabric's concrete *fabricengine.Fabric return type onto
	// the parentMerger seam. It is called once per Call, never at construction: the pair
	// constructor it wraps stat-checks a layout that may not be wired yet, so opening eagerly
	// would fail before the run's own preflight has confirmed anything is wired.
	parentOpener func() (parentMerger, error)
}

var _ shedengine.ShedProducer = (*Finalize)(nil)

// NewFinalize constructs a Finalize from deps, rejecting a nil OpenFabric or OpenParentFabric
// closure up front with a distinct error each -- rather than nil-panicking at call time. A nil
// parent opener is a construction error rather than a silent no-op: this producer never creates a
// worktree to merge into, so without a working opener it could never reach the parent pair at all.
//
// It builds its resolver at construction time from the told values and stores it behind the
// package's own resolver seam, exactly as the sibling producer's constructor does and for the same
// reason -- the resolver's constructor performs no I/O, so a mis-wired resolver is a construction
// error rather than a first-call surprise.
func NewFinalize(deps Deps) (*Finalize, error) {
	if deps.OpenFabric == nil {
		return nil, fmt.Errorf("landingshed: NewFinalize: Deps.OpenFabric must not be nil")
	}
	if deps.OpenParentFabric == nil {
		return nil, fmt.Errorf("landingshed: NewFinalize: Deps.OpenParentFabric must not be nil")
	}

	fabricHandle, err := deps.OpenFabric()
	if err != nil {
		return nil, fmt.Errorf("landingshed: NewFinalize: open pair: %w", err)
	}

	res, err := mergeresolve.New(mergeresolve.Deps{
		Fabric:       fabricHandle,
		Shuttle:      deps.Shuttle,
		WorktreeRoot: deps.WorktreeRoot,
		ScratchDir:   deps.ScratchDir,
		StencilsDir:  deps.StencilsDir,
		ConflictSpec: deps.Config.Conflict,
		Registry:     deps.Registry,
		Timeout:      time.Duration(deps.Config.ConflictTimeoutMin) * time.Minute,
	})
	if err != nil {
		return nil, fmt.Errorf("landingshed: NewFinalize: build resolver: %w", err)
	}

	return &Finalize{
		deps:     deps,
		resolver: res,
		parentOpener: func() (parentMerger, error) {
			h, err := deps.OpenParentFabric()
			if err != nil {
				return nil, err
			}
			return h, nil
		},
	}, nil
}

// Call runs one Finalize iteration.
func (fz *Finalize) Call(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
	if err := entryErr(ctx, finalizeName); err != nil {
		return "", shedengine.OutputPointer{}, err
	}

	// Step 2: catch the task worktree up with the parent branch.
	if outcome, out, err, done := fz.mergeInStep(ctx); done {
		return outcome, out, err
	}

	// Step 3: obtain the parent pair's handle. This producer never creates a worktree to merge
	// into; materializing a pair is a separate command's job and a human's decision.
	parentHandle, err := fz.parentOpener()
	if err != nil {
		return fz.stuckOrCancelled(ctx, fmt.Sprintf("no live pair for parent branch %q: %v", fz.deps.ParentBranch, err), "error", err)
	}

	// Step 4: the parent-side merge, this producer's own merge critical section.
	mergeOpts := fabricengine.MergeOptions{Squash: fz.deps.Config.Squash}
	_, mergeErr := parentHandle.Merge(fz.deps.TaskBranch, mergeOpts)
	if mergeErr == nil {
		return shedengine.Done, shedengine.OutputPointer{}, nil
	}

	// Step 5: on the merge-in-required error, re-run the resolver in the task worktree and retry
	// the parent-side merge exactly once -- no lock spans the window between this producer's own
	// catch-up merge-in and this parent-side merge, so a competing task can genuinely land in the
	// parent in between.
	var mergeInRequired *fabricengine.ErrMergeInRequired
	if errors.As(mergeErr, &mergeInRequired) {
		if outcome, out, err, done := fz.mergeInStep(ctx); done {
			return outcome, out, err
		}
		_, retryErr := parentHandle.Merge(fz.deps.TaskBranch, mergeOpts)
		if retryErr == nil {
			return shedengine.Done, shedengine.OutputPointer{}, nil
		}
		mergeErr = retryErr
	}

	// Steps 6-7: a guard error carrying the dirty-worktree reason is recognized through the
	// accessor rather than a matched string, and surfaced verbatim -- never stash, never reset,
	// never retry. Any other merge error is stuck with the error surfaced.
	var guardErr *fabricengine.MergeGuardError
	if errors.As(mergeErr, &guardErr) && guardErr.WorktreeDirty() {
		return fz.stuckOrCancelled(ctx, guardErr.Error())
	}
	return fz.stuckOrCancelled(ctx, fmt.Sprintf("parent-side merge failed: %v", mergeErr), "error", mergeErr)
}

// mergeInStep runs the resolver's merge-in against the parent branch from the task worktree. When
// the merge-in produced no stuck verdict and no error, it reports done=false so Call proceeds to the
// parent-side merge; otherwise it reports done=true along with the outcome/output/error Call should
// return immediately.
func (fz *Finalize) mergeInStep(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error, bool) {
	result, err := fz.resolver.Resolve(ctx, fz.deps.ParentBranch)
	if err != nil {
		if cerr := cancelErr(ctx, finalizeName); cerr != nil {
			return "", shedengine.OutputPointer{}, cerr, true
		}
		return "", shedengine.OutputPointer{}, fmt.Errorf("landingshed: %s: merge-in against parent branch %q: %w", finalizeName, fz.deps.ParentBranch, err), true
	}
	if result.Outcome == mergeresolve.OutcomeStuck {
		outcome, out, err := fz.stuckOrCancelled(ctx, result.Reason)
		return outcome, out, err, true
	}
	return "", shedengine.OutputPointer{}, nil, false
}

// stuckOrCancelled consults cancelErr first -- the obligation every non-success exit discharges --
// and otherwise records reason via reportStuck and returns a bare Stuck verdict.
func (fz *Finalize) stuckOrCancelled(ctx context.Context, reason string, fields ...any) (shedengine.Outcome, shedengine.OutputPointer, error) {
	if cerr := cancelErr(ctx, finalizeName); cerr != nil {
		return "", shedengine.OutputPointer{}, cerr
	}
	reportStuck(finalizeName, reason, fz.deps.ScratchDir, fields...)
	return shedengine.Stuck, shedengine.OutputPointer{}, nil
}
