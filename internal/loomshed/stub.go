// stub.go implements stubProducer, the placeholder ShedProducer backing every row of loom's
// 12-row producer list this task does not build for real.

package loomshed

import (
	"context"

	"github.com/Knatte18/loomyard/internal/shedengine"
)

// stubProducer is a placeholder ShedProducer. It backs five rows of loom's 12-row producer list
// this task does not build for real -- Discussion-Write, Discussion-Review, Plan-Write,
// Plan-Review, and Webster-Review -- each replaced by a real producer in a later task, so the
// list's sequencing, resume,
// crash-recovery, and pause behaviour is real from the start rather than retrofitted.
type stubProducer struct {
	name string
}

var _ shedengine.ShedProducer = (*stubProducer)(nil)

// newStub returns a stubProducer identified as name.
func newStub(name string) *stubProducer {
	return &stubProducer{name: name}
}

// Call implements shedengine.ShedProducer: after consulting entryErr, it unconditionally reports
// Done with an empty OutputPointer and a nil error -- even a stub honours the cancellation
// obligation rather than reporting a verdict for a run an operator already stopped.
func (p *stubProducer) Call(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
	if err := entryErr(ctx, p.name); err != nil {
		return "", shedengine.OutputPointer{}, err
	}
	return shedengine.Done, shedengine.OutputPointer{}, nil
}
