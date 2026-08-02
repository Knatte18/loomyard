// branchname.go — the single derivation of a fabric weft branch name from its
// paired host branch name, enforcing the uniform <host>/<host>-weft scheme.

package fabricengine

import "github.com/Knatte18/loomyard/internal/hubgeometry"

// WeftBranchName returns the weft branch name paired with hostBranch: `<branch>-weft`.
// This is the sole place fabric composes weft branch names, enforcing the Hub Geometry Invariant's token ban on "-weft" outside internal/hubgeometry.
// The inverse is hubgeometry.WeftHostSlug.
func WeftBranchName(hostBranch string) string {
	return hostBranch + hubgeometry.WeftSuffix
}
