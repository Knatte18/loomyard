//go:build integration

// clone_emptyweft_integration_test.go pins the empty-weft-remote bootstrap: the documented
// first-ever-setup path that probeWeftBinding's unborn-HEAD check and ensureBoardWorktree's orphan
// branch both exist to serve.
//
// `git checkout -b <branch>` on an unborn HEAD succeeds but writes no ref, so a clone against a
// genuinely empty weft remote used to leave the weft primary sitting on a branch that did not
// exist. Nothing later filled it in — the clone-time commit lands on _board's own unsuffixed branch
// — so every verb that forks a new pair from the primary died on
// `fatal: invalid reference: <branch>-weft`, `lyx fabric add` included, which is the example both
// the parent command and `add` document.
//
// Fixture helpers (makeBareRemote, makeEmptyBareRemote) are reused from clone_adopt_test.go;
// package fabricengine_test shares the TestMain in testmain_test.go.

package fabricengine_test

import (
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
)

// TestCloneHub_EmptyWeftRemoteLeavesPrimaryBranchBorn clones against an empty weft remote and
// asserts the weft primary's suffixed branch is a real ref afterwards, and that Add — the verb the
// documented Example runs — actually works on the resulting hub.
func TestCloneHub_EmptyWeftRemoteLeavesPrimaryBranchBorn(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	warpRemote := makeBareRemote(t, dir, "warp")
	weftRemote := makeEmptyBareRemote(t, dir, "weft")

	cloneParent := t.TempDir()

	res, err := fabricengine.CloneHub(cloneParent, fabricengine.CloneOptions{
		WeftURL: weftRemote,
		WarpURL: warpRemote,
	})
	if err != nil {
		t.Fatalf("CloneHub against an empty weft remote: %v", err)
	}

	weftPrimary := filepath.Join(res.HubPath, "warp-weft")
	suffixed := fabricengine.WeftBranchName("main")

	// The ref must EXIST, not merely be the checked-out name: an unborn branch is checked out and
	// reports as current while refs/heads/<branch> resolves to nothing, which is exactly the state
	// that made every later fork fail.
	got := gitOutput(t, weftPrimary, "rev-parse", "--verify", "--quiet", "refs/heads/"+suffixed)
	if got == "" {
		t.Fatalf("refs/heads/%s does not resolve after a clone against an empty weft remote; the weft primary is on an unborn branch", suffixed)
	}

	// And the git operation every pair-creating verb performs against that branch must succeed.
	// This is Add's own step 8 (createWeftWorktree), reduced to the one git call that used to die on
	// `fatal: invalid reference: main-weft`; driving Add itself is impossible here, since the
	// repo-wide fabric.yaml is materialised by the CLI layer through configsync, which
	// fabricengine must never import.
	gitkit.MustRun(t, weftPrimary, "git", "worktree", "add", "-b",
		fabricengine.WeftBranchName("my-task"), filepath.Join(res.HubPath, "my-task-weft"), suffixed)
}

// TestCloneHub_NonEmptyWeftRemoteBranchUnchanged is the counter-test: the ordinary clone path must
// keep pairing the suffixed branch with the cloned HEAD rather than gaining an extra empty commit
// from the unborn-branch repair.
func TestCloneHub_NonEmptyWeftRemoteBranchUnchanged(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	warpRemote := makeBareRemote(t, dir, "warp")
	weftRemote := makeBareRemote(t, dir, "weft")

	cloneParent := t.TempDir()

	res, err := fabricengine.CloneHub(cloneParent, fabricengine.CloneOptions{
		WeftURL: weftRemote,
		WarpURL: warpRemote,
		// The weft fixture carries a commit but no .lyx-anchor, so the old-order bootstrap guard
		// would refuse it; this test is about the branch, not the guard.
		ForceBootstrap: true,
	})
	if err != nil {
		t.Fatalf("CloneHub: %v", err)
	}

	weftPrimary := filepath.Join(res.HubPath, "warp-weft")
	suffixed := fabricengine.WeftBranchName("main")

	suffixedSHA := gitOutput(t, weftPrimary, "rev-parse", "refs/heads/"+suffixed)
	clonedSHA := gitOutput(t, weftPrimary, "rev-parse", "refs/remotes/origin/main")
	if suffixedSHA != clonedSHA {
		t.Errorf("refs/heads/%s = %s; want the cloned HEAD %s — the unborn repair must not add a commit on a non-empty remote",
			suffixed, suffixedSHA, clonedSHA)
	}
}
