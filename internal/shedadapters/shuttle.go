// shuttle.go declares Shuttle, the seam over shuttleengine.Runner that the shed adapters use to run and probe agents.

package shedadapters

import (
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// Shuttle is the seam over shuttleengine.Runner that runs agents and probes for live ones.
// Run starts a fresh agent and waits on it.
// Attach probes for a live, never-terminated run matching the Spec before a caller archives and respawns over it.
// A false bool comes back with a zero Result and a nil error, meaning nothing was found and one should be started.
// A true bool means a run was found and waited on, and its Result and error (if any) are returned alongside.
// The production implementation is *shuttleengine.Runner; every other implementation is a test fake.
//
// RunGated and AttachGated are separate methods rather than a widened Run and Attach.
// A widened signature would make every caller with no gate pass a zero GateSpec that means nothing to it.
type Shuttle interface {
	Run(shuttleengine.Spec) (shuttleengine.Result, error)
	Attach(shuttleengine.Spec) (shuttleengine.Result, bool, error)
	// AttachIfLive probes for a live run matching the Spec and waits on it if found, without removing superseded strands.
	// A caller that only waits on an existing run uses it, and starts nothing when no run is found.
	AttachIfLive(shuttleengine.Spec) (shuttleengine.Result, bool, error)
	// RunGated starts a fresh agent and waits on it.
	// Its declared output artifacts are validated by the gate's entries, if any, before the round's report is trusted.
	RunGated(shuttleengine.Spec, shuttleengine.GateSpec) (shuttleengine.Result, error)
	// AttachGated attaches to a live run and gates its declared output artifacts against the gate's entries, if any.
	AttachGated(shuttleengine.Spec, shuttleengine.GateSpec) (shuttleengine.Result, bool, error)
}

var _ Shuttle = (*shuttleengine.Runner)(nil)
