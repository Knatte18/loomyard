// reed.go defines ReedOps, the package-local seam through which the run loop drives reed.
// It holds only the subset of *reedengine.Engine's public API that the run loop and the Interrupt and Send methods need:
// registering and removing a strand, reading session status, and the pane-transport ops.
// The seam lets the run loop be tested against a hermetic fake, without a live tmux server.
// *reedengine.Engine satisfies ReedOps as-is.
// The seam adds no adapter layer; it only narrows the type the run loop depends on.

package shuttleengine

import (
	"time"

	"github.com/Knatte18/loomyard/internal/reedengine"
)

// ReedOps is the subset of reed's engine API the shuttle run loop drives.
// AddStrand and RemoveStrand register and tear down the strand a run's pane lives in.
// Status reports strand liveness.
// SendText, SendKey, CapturePane, and CapturePaneStyled are the pane-transport ops.
// PasteSubmit is the pane-write transport op: one bracketed paste with an optional Enter behind it.
// OperatorActive is the operator-activity query.
// It answers whether an attached client is active on the strand's pane within a quiet window.
// SetWaitMark sets the strand's pane wait mark, or clears it when label is empty; the mark is display only.
// shuttleengine depends on this interface, never on the concrete *reedengine.Engine directly.
type ReedOps interface {
	AddStrand(spec reedengine.AddSpec) (reedengine.Strand, error)
	RemoveStrand(guid string, recursive bool) (reedengine.Removed, error)
	Status() (reedengine.StatusResult, error)
	SendText(guid, text string, submit bool) error
	SendKey(guid, key string) error
	CapturePane(guid string) (string, error)
	CapturePaneStyled(guid string) (string, error)
	PasteSubmit(guid, text string, submit bool) error
	OperatorActive(guid string) (active bool, quiet time.Duration, err error)
	SetWaitMark(guid, label string, start time.Time) error
}

// Compile-time proof that *reedengine.Engine satisfies ReedOps as-is.
var _ ReedOps = (*reedengine.Engine)(nil)
