// driverlaunch.go defines the seam interfaces loomCLI uses to start the loom driver session.
// The seams also reach the session's strand in reed.
// Its production adapters satisfy those seams over *shuttleengine.Runner and *reedengine.Engine.
// The seams exist because the Test Tier Purity Invariant bars a real spawn from an untagged file,
// and both engine values are concrete types.

package loomcli

import (
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// driverHandle is the surface of a started driver run that the bootstrap reads after launch.
// It exposes the run's reed strand guid and its run directory.
// Shuttle's Start blocks through the provider's startup gates before returning a handle,
// so there is no startup await here.
type driverHandle interface {
	// StrandGUID returns the reed strand guid bound to this run.
	StrandGUID() string
	// RunDir returns the directory holding this run's artifacts.
	RunDir() string
}

// driverStarter starts the loom driver session's shuttle run.
type driverStarter interface {
	// StartDriver returns only once the run's provider is past its startup gates;
	// shuttle's Start blocks through them before it returns a handle.
	// A provider that never became ready is returned as an error wrapping shuttleengine.ErrNotStarted.
	StartDriver(spec shuttleengine.Spec) (driverHandle, error)
}

// driverSender types one line into a live driver strand's pane.
type driverSender interface {
	// SendDriver types text as the strand's next turn and verifies it arrived.
	// A busy or not-ready pane waits inside the send;
	// a pane that stays busy fails with an error wrapping shuttleengine.ErrSessionBusy.
	// An engine without the idle reading refuses a pane whose provider is not input-ready at once;
	// the error wraps shuttleengine.ErrPaneNotReady.
	// Any other error may follow a delivery the pane hid, so a caller must not blindly re-send after it.
	SendDriver(guid, text string) error
}

// driverPaneProbe reads and removes driver strands.
type driverPaneProbe interface {
	// Strands returns this session's tracked strands and their live/dead state.
	Strands() ([]reedengine.StrandStatus, error)
	// RemoveDriverStrand removes the driver strand identified by guid.
	RemoveDriverStrand(guid string) error
}

// driverDirectoryProbe reads the driver strand's row from reed's sessionless directory.
type driverDirectoryProbe interface {
	// DriverRow returns the first row loomengine.IsDriverStrand accepts, and false when the directory holds none.
	// A session that died with the machine still lists its strands as dormant rows,
	// so a dead driver is a row with Live false rather than an absent one.
	DriverRow() (reedengine.DirectoryRow, bool, error)
}

// runnerDriverStarter adapts *shuttleengine.Runner to driverStarter.
type runnerDriverStarter struct {
	runner *shuttleengine.Runner
}

// StartDriver implements driverStarter by delegating to the runner's Start.
// The returned *shuttleengine.Run already satisfies driverHandle through its StrandGUID and RunDir methods.
func (s runnerDriverStarter) StartDriver(spec shuttleengine.Spec) (driverHandle, error) {
	run, err := s.runner.Start(spec)
	if err != nil {
		return nil, err
	}
	return run, nil
}

// SendDriver implements driverSender through the runner's Send.
// Send waits for an idle session, verifies delivery, and fails with ErrSessionBusy when the session stays busy.
func (s runnerDriverStarter) SendDriver(guid, text string) error {
	return s.runner.Send(guid, text)
}

// reedDriverPaneProbe adapts *reedengine.Engine to driverPaneProbe.
//
// It holds the two engine methods it wraps as plain function fields instead of the engine value.
//
// Both methods need a live tmux session to exercise for real,
// so a test of the cascade argument this adapter passes to remove needs a seam narrower than the engine.
type reedDriverPaneProbe struct {
	status func() (reedengine.StatusResult, error)
	remove func(guid string, recursive bool) (reedengine.Removed, error)
}

// newReedDriverPaneProbe builds a reedDriverPaneProbe wrapping the engine's Status and RemoveStrand methods.
// It is production code's only constructor, so no code outside this file may build one with different function values.
func newReedDriverPaneProbe(reed *reedengine.Engine) reedDriverPaneProbe {
	return reedDriverPaneProbe{status: reed.Status, remove: reed.RemoveStrand}
}

// Strands implements driverPaneProbe by delegating to the engine's Status and returning its strand slice.
func (p reedDriverPaneProbe) Strands() ([]reedengine.StrandStatus, error) {
	result, err := p.status()
	if err != nil {
		return nil, err
	}
	return result.Strands, nil
}

// RemoveDriverStrand implements driverPaneProbe by delegating to the engine's remove with the cascade argument false.
// A driver strand is parentless and childless, so the cascade is inert.
func (p reedDriverPaneProbe) RemoveDriverStrand(guid string) error {
	_, err := p.remove(guid, false)
	return err
}

// reedDriverDirectory adapts *reedengine.Engine to driverDirectoryProbe.
// It holds the engine's Directory method as a plain function field, so a test can substitute it.
type reedDriverDirectory struct {
	directory func() ([]reedengine.DirectoryRow, error)
}

// newReedDriverDirectory builds a reedDriverDirectory wrapping the engine's Directory method.
func newReedDriverDirectory(reed *reedengine.Engine) reedDriverDirectory {
	return reedDriverDirectory{directory: reed.Directory}
}

// DriverRow implements driverDirectoryProbe over the engine's Directory.
func (d reedDriverDirectory) DriverRow() (reedengine.DirectoryRow, bool, error) {
	rows, err := d.directory()
	if err != nil {
		return reedengine.DirectoryRow{}, false, err
	}
	for _, row := range rows {
		if loomengine.IsDriverStrand(row.Name) {
			return row, true, nil
		}
	}
	return reedengine.DirectoryRow{}, false, nil
}
