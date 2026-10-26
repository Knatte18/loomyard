// beamdown.go implements BeamDown, the pawl recipe's Beam-Down row: the Go producer that takes a beam's dobby and orch down, with no agent.

package pawlengine

import (
	"context"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// NameBeamDown is the pawl recipe's Beam-Down row name.
const NameBeamDown = "Beam-Down"

// BeamDown is the Beam-Down producer.
type BeamDown struct {
	name string
	deps Deps
}

var _ shedengine.ShedProducer = (*BeamDown)(nil)

// NewBeamDown returns the Beam-Down producer for the row called name.
// It refuses a Deps with an unset Down.
func NewBeamDown(name string, deps Deps) (*BeamDown, error) {
	if err := requireSeams(name, seam{"Down", deps.Down != nil}); err != nil {
		return nil, err
	}
	return &BeamDown{name: name, deps: deps}, nil
}

// Call takes the beam down.
// A failing stop is logged as a warning and the row is Done, since the run's Worktree-Teardown ends what is left; a cancelled context is an error.
func (p *BeamDown) Call(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
	if err := entryErr(ctx, p.name); err != nil {
		return "", shedengine.OutputPointer{}, err
	}
	if err := p.deps.Down(); err != nil {
		if cerr := cancelErr(ctx, p.name); cerr != nil {
			return "", shedengine.OutputPointer{}, cerr
		}
		logger.Warn("pawlengine: the beam's stop failed, leaving what is left to the worktree teardown", "producer", p.name, "error", err)
	}
	return shedengine.Done, shedengine.OutputPointer{}, nil
}
