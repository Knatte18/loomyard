// export_test.go exposes the takeover over a real hub, run as a named machine, to the external integration test.

package adoptcli

import (
	"io"

	"github.com/Knatte18/loomyard/internal/buildvcs"
	"github.com/Knatte18/loomyard/internal/lymcwd"
)

// Outcome, SlugOutcome and DeployState name the verb's result types for the external test package.
type (
	Outcome     = outcome
	SlugOutcome = slugOutcome
	DeployState = deployState
)

// ReadDeployState reads the deploy step's state over prime:
// it judges the build readInstalled returns against the prime's head.
// It exists so the external integration test, a separate package, can read that state.
func ReadDeployState(prime *lymcwd.Location, readInstalled func() (buildvcs.Identity, error)) (DeployState, error) {
	return readDeployState(prime, readInstalled)
}

// AdoptAs runs the foreground adopt over the hub prime belongs to, in this process.
// It covers the beam projects subjects name, and root when the word `root` is among them.
// It moves another machine's hold when takeHold is set.
// It reads machine as this machine's name, so two clones of one host stand for two machines.
func AdoptAs(prime *lymcwd.Location, machine func() (string, error), subjects []string, takeHold bool) (Outcome, error) {
	deps, err := hubDepsAs(prime, machine)
	if err != nil {
		return Outcome{}, err
	}
	return runForeground(deps, request{Slugs: subjects, TakeHold: takeHold}, io.Discard)
}
