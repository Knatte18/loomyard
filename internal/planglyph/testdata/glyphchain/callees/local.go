// local.go calls Target from a second file of the callees package.

package callees

// CallsTarget calls Target.
func CallsTarget() int { return Target() }
