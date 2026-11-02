// roles.go declares the reed strand roles a fixer runs under and the check that a role is one of them.

package gatefixer

import "github.com/Knatte18/loomyard/internal/agentname"

const (
	// RoleGateFixer is the strand role of a fixer a writer's or an agent's gate hands its failure to.
	RoleGateFixer = "gate-fixer"
	// RoleVerifyFixer is the strand role of the cold fixer webster's verify gate escalates a failure to.
	RoleVerifyFixer = "verify-fixer"
)

// Roles returns every fixer role, RoleGateFixer first.
func Roles() []string {
	return []string{RoleGateFixer, RoleVerifyFixer}
}

// IsFixerRole reports whether role is a fixer role or a numbered form of one, such as "gate-fixer-2".
func IsFixerRole(role string) bool {
	for _, fixerRole := range Roles() {
		if agentname.MatchesRole(role, fixerRole) {
			return true
		}
	}
	return false
}
