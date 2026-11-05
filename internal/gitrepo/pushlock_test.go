// pushlock_test.go covers where the single-pusher lock of PushCoalesced sits: in the checkout's git dir, never in its working tree.

package gitrepo_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestPushLockPath_InGitDirAndCreatedThereByPushCoalesced(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	container := t.TempDir()
	bareRemote := newBareRemote(t, container)
	repoPath, repo := newRepoWithRemote(t, container, "clone", bareRemote)
	writeFile(t, repoPath, "a.txt", "initial")
	commitAll(t, repoPath, "init")
	if err := repo.Push(); err != nil {
		t.Fatalf("Push() (establish upstream) error = %v; want nil", err)
	}

	lockPath, err := repo.PushLockPath()
	if err != nil {
		t.Fatalf("PushLockPath() error = %v; want nil", err)
	}
	if got := filepath.Base(lockPath); got != gitrepo.PushLockFileName {
		t.Errorf("PushLockPath() base = %q; want %q", got, gitrepo.PushLockFileName)
	}
	wantDir, err := filepath.EvalSymlinks(filepath.Join(repoPath, ".git"))
	if err != nil {
		t.Fatalf("EvalSymlinks: %v", err)
	}
	gotDir, err := filepath.EvalSymlinks(filepath.Dir(lockPath))
	if err != nil {
		t.Fatalf("EvalSymlinks: %v", err)
	}
	if gotDir != wantDir {
		t.Errorf("PushLockPath() dir = %q; want the git dir %q", gotDir, wantDir)
	}

	writeFile(t, repoPath, "b.txt", "more")
	commitAll(t, repoPath, "more")
	if err := repo.PushCoalesced(); err != nil {
		t.Fatalf("PushCoalesced() error = %v; want nil", err)
	}
	if _, err := os.Stat(lockPath); err != nil {
		t.Errorf("Stat(%q) error = %v; want the push lock created in the git dir", lockPath, err)
	}
	if _, err := os.Stat(filepath.Join(repoPath, gitrepo.PushLockFileName)); !os.IsNotExist(err) {
		t.Errorf("Stat(working-tree %s) error = %v; want it absent", gitrepo.PushLockFileName, err)
	}
}
