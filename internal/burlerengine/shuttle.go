// shuttle.go declares the seam a burler round starts its two halves through: Shuttle starts one gated run and hands back a Handle the engine waits on, and RunnerShuttle adapts a shuttleengine.Runner to it.

package burlerengine

import "github.com/Knatte18/loomyard/internal/shuttleengine"

// Handle is one started half of a round.
// The engine reads its strand guid to stop it, and waits on it for the half's terminal result.
// *shuttleengine.Run satisfies it.
type Handle interface {
	StrandGUID() string
	RunDir() string
	Wait() (shuttleengine.Result, error)
}

var _ Handle = (*shuttleengine.Run)(nil)

// Shuttle is the seam Engine starts a round's halves through.
// StartGated returns only once the half's provider is past its startup gates;
// a provider that never came up is an error wrapping shuttleengine.ErrNotStarted, with a nil Handle.
type Shuttle interface {
	StartGated(shuttleengine.Spec, shuttleengine.GateSpec) (Handle, error)
}

// runnerShuttle adapts a shuttleengine.Runner to Shuttle.
type runnerShuttle struct {
	runner *shuttleengine.Runner
}

// RunnerShuttle returns the Shuttle that starts runs on r.
func RunnerShuttle(r *shuttleengine.Runner) Shuttle {
	return runnerShuttle{runner: r}
}

// StartGated starts spec on the runner.
// A failed start returns an interface-nil Handle, never a nil *shuttleengine.Run wrapped in one.
func (s runnerShuttle) StartGated(spec shuttleengine.Spec, gate shuttleengine.GateSpec) (Handle, error) {
	run, err := s.runner.StartGated(spec, gate)
	if err != nil {
		return nil, err
	}
	return run, nil
}
