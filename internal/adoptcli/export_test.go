// export_test.go exposes the takeover over a real hub, run as a named machine, to the external integration test.

package adoptcli

import "github.com/Knatte18/loomyard/internal/lymcwd"

// Outcome and SlugOutcome name the verb's result types for the external test package.
type (
	Outcome     = outcome
	SlugOutcome = slugOutcome
)

// AdoptAs runs the takeover of the beam projects slugs name, and of root when the word `root` is among them, over the hub prime belongs to, reading this machine's name as machine, so two clones of one host stand for two machines.
func AdoptAs(prime *lymcwd.Location, machine string, slugs []string, keepLocal, dropLocal bool) (Outcome, error) {
	deps, err := hubDepsAs(prime, func() (string, error) { return machine, nil })
	if err != nil {
		return Outcome{}, err
	}
	return adopt(deps, request{Slugs: slugs, KeepLocal: keepLocal, DropLocal: dropLocal})
}
