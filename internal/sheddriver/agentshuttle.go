// agentshuttle.go declares the shuttle seam a spawned judge agent runs through, and the hand-back vocabulary every such judge shares.
// The seam is the part of shuttle a judge call drives; RunnerShuttle adapts the real runner to it.

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
)

// HandBackWayForward is the way-forward clause every refusal of a judge call ends with: the hand-back to the run's parent.
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
	// Start launches spec and returns its handle.
	Start(spec shuttleengine.Spec) (AgentRun, error)
	// AttachIfLive waits on a live run matching spec's output files, and reports false when none is live.
	AttachIfLive(spec shuttleengine.Spec) (shuttleengine.Result, bool, error)
	// StopStrand ends the strand guid names and records the stop.
	StopStrand(guid string) error
}

// RunnerShuttle adapts the shuttle runner to AgentShuttle.
type RunnerShuttle struct {
	Runner *shuttleengine.Runner
}

// Start launches spec through the runner and returns the started run.
func (s RunnerShuttle) Start(spec shuttleengine.Spec) (AgentRun, error) {
	run, err := s.Runner.Start(spec)
	if err != nil {
		return nil, err
	}
	return run, nil
}

// AttachIfLive waits on a live run matching spec's output files.
func (s RunnerShuttle) AttachIfLive(spec shuttleengine.Spec) (shuttleengine.Result, bool, error) {
	return s.Runner.AttachIfLive(spec)
}

// StopStrand ends the strand guid names through shuttle's stop verb.
func (s RunnerShuttle) StopStrand(guid string) error {
	return s.Runner.StopStrand(guid)
}
