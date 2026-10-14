// switched.go implements SwitchedProducer, the producer that runs one told operator-convenience action when its told flag is on and passes straight on when it is off.

package shedadapters

import (
	"context"
	"fmt"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// SwitchedAction is one switched row's two seams, filled by the recipe's owner.
type SwitchedAction struct {
	// Enabled reads the row's flag, evaluated fresh on every Call, so a flag the run wrote after wiring is honoured.
	Enabled func() (bool, error)
	// Act performs the row's action.
	Act func(ctx context.Context) error
}

// SwitchedProducer runs its action when its flag is on and is Done either way.
type SwitchedProducer struct {
	name   string
	engine string
	action SwitchedAction
}

var _ shedengine.ShedProducer = (*SwitchedProducer)(nil)

// NewSwitchedProducer returns the producer of the row name, whose engine label engine names it in logs and errors.
func NewSwitchedProducer(name, engine string, action SwitchedAction) *SwitchedProducer {
	return &SwitchedProducer{name: name, engine: engine, action: action}
}

// Call implements shedengine.ShedProducer.
// A flag that is off is Done without acting.
// A flag that is on runs the action once and is Done whether or not the action succeeded, since the action is a convenience no run should halt over; a failed action is logged at Warn.
// A flag that cannot be read is an error and runs no action.
func (p *SwitchedProducer) Call(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
	if err := entryErr(ctx, p.name, p.engine); err != nil {
		return "", shedengine.OutputPointer{}, err
	}
	enabled, err := p.action.Enabled()
	if err != nil {
		return "", shedengine.OutputPointer{}, fmt.Errorf("shedadapters: %s (%s): read the row's flag: %w", p.name, p.engine, err)
	}
	if !enabled {
		logger.Info("shedadapters: switched row off; passing on", "producer", p.name, "engine", p.engine)
		return shedengine.Done, shedengine.OutputPointer{}, nil
	}
	if err := p.action.Act(ctx); err != nil {
		if cerr := cancelErr(ctx, p.name, p.engine); cerr != nil {
			return "", shedengine.OutputPointer{}, cerr
		}
		logger.Warn("shedadapters: switched row's action failed; passing on", "producer", p.name, "engine", p.engine, "error", err)
	}
	return shedengine.Done, shedengine.OutputPointer{}, nil
}
