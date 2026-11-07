// landingbranches.go resolves the task branch and the parent branch it lands on.
// It is the one branch resolution every landing-facing loom path shares.

package loomcli

import (
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lymcwd"
)

// landingBranches resolves the task branch and the branch it lands on over an already-opened fabric handle.
// The task branch is the handle's current branch; the branch it lands on is the pair's recorded parent.
// It is the one home of that resolution, so no landing path can drift from another.
// It takes the handle rather than opening one, so a caller that already holds the fabric never opens it twice.
func landingBranches(handle *fabricengine.Fabric, location *lymcwd.Location) (taskBranch, parentBranch string, err error) {
	taskBranch, err = handle.CurrentBranch()
	if err != nil {
		return "", "", err
	}
	recorded, found, err := fabricengine.ReadOrigin(location)
	if err != nil {
		return "", "", err
	}
	parentBranch, err = resolveLandingParent(recorded, found, taskBranch)
	if err != nil {
		return "", "", err
	}
	return taskBranch, parentBranch, nil
}
