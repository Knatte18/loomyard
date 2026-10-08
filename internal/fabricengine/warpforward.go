// warpforward.go collects the warp-only forwarding methods on *Fabric: thin, one-line delegations
// to the paired gitrepo.Repo verb on f.code, added so an out-of-package caller can invoke a
// warp-mutating git verb through Fabric's public API — preserving the one-repo illusion — without
// ever touching f.code directly.
// Kept in its own file rather than folded into fabric.go to keep the delegation cluster isolated
// from Fabric's construction and cross-repo plumbing.

package fabricengine

// CurrentBranch returns the short name of the branch the warp checkout's HEAD currently points at.
// It is a thin delegation to gitrepo.Repo.CurrentBranch on f.code,
// and inherits that method's rejection of detached HEAD (returns wrapped error).
func (f *Fabric) CurrentBranch() (string, error) {
	return f.code.CurrentBranch()
}

// HeadSHA returns the full commit SHA the warp checkout's HEAD currently points at.
// It is a thin, read-only delegation to gitrepo.Repo.CurrentSHA on f.code,
// so a caller reads a commit without naming a fabric side.
func (f *Fabric) HeadSHA() (string, error) {
	return f.code.CurrentSHA()
}

// IsAncestor reports whether sha is an ancestor of ref in the warp checkout.
// It is a thin, read-only delegation to gitrepo.Repo.IsAncestor on f.code.
func (f *Fabric) IsAncestor(sha, ref string) (bool, error) {
	return f.code.IsAncestor(sha, ref)
}

// ResetHard has moved to destroy.go, where it becomes the gated executor for the ResetHard
// primitive — see that file's own doc comment. It is not a thin delegation like its neighbours
// above: there is exactly one correct ownership/dirtiness declaration for "reset this Fabric's warp
// checkout", so the gated wrapper hardcodes it rather than exposing it, and it lives beside the rest
// of the destructive surface instead of here.
