// weaver.go holds the lazy Weaver producer and the guard both implementer rows run before they spawn anything.
// The guard keeps one run on one implementer: a row refuses to start over the other implementer's begun batches.

package loomshed

import (
	"context"
	"fmt"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/shedadapters"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/weaverengine"
)

// resetAdvice maps each implementer to the command that ends its run, so the other implementer's row can name it.
var resetAdvice = map[weaverengine.Implementer]string{
	weaverengine.ImplementerWebster: "`lym webster reset --to start`",
	weaverengine.ImplementerWeaver:  "`lym weaver reset`",
}

// implementerGuard reports whether this implementer's row may run over the run's recorded state.
// It asks the active-implementer seam.
// A refusal is returned as the text of a stop, with true.
// One cause is that the other implementer holds begun batches in this run's current generation.
// The other is that the seam cannot tell which implementer the run belongs to.
// Each refusal ends in its way forward.
// A neither-begun run proceeds, whichever implementer the setting names.
// A zero active.AnchorRoot is a wiring fault, returned as an error.
func implementerGuard(this weaverengine.Implementer, active weaverengine.ActiveDeps) (string, bool, error) {
	if active.AnchorRoot == "" {
		return "", false, fmt.Errorf("loomshed: the %s row has no implementer seam anchor; way forward: fill the recipe Env's ImplementerSeam where the recipe is wired", this)
	}

	implementer, state, err := weaverengine.Active(active)
	if err != nil {
		return err.Error(), true, nil
	}
	if implementer == this || state == nil || len(state.Batches) == 0 {
		return "", false, nil
	}

	return fmt.Sprintf("the %s row cannot run: this run's %s already holds begun batches; way forward: end that run with %s, "+
		"or set the implementer back with `lym config tapestry --set implementer=%s`",
		this, implementer, resetAdvice[implementer], implementer), true, nil
}

// weaverProducer is the lazy Weaver producer.
// On every Call it runs the implementer guard and resolves the batchifier weaver.yaml names.
// It then delegates to shedadapters.WeaverProducer.
type weaverProducer struct {
	name   string
	dirs   configengine.Dirs
	run    shedadapters.WeaverRunner
	deps   weaverengine.RunDeps
	active weaverengine.ActiveDeps
}

var _ shedengine.ShedProducer = (*weaverProducer)(nil)

// NewWeaverProducer returns a producer identified as name.
// On every Call it runs the implementer guard over active,
// and resolves from dirs the batchifier of the profile deps.Config names.
// It then drives run with a copy of deps that carries the resolved batchifier; the caller leaves deps.Batcher nil.
// Weaver commits its run record itself, so the producer takes no commit seam.
// The result is typed as shedengine.ShedProducer.
// A registry in another package can call this constructor while the concrete type stays unexported.
func NewWeaverProducer(name string, dirs configengine.Dirs, run shedadapters.WeaverRunner, deps weaverengine.RunDeps, active weaverengine.ActiveDeps) shedengine.ShedProducer {
	return &weaverProducer{name: name, dirs: dirs, run: run, deps: deps, active: active}
}

// Call implements shedengine.ShedProducer.
// A guard refusal and a batcher.yaml fault each map to Stuck with their way forward, spawning nothing,
// so the run blocks for a human instead of failing.
// A guard wiring fault is a returned error.
func (w *weaverProducer) Call(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
	if err := entryErr(ctx, w.name); err != nil {
		return "", shedengine.OutputPointer{}, err
	}

	reason, refused, err := implementerGuard(weaverengine.ImplementerWeaver, w.active)
	if err != nil {
		return "", shedengine.OutputPointer{}, err
	}
	if refused {
		if cerr := cancelErr(ctx, w.name); cerr != nil {
			return "", shedengine.OutputPointer{}, cerr
		}
		return shedengine.Stuck, shedengine.OutputPointer{Reason: reason}, nil
	}

	resolved, stuckReason, err := resolveBatchifier(ctx, w.name, w.dirs, w.deps.Config.BatchProfile, weaverengine.BatchProfileKeyName())
	if err != nil {
		return "", shedengine.OutputPointer{}, err
	}
	if stuckReason != "" {
		return shedengine.Stuck, shedengine.OutputPointer{Reason: stuckReason}, nil
	}

	deps := w.deps
	deps.Batcher = resolved
	return shedadapters.NewWeaverProducer(w.name, w.run, deps).Call(ctx)
}
