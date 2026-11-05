// beamup.go implements BeamUp, the pawl recipe's Beam-Up row: the Go producer that brings a beam's pair up as a beam, with no agent.

package pawlengine

import (
	"context"
	"fmt"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// NameBeamUp is the pawl recipe's Beam-Up row name.
const NameBeamUp = "Beam-Up"

// BeamUp is the Beam-Up producer.
type BeamUp struct {
	name string
	deps Deps
}

var _ shedengine.ShedProducer = (*BeamUp)(nil)

// NewBeamUp returns the Beam-Up producer for the row called name.
// It refuses a Deps with an unset seam it uses, or a step with no name or no function.
func NewBeamUp(name string, deps Deps) (*BeamUp, error) {
	seams := append(deps.pathsSeams(),
		seam{"MarkBeam", deps.MarkBeam != nil},
		seam{"SpawnPush", deps.SpawnPush != nil},
		seam{"RegenerateWorkspace", deps.RegenerateWorkspace != nil},
		seam{"HandOff", deps.HandOff != nil},
		seam{"Notify", deps.Notify != nil},
	)
	for i, step := range deps.Steps {
		seams = append(seams, seam{fmt.Sprintf("Steps[%d]", i), step.Name != "" && step.Run != nil})
	}
	if err := requireSeams(name, seams...); err != nil {
		return nil, err
	}
	return &BeamUp{name: name, deps: deps}, nil
}

// Call marks the pair a beam, spawns its push, regenerates the hub workspace, removes a land request and the await marker a run before left, runs the beam steps in order, and then hands the beam's Sketch and proposal to its orch.
// Every step is idempotent, so a resumed row runs them all again.
// The hand-off runs only while the hand-off marker is absent: a found line is mailed through Notify and the marker is written after it, a beam with no proposal writes the marker with no mail.
// An interruption between the mail and the marker lets a re-run mail the same line again.
// A failing step, a hand-off that fails or one whose post reached no orch is a halting Stuck naming its cause and `lym loom resume`, with no marker written; a cancelled context is an error.
func (p *BeamUp) Call(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
	if err := entryErr(ctx, p.name); err != nil {
		return "", shedengine.OutputPointer{}, err
	}
	steps := append([]Step{
		{"mark the pair a beam", p.deps.MarkBeam},
		{"spawn the beam's push", p.deps.SpawnPush},
		{"regenerate the hub workspace", p.deps.RegenerateWorkspace},
		{"remove a land request an earlier run left", func() error { return RemoveLandRequest(p.deps.Paths) }},
		{"remove the await marker an earlier run left", func() error { return removeFile(p.deps.Paths.AwaitMarker) }},
	}, p.deps.Steps...)
	steps = append(steps, Step{"hand the Sketch and proposal to the beam's orch", p.handOff})
	for _, step := range steps {
		if err := step.Run(); err != nil {
			if cerr := cancelErr(ctx, p.name); cerr != nil {
				return "", shedengine.OutputPointer{}, cerr
			}
			reason := fmt.Sprintf("%s: %s: %v; way forward: fix the cause, then run \"lym loom resume\" in the beam's worktree", p.name, step.Name, err)
			logger.Warn("pawlengine: producer stuck", "producer", p.name, "reason", reason)
			return shedengine.Stuck, shedengine.OutputPointer{Reason: reason, Halt: true}, nil
		}
	}
	return shedengine.Done, shedengine.OutputPointer{}, nil
}

// handOff mails the beam's hand-off line to its orch and writes the hand-off marker, doing nothing while the marker stands.
func (p *BeamUp) handOff() error {
	settled, err := markerStands(p.deps.Paths.HandOffMarker)
	if err != nil {
		return err
	}
	if settled {
		return nil
	}
	line, found, err := p.deps.HandOff()
	if err != nil {
		return err
	}
	if found {
		posted, err := p.deps.Notify(line)
		if err != nil {
			return err
		}
		if !posted {
			return fmt.Errorf("the hand-off reached no orch, since the beam's orch state records no strand")
		}
	}
	return writeMarker(p.deps.Paths.HandOffMarker, time.Now())
}
