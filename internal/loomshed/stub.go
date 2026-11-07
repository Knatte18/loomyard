// stub.go implements stubProducer, a placeholder ShedProducer that loom's own producer list does not use.

package loomshed

import (
	"context"

	"github.com/Knatte18/loomyard/internal/shedengine"
)

// stubProducer is a placeholder ShedProducer.
// It makes a producer list's sequencing, resume, crash recovery, and pause behaviour real from the start.
// It is kept because internal/shedrecipe's registry is generic Shed machinery,
// and a future product's producer list shares it by reference.
type stubProducer struct {
	name string
}

var _ shedengine.ShedProducer = (*stubProducer)(nil)

// NewStub returns a stub producer identified as name.
// It returns the interface shedengine.ShedProducer, not the unexported type.
// Code outside this package can therefore construct and hold it.
func NewStub(name string) shedengine.ShedProducer {
	return &stubProducer{name: name}
}

// Call implements shedengine.ShedProducer.
// It first runs the entry check, so a run an operator already stopped gets that error instead of a verdict.
// Otherwise it reports Done with an empty OutputPointer and a nil error.
func (p *stubProducer) Call(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
	if err := entryErr(ctx, p.name); err != nil {
		return "", shedengine.OutputPointer{}, err
	}
	return shedengine.Done, shedengine.OutputPointer{}, nil
}
