// webster.go holds the lazy Webster producer, which resolves the active batchifier inside Call.
// It commits webster's durable run record once the run reports Done.

package loomshed

import (
	"context"
	"fmt"

	"github.com/Knatte18/loomyard/internal/batcher"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/shedadapters"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/weaverengine"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// batchifierReasonPrefix opens the Stuck reason of every row that resolves the active batchifier,
// so each row words the fault the same way; the error text follows.
const batchifierReasonPrefix = "active batchifier did not resolve: "

// batchifierWayForward closes the Stuck reason with the next step for the operator.
// The fault sits in batcher.yaml, so that is where the operator fixes it.
var batchifierWayForward = "; way forward: fix batcher.yaml (" + configengine.KeyName("batcher", "active") + " or the profile it names), then re-step"

// websterProducer is the lazy Webster producer.
// It resolves the active batchifier on every Call and then delegates to shedadapters.WebsterProducer.
//
// The batchifier is resolved at the start of every Call rather than at construction:
// a resumed run re-resolves it in the new process,
// and a config edited after an earlier load is the config Webster partitions the plan with.
type websterProducer struct {
	name   string
	dirs   configengine.Dirs
	run    shedadapters.WebsterRunner
	deps   websterengine.RunDeps
	commit func() error
	active weaverengine.ActiveDeps
}

var _ shedengine.ShedProducer = (*websterProducer)(nil)

// NewWebsterProducer returns a producer identified as name.
// On every Call it runs the implementer guard over active and resolves the active batchifier from dirs.
// It then drives run with a copy of deps that carries the resolved value; the caller leaves deps.Batcher nil,
// and every Call overwrites it.
// The commit function commits webster's durable run directory and is invoked once the run reports Done.
// The result is typed as shedengine.ShedProducer, the seam interface,
// so a registry in another package can call this constructor while the concrete type stays unexported.
func NewWebsterProducer(name string, dirs configengine.Dirs, run shedadapters.WebsterRunner, deps websterengine.RunDeps, commit func() error, active weaverengine.ActiveDeps) shedengine.ShedProducer {
	return &websterProducer{name: name, dirs: dirs, run: run, deps: deps, commit: commit, active: active}
}

// resolveActiveBatchifier resolves batcher.Active(dirs) for the row named name.
// A batcher.Active error maps to a non-empty stuckReason, never to a returned error, so the run blocks for a human.
// Stuck with an empty OnStuck persists the run as blocked and returns RunBlocked;
// a human resumes the run after fixing the config.
// Returning the error would instead persist the run as failed and abort it.
// The only returned error is a cancellation, which replaces the verdict.
func resolveActiveBatchifier(ctx context.Context, name string, dirs configengine.Dirs) (batcher.Batcher, string, error) {
	active, err := batcher.Active(dirs)
	if err == nil {
		return active, "", nil
	}
	if cerr := cancelErr(ctx, name); cerr != nil {
		return nil, "", cerr
	}
	// The fault is surfaced rather than discarded.
	// Neither implementer row carries an OnStuck, so its Stuck halts the run for a human.
	// The fault reaches the row only when the config changed after an earlier check passed,
	// and an operator would not guess that without being told.
	// The cause is returned as the row's reason, which reaches the persisted error and activity.wait, and also logged.
	logger.Warn("loomshed: active batchifier did not resolve", "producer", name, "anchorPath", dirs.Anchor, "cause", err)
	return nil, batchifierReasonPrefix + err.Error() + batchifierWayForward, nil
}

// Call implements shedengine.ShedProducer: it runs the implementer guard, resolves the active batchifier itself, fills
// the resolved value into a copy of w.deps, constructs a shedadapters.WebsterProducer over the
// result, and delegates to that producer's own Call.
//
// A guard refusal and a batcher.Active error each map to shedengine.Stuck, never to a returned error,
// so the run blocks for a human and nothing is spawned.
// A guard wiring fault is a returned error.
func (w *websterProducer) Call(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
	if err := entryErr(ctx, w.name); err != nil {
		return "", shedengine.OutputPointer{}, err
	}

	reason, refused, err := implementerGuard(weaverengine.ImplementerWebster, w.active)
	if err != nil {
		return "", shedengine.OutputPointer{}, err
	}
	if refused {
		if cerr := cancelErr(ctx, w.name); cerr != nil {
			return "", shedengine.OutputPointer{}, cerr
		}
		return shedengine.Stuck, shedengine.OutputPointer{Reason: reason}, nil
	}

	active, stuckReason, err := resolveActiveBatchifier(ctx, w.name, w.dirs)
	if err != nil {
		return "", shedengine.OutputPointer{}, err
	}
	if stuckReason != "" {
		return shedengine.Stuck, shedengine.OutputPointer{Reason: stuckReason}, nil
	}

	deps := w.deps
	deps.Batcher = active

	outcome, pointer, err := shedadapters.NewWebsterProducer(w.name, w.run, deps).Call(ctx)
	if err != nil || outcome != shedengine.Done {
		return outcome, pointer, err
	}

	// Master writes its contract files (outcome.yaml, summary.md) into the durable webster directory.
	// The verify gate writes its report there too, and nothing on webster's side commits them.
	// Per the Fabric Git Invariant, an agent writes into _lym and Go commits.
	// Left uncommitted, they ride through Publish and Finalize as untracked dirt.
	// Untracked dirt refuses the task worktree's later removal.
	// A commit failure is returned as an error, not Stuck, because re-running Webster cannot fix a git fault.
	if w.commit == nil {
		return "", shedengine.OutputPointer{}, fmt.Errorf("loomshed: %s: no commit seam wired; webster's run record would never be committed", w.name)
	}
	if err := w.commit(); err != nil {
		return "", shedengine.OutputPointer{}, fmt.Errorf("loomshed: %s: commit webster's run record: %w; way forward: the fault is transient, re-step the %s row", w.name, err, w.name)
	}
	return outcome, pointer, nil
}
