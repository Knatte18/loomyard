// export_test.go exposes the role list to the external tests, which check the template against it.

package agentconfig

import "slices"

// Roles returns every role lym starts, in sort order.
func Roles() []Role {
	return slices.Clone(roles)
}
