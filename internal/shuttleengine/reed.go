// reed.go defines ReedOps, the package-local seam through which the run loop
// drives reed: only the subset of *reedengine.Engine's public API the run loop
// and the Interrupt/Send handle methods need (registering/removing a
// strand, reading session status, and the pane-transport ops). The seam
// exists so Runner/Wait/Interrupt/Send are testable against a hermetic fake
// without a live tmux server; *reedengine.Engine satisfies ReedOps as-is —
// the seam adds no adapter layer, it only narrows the type the run loop
// depends on.

package shuttleengine

import "github.com/Knatte18/loomyard/internal/reedengine"

// ReedOps is the subset of reed's engine API the shuttle run loop drives:
// AddStrand/RemoveStrand register and tear down the strand a run's pane
// lives in, Status reports strand liveness for the wait loop's periodic
// check, and SendText/SendKey/CapturePane are the pane-transport ops the
// startup probe and the Interrupt/Send handle methods need. shuttleengine
// depends on this interface, never the concrete *reedengine.Engine directly.
type ReedOps interface {
	// AddStrand registers a new strand from spec and returns it (with its
	// minted guid and resolved pane binding), mirroring
	// (*reedengine.Engine).AddStrand.
	AddStrand(spec reedengine.AddSpec) (reedengine.Strand, error)
	// RemoveStrand removes guid (and, when recursive is true, its
	// descendant subtree), mirroring (*reedengine.Engine).RemoveStrand.
	RemoveStrand(guid string, recursive bool) (reedengine.Removed, error)
	// Status reports this session's tracked strands and their live/dead
	// state, mirroring (*reedengine.Engine).Status.
	Status() (reedengine.StatusResult, error)
	// SendText types text into guid's live pane, submitting it with a
	// trailing Enter when submit is true, mirroring
	// (*reedengine.Engine).SendText.
	SendText(guid, text string, submit bool) error
	// SendKey sends a single named key into guid's live pane, mirroring
	// (*reedengine.Engine).SendKey.
	SendKey(guid, key string) error
	// CapturePane returns guid's live pane's current screen contents,
	// mirroring (*reedengine.Engine).CapturePane.
	CapturePane(guid string) (string, error)
}

// var _ ReedOps = (*reedengine.Engine)(nil) is the compile-time proof that
// *reedengine.Engine satisfies ReedOps as-is, so the run loop can be
// constructed against a real engine with no adapter glue.
var _ ReedOps = (*reedengine.Engine)(nil)
