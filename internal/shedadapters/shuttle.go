// shuttle.go declares Shuttle, the seam over shuttleengine.Runner that Bouncer drives and the Burler rows' attach probes read.

package shedadapters

import (
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// Shuttle is the seam Bouncer drives and the Burler rows probe through.
// Run starts a fresh agent and waits on it, matching burlerengine's own identically-shaped seam over
// shuttleengine.Runner.
// Attach probes for a still-live, never-terminated run matching the given Spec before a caller
// archives and respawns over it: a false bool comes back with a zero Result and a nil error, meaning
// "nothing to attach to, start one"; a true bool means a run was found and waited on, whose own
// Result and error (if any) are returned alongside.
// The sole production implementor is *shuttleengine.Runner, and every other implementor is a test fake in this repo.
// A method added to the seam is therefore a compile error in a fake rather than a producer that quietly stops probing.
//
// RunGated and AttachGated are added beside Run and Attach rather than a widening of either:
// Bouncer has no gate and never will, so widening Run and Attach would rewrite its call sites to pass a zero value that means nothing there.
type Shuttle interface {
	Run(shuttleengine.Spec) (shuttleengine.Result, error)
	Attach(shuttleengine.Spec) (shuttleengine.Result, bool, error)
	// AttachIfLive is Attach's probe without the removal of superseded strands, for a caller that waits on a live run and starts nothing after a not-found answer.
	AttachIfLive(shuttleengine.Spec) (shuttleengine.Result, bool, error)
	// RunGated is Run, gated: the run's declared output artifacts are additionally validated by the gate's entries (if any) before the round's report is trusted.
	RunGated(shuttleengine.Spec, shuttleengine.GateSpec) (shuttleengine.Result, error)
	// AttachGated is Attach, gated: an attached run's declared output artifacts are gated exactly as
	// a freshly-spawned run's are.
	AttachGated(shuttleengine.Spec, shuttleengine.GateSpec) (shuttleengine.Result, bool, error)
}

var _ Shuttle = (*shuttleengine.Runner)(nil)
