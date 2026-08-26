// webster.go implements the lazy Webster wrapper around shedadapters.WebsterProducer: it resolves
// the active batchifier itself, inside Call, rather than at construction.

package loomshed

import (
	"context"

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
}

var _ shedengine.ShedProducer = (*websterProducer)(nil)

// NewWebsterProducer returns a websterProducer identified as name, resolving the active batchifier
// from anchorPath on every Call and driving run with a copy of deps carrying the resolved value.
// deps.Batcher is left nil by the caller; it is overwritten on every Call regardless. The return
// type is shedengine.ShedProducer, the seam interface, so the internal/shedrecipe registry can
// call this constructor from outside this package while websterProducer itself stays unexported.
func NewWebsterProducer(name, anchorPath string, run shedadapters.WebsterRunner, deps websterengine.RunDeps) shedengine.ShedProducer {
	return &websterProducer{name: name, anchorPath: anchorPath, run: run, deps: deps}
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

	return shedadapters.NewWebsterProducer(w.name, w.run, deps).Call(ctx)
}
