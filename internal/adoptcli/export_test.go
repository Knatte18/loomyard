// export_test.go exposes the takeover over a real hub, run as a named machine, to the external integration test.

package adoptcli

import (
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

// AdoptAs runs the takeover over the hub prime belongs to.
// It covers the beam projects slugs name, and root when the word `root` is among them.
// It reads machine as this machine's name, so two clones of one host stand for two machines.
func AdoptAs(prime *lymcwd.Location, machine string, slugs []string, keepLocal, dropLocal bool) (Outcome, error) {
	deps, err := hubDepsAs(prime, func() (string, error) { return machine, nil })
	if err != nil {
		return Outcome{}, err
	}
	return adopt(deps, request{Slugs: slugs, KeepLocal: keepLocal, DropLocal: dropLocal})
}
