// export_test.go re-exports newPaired, the now-private warp/weft fields, and the destructive gate's
// unexported executors and ownership predicates, for the handful of package fabricengine_test files
// that need to drive them directly rather than through an application-level verb whose own
// higher-level checks would short-circuit before the gate is ever reached — the standard Go
// export_test.go idiom, per the export-test-shim decision.

package fabricengine

import (
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

// NewPairedForTest re-exports newPaired for package fabricengine_test files that construct a Fabric
// from raw warp/weft paths rather than a lyxcwd.Location.
var NewPairedForTest = newPaired

// WarpForTest returns f's private warp field, for package fabricengine_test files that need
// warp-side gitrepo.Repo access no other exported accessor provides.
func WarpForTest(f *Fabric) *gitrepo.Repo {
	return f.warp
}

// WeftForTest returns f's private weft field, for package fabricengine_test files that need
// weft-side gitrepo.Repo access no other exported accessor provides.
func WeftForTest(f *Fabric) *gitrepo.Repo {
	return f.weft
}

// WarpProbeDirPrefixForTest re-exports warpProbeDirPrefix for package fabricengine_test files that
// assert on the probe's throwaway-clone directory naming without duplicating the literal.
const WarpProbeDirPrefixForTest = warpProbeDirPrefix

// ExcludePatternForTest re-exports excludePatternFor for package fabricengine_test files that
// assert on the anchored .git/info/exclude pattern without duplicating its spelling.
var ExcludePatternForTest = excludePatternFor

// RemoveWarpWorktreeDirForTest re-exports removeWarpWorktreeDir for package fabricengine_test
// integration tests that need to drive the gate's registered-linked-worktree ownership kind
// directly against an arbitrary target, rather than through Topology.Remove — whose own top-level
// dirty check (remove.go) refuses an untracked warp worktree before removeWarpWorktreeDir's own
// git-refusal-triggered fallback is ever reached.
var RemoveWarpWorktreeDirForTest = removeWarpWorktreeDir

// TeardownHubForTest re-exports teardownHub for package fabricengine_test integration tests that
// need to drive it directly rather than through a full CloneHub run, whose hub path is always
// derived as a child of its own cwd argument and so can never itself be outside the operator-named
// parent.
// It mints the createdToken teardownHub requires by creating hubPath itself via createExclusiveDir
// (hubPath must not already exist), running seed against the freshly-created directory before
// teardownHub runs, if seed is non-nil.
func TeardownHubForTest(cwd, hubPath string, seed func(hubPath string) error, cause error) error {
	tok, err := createExclusiveDir(hubPath)
	if err != nil {
		return err
	}
	if seed != nil {
		if err := seed(hubPath); err != nil {
			return err
		}
	}
	return teardownHub(cwd, hubPath, tok, cause)
}

// LooksLikeHubForTest re-exports looksLikeHub for package fabricengine_test integration tests
// covering the gate's fabric-hub ownership kind.
var LooksLikeHubForTest = looksLikeHub

// IsWarpCheckoutForTest re-exports isWarpCheckout for package fabricengine_test integration tests
// covering the gate's warp-checkout ownership kind, which ResetHard's own exported entry point
// cannot exercise with an arbitrary (repoDir, target) pair since it always calls it with both equal
// to the same f.warpPath.
var IsWarpCheckoutForTest = isWarpCheckout

// IsRegisteredLinkedWorktreeInForTest re-exports isRegisteredLinkedWorktreeIn for package
// fabricengine_test integration tests covering the gate's registered-linked-worktree ownership
// kind against an arbitrary (repoDir, target) pair.
var IsRegisteredLinkedWorktreeInForTest = isRegisteredLinkedWorktreeIn

// WorktreeDirtyTrackedForTest re-exports worktreeDirty(scopeTracked, dir) for package
// fabricengine_test integration tests, since dirtyScope itself is unexported.
func WorktreeDirtyTrackedForTest(dir string) (dirty bool, detail string, err error) {
	return worktreeDirty(scopeTracked, dir)
}

// WorktreeDirtyAllForTest re-exports worktreeDirty(scopeAll, dir) for package fabricengine_test
// integration tests, since dirtyScope itself is unexported.
func WorktreeDirtyAllForTest(dir string) (dirty bool, detail string, err error) {
	return worktreeDirty(scopeAll, dir)
}

// DeleteBranchForTest re-exports the gate's deleteBranch executor, built from
// ownedManagedBranch(l, branchPrefix) and dirtyCheckedOutBranch(), for package fabricengine_test
// integration tests that need to drive the branch-ownership gate directly rather than through
// Cleanup's application-level orphan/liveness filtering — which never reaches the gate at all for
// the primary weft branch or a branch checked out at some worktree.
func DeleteBranchForTest(l *lyxcwd.Location, repoDir, branch, branchPrefix string, force bool) (exitCode int, stderr string, err error) {
	req := branchRequest{
		what:      "test delete branch",
		repoDir:   repoDir,
		branch:    branch,
		ownership: ownedManagedBranch(l, branchPrefix),
		dirtiness: dirtyCheckedOutBranch(),
		force:     force,
	}
	return deleteBranch(req)
}
