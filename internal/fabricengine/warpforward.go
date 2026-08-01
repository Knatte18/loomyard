// warpforward.go collects the warp-only forwarding methods on *Fabric: thin,
// one-line delegations to the paired gitrepo.Repo verb on f.Warp, added so an
// out-of-package caller can invoke a warp-mutating git verb through Fabric's
// public API — preserving the one-repo illusion — without ever touching
// f.Warp directly. Kept in its own file rather than folded into fabric.go to
// keep the delegation cluster isolated from Fabric's construction and
// cross-repo plumbing.

package fabricengine

// CheckoutDetached moves the warp checkout's HEAD to sha without updating any
// branch ref, leaving the warp working tree at that commit's contents. It is
// a thin delegation to gitrepo.Repo.CheckoutDetached on f.Warp — the
// underlying method validates sha (returning ErrInvalidSHA) before any git
// spawn; CheckoutDetached adds no validation of its own. This method touches
// warp exclusively; it never mutates weft.
func (f *Fabric) CheckoutDetached(sha string) error {
	return f.Warp.CheckoutDetached(sha)
}

// RestoreBranch moves the warp checkout's HEAD back onto ref, ending a
// detached-HEAD state (typically one CheckoutDetached left behind). It is a
// thin delegation to gitrepo.Repo.RestoreBranch on f.Warp. This method
// touches warp exclusively; it never mutates weft.
func (f *Fabric) RestoreBranch(ref string) error {
	return f.Warp.RestoreBranch(ref)
}

// CurrentBranch returns the short name of the branch the warp checkout's HEAD
// currently points at. It is a thin delegation to gitrepo.Repo.CurrentBranch
// on f.Warp, and inherits that method's documented rejection of a detached
// HEAD — a wrapped error is returned rather than an empty string, since a
// caller that failed to capture a branch name has no safe ref to hand
// RestoreBranch afterwards. This method reads warp exclusively; it never
// touches weft.
func (f *Fabric) CurrentBranch() (string, error) {
	return f.Warp.CurrentBranch()
}

// ResetHard resets the warp checkout's HEAD, index, and working tree to sha,
// discarding any local commits or uncommitted changes past that point. It is
// a thin delegation to gitrepo.Repo.ResetHard on f.Warp — the underlying
// method validates sha (returning ErrInvalidSHA) before any git spawn;
// ResetHard adds no validation of its own. This method touches warp
// exclusively; it never mutates weft.
func (f *Fabric) ResetHard(sha string) error {
	return f.Warp.ResetHard(sha)
}
