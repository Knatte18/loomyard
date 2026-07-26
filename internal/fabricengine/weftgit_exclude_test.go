//go:build integration

// weftgit_exclude_test.go proves fabric's own lock artifacts (.weft/ from
// CommitWeft's write lock, .gitrepo-push.lock from PushCoalesced) never
// surface as untracked dirt in the weft worktree: ensureWeftLockDir seeds
// them into the weft repo's info/exclude, so Remove's no-force dirty gate
// (a raw `git status --porcelain`, untracked included) cannot dead-end on
// artifacts a pathspec-scoped `fabric sync` can never clear.
//
// Package fabricengine_test to construct a real *fabricengine.Fabric against
// isolated warp/weft fixtures; shares the TestMain in testmain_test.go.
// newWarpFixture/newFabricPair/writeWeftConfig/gitStatusPorcelain below are
// this file's own fixture helpers, relocated here from
// weftgit_differential_test.go before its deletion (this file was already
// its only other consumer).

package fabricengine_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/lyxtest"
)

// newWarpFixture creates a minimal, isolated git repo at t.TempDir() on
// branch main with one commit — everything fabricengine.New/CommitWeft need
// from a warp repo, without any of fabric's own topology wiring.
func newWarpFixture(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	lyxtest.MustRun(t, dir, "git", "init", "-q", "-b", "main")
	lyxtest.MustRun(t, dir, "git", "config", "user.email", "test@test.com")
	lyxtest.MustRun(t, dir, "git", "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(dir, "README"), []byte("warp"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	lyxtest.MustRun(t, dir, "git", "add", ".")
	lyxtest.MustRun(t, dir, "git", "commit", "-q", "-m", "init")
	return dir
}

// newFabricPair returns a *fabricengine.Fabric paired with a fresh
// newWarpFixture warp repo and a fresh lyxtest.CopyWeft weft fixture.
func newFabricPair(t *testing.T) (*fabricengine.Fabric, lyxtest.WeftFixture) {
	t.Helper()

	warpPath := newWarpFixture(t)
	weftFixture := lyxtest.CopyWeft(t)
	f, err := fabricengine.New(warpPath, weftFixture.WeftPath)
	if err != nil {
		t.Fatalf("fabricengine.New(%q, %q): %v", warpPath, weftFixture.WeftPath, err)
	}
	return f, weftFixture
}

// writeWeftConfig overwrites the tracked _lyx/config.yaml file both
// CopyWeft fixtures ship with, the standard way this file dirties a weft
// worktree's pathspec-covered content.
func writeWeftConfig(t *testing.T, weftPath, content string) {
	t.Helper()

	if err := os.WriteFile(filepath.Join(weftPath, "_lyx", "config.yaml"), []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

// gitStatusPorcelain returns `git status --porcelain`'s raw output for
// repoPath.
func gitStatusPorcelain(t *testing.T, repoPath string) string {
	t.Helper()

	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = repoPath
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git status --porcelain in %s: %v", repoPath, err)
	}
	return string(out)
}

// TestCommitWeft_LockArtifactsExcludedFromStatus commits scoped weft content
// (which creates the .weft lock dir) and drops a push lock file, then asserts
// neither artifact appears in `git status --porcelain` — the exact check
// Remove's no-force weft dirty gate runs.
func TestCommitWeft_LockArtifactsExcludedFromStatus(t *testing.T) {
	f, weftFixture := newFabricPair(t)
	writeWeftConfig(t, weftFixture.WeftPath, "modified for exclude test")

	if _, committed, err := f.CommitWeft([]string{"_lyx"}, fabricengine.DefaultCommitMessage, fabricengine.SyncOptions{}); err != nil {
		t.Fatalf("CommitWeft: %v", err)
	} else if !committed {
		t.Fatal("CommitWeft committed = false; want true")
	}

	// Precondition: the commit really did materialize the lock dir — the
	// artifact this test claims is excluded must actually exist on disk.
	if _, err := os.Stat(filepath.Join(weftFixture.WeftPath, ".weft")); err != nil {
		t.Fatalf(".weft lock dir missing after CommitWeft: %v", err)
	}

	// Materialize the push lock artifact the way an interrupted PushCoalesced
	// would leave it: a plain file at the worktree root.
	pushLock := filepath.Join(weftFixture.WeftPath, gitrepo.PushLockFileName)
	if err := os.WriteFile(pushLock, nil, 0o644); err != nil {
		t.Fatalf("write push lock artifact: %v", err)
	}

	status := gitStatusPorcelain(t, weftFixture.WeftPath)
	if strings.Contains(status, ".weft") {
		t.Errorf("git status --porcelain reports .weft as dirt: %q; want it git-excluded", status)
	}
	if strings.Contains(status, gitrepo.PushLockFileName) {
		t.Errorf("git status --porcelain reports %s as dirt: %q; want it git-excluded", gitrepo.PushLockFileName, status)
	}
}
