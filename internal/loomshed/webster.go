// webster.go implements the lazy Webster wrapper around shedadapters.WebsterProducer: it resolves
// the active batchifier itself, inside Call, rather than at construction, and commits webster's
// durable run record once the run reports Done.

package loomshed

import (
	"context"
	"fmt"

	"github.com/Knatte18/loomyard/internal/batcher"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/shedadapters"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// websterProducer is the Webster producer: a lazy wrapper delegating to
// shedadapters.WebsterProducer once the active batchifier is resolved.
//
// Resolution is lazy, not injected at construction, because shedadapters.NewWebsterProducer takes
// websterengine.RunDeps by value, so injecting a resolved Batcher would require batcher.Active to
// have already succeeded before Shed.Run ever starts, which makes the Batchifier gate's stated
// value -- catching a broken config before Webster spawns LLM sessions -- unreachable; and after a
// crash-restart with current_producer naming Webster, the gate never re-runs in the new process, so
// an injected value would have to be re-resolved anyway.
//
// The consequence is correct behaviour, not staleness: if the batch config changes between the
// Batchifier and Webster rows, Webster uses the newer config, because there is no cached value to
// go stale -- the gate's guarantee is precisely "the config was resolvable at the Batchifier row",
// never "the config Webster will use is the one the Batchifier row saw".
type websterProducer struct {
	name       string
	anchorPath string
	run        shedadapters.WebsterRunner
	deps       websterengine.RunDeps
	commit     func() error
}

var _ shedengine.ShedProducer = (*websterProducer)(nil)

// NewWebsterProducer returns a websterProducer identified as name, resolving the active batchifier
// from anchorPath on every Call and driving run with a copy of deps carrying the resolved value.
// deps.Batcher is left nil by the caller; it is overwritten on every Call regardless. commit
// commits webster's durable run directory and is invoked once the run reports Done. The return
// type is shedengine.ShedProducer, the seam interface, so the internal/shedrecipe registry can
// call this constructor from outside this package while websterProducer itself stays unexported.
func NewWebsterProducer(name, anchorPath string, run shedadapters.WebsterRunner, deps websterengine.RunDeps, commit func() error) shedengine.ShedProducer {
	return &websterProducer{name: name, anchorPath: anchorPath, run: run, deps: deps, commit: commit}
}

// Call implements shedengine.ShedProducer: it resolves batcher.Active(w.anchorPath) itself, fills
// the resolved value into a copy of w.deps, constructs a shedadapters.WebsterProducer over the
// result, and delegates to that producer's own Call.
//
// A batcher.Active error here maps to shedengine.Stuck, never to a returned error -- identically to
// the Batchifier gate. The two outcomes differ materially in shedengine.Run: Stuck under
// OnStuck: "" persists blocked and returns RunBlocked, which a human resumes after fixing the
// config, whereas returning the error persists failed and aborts the run. The same fault must not
// end the run one way before Webster and another way at Webster.
func (w *websterProducer) Call(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
	if err := entryErr(ctx, w.name); err != nil {
		return "", shedengine.OutputPointer{}, err
	}

	active, err := batcher.Active(w.anchorPath)
	if err != nil {
		if cerr := cancelErr(ctx, w.name); cerr != nil {
			return "", shedengine.OutputPointer{}, cerr
		}
		// Surfaced rather than discarded, exactly as the Batchifier gate surfaces the identical
		// failure. This row carries no OnStuck either, so its Stuck halts the run for a human, and
		// the resolved-batchifier fault is the sort that reaches this row only when the config
		// changed after the gate already passed -- which is precisely the case an operator will not
		// guess without being told.
		logger.Warn("loomshed: active batchifier did not resolve", "producer", w.name, "anchorPath", w.anchorPath, "cause", err)
		return shedengine.Stuck, shedengine.OutputPointer{}, nil
	}

	deps := w.deps
	deps.Batcher = active

	outcome, pointer, err := shedadapters.NewWebsterProducer(w.name, w.run, deps).Call(ctx)
	if err != nil || outcome != shedengine.Done {
		return outcome, pointer, err
	}

	// Master writes its contract files (outcome.yaml, summary.md) and the integration report into
	// the durable webster directory, and nothing on webster's own side commits them: per the Fabric
	// Git Invariant an agent writes into _lyx and Go commits. Left uncommitted they ride through
	// Publish and Finalize as untracked dirt, which refuses the task worktree's later removal.
	// A commit failure is a returned error, not Stuck, for the reason the Discussion-Write commit
	// decorator gives: re-running Webster cannot fix a git fault.
	if w.commit == nil {
		return "", shedengine.OutputPointer{}, fmt.Errorf("loomshed: %s: no commit seam wired; webster's run record would never be committed", w.name)
	}
	if err := w.commit(); err != nil {
		return "", shedengine.OutputPointer{}, fmt.Errorf("loomshed: %s: commit webster's run record: %w", w.name, err)
	}
	return outcome, pointer, nil
}
