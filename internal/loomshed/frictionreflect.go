// frictionreflect.go implements frictionReflect, loom's terminal producer: it runs the injected
// friction-reflection closure once and always reports Done.

package loomshed

import (
	"context"
	"fmt"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// frictionReflect is loom's terminal row, placed after Finalize so the engine persists state: done
// only once the reflection returns (batten's Run-Shed row watches the child's status file for done
// and tears the worktree down right after). It returns Done on every closure result, because
// failing or blocking a run whose landing already happened over an optional bookkeeping agent is
// strictly worse than filing nothing. It derives no path and imports nothing friction-specific:
// whether to reflect at all (Tier 2 off, armed for step) is the injected closure's decision, per
// the Told-Geometry Invariant. There is deliberately no cancelErr check after the closure returns:
// once the reflection has run the row's work is done, and done must persist.
type frictionReflect struct {
	name    string
	reflect func() string
}

var _ shedengine.ShedProducer = (*frictionReflect)(nil)

// NewFrictionReflect returns a frictionReflect identified as name that calls reflectFriction once
// per Call. The return type is shedengine.ShedProducer, the seam interface, so the registry can
// call this constructor from outside the package while frictionReflect stays unexported. The nil
// check lives here so the registry entry need not duplicate it.
func NewFrictionReflect(name string, reflectFriction func() string) (shedengine.ShedProducer, error) {
	if reflectFriction == nil {
		return nil, fmt.Errorf("loomshed: %s: reflect closure must not be nil", name)
	}
	return &frictionReflect{name: name, reflect: reflectFriction}, nil
}

// Call implements shedengine.ShedProducer: after consulting entryErr, it calls the reflection
// closure exactly once, logs the status it returns, and reports Done.
func (p *frictionReflect) Call(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
	if err := entryErr(ctx, p.name); err != nil {
		return "", shedengine.OutputPointer{}, err
	}
	status := p.reflect()
	logger.Info("loomshed: friction reflection row finished", "producer", p.name, "status", status)
	return shedengine.Done, shedengine.OutputPointer{}, nil
}
