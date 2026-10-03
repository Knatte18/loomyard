//go:build integration

// clone_hubscratch_integration_test.go — CloneHub's hub-scratch materialisation at
// <hub>/_board/.lyx, which needs real git repositories.

package fabricengine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitkit"
)

// initTinyRepo initializes a minimal single-commit git repository at dir on branch "main", suitable
// as a CloneHub source: cloneRepo works against any git repo, not only a bare one.
func initTinyRepo(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	gitkit.MustRun(t, dir, "git", "init", "-b", "main")
	gitkit.MustRun(t, dir, "git", "config", "user.email", "test@test.com")
	gitkit.MustRun(t, dir, "git", "config", "user.name", "Test")
	readme := filepath.Join(dir, "README.md")
	if err := os.WriteFile(readme, []byte("# "+filepath.Base(dir)), 0o644); err != nil {
		t.Fatalf("write README: %v", err)
	}
	gitkit.MustRun(t, dir, "git", "add", "README.md")
	gitkit.MustRun(t, dir, "git", "commit", "-m", "init")
}

// TestCloneHub_CreatesHubScratchDir asserts that CloneHub's hub-materialisation step creates
// HubScratchDir(res.HubPath) (<hub>/_board/.lyx): the hub-wide ephemeral sibling of
// <hub>/_board/_lyx, created only after the board worktree exists, a real directory rather than a
// junction.
func TestCloneHub_CreatesHubScratchDir(t *testing.T) {
	fixtures := t.TempDir()
	warpSrc := filepath.Join(fixtures, "warp-src")
	weftSrc := filepath.Join(fixtures, "weft-src")
	initTinyRepo(t, warpSrc)
	initTinyRepo(t, weftSrc)

	cloneParent := t.TempDir()
	// ForceBootstrap: true — the weft fixture here is a non-bare working repo built by
	// initTinyRepo, an ordinary seeded repo standing in for a weft, not a repo that has ever
	// been one, so it carries no .lyx-anchor and would otherwise trip the old-order guard.
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
