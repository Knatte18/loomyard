// webster.go implements the lazy Webster wrapper around shedadapters.WebsterProducer: it resolves
// the active batchifier itself, inside Call, rather than at construction, and commits webster's
// durable run record once the run reports Done.

package loomshed

import (
	"context"
	"fmt"

	"github.com/Knatte18/loomyard/internal/batcher"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/shedadapters"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// batchifierReasonPrefix leads the Stuck reason of every row that loads batcher.yaml with batcher.Active, so each words the fault identically; the error text follows.
const batchifierReasonPrefix = "active batchifier did not resolve: "

// batchifierWayForward closes that reason;
// the fault sits in batcher.yaml, so the operator fixes it there.
const batchifierWayForward = "; way forward: fix batcher.yaml (its active: key or the profile it names), then re-step"

// websterProducer is the Webster producer: a lazy wrapper delegating to
// shedadapters.WebsterProducer once the active batchifier is resolved.
//
// Batching is the implementer's own concern, so the producer resolves the batchifier itself at the start of every Call rather than taking one at construction:
// a resumed run re-resolves it in the new process, and a config changed since Loom-Preflight loaded it is the config Webster partitions the plan with.
type websterProducer struct {
	name   string
	dirs   configengine.Dirs
	run    shedadapters.WebsterRunner
	deps   websterengine.RunDeps
	commit func() error
}

var _ shedengine.ShedProducer = (*websterProducer)(nil)

// NewWebsterProducer returns a websterProducer identified as name, resolving the active batchifier
// from the told dirs on every Call and driving run with a copy of deps carrying the resolved value.
// deps.Batcher is left nil by the caller; it is overwritten on every Call regardless. commit
// commits webster's durable run directory and is invoked once the run reports Done. The return
// type is shedengine.ShedProducer, the seam interface, so the internal/shedrecipe registry can
// call this constructor from outside this package while websterProducer itself stays unexported.
func NewWebsterProducer(name string, dirs configengine.Dirs, run shedadapters.WebsterRunner, deps websterengine.RunDeps, commit func() error) shedengine.ShedProducer {
	return &websterProducer{name: name, dirs: dirs, run: run, deps: deps, commit: commit}
}

// Call implements shedengine.ShedProducer: it resolves batcher.Active(w.dirs) itself, fills
// the resolved value into a copy of w.deps, constructs a shedadapters.WebsterProducer over the
// result, and delegates to that producer's own Call.
//
// A batcher.Active error here maps to shedengine.Stuck, never to a returned error -- identically to Loom-Preflight's batcher.yaml check.
// The two outcomes differ materially in shedengine.Run:
// Stuck under OnStuck: "" persists blocked and returns RunBlocked, which a human resumes after fixing the config, whereas returning the error persists failed and aborts the run.
// The same fault must not end the run one way before Webster and another way at Webster.
func (w *websterProducer) Call(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
	if err := entryErr(ctx, w.name); err != nil {
		return "", shedengine.OutputPointer{}, err
	}

	active, err := batcher.Active(w.dirs)
	if err != nil {
		if cerr := cancelErr(ctx, w.name); cerr != nil {
			return "", shedengine.OutputPointer{}, cerr
		}
		// Surfaced rather than discarded, exactly as Loom-Preflight surfaces the identical
		// failure. This row carries no OnStuck, so its Stuck halts the run for a human, and
		// the resolved-batchifier fault is the sort that reaches this row only when the config
		// changed after Loom-Preflight already passed -- which is precisely the case an operator will not
		// guess without being told. The cause is returned as the row's reason, which reaches the
		// persisted error and activity.wait, and also logged.
		logger.Warn("loomshed: active batchifier did not resolve", "producer", w.name, "anchorPath", w.dirs.Anchor, "cause", err)
		return shedengine.Stuck, shedengine.OutputPointer{Reason: batchifierReasonPrefix + err.Error() + batchifierWayForward}, nil
	}

	deps := w.deps
	deps.Batcher = active

	outcome, pointer, err := shedadapters.NewWebsterProducer(w.name, w.run, deps).Call(ctx)
	if err != nil || outcome != shedengine.Done {
		return outcome, pointer, err
	}

	// Master writes its contract files (outcome.yaml, summary.md), and the verify gate its report,
	// into the durable webster directory, and nothing on webster's own side commits them: per the Fabric
	// Git Invariant an agent writes into _lym and Go commits. Left uncommitted they ride through
	// Publish and Finalize as untracked dirt, which refuses the task worktree's later removal.
	// A commit failure is a returned error, not Stuck, for the reason the Discussion-Write commit
	// decorator gives: re-running Webster cannot fix a git fault.
	if w.commit == nil {
		return "", shedengine.OutputPointer{}, fmt.Errorf("loomshed: %s: no commit seam wired; webster's run record would never be committed", w.name)
	}
	if err := w.commit(); err != nil {
		return "", shedengine.OutputPointer{}, fmt.Errorf("loomshed: %s: commit webster's run record: %w; way forward: the fault is transient, re-step the %s row", w.name, err, w.name)
	}
	return outcome, pointer, nil
}
