// clone_hubscratch_integration_test.go covers CloneHub's hub-scratch materialisation at the Bolt checkout's .lym.
// It needs real git repositories.

package fabricengine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// initTinyRepo initializes a minimal single-commit git repository at dir on branch "main".
// The repository works as a CloneHub source because cloneRepo accepts any git repo, not only a bare one.
func initTinyRepo(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	gitkit.Git(t, dir, "init", "-b", "main")
	gitkit.Git(t, dir, "config", "user.email", "test@test.com")
	gitkit.Git(t, dir, "config", "user.name", "Test")
	gitkit.CommitFile(t, dir, "README.md", "# "+filepath.Base(dir), "init")
}

// TestCloneHub_CreatesHubScratchDir asserts that CloneHub creates the hub scratch directory:
// HubScratchDir(res.HubPath), the Bolt checkout's .lym, a hub-wide ephemeral sibling of its _lym.
// It is created only after the board worktree exists, as a real directory, not a junction.
func TestCloneHub_CreatesHubScratchDir(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	fixtures := t.TempDir()
	warpSrc := filepath.Join(fixtures, "warp-src")
	weftSrc := filepath.Join(fixtures, "weft-src")
	initTinyRepo(t, warpSrc)
	initTinyRepo(t, weftSrc)

	cloneParent := t.TempDir()
	// ForceBootstrap is set because the weft fixture is a non-bare working repo built by initTinyRepo.
	// It stands in for a weft as an ordinary seeded repo, not one that has ever been a weft:
	// it carries no .lym-anchor and would otherwise trip the old-order guard.
	res, err := CloneHub(cloneParent, CloneOptions{
		WeftURL:        filepath.ToSlash(weftSrc),
		WarpURL:        filepath.ToSlash(warpSrc),
		Subpath:        ".",
		ForceBootstrap: true,
		Shortname:      "tst",
	})
	if err != nil {
		t.Fatalf("CloneHub() error = %v; want nil", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(res.HubPath) })

	scratchDir := HubScratchDir(res.HubPath)
	info, statErr := os.Stat(scratchDir)
	if statErr != nil {
		t.Fatalf("stat %s: %v; want CloneHub to have created it", scratchDir, statErr)
	}
	if !info.IsDir() {
		t.Errorf("%s exists but is not a directory", scratchDir)
	}
}
