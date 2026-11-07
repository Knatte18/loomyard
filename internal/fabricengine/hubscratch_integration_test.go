// hubscratch_integration_test.go pins the ordering CloneHub guarantees at its own boundary:
// the board worktree's weft-common-gitdir excludes carry a `.lym/` entry the instant CloneHub returns,
// and the board's stage-all commit never picks up anything planted inside the hub scratch tree.
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
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestCloneHub_SeedsBoardArtifactExcludesBeforeReturning asserts that at the instant
// fabricengine.CloneHub returns, the weft common gitdir's info/exclude file reached from
// res.BoardDir already carries the `.lym/` entry.
//
// It calls CloneHub directly: a later step that seeds the excludes would hide the omission this test checks.
// The exclude file is resolved with `git rev-parse --git-path info/exclude` run in res.BoardDir.
// The path is not assumed.
// The test checks the exclude entry itself, since a clean `git status` passes while the scratch directory is empty.
func TestCloneHub_SeedsBoardArtifactExcludesBeforeReturning(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	fixtures := t.TempDir()
	warpBare := makeBareRemote(t, fixtures, "hubscratch-order-warp")
	weftBare := makeBareRemote(t, fixtures, "hubscratch-order-weft")

	cloneParent := t.TempDir()
	// ForceBootstrap is true because weftBare is an ordinary seeded bare remote standing in for a weft.
	// It has never been a weft itself, so it carries no .lym-anchor.
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

	lines := gitkit.ExcludeLines(t, res.BoardDir)
	if !containsLine(lines, ".lym/") {
		t.Fatalf("weft common gitdir info/exclude reached from %s = %v; want it to already contain %q at the instant CloneHub returns", res.BoardDir, lines, ".lym/")
	}
}

// TestCloneHub_BoardStageAllCommitNeverStagesHubScratch checks that the board's stage-all commit omits hub scratch.
// After CloneHub returns, fabricengine.HubScratchDir(res.HubPath) exists as a real directory;
// a file planted inside it must be absent from the tree of the board's own stage-all commit.
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
