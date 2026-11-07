// clone_emptyweft_integration_test.go pins the empty-weft-remote bootstrap.
// That path is the first-ever setup.
// probeWeftBinding's unborn-HEAD check and ensureBoardWorktree's orphan branch exist to serve it.
//
// `git checkout -b <branch>` on an unborn HEAD succeeds but writes no ref.
// A clone against a genuinely empty weft remote must leave the weft primary on a branch that exists as a ref;
// nothing later creates it, since the clone-time commit lands on the Bolt checkout's own unsuffixed branch.
// Every verb that forks a new pair from the primary then dies on `fatal: invalid reference: <branch>-weft`:
// `lym fabric add` is one of them.
//
// Fixture helpers (makeBareRemote, makeEmptyBareRemote) are reused from clone_adopt_test.go;
// package fabricengine_test shares the TestMain in testmain_test.go.

package fabricengine_test

import (
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestCloneHub_EmptyWeftRemoteLeavesPrimaryBranchBorn clones against an empty weft remote.
// It asserts that the weft primary's suffixed branch is a real ref afterwards;
// and that `Add`, the verb the documented example runs, works on the resulting hub.
//
//testtiming:keep a clone against an empty weft remote leaving the suffixed primary branch a real ref so Add works; coverage of its blocks by other tests does not show an assertion of this
func TestCloneHub_EmptyWeftRemoteLeavesPrimaryBranchBorn(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	dir := t.TempDir()
	warpRemote := makeBareRemote(t, dir, "warp")
	weftRemote := makeEmptyBareRemote(t, dir, "weft")

	cloneParent := t.TempDir()

	res, err := fabricengine.CloneHub(cloneParent, fabricengine.CloneOptions{
		WeftURL:   weftRemote,
		WarpURL:   warpRemote,
		Shortname: "tst",
	})
	if err != nil {
		t.Fatalf("CloneHub against an empty weft remote: %v", err)
	}

	weftPrimary := filepath.Join(res.HubPath, "warp-weft")
	suffixed := fabricengine.RecordsBranchName("main")

	// The ref must exist, not merely be the checked-out name.
	// An unborn branch is checked out and reports as current, while refs/heads/<branch> resolves to nothing;
	// that is the state which makes every later fork fail.
	got := gitOutput(t, weftPrimary, "rev-parse", "--verify", "--quiet", "refs/heads/"+suffixed)
	if got == "" {
		t.Fatalf("refs/heads/%s does not resolve after a clone against an empty weft remote; the weft primary is on an unborn branch", suffixed)
	}

	// The git operation every pair-creating verb performs against that branch must succeed.
	// It is the git call of Add's createWeftWorktree step, run on its own.
	// That call can fail with `fatal: invalid reference: main-weft`.
	// Driving Add itself is impossible here;
	// the repo-wide fabric.yaml is materialised by the CLI layer through configsync, which fabricengine must never import.
	gitkit.MustRun(t, weftPrimary, "git", "worktree", "add", "-b",
		fabricengine.RecordsBranchName("my-task"), filepath.Join(res.HubPath, "my-task-weft"), suffixed)
}

// TestCloneHub_NonEmptyWeftRemoteBranchUnchanged asserts that the ordinary clone path does this:
// it keeps the suffixed branch paired with the cloned HEAD and adds no empty commit from the unborn-branch repair.
// The only commit the clone adds on top is the fresh bind's anchor marker.
//
//testtiming:keep the ordinary clone path pairing the suffixed branch with the cloned HEAD without an extra empty commit; coverage of its blocks by other tests does not show an assertion of this
func TestCloneHub_NonEmptyWeftRemoteBranchUnchanged(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	dir := t.TempDir()
	warpRemote := makeBareRemote(t, dir, "warp")
	weftRemote := makeBareRemote(t, dir, "weft")

	cloneParent := t.TempDir()

	res, err := fabricengine.CloneHub(cloneParent, fabricengine.CloneOptions{
		WeftURL: weftRemote,
		WarpURL: warpRemote,
		// The weft fixture carries a commit but no .lym-anchor,
		// so the old-order bootstrap guard would refuse it; this test is about the branch, not the guard.
		ForceBootstrap: true,
		Shortname:      "tst",
	})
	if err != nil {
		t.Fatalf("CloneHub: %v", err)
	}

	weftPrimary := filepath.Join(res.HubPath, "warp-weft")
	suffixed := fabricengine.RecordsBranchName("main")

	suffixedParentSHA := gitOutput(t, weftPrimary, "rev-parse", "refs/heads/"+suffixed+"^")
	clonedSHA := gitOutput(t, weftPrimary, "rev-parse", "refs/remotes/origin/main")
	if suffixedParentSHA != clonedSHA {
		t.Errorf("parent of refs/heads/%s = %s; want the cloned HEAD %s — the unborn repair must not add a commit on a non-empty remote",
			suffixed, suffixedParentSHA, clonedSHA)
	}
	if changed := gitOutput(t, weftPrimary, "diff", "--name-only", clonedSHA, "refs/heads/"+suffixed); changed != lymcwd.AnchorFileName {
		t.Errorf("files the clone changed on refs/heads/%s = %q; want only %q", suffixed, changed, lymcwd.AnchorFileName)
	}
}
