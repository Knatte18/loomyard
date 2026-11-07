// agentshuttle.go declares the shuttle seam that a spawned judge agent runs through.
// It also declares the hand-back vocabulary that every judge shares, and RunnerShuttle, the seam's runner adapter.

package sheddriver

import (
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// The causes an envelope carries when the call, not the agent, made the hand-back.
const (
	CauseTimeout      = "timeout"
	CauseDied         = "died"
	CauseMissing      = "missing"
	CauseUnparseable  = "unparseable"
	CausePrecondition = "precondition"
	CauseAPIError     = "api-error"
)

// SettlerAPIErrorRetries is how many times a judge re-sends its turn after the API ends it on an error.
// A judge holds its stop alone, so a session idle after an API error would hold the stop silently until its deadline.
// Only judges opt in, since each hands its stop on to a parent that can act on the api-error cause.
// Any other caller reads every outcome other than done as one more failure, so a re-send there would only delay it.
const SettlerAPIErrorRetries = 3

// HandBackQuestionCause returns the cause that a judge call's hand-back question names.
// For CauseAPIError it appends the error detail after the cause, separated by a colon.
func HandBackQuestionCause(cause, detail string) string {
	if cause != CauseAPIError {
		return cause
	}
	return cause + ": " + detail
}

// HandBackWayForward is the way-forward clause that ends every refusal of a judge call.
// It tells the judge to report the stop to the run's parent in the stop report.
const HandBackWayForward = "way forward: report the stop to the run's parent in the stop report"

// AgentRun is the handle of a started judge agent.
type AgentRun interface {
	// Wait blocks until the agent ends and classifies how.
	Wait() (shuttleengine.Result, error)
	// Stop ends the agent's strand and records the stop.
	Stop() error
	// StrandGUID names the agent's strand.
	StrandGUID() string
}

// AgentShuttle is the part of shuttle a judge call drives.
type AgentShuttle interface {
	// Start launches spec gated by gate and returns its handle; an empty gate runs ungated.
	Start(spec shuttleengine.Spec, gate shuttleengine.GateSpec) (AgentRun, error)
	// AttachIfLive waits, gated by gate, on a live run matching spec's output files, and reports false when none is live.
	AttachIfLive(spec shuttleengine.Spec, gate shuttleengine.GateSpec) (shuttleengine.Result, bool, error)
	// StopStrand ends the strand guid names and records the stop.
	StopStrand(guid string) error
}

// RunnerShuttle implements the shuttle seam by delegating each operation to a shuttle runner.
type RunnerShuttle struct {
	Runner *shuttleengine.Runner
}

// Start launches spec gated by gate through the runner and returns the started run.
func (s RunnerShuttle) Start(spec shuttleengine.Spec, gate shuttleengine.GateSpec) (AgentRun, error) {
	run, err := s.Runner.StartGated(spec, gate)
	if err != nil {
		return nil, err
	}
	return run, nil
}

// AttachIfLive waits, gated by gate, on a live run matching spec's output files.
func (s RunnerShuttle) AttachIfLive(spec shuttleengine.Spec, gate shuttleengine.GateSpec) (shuttleengine.Result, bool, error) {
	return s.Runner.AttachIfLiveGated(spec, gate)
}

// StopStrand ends the strand guid names through shuttle's stop verb.
func (s RunnerShuttle) StopStrand(guid string) error {
	return s.Runner.StopStrand(guid)
}
