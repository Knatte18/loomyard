// bolt_integration_test.go — integration coverage for Bolt.Commit/Push/Sync, the unpaired weft:main handle.
// A dirty repo commits and pushes; SkipGit short-circuits before any git runs.
// Sync holds its single absorbing lock across a burst of steps.
// Each test builds a plain repo with a bare remote beside it, not nested in the worktree.
// A bare remote nested in the worktree would be staged by Bolt's wildcard commit.
// The package-wide TestMain in testmain_test.go provides HermeticGitEnv(), so this file adds none.

package fabricengine

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// newBoltBareRemote creates a bare git repository at <dir>/bolt-remote.git.
// It returns the path, ready to be added as an "origin" remote.
func newBoltBareRemote(t *testing.T, dir string) string {
	t.Helper()

	bare := filepath.Join(dir, "bolt-remote.git")
	if err := os.Mkdir(bare, 0o755); err != nil {
		t.Fatalf("mkdir bare remote: %v", err)
	}
	gitkit.MustRun(t, bare, "git", "init", "--bare", "-b", "main")
	return bare
}

// newBoltRepo creates a fresh, non-cloned git repository under dir/name on branch main.
// It configures bareRemote as "origin", with no upstream tracking branch yet.
// That is the state of a Bolt-backed checkout before its first sync.
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

// TestBolt_DirtyRepo_CommitsAndPushes asserts that Commit followed by Push lands a real commit,
// and that the bare origin advances to that same SHA.
// It also asserts that the commit leaves machine-local scratch under .lym untracked, as no seed excludes it.
//
//testtiming:keep machine-local scratch under .lym staying untracked by Bolt's wildcard commit while the pushed SHA lands on origin; coverage of its blocks by other tests does not show an assertion of this
func TestBolt_DirtyRepo_CommitsAndPushes(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	container := t.TempDir()
	bareRemote := newBoltBareRemote(t, container)
	repoPath := newBoltRepo(t, container, "bolt", bareRemote)
	if err := os.WriteFile(filepath.Join(repoPath, "bolt-note.md"), []byte("a note"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	logsDir := filepath.Join(repoPath, lymdirs.DotLymDirName, "logs")
	if err := os.MkdirAll(logsDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(logsDir, "trace.log"), []byte("a trace"), 0o644); err != nil {
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
	if got, want := gitkit.LsFiles(t, repoPath), []string{"bolt-note.md"}; !slices.Equal(got, want) {
		t.Errorf("git ls-files after Commit() = %q; want %q", got, want)
	}

	if err := b.Push(SyncOptions{}); err != nil {
		t.Fatalf("Push() error = %v; want nil", err)
	}
	if got := gitkit.RevParse(t, bareRemote, "main"); got != sha {
		t.Errorf("bare remote main = %q; want it advanced to %q", got, sha)
	}
}

// TestBolt_SkipGit_ShortCircuits asserts that opts.SkipGit stops Commit before any git is spawned,
// and that a dirty untracked file is left untouched.
func TestBolt_SkipGit_ShortCircuits(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

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

// TestBolt_CommitWritten asserts that CommitWritten commits only the paths write returns.
// Other dirty files are left for the next sync.
// It also asserts that a write error is returned as is, with HEAD unchanged.
// It also pins deletion staging and the omission of paths HEAD never held.
// It also pins an unborn HEAD, the empty path list and the SkipGit gate.
func TestBolt_CommitWritten(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	container := t.TempDir()
	bareRemote := newBoltBareRemote(t, container)
	repoPath := newBoltRepo(t, container, "bolt", bareRemote)
	b := NewBolt(repoPath)

	if err := os.WriteFile(filepath.Join(repoPath, "seed.md"), []byte("seed"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, _, err := b.Commit("bolt seed", SyncOptions{}); err != nil {
		t.Fatalf("Commit() (seed) error = %v; want nil", err)
	}

	t.Run("commits only the written paths", func(t *testing.T) {
		if err := os.WriteFile(filepath.Join(repoPath, "other.md"), []byte("other"), 0o644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}

		sha, committed, err := b.CommitWritten("bolt written", func() ([]string, error) {
			return []string{"written.md"}, os.WriteFile(filepath.Join(repoPath, "written.md"), []byte("written"), 0o644)
		}, SyncOptions{})
		if err != nil {
			t.Fatalf("CommitWritten() error = %v; want nil", err)
		}
		if !committed || sha == "" {
			t.Fatalf("CommitWritten() = (%q, %v); want a new commit", sha, committed)
		}

		status := gitStatusPorcelain(t, repoPath)
		if !strings.Contains(status, "other.md") || strings.Contains(status, "written.md") {
			t.Errorf("git status --porcelain = %q; want other.md still dirty and written.md committed", status)
		}
	})

	t.Run("write error leaves HEAD unchanged", func(t *testing.T) {
		headBefore := gitkit.RevParse(t, repoPath, "HEAD")
		writeErr := errors.New("refused write")

		sha, committed, err := b.CommitWritten("bolt refused", func() ([]string, error) {
			return []string{"other.md"}, writeErr
		}, SyncOptions{})
		if err != writeErr {
			t.Errorf("CommitWritten() error = %v; want the write error unwrapped", err)
		}
		if committed || sha != "" {
			t.Errorf("CommitWritten() = (%q, %v); want no commit", sha, committed)
		}
		if got := gitkit.RevParse(t, repoPath, "HEAD"); got != headBefore {
			t.Errorf("HEAD = %q; want it unchanged at %q", got, headBefore)
		}
	})

	t.Run("a deleted path is committed as a deletion and one HEAD never held is left out", func(t *testing.T) {
		sha, committed, err := b.CommitWritten("bolt deleted", func() ([]string, error) {
			return []string{"never.md", "seed.md"}, os.Remove(filepath.Join(repoPath, "seed.md"))
		}, SyncOptions{})
		if err != nil {
			t.Fatalf("CommitWritten() error = %v; want nil", err)
		}
		if !committed || sha == "" {
			t.Fatalf("CommitWritten() = (%q, %v); want a new commit", sha, committed)
		}
		if status := gitStatusPorcelain(t, repoPath); strings.Contains(status, "seed.md") {
			t.Errorf("git status --porcelain = %q; want the seed.md deletion committed", status)
		}
		if got := gitkit.Git(t, repoPath, "show", "--name-status", "--format=", "HEAD"); strings.TrimSpace(got) != "D\tseed.md" {
			t.Errorf("HEAD's changes = %q; want only the seed.md deletion", got)
		}
	})

	t.Run("a path neither on disk nor in HEAD commits nothing", func(t *testing.T) {
		headBefore := gitkit.RevParse(t, repoPath, "HEAD")

		sha, committed, err := b.CommitWritten("bolt never", func() ([]string, error) {
			return []string{"never.md"}, nil
		}, SyncOptions{})
		if err != nil || committed || sha != "" {
			t.Errorf("CommitWritten() = (%q, %v, %v); want no commit and no error", sha, committed, err)
		}
		if got := gitkit.RevParse(t, repoPath, "HEAD"); got != headBefore {
			t.Errorf("HEAD = %q; want it unchanged at %q", got, headBefore)
		}
	})

	t.Run("a repo with no commits stages a written file and leaves out a missing one", func(t *testing.T) {
		unborn := newBoltRepo(t, container, "unborn", bareRemote)

		sha, committed, err := NewBolt(unborn).CommitWritten("bolt first", func() ([]string, error) {
			return []string{"missing.md", "first.md"}, os.WriteFile(filepath.Join(unborn, "first.md"), []byte("first"), 0o644)
		}, SyncOptions{})
		if err != nil {
			t.Fatalf("CommitWritten() error = %v; want nil", err)
		}
		if !committed || sha == "" {
			t.Fatalf("CommitWritten() = (%q, %v); want a new commit", sha, committed)
		}
		if got := gitkit.Git(t, unborn, "show", "--name-status", "--format=", "HEAD"); strings.TrimSpace(got) != "A\tfirst.md" {
			t.Errorf("HEAD's changes = %q; want only first.md added", got)
		}
	})

	t.Run("an empty path list commits nothing", func(t *testing.T) {
		headBefore := gitkit.RevParse(t, repoPath, "HEAD")

		sha, committed, err := b.CommitWritten("bolt empty", func() ([]string, error) {
			return nil, nil
		}, SyncOptions{})
		if err != nil || committed || sha != "" {
			t.Errorf("CommitWritten() = (%q, %v, %v); want no commit and no error", sha, committed, err)
		}
		if got := gitkit.RevParse(t, repoPath, "HEAD"); got != headBefore {
			t.Errorf("HEAD = %q; want it unchanged at %q", got, headBefore)
		}
	})

	t.Run("SkipGit runs write and commits nothing", func(t *testing.T) {
		headBefore := gitkit.RevParse(t, repoPath, "HEAD")

		sha, committed, err := b.CommitWritten("bolt skipped", func() ([]string, error) {
			return []string{"skipped.md"}, os.WriteFile(filepath.Join(repoPath, "skipped.md"), []byte("skipped"), 0o644)
		}, SyncOptions{SkipGit: true})
		if err != nil || committed || sha != "" {
			t.Errorf("CommitWritten(SkipGit) = (%q, %v, %v); want no commit and no error", sha, committed, err)
		}
		if _, statErr := os.Stat(filepath.Join(repoPath, "skipped.md")); statErr != nil {
			t.Errorf("skipped.md was not written under SkipGit: %v", statErr)
		}
		if got := gitkit.RevParse(t, repoPath, "HEAD"); got != headBefore {
			t.Errorf("HEAD = %q; want it unchanged at %q", got, headBefore)
		}
	})
}

// TestBolt_Sync_HoldsSingleAbsorbingLockAcrossBurst asserts that Sync acquires its absorbing lock
// at board.push.lock, held once for the whole loop.
// An externally-held lock at that path blocks a concurrent Sync call until released.
func TestBolt_Sync_HoldsSingleAbsorbingLockAcrossBurst(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	container := t.TempDir()
	bareRemote := newBoltBareRemote(t, container)
	repoPath := newBoltRepo(t, container, "bolt", bareRemote)
	b := NewBolt(repoPath)

	pushLockPath, err := BoltLockPath(repoPath, BoardPushLockFile)
	if err != nil {
		t.Fatalf("BoltLockPath() error = %v", err)
	}
	held, err := lock.AcquireWriteLock(pushLockPath)
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
