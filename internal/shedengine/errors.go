// errors.go declares the package's single exported sentinel error.

package shedengine

import "errors"

// ErrShedBusy marks Run's fail-fast refusal when another invocation already holds LockPath.
// It is a sentinel (matched via errors.Is) so a caller can retry later rather than treating the
// refusal as a real failure, mirroring treadleengine.ErrBlockBusy's role.
// It is returned wrapped with %w.
var ErrShedBusy = errors.New("shed is already running")

// ShedBusyKind is the envelope `kind` of a verb refusal whose error wraps ErrShedBusy, so a caller reads the kind rather than the message.
const ShedBusyKind = "shed_busy"
