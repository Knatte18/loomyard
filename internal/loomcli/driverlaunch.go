// driverlaunch.go implements the two driver seams on loomCLI -- driverStarter and driverPaneProbe --
// and their production adapters, modelled on runnerMasterStarter (cli.go), which already adapts the
// same *shuttleengine.Runner value for the same reason: the Test Tier Purity Invariant bars a real
// spawn from an untagged file, and both underlying engine values (*shuttleengine.Runner,
// *reedengine.Engine) are concrete types.

package loomcli

import (
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
