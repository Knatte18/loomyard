//go:build integration

// export_integration_test.go holds the git-spawning setup helpers behind export_test.go's seams:
// they spawn git through gitkit, so they live in a tagged file and every caller is tagged too.

package fabricengine

import (
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
)

// NewPlainWarpRepoForTest re-exports newPlainWarpRepo (relocated fixture helper, formerly
// index_integration_test.go): commitweftat_test.go (package fabricengine, never migrating) calls it
// unqualified, and several of the nine relocating files need it before
// index_integration_test.go's own migration card lands.
var NewPlainWarpRepoForTest = newPlainWarpRepo

// newPlainWarpRepo creates a minimal, isolated git repo at t.TempDir() on branch main with one
// commit — everything RecordCorrespondence/warpSeq needs from a warp repo, without any of fabric's
// own topology wiring (junctions, weft pairing), which the callers of this helper do not exercise.
func newPlainWarpRepo(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	gitkit.Git(t, dir, "init", "-q", "-b", "main")
	gitkit.Git(t, dir, "config", "user.email", "test@test.com")
	gitkit.Git(t, dir, "config", "user.name", "Test")
	gitkit.CommitFile(t, dir, "README", "warp", "init")
	return dir
}

// NewCommitFixtureForTest re-exports newCommitFixture (relocated fixture helper, formerly
// commit_integration_test.go): commit_gating_integration_test.go, committed_lyxonly_integration_test.go,
// commit_partial_integration_test.go and commit_lock_integration_test.go (package fabricengine, never
// migrating) call it unqualified; the nine relocating files call the exported form.
var NewCommitFixtureForTest = newCommitFixture

// newCommitFixture builds a fresh warp/weft pair with the fabric config seeded, returning the Fabric
// handle and both repo paths.
func newCommitFixture(t *testing.T) (f *Fabric, warpPath, weftPath string) {
	t.Helper()

	warpPath = newPlainWarpRepo(t)
	weftPath = newPlainWeftRepo(t)
	seedFabricConfig(t, warpPath)
	f = newFabric(t, warpPath, weftPath)
	return f, warpPath, weftPath
}

// newPlainWeftRepo creates a minimal, isolated git repo at t.TempDir() on branch main with a single
// tracked _lyx/config.yaml file and one commit — the weft-side sibling of newPlainWarpRepo, replacing
// gitkit's own retired weft-only template for newCommitFixture's four in-package callers, which
// cannot import the hubforge package.
func newPlainWeftRepo(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	gitkit.Git(t, dir, "init", "-q", "-b", "main")
	gitkit.Git(t, dir, "config", "user.email", "test@test.com")
	gitkit.Git(t, dir, "config", "user.name", "Test")
	gitkit.CommitFile(t, dir, filepath.Join(lyxdirs.LyxDirName, "config.yaml"), "test", "init")
	return dir
}

// PushLockPathForTest returns the absorbing push lock file under weftPath, creating its directory,
// so a test can hold the lock a push contends on.
func PushLockPathForTest(t *testing.T, weftPath string) string {
	t.Helper()

	lockDir, err := ensureWeftLockDirAt(weftPath)
	if err != nil {
		t.Fatalf("ensureWeftLockDirAt(%s): %v", weftPath, err)
	}
	return filepath.Join(lockDir, weftPushLockFile)
}
