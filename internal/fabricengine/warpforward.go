// warpforward.go collects the warp-only forwarding methods on *Fabric: thin, one-line delegations
// to the paired gitrepo.Repo verb on f.warp, added so an out-of-package caller can invoke a
// warp-mutating git verb through Fabric's public API — preserving the one-repo illusion — without
// ever touching f.warp directly.
// Kept in its own file rather than folded into fabric.go to keep the delegation cluster isolated
// from Fabric's construction and cross-repo plumbing.

package fabricengine

// CheckoutDetached moves the warp checkout's HEAD to sha without updating any branch ref, leaving
// the warp working tree at that commit's contents.
// It is a thin delegation to gitrepo.Repo.CheckoutDetached on f.warp.
func (f *Fabric) CheckoutDetached(sha string) error {
	return f.warp.CheckoutDetached(sha)
}

// RestoreBranch moves the warp checkout's HEAD back onto ref, ending a detached-HEAD state.
// It is a thin delegation to gitrepo.Repo.RestoreBranch on f.warp.
func (f *Fabric) RestoreBranch(ref string) error {
	return f.warp.RestoreBranch(ref)
}

// CurrentBranch returns the short name of the branch the warp checkout's HEAD currently points at.
// It is a thin delegation to gitrepo.Repo.CurrentBranch on f.warp,
// and inherits that method's rejection of detached HEAD (returns wrapped error).
func (f *Fabric) CurrentBranch() (string, error) {
	return f.warp.CurrentBranch()
}

// ResetHard has moved to destroy.go, where it becomes the gated executor for the ResetHard
// primitive — see that file's own doc comment. It is not a thin delegation like its neighbours
// above: there is exactly one correct ownership/dirtiness declaration for "reset this Fabric's warp
// checkout", so the gated wrapper hardcodes it rather than exposing it, and it lives beside the rest
// of the destructive surface instead of here.
