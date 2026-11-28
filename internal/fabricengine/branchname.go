// branchname.go derives the fabric weft branch name from its paired warp branch name.
// The derivation enforces the uniform <warp>/<warp>-weft scheme.

package fabricengine

import "github.com/Knatte18/loomyard/internal/weftname"

// RecordsBranchName returns the weft branch name paired with warpBranch: `<branch>-weft`.
// Fabric composes weft branch names only here.
// Code outside internal/weftname must not spell the "-weft" token.
func RecordsBranchName(warpBranch string) string {
	return warpBranch + weftname.Suffix
}
