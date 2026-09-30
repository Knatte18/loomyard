package loomcli

import (
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

// landingBranches resolves the task branch and the branch it lands on, over an already-opened
// fabric handle: CurrentBranch, then ReadOrigin, then resolveLandingParent. It is the one home of
// that sequence, so Publish's route, Describe's and approve's cannot drift apart. It takes the
// handle rather than opening one so a caller that already holds the fabric never opens it twice.
func landingBranches(handle *fabricengine.Fabric, location *lyxcwd.Location) (taskBranch, parentBranch string, err error) {
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
