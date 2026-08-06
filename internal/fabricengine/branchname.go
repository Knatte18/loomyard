// branchname.go — the single derivation of a fabric weft branch name from its paired host branch
// name, enforcing the uniform <host>/<host>-weft scheme.

package fabricengine

import "github.com/Knatte18/loomyard/internal/weftname"

// WeftBranchName returns the weft branch name paired with hostBranch: `<branch>-weft`.
// This is the sole place fabric composes weft branch names, enforcing the Cwd Resolution
// Invariant's token ban on "-weft" outside internal/weftname.
// The inverse is lyxcwd.WeftHostSlug.
func WeftBranchName(hostBranch string) string {
	return hostBranch + weftname.Suffix
}
