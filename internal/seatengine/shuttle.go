// shuttle.go declares the seam a seat run starts and probes its seats' shuttle runs through, and RunnerShuttle, which adapts a shuttleengine.Runner to it.

package seatengine

import "github.com/Knatte18/loomyard/internal/shuttleengine"

// Handle is one started seat.
// The engine stops it through Stop, which records the stop, waits on it for the seat's terminal result, and sends the chair a line through Send while it waits.
// StrandName is the name reed formed for the seat's strand.
// *shuttleengine.Run satisfies it.
type Handle interface {
	StrandGUID() string
	StrandName() string
	RunDir() string
	Wait() (shuttleengine.Result, error)
	Send(text string) error
	Stop() error
}

var _ Handle = (*shuttleengine.Run)(nil)

// Shuttle is the seam Engine starts and probes its seats through.
// StartGated returns only once the seat's provider is past its startup gates;
// a provider that never came up is an error wrapping shuttleengine.ErrNotStarted, with a nil Handle.
// ProbeGated returns a live run matching the spec's output files as an unwaited Handle and true, or a nil Handle and false when there is none.
type Shuttle interface {
	StartGated(shuttleengine.Spec, shuttleengine.GateSpec) (Handle, error)
	ProbeGated(shuttleengine.Spec, shuttleengine.GateSpec) (Handle, bool, error)
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

// ProbeGated probes the runner for a live run of spec.
// The not-found answer and an error both return an interface-nil Handle, never a nil *shuttleengine.Run wrapped in one.
func (s runnerShuttle) ProbeGated(spec shuttleengine.Spec, gate shuttleengine.GateSpec) (Handle, bool, error) {
	run, found, err := s.runner.ProbeGated(spec, gate)
	if err != nil || !found {
		return nil, false, err
	}
	return run, true, nil
}
