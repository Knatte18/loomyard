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

// ReedOps is the subset of reed's engine API the shuttle run loop drives.
// AddStrand/RemoveStrand register and tear down the strand a run's pane lives in;
// Status reports strand liveness; SendText/SendKey/CapturePane are the pane-transport ops.
// shuttleengine depends on this interface, never the concrete *reedengine.Engine directly.
type ReedOps interface {
	AddStrand(spec reedengine.AddSpec) (reedengine.Strand, error)
	RemoveStrand(guid string, recursive bool) (reedengine.Removed, error)
	Status() (reedengine.StatusResult, error)
	SendText(guid, text string, submit bool) error
	SendKey(guid, key string) error
	CapturePane(guid string) (string, error)
}

// Compile-time proof that *reedengine.Engine satisfies ReedOps as-is.
var _ ReedOps = (*reedengine.Engine)(nil)
