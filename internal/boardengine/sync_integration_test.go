//go:build integration

// sync_integration_test.go covers boardengine.Sync's parity after delegating
// its absorbing-lock coalescing loop to fabricengine.CoalescePush: a dirty
// board still commits and pushes, .gitignore is still seeded with the
// lock/manifest patterns, the absorbing push lock still lives at the
// unchanged board.push.lock path, and skipPush still commits locally without
// advancing the remote. The board+bare-origin fixture is built inline via
// lyxtest.MustRun, mirroring the fixture shape of
// internal/gitrepo/push_test.go's newBareRemote/newRepoWithRemote —
// reimplemented locally since those helpers live in a different test
// package (gitrepo_test) and are not importable across that boundary.

package boardengine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/lyxtest"
)

// newBareRemote creates a bare git repository for testing.
func newBareRemote(t *testing.T, dir string) string {
	t.Helper()

	bare := filepath.Join(dir, "remote.git")
	if err := os.Mkdir(bare, 0o755); err != nil {
		t.Fatalf("mkdir bare remote: %v", err)
	}
	lyxtest.MustRun(t, bare, "git", "init", "--bare", "-b", "main")
	return bare
}

// newBoardRepo creates a fresh git repository in state before its very first sync.
func newBoardRepo(t *testing.T, dir, name, bareRemote string) string {
	t.Helper()

	path := filepath.Join(dir, name)
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", name, err)
	}
	lyxtest.MustRun(t, path, "git", "init", "-b", "main")
	lyxtest.MustRun(t, path, "git", "remote", "add", "origin", bareRemote)
	return path
}

// writeBoardFile creates or overwrites a file to dirty the working tree for Sync.
func writeBoardFile(t *testing.T, dir, name, content string) {
	t.Helper()

	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

// bareRemoteHead returns the bare remote's main branch SHA, or "" if nothing was pushed.
func bareRemoteHead(t *testing.T, bareRemote string) string {
	t.Helper()

	stdout, _, code, err := gitexec.RunGit([]string{"rev-parse", "main"}, bareRemote)
	if err != nil {
		t.Fatalf("git rev-parse main (bare) error = %v", err)
	}
	if code != 0 {
		return ""
	}
	return strings.TrimSpace(stdout)
}

// TestSync_DirtyBoard_CommitsAndPushes asserts a dirty board commits and pushes.
func TestSync_DirtyBoard_CommitsAndPushes(t *testing.T) {
	container := t.TempDir()
	bareRemote := newBareRemote(t, container)
	boardPath := newBoardRepo(t, container, "board", bareRemote)

	writeBoardFile(t, boardPath, "tasks.json", `{"tasks":[]}`)

	if err := Sync(boardPath, false, false); err != nil {
		t.Fatalf("Sync() error = %v; want nil", err)
	}

	if got := bareRemoteHead(t, bareRemote); got == "" {
		t.Fatal("bare remote main after Sync() = \"\"; want the dirty commit pushed")
	}
}

// TestSync_SeedsGitignoreWithLockAndManifestPatterns asserts Sync seeds .gitignore with lock and
// manifest patterns.
func TestSync_SeedsGitignoreWithLockAndManifestPatterns(t *testing.T) {
	container := t.TempDir()
	bareRemote := newBareRemote(t, container)
	boardPath := newBoardRepo(t, container, "board", bareRemote)

	writeBoardFile(t, boardPath, "tasks.json", `{"tasks":[]}`)

	if err := Sync(boardPath, false, false); err != nil {
		t.Fatalf("Sync() error = %v; want nil", err)
	}

	got, err := os.ReadFile(filepath.Join(boardPath, ".gitignore"))
	if err != nil {
		t.Fatalf("read .gitignore: %v", err)
	}
	for _, pat := range []string{"*.lock", "*.swaplock", renderManifestFile} {
		if !strings.Contains(string(got), pat) {
			t.Errorf(".gitignore = %q; want it to contain %q", got, pat)
		}
	}
}

// TestSync_ConcurrentCallSerializesOnBoardPushLock asserts concurrent Sync calls serialize on
// board.push.lock.
func TestSync_ConcurrentCallSerializesOnBoardPushLock(t *testing.T) {
	container := t.TempDir()
	bareRemote := newBareRemote(t, container)
	boardPath := newBoardRepo(t, container, "board", bareRemote)

	writeBoardFile(t, boardPath, "tasks.json", `{"tasks":[]}`)
	if err := Sync(boardPath, false, false); err != nil {
		t.Fatalf("Sync() (seed) error = %v; want nil", err)
	}

	held, err := lock.AcquireWriteLock(filepath.Join(boardPath, "board.push.lock"))
	if err != nil {
		t.Fatalf("AcquireWriteLock() error = %v", err)
	}

	done := make(chan error, 1)
	go func() {
		writeBoardFile(t, boardPath, "tasks.json", `{"tasks":["one"]}`)
		done <- Sync(boardPath, false, false)
	}()

	select {
	case err := <-done:
		_ = held.Release()
		t.Fatalf("Sync() returned (err=%v) while board.push.lock was externally held; want it to block", err)
	case <-time.After(200 * time.Millisecond):
	}

	if err := held.Release(); err != nil {
		t.Fatalf("Release() error = %v", err)
	}

	if err := <-done; err != nil {
		t.Fatalf("Sync() error = %v; want nil once the external hold releases", err)
	}
}

// TestSync_SkipPush_CommitsLocallyButDoesNotPush asserts skipPush commits locally but doesn't push.
func TestSync_SkipPush_CommitsLocallyButDoesNotPush(t *testing.T) {
	container := t.TempDir()
	bareRemote := newBareRemote(t, container)
	boardPath := newBoardRepo(t, container, "board", bareRemote)

	writeBoardFile(t, boardPath, "tasks.json", `{"tasks":[]}`)

	if err := Sync(boardPath, false, true); err != nil {
		t.Fatalf("Sync() (skipPush) error = %v; want nil", err)
	}

	if got := bareRemoteHead(t, bareRemote); got != "" {
		t.Errorf("bare remote main after skipPush Sync() = %q; want \"\" (nothing pushed)", got)
	}

	stdout, stderr, code, err := gitexec.RunGit([]string{"log", "--oneline"}, boardPath)
	if err != nil {
		t.Fatalf("git log error = %v", err)
	}
	if code != 0 {
		t.Fatalf("git log exited %d: %s", code, stderr)
	}
	if strings.TrimSpace(stdout) == "" {
		t.Fatal("git log --oneline after skipPush Sync() = \"\"; want a local commit")
	}
}
