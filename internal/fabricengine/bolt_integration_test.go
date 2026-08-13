//go:build integration

// bolt_integration_test.go — integration coverage for Bolt.Commit/Push/Sync,
// the unpaired weft:main handle: a dirty repo commits and pushes, a clean
// repo is a true no-op, SkipGit short-circuits before any git runs, and
// Sync holds its single absorbing lock across a burst of steps. Package
// fabricengine (internal), building its own plain-repo-plus-bare-remote
// fixture locally (mirroring boardengine/sync_integration_test.go's
// newBoardRepo/newBareRemote shape) rather than the package's own
// newPlainWeftRepo, whose fixture carries its bare remote nested inside the
// copied worktree — fine for that fixture's own explicit-pathspec callers, but not for Bolt's
// wildcard-stage commit, which would otherwise stage that untracked
// directory and defeat the clean-repo no-op assertion. Reuses this
// package's own bareBranchSHA helper (defined in
// coalesce_integration_test.go). Relies on testmain_test.go's package-wide
// TestMain for HermeticGitEnv(); no new TestMain is added here.

package fabricengine

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/lock"
)

// newBoltBareRemote creates a bare git repository at <dir>/bolt-remote.git
// and returns its path, ready to be added as an "origin" remote.
func newBoltBareRemote(t *testing.T, dir string) string {
	t.Helper()

	bare := filepath.Join(dir, "bolt-remote.git")
	if err := os.Mkdir(bare, 0o755); err != nil {
		t.Fatalf("mkdir bare remote: %v", err)
	}
	gitkit.MustRun(t, bare, "git", "init", "--bare", "-b", "main")
	return bare
}

// newBoltRepo creates a fresh (non-cloned) git repository under dir/name on
// branch main, with bareRemote configured as "origin" but no upstream
// tracking branch yet — the state a real Bolt-backed checkout is in before
// its very first sync.
func newBoltRepo(t *testing.T, dir, name, bareRemote string) string {
	t.Helper()

	path := filepath.Join(dir, name)
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", name, err)
	}
	gitkit.MustRun(t, path, "git", "init", "-b", "main")
	gitkit.MustRun(t, path, "git", "remote", "add", "origin", bareRemote)
	return path
}

// TestBolt_DirtyRepo_CommitsAndPushes asserts that Commit followed by Push lands a real commit and
// advances the bare origin to that same SHA.
func TestBolt_DirtyRepo_CommitsAndPushes(t *testing.T) {
	container := t.TempDir()
	bareRemote := newBoltBareRemote(t, container)
	repoPath := newBoltRepo(t, container, "bolt", bareRemote)
	if err := os.WriteFile(filepath.Join(repoPath, "bolt-note.md"), []byte("a note"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	b := NewBolt(repoPath)

	sha, committed, err := b.Commit("bolt sync", SyncOptions{})
	if err != nil {
		t.Fatalf("Commit() error = %v; want nil", err)
	}
	if !committed {
		t.Fatalf("Commit() committed = false; want true")
	}
	if sha == "" {
		t.Fatalf("Commit() sha = %q; want a non-empty new HEAD SHA", sha)
	}

	if err := b.Push(SyncOptions{}); err != nil {
		t.Fatalf("Push() error = %v; want nil", err)
	}
	if got := bareBranchSHA(t, bareRemote, "main"); got != sha {
		t.Errorf("bare remote main = %q; want it advanced to %q", got, sha)
	}
}

// TestBolt_CleanRepo_CommitAndPushAreNoOps asserts that Commit reports committed=false on a repo
// with nothing new to stage,
// and a subsequent Push is a true no-op (never touches the network, never errors) once nothing is
// ahead of upstream.
func TestBolt_CleanRepo_CommitAndPushAreNoOps(t *testing.T) {
	container := t.TempDir()
	bareRemote := newBoltBareRemote(t, container)
	repoPath := newBoltRepo(t, container, "bolt", bareRemote)

	b := NewBolt(repoPath)

	// A first real commit establishes a HEAD to be clean against; the
	// second, no-change call below is what this test actually asserts.
	if err := os.WriteFile(filepath.Join(repoPath, "seed.md"), []byte("seed"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, committed, err := b.Commit("bolt sync init", SyncOptions{}); err != nil {
		t.Fatalf("Commit() (seed) error = %v; want nil", err)
	} else if !committed {
		t.Fatalf("Commit() (seed) committed = false; want true")
	}
	if err := b.Push(SyncOptions{}); err != nil {
		t.Fatalf("Push() (seed) error = %v; want nil", err)
	}
	// PushCoalesced leaves its own untracked .gitrepo-push.lock at the repo
	// root once it has run; a real board/clone checkout never has this test's
	// bare "no .gitignore at all" shape, but a plain unseeded fixture does, so
	// clear it here rather than have Bolt's own wildcard-stage commit sweep it
	// up and falsely report the next call as a real commit.
	if err := os.Remove(filepath.Join(repoPath, gitrepo.PushLockFileName)); err != nil && !os.IsNotExist(err) {
		t.Fatalf("remove push lock artifact: %v", err)
	}

	sha, committed, err := b.Commit("bolt sync", SyncOptions{})
	if err != nil {
		t.Fatalf("Commit() error = %v; want nil", err)
	}
	if committed {
		t.Fatalf("Commit() committed = true; want false (nothing changed)")
	}
	if sha != "" {
		t.Errorf("Commit() sha = %q; want empty", sha)
	}

	if err := b.Push(SyncOptions{}); err != nil {
		t.Fatalf("Push() error = %v; want nil (nothing to push)", err)
	}
}

// TestBolt_SkipGit_ShortCircuits asserts that opts.SkipGit stops Commit before any git is spawned,
// leaving a dirty untracked file untouched.
func TestBolt_SkipGit_ShortCircuits(t *testing.T) {
	container := t.TempDir()
	bareRemote := newBoltBareRemote(t, container)
	repoPath := newBoltRepo(t, container, "bolt", bareRemote)
	if err := os.WriteFile(filepath.Join(repoPath, "bolt-note.md"), []byte("a note"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	b := NewBolt(repoPath)

	sha, committed, err := b.Commit("bolt sync", SyncOptions{SkipGit: true})
	if err != nil {
		t.Fatalf("Commit(SkipGit) error = %v; want nil", err)
	}
	if committed {
		t.Fatalf("Commit(SkipGit) committed = true; want false")
	}
	if sha != "" {
		t.Errorf("Commit(SkipGit) sha = %q; want empty", sha)
	}

	if status := gitStatusPorcelain(t, repoPath); status == "" {
		t.Errorf("git status --porcelain = %q; want non-empty (the untracked file must remain uncommitted)", status)
	}
}

// TestBolt_Sync_HoldsSingleAbsorbingLockAcrossBurst asserts that Sync acquires its absorbing lock
// at the same board.push.lock path Bolt.Sync documents, held once for the whole loop: an
// externally-held lock at that exact path blocks a concurrent Sync call until released.
func TestBolt_Sync_HoldsSingleAbsorbingLockAcrossBurst(t *testing.T) {
	container := t.TempDir()
	bareRemote := newBoltBareRemote(t, container)
	repoPath := newBoltRepo(t, container, "bolt", bareRemote)
	b := NewBolt(repoPath)

	held, err := lock.AcquireWriteLock(filepath.Join(repoPath, "board.push.lock"))
	if err != nil {
		t.Fatalf("AcquireWriteLock() error = %v", err)
	}

	calls := 0
	done := make(chan error, 1)
	go func() {
		done <- b.Sync(func() (bool, error) {
			calls++
			return false, nil
		})
	}()

	select {
	case err := <-done:
		_ = held.Release()
		t.Fatalf("Sync() returned (err=%v) while board.push.lock was externally held; want it to block", err)
	case <-time.After(200 * time.Millisecond):
		// Expected: Sync is blocked behind the held lock.
	}

	if err := held.Release(); err != nil {
		t.Fatalf("Release() error = %v", err)
	}

	if err := <-done; err != nil {
		t.Fatalf("Sync() error = %v; want nil once the external hold releases", err)
	}
	if calls != 1 {
		t.Errorf("step call count = %d; want 1 (step reported no progress on its first call)", calls)
	}
}
