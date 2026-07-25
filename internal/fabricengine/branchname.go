// branchname.go — the single derivation of a fabric weft branch name from its
// paired host branch name, enforcing the uniform <host>/<host>-weft scheme.

package fabricengine

import "github.com/Knatte18/loomyard/internal/hubgeometry"

// WeftBranchName returns the weft branch name paired with hostBranch under
// fabric's uniform branch-naming scheme: host `<branch>` always pairs with weft
// `<branch>-weft`, no exceptions — including the primary worktree (host "main"
// pairs with weft "main-weft"). This is the ONLY place fabric composes a weft
// branch name; every code path that needs one (add, remove, checkout, reconcile,
// status, drift, cleanup, clone) must call this function rather than
// concatenating the suffix itself, so the Hub Geometry Invariant's token ban on
// the literal "-weft" outside internal/hubgeometry holds for fabric's Go source.
// The inverse operation — recovering the host branch from a weft branch name —
// is hubgeometry.WeftHostSlug.
func WeftBranchName(hostBranch string) string {
	return hostBranch + hubgeometry.WeftSuffix
}
