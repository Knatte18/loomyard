// errors.go declares the package's exported sentinel errors.

package shedengine

import "errors"

// ErrShedBusy marks Run's fail-fast refusal when another invocation already holds LockPath.
// It is a sentinel (matched via errors.Is) so a caller can retry later rather than treating the
// refusal as a real failure, mirroring treadleengine.ErrBlockBusy's role.
// It is returned wrapped with %w.
var ErrShedBusy = errors.New("shed is already running")

// ErrLockedCheckRefused marks a Run or Step that Shed.LockedCheck refused after the run lock was taken.
// The returned error carries this sentinel's text ahead of the check's own, so the check's trailing way forward stays trailing, and matches both through errors.Is.
var ErrLockedCheckRefused = errors.New("refused under the run lock")

// ShedBusyKind is the envelope `kind` of a verb refusal whose error wraps ErrShedBusy, so a caller reads the kind rather than the message.
const ShedBusyKind = "shed_busy"
