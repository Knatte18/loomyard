// recordsstop.go builds the pull stop error that tests outside fabricengine hand to the code under test.

package hubforge

import "github.com/Knatte18/loomyard/internal/fabricengine"

// NewRecordsStopError returns the *fabricengine.WeftStopError a Fabric.Pull returns for branch.
// The error carries cause as the stop and wayForward as the step that settles it.
// It lets a test outside fabricengine build the stop without spelling out the stop type.
func NewRecordsStopError(branch string, cause error, wayForward string) error {
	return &fabricengine.WeftStopError{Branch: branch, Err: cause, WayForward: wayForward}
}
