// branchname.go — the single derivation of a fabric weft branch name from its paired warp branch
// name, enforcing the uniform <warp>/<warp>-weft scheme.

package fabricengine

import "github.com/Knatte18/loomyard/internal/weftname"

// WeftBranchName returns the weft branch name paired with warpBranch: `<branch>-weft`.
// This is the sole place fabric composes weft branch names, enforcing the Cwd Resolution
// Invariant's token ban on "-weft" outside internal/weftname.
// The inverse is WeftWarpSlug (weftwiring.go).
func WeftBranchName(warpBranch string) string {
	return warpBranch + weftname.Suffix
}
