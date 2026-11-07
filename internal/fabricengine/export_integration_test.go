// export_integration_test.go holds the git-spawning setup helpers behind export_test.go's seams.
// They spawn git through gitkit, so this file and every caller of them are tagged.

package fabricengine

import (
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/lymdirs"
)

// NewPlainWarpRepoForTest creates a minimal, isolated warp git repo for tests in package fabricengine_test.
var NewPlainWarpRepoForTest = newPlainWarpRepo

// newPlainWarpRepo creates a minimal, isolated git repo at t.TempDir() on branch main with one commit.
// The repo has no fabric topology wiring (junctions, weft pairing); the callers of this helper do not exercise it.
func newPlainWarpRepo(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	gitkit.Git(t, dir, "init", "-q", "-b", "main")
	gitkit.Git(t, dir, "config", "user.email", "test@test.com")
	gitkit.Git(t, dir, "config", "user.name", "Test")
	gitkit.CommitFile(t, dir, "README", "warp", "init")
	return dir
}

// NewCommitFixtureForTest builds a fresh warp/weft pair with the fabric config seeded.
// Tests in package fabricengine_test call it, since they cannot reach package fabricengine's unexported helpers.
var NewCommitFixtureForTest = newCommitFixture

// newCommitFixture builds a fresh warp/weft pair with the fabric config seeded.
// It returns the Fabric handle and both repo paths.
func newCommitFixture(t *testing.T) (f *Fabric, warpPath, weftPath string) {
	t.Helper()

	warpPath = newPlainWarpRepo(t)
	weftPath = newPlainWeftRepo(t)
	seedFabricConfig(t, warpPath)
	f = newFabric(t, warpPath, weftPath)
	return f, warpPath, weftPath
}

// newPlainWeftRepo creates a minimal, isolated git repo at t.TempDir() on branch main.
// Its one commit tracks a single _lym/config.yaml file.
// The in-package callers of newCommitFixture cannot import the hubforge package, so they need this helper.
func newPlainWeftRepo(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	gitkit.Git(t, dir, "init", "-q", "-b", "main")
	gitkit.Git(t, dir, "config", "user.email", "test@test.com")
	gitkit.Git(t, dir, "config", "user.name", "Test")
	gitkit.CommitFile(t, dir, filepath.Join(lymdirs.LymDirName, "config.yaml"), "test", "init")
	return dir
}

// PushLockPathForTest returns the absorbing push lock file under weftPath, creating its directory.
// A test holds that lock to contend with a push.
func PushLockPathForTest(t *testing.T, weftPath string) string {
	t.Helper()

	lockDir, err := ensureWeftLockDirAt(weftPath)
	if err != nil {
		t.Fatalf("ensureWeftLockDirAt(%s): %v", weftPath, err)
	}
	return filepath.Join(lockDir, weftPushLockFile)
}
