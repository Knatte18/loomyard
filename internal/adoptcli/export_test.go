// export_test.go exposes the takeover over a real hub, run as a named machine, to the external integration test.

package adoptcli

import "github.com/Knatte18/loomyard/internal/lyxcwd"

// Outcome and SlugOutcome name the verb's result types for the external test package.
type (
	Outcome     = outcome
	SlugOutcome = slugOutcome
)

// AdoptAs runs the takeover of slugs, and of the prime when adoptPrime is set, over the hub prime belongs to, reading this machine's name as machine, so two clones of one host stand for two machines.
func AdoptAs(prime *lyxcwd.Location, machine string, slugs []string, adoptPrime, keepLocal, dropLocal bool) (Outcome, error) {
	deps, err := hubDepsAs(prime, func() (string, error) { return machine, nil })
	if err != nil {
		return Outcome{}, err
	}
	return adopt(deps, request{Slugs: slugs, Prime: adoptPrime, KeepLocal: keepLocal, DropLocal: dropLocal})
}
