// driverlaunch.go implements the two driver seams on loomCLI -- driverStarter and driverPaneProbe --
// and their production adapters, modelled on runnerMasterStarter (cli.go), which already adapts the
// same *shuttleengine.Runner value for the same reason: the Test Tier Purity Invariant bars a real
// spawn from an untagged file, and both underlying engine values (*shuttleengine.Runner,
// *reedengine.Engine) are concrete types.

package loomcli

import (
	"time"

	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// driverHandle is the started driver run's own seam-facing surface: the two identities the bootstrap
// needs after launch, without depending on *shuttleengine.Run's concrete type.
type driverHandle interface {
	// StrandGUID returns the reed strand guid bound to this run.
	StrandGUID() string
	// RunDir returns the directory holding this run's artifacts.
	RunDir() string
}

// driverStarter starts the ly-drive session's shuttle run.
type driverStarter interface {
	// StartDriver starts spec and returns a handle without blocking.
	StartDriver(spec shuttleengine.Spec) (driverHandle, error)
}

// driverPaneProbe reads and removes driver strands.
type driverPaneProbe interface {
	// Strands returns this session's tracked strands and their live/dead state.
	Strands() ([]reedengine.StrandStatus, error)
	// RemoveDriverStrand removes the driver strand identified by guid.
	RemoveDriverStrand(guid string) error
}

// runnerDriverStarter adapts *shuttleengine.Runner to driverStarter.
type runnerDriverStarter struct {
	runner *shuttleengine.Runner
}

// StartDriver implements driverStarter by delegating to the runner's own Start, whose returned
// *shuttleengine.Run already satisfies driverHandle via its StrandGUID and RunDir accessors.
func (s runnerDriverStarter) StartDriver(spec shuttleengine.Spec) (driverHandle, error) {
	run, err := s.runner.Start(spec)
	if err != nil {
		return nil, err
	}
	return run, nil
}

// reedDriverPaneProbe adapts *reedengine.Engine to driverPaneProbe.
//
// It holds the two engine methods it wraps as plain function fields, filled from a real
// *reedengine.Engine by newReedDriverPaneProbe, rather than holding the engine value directly: both
// methods require a live tmux session to exercise for real, so a test proving the cascade argument
// this adapter passes to remove needs a seam narrower than the whole engine to substitute.
type reedDriverPaneProbe struct {
	status func() (reedengine.StatusResult, error)
	remove func(guid string, recursive bool) (reedengine.Removed, error)
}

// newReedDriverPaneProbe builds a reedDriverPaneProbe wrapping reed's own Status and RemoveStrand
// methods -- production code's only constructor, so no code outside this file may build one with
// different function values.
func newReedDriverPaneProbe(reed *reedengine.Engine) reedDriverPaneProbe {
	return reedDriverPaneProbe{status: reed.Status, remove: reed.RemoveStrand}
}

// Strands implements driverPaneProbe by delegating to the engine's own Status and returning its
// strand slice.
func (p reedDriverPaneProbe) Strands() ([]reedengine.StrandStatus, error) {
	result, err := p.status()
	if err != nil {
		return nil, err
	}
	return result.Strands, nil
}

// RemoveDriverStrand implements driverPaneProbe by delegating to the engine's own remove, with the
// cascade argument false. The cascade is inert for a driver strand, which is parentless and
// childless, so a reader does not wonder why the recursive form is not used here.
func (p reedDriverPaneProbe) RemoveDriverStrand(guid string) error {
	_, err := p.remove(guid, false)
	return err
}

// driverPanePollInterval and driverPaneAttempts bound awaitDriverPane, declared beside it in the same
// shape the handshake constants (bootstrapHandshakePollInterval, bootstrapHandshakeAttempts, start.go)
// carry. The budget is kept short deliberately: this probe answers "did the provider binary boot at
// all", a question settled in seconds, not the minutes the run-lock handshake allows for a full
// producer pass.
const (
	driverPanePollInterval = 100 * time.Millisecond
	driverPaneAttempts     = 50
)

// findStrandByGUID returns the first strand in strands whose GUID exactly matches guid.
func findStrandByGUID(strands []reedengine.StrandStatus, guid string) (reedengine.StrandStatus, bool) {
	for _, s := range strands {
		if s.GUID == guid {
			return s, true
		}
	}
	return reedengine.StrandStatus{}, false
}

// awaitDriverPane polls at most attempts times for the just-launched driver strand identified by
// guid to show up live in the slice strands returns, reporting ready as soon as it is present and
// live. A strand that is absent or present-but-not-live continues the loop; the seam erroring
// returns that error immediately; exhausting the attempt budget reports not-ready with a nil error.
//
// Cap attempt COUNT, not only elapsed time, per the Live-Substrate Spawn Observability invariant's
// retry clause. wait is an injected seam, the same four-seam shape awaitRunLock (bootstrap.go)
// already uses, so a test can drive the whole poll with no wall-clock sleep.
//
// This probe closes the common half of "did the provider boot at all" -- binary absent, immediate
// exit, launch line rejected by the shell -- using reed alone, with no new shuttle API and no change
// to the wait loop's contract. Starting a run only proves the pane was created and the launch line
// sent into it, never that the provider inside it booted; this probe is what closes that gap.
//
// It does NOT close the case of a provider that boots, takes the pane, and then never reads its
// prompt: that leaves a live pane and a run that never moves, and this probe reports it ready. That
// residual degrades to the outer run's watch budget landing in a blocked state, where an operator
// attaching sees a session sitting idle -- legible in a way a dead pane is not.
func awaitDriverPane(strands func() ([]reedengine.StrandStatus, error), guid string, wait func(), attempts int) (bool, error) {
	for i := 0; i < attempts; i++ {
		list, err := strands()
		if err != nil {
			return false, err
		}
		if strand, found := findStrandByGUID(list, guid); found && strand.Live {
			return true, nil
		}
		wait()
	}
	return false, nil
}
