//go:build integration

// hubscratch_integration_test.go pins card 2's ordering guarantee at CloneHub's own boundary: the
// board worktree's weft-common-gitdir excludes carry a `.lyx/` entry the instant CloneHub returns,
// and the board's stage-all commit path never picks up anything planted inside the hub scratch tree
// in between.
//
// Package fabricengine_test to reuse makeBareRemote (clone_adopt_test.go) and readExcludeLines
// (junction_pattern_integration_test.go) rather than re-declaring either; shares the single TestMain
// in testmain_test.go — no new TestMain is added here.

package fabricengine_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitexec"
)

// TestCloneHub_SeedsBoardArtifactExcludesBeforeReturning asserts that at the instant
// fabricengine.CloneHub returns, the weft common gitdir's info/exclude file reached from
// res.BoardDir already carries the `.lyx/` entry.
//
// This calls CloneHub directly, never fabriccli.CloneAndWire, whose later WireJunctionsWith seeding
// would mask the omission this test exists to catch.
// The exclude file is resolved via `git rev-parse --git-path info/exclude` run in res.BoardDir, the
// same resolution mutateGitExclude uses for a linked worktree, rather than assuming a path.
// A `git status`-is-clean assertion would not qualify: it passes either way while the scratch
// directory is empty, so this checks the exclude entry itself — the load-bearing assertion for
// card 2's ordering.
func TestCloneHub_SeedsBoardArtifactExcludesBeforeReturning(t *testing.T) {
	fixtures := t.TempDir()
	warpBare := makeBareRemote(t, fixtures, "hubscratch-order-warp")
	weftBare := makeBareRemote(t, fixtures, "hubscratch-order-weft")

	cloneParent := t.TempDir()
	// ForceBootstrap: true — weftBare is an ordinary seeded bare remote standing in for a weft, not
	// a repo that has ever been one, so it carries no .lyx-anchor.
	res, err := fabricengine.CloneHub(cloneParent, fabricengine.CloneOptions{
		WeftURL:        filepath.ToSlash(weftBare),
		WarpURL:        filepath.ToSlash(warpBare),
		Subpath:        ".",
		ForceBootstrap: true,
	})
	if err != nil {
		t.Fatalf("CloneHub() error = %v; want nil", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(res.HubPath) })

	lines := readWeftExcludeLinesForHubScratch(t, res.BoardDir)
	if !containsLine(lines, ".lyx/") {
		t.Fatalf("weft common gitdir info/exclude reached from %s = %v; want it to already contain %q at the instant CloneHub returns", res.BoardDir, lines, ".lyx/")
	}
}

// TestCloneHub_BoardStageAllCommitNeverStagesHubScratch is the failure card 2's ordering exists to
// prevent, stated directly: after CloneHub returns, fabricengine.HubScratchDir(res.HubPath) exists as
// a real directory; a file planted inside it must be absent from the tree of the board's own
// stage-all commit.
func TestCloneHub_BoardStageAllCommitNeverStagesHubScratch(t *testing.T) {
	fixtures := t.TempDir()
	warpBare := makeBareRemote(t, fixtures, "hubscratch-stage-warp")
	weftBare := makeBareRemote(t, fixtures, "hubscratch-stage-weft")

	cloneParent := t.TempDir()
	res, err := fabricengine.CloneHub(cloneParent, fabricengine.CloneOptions{
		WeftURL:        filepath.ToSlash(weftBare),
		WarpURL:        filepath.ToSlash(warpBare),
		Subpath:        ".",
		ForceBootstrap: true,
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

// readWeftExcludeLinesForHubScratch resolves and reads the weft common gitdir's info/exclude file
// reached from worktreePath, mirroring the resolution logic seedWeftArtifactExcludes uses (`git
// rev-parse --git-path info/exclude`, joined with worktreePath if relative) — the same approach
// dotlyxjunction_integration_test.go's readWeftExcludeLines takes for a weft worktree, named
// distinctly here to avoid colliding with that unexported package-level helper.
func readWeftExcludeLinesForHubScratch(t *testing.T, worktreePath string) []string {
	t.Helper()

	stdout, _, exitCode, err := gitexec.RunGit([]string{"rev-parse", "--git-path", "info/exclude"}, worktreePath)
	if err != nil || exitCode != 0 {
		t.Fatalf("git rev-parse --git-path info/exclude in %s failed: %v (exit %d)", worktreePath, err, exitCode)
	}

	excludePath := strings.TrimSpace(stdout)
	if !filepath.IsAbs(excludePath) {
		excludePath = filepath.Join(worktreePath, excludePath)
	}

	content, err := os.ReadFile(excludePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatalf("read exclude file: %v", err)
	}
	return strings.Split(string(content), "\n")
}
