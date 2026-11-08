// hubscratch_integration_test.go pins the board's stage-all commit after CloneHub.
// That commit never picks up anything planted inside the hub scratch tree.
//
// It is an external test package to reuse makeBareRemote from clone_adopt_test.go.
// It shares the single TestMain in testmain_test.go.

package fabricengine_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestCloneHub_BoardStageAllCommitNeverStagesHubScratch checks that the board's stage-all commit omits hub scratch.
// After CloneHub returns, fabricengine.HubScratchDir(res.HubPath) exists as a real directory;
// a file planted inside it must be absent from the tree of the board's own stage-all commit.
//
//testtiming:keep a file planted in the hub scratch tree never appearing in the tree of the board's stage-all commit; coverage of its blocks by other tests does not show an assertion of this
func TestCloneHub_BoardStageAllCommitNeverStagesHubScratch(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	fixtures := t.TempDir()
	warpBare := makeBareRemote(t, fixtures, "hubscratch-stage-warp")
	weftBare := makeBareRemote(t, fixtures, "hubscratch-stage-weft")

	cloneParent := t.TempDir()
	res, err := fabricengine.CloneHub(cloneParent, fabricengine.CloneOptions{
		WeftURL:        filepath.ToSlash(weftBare),
		WarpURL:        filepath.ToSlash(warpBare),
		Subpath:        ".",
		ForceBootstrap: true,
		Shortname:      "tst",
	})
	if err != nil {
		t.Fatalf("CloneHub() error = %v; want nil", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(res.HubPath) })

	scratchDir := fabricengine.HubScratchDir(res.HubPath)
	info, statErr := os.Stat(scratchDir)
	if statErr != nil {
		t.Fatalf("stat %s: %v; want CloneHub to have created it", scratchDir, statErr)
	}
	if !info.IsDir() {
		t.Fatalf("%s exists but is not a directory", scratchDir)
	}

	plantedName := "scratch-plant.txt"
	if err := os.WriteFile(filepath.Join(scratchDir, plantedName), []byte("never tracked"), 0o644); err != nil {
		t.Fatalf("plant file in hub scratch: %v", err)
	}

	sha, committed, err := fabricengine.NewBolt(res.BoardDir).Commit("test: board stage-all commit", fabricengine.SyncOptions{})
	if err != nil {
		t.Fatalf("Bolt.Commit: %v", err)
	}
	if !committed {
		t.Fatalf("Bolt.Commit() committed = false; want true (the board worktree should have at least the fresh clone's committed content to stage)")
	}

	stdout, _, exitCode, err := gitexec.RunGit([]string{"ls-tree", "-r", "--name-only", sha}, res.BoardDir)
	if err != nil || exitCode != 0 {
		t.Fatalf("git ls-tree -r --name-only %s in %s failed: %v (exit %d)", sha, res.BoardDir, err, exitCode)
	}
	if strings.Contains(stdout, plantedName) {
		t.Errorf("board commit %s tree contains %q; want the hub scratch tree never staged into the board's own commit:\n%s", sha, plantedName, stdout)
	}
}
