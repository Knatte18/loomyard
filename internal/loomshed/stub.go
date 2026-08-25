// stub.go implements stubProducer, a placeholder ShedProducer that loom's own producer list no
// longer uses.

package loomshed

import (
	"context"

	"github.com/Knatte18/loomyard/internal/shedengine"
)

// stubProducer is a placeholder ShedProducer that loom's own producer list no longer uses. It is
// kept because internal/shedrecipe's registry is generic Shed machinery shared by reference with a
// future product's producer list rather than loom's private property, and it exists so a list's
// sequencing, resume, crash-recovery, and pause behaviour is real from the start rather than
// retrofitted.
type stubProducer struct {
	name string
}

var _ shedengine.ShedProducer = (*stubProducer)(nil)

// NewStub returns a stubProducer identified as name. The return type is shedengine.ShedProducer,
// the seam interface, so the internal/shedrecipe registry can call this constructor from outside
// this package while stubProducer itself stays unexported.
func NewStub(name string) shedengine.ShedProducer {
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
