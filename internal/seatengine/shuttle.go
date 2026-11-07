// shuttle.go declares the Shuttle seam that a seat run starts and probes its shuttle runs through.
// RunnerShuttle adapts a shuttleengine.Runner to that seam.

package seatengine

import "github.com/Knatte18/loomyard/internal/shuttleengine"

// Handle is one started seat.
// The engine stops it through Stop, which records the stop and waits for the seat's terminal result.
// While it waits, Stop sends the chair a line through Send.
// StrandName is the name reed formed for the seat's strand.
// SessionState is the seat's own session-state reading, read from files, so it is safe to call while Wait runs.
type Handle interface {
	StrandGUID() string
	StrandName() string
	RunDir() string
	SessionState() (shuttleengine.RunSessionState, error)
	Wait() (shuttleengine.Result, error)
	Send(text string) error
	Stop() error
}

var _ Handle = (*shuttleengine.Run)(nil)

// Shuttle is the seam through which the engine starts and probes seats.
// StartGated returns only once the seat's provider is past its startup gates;
// a provider that never came up is an error wrapping shuttleengine.ErrNotStarted, with a nil Handle.
// ProbeGated returns an unwaited Handle and true for a live run matching the spec's output files.
// It returns a nil Handle and false when there is none.
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
