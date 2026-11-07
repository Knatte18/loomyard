// frictionreflect.go implements frictionReflect, loom's terminal producer:
// it runs the injected friction-reflection closure once and always reports Done.

package loomshed

import (
	"context"
	"fmt"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// frictionReflect is loom's terminal row, placed last so the engine persists state once the row reports done.
// The row reports done only after the reflection returns:
// the process that watches the run's status file for done tears the worktree down right after.
// It returns Done on every closure result.
// Failing or blocking a run that has already landed over an optional bookkeeping agent is worse than filing nothing.
// It derives no path and imports nothing friction-specific.
// Whether to reflect is the injected closure's decision, for example when Tier 2 is off or the run is armed for step.
// The row does not check for cancellation after the closure returns:
// once the reflection has run, the row's work is done, and done must persist.
type frictionReflect struct {
	name    string
	reflect func() string
}

var _ shedengine.ShedProducer = (*frictionReflect)(nil)

// NewFrictionReflect returns a producer identified as name that calls reflectFriction once per Call.
// The return type is an interface.
// Callers outside the package can therefore use the constructor while the implementation stays unexported.
// Nil reflectFriction is rejected here, so callers need not check for it.
func NewFrictionReflect(name string, reflectFriction func() string) (shedengine.ShedProducer, error) {
	if reflectFriction == nil {
		return nil, fmt.Errorf("loomshed: %s: reflect closure must not be nil", name)
	}
	return &frictionReflect{name: name, reflect: reflectFriction}, nil
}

// Call runs the entry check and returns its error if it fails.
// Otherwise it calls the reflection closure exactly once, logs the status it returns, and reports Done.
func (p *frictionReflect) Call(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
	if err := entryErr(ctx, p.name); err != nil {
		return "", shedengine.OutputPointer{}, err
	}
	status := p.reflect()
	logger.Info("loomshed: friction reflection row finished", "producer", p.name, "status", status)
	return shedengine.Done, shedengine.OutputPointer{}, nil
}
