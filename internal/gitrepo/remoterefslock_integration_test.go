//go:build tier1

// remoterefslock_integration_test.go covers the remote-refs lock against real git repositories: one lock per repository, every fetch, pull and push method waiting on it, and concurrent fetches and pushes of one repository that all succeed under it.

package gitrepo_test

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/lock"
)

// remoteRefsLockPath returns where the remote-refs lock of the repository at dir lives.
func remoteRefsLockPath(t *testing.T, dir string) string {
	t.Helper()
	commonDir, err := gitrepo.New(dir).CommonDir()
	if err != nil {
		t.Fatalf("CommonDir(%s) error = %v", dir, err)
	}
	return filepath.Join(commonDir, gitrepo.RemoteRefsLockFileName)
}

// publishedClone returns a clone of a bare remote whose main has one commit, with the bare remote's path.
func publishedClone(t *testing.T) (clone, bareRemote string) {
	t.Helper()
	container := t.TempDir()
	bareRemote = newBareRemote(t, container)
	seedPath, seed := newRepoWithRemote(t, container, "seed", bareRemote)
	writeFile(t, seedPath, "seed.txt", "seed")
	commitAll(t, seedPath, "seed")
	if err := seed.Push(); err != nil {
		t.Fatalf("Push() of the seed error = %v", err)
	}
	clone, _ = cloneFromBare(t, container, "clone", bareRemote)
	return clone, bareRemote
}

func TestHoldRemoteRefs_OneLockServesEveryWorktreeOfARepository(t *testing.T) {
	t.Parallel()
	clone, _ := publishedClone(t)
	linked := filepath.Join(t.TempDir(), "linked")
	gitkit.MustRun(t, clone, "git", "worktree", "add", "-b", "linked", linked)

	release, err := gitrepo.New(linked).HoldRemoteRefs()
	if err != nil {
		t.Fatalf("HoldRemoteRefs() in the linked worktree error = %v", err)
	}
	path := remoteRefsLockPath(t, clone)
	probe, acquired, err := lock.TryAcquireWriteLock(path)
	if err != nil {
		t.Fatalf("TryAcquireWriteLock(%s) error = %v", path, err)
	}
	if acquired {
		_ = probe.Release()
		t.Errorf("the primary worktree's remote-refs lock was free while the linked worktree held it; want one lock per repository")
	}

	if err := release(); err != nil {
		t.Fatalf("release() error = %v", err)
	}
	probe, acquired, err = lock.TryAcquireWriteLock(path)
	if err != nil || !acquired {
		t.Fatalf("TryAcquireWriteLock(%s) after release = %v, %v; want the lock free", path, acquired, err)
	}
	_ = probe.Release()
}

// remoteRefsHeldWindow is how long a call must stay blocked behind a held remote-refs lock to count as waiting on it.
// A call that ignored the lock finishes a local git operation well inside it; a blocked call can never fail the check, so the window only bounds how fast a missing lock is caught.
const remoteRefsHeldWindow = 500 * time.Millisecond

func TestRemoteRefsLock_EveryFetchPullAndPushMethodWaitsWhileTheLockIsHeld(t *testing.T) {
	t.Parallel()
	absentSHA := strings.Repeat("a", 40)
	tests := []struct {
		name string
		call func(repo *gitrepo.Repo)
	}{
		{"Fetch", func(repo *gitrepo.Repo) { _ = repo.Fetch() }},
		{"FetchWithArgs", func(repo *gitrepo.Repo) { _, _ = repo.FetchWithArgs("origin") }},
		{"Pull", func(repo *gitrepo.Repo) { _ = repo.Pull() }},
		{"Push", func(repo *gitrepo.Repo) { _ = repo.Push() }},
		{"PushRebaseFree", func(repo *gitrepo.Repo) { _ = repo.PushRebaseFree() }},
		{"DeleteRemoteBranch", func(repo *gitrepo.Repo) { _, _ = repo.DeleteRemoteBranch("origin", "absent") }},
		{"DeleteRemoteBranchLeased", func(repo *gitrepo.Repo) { _ = repo.DeleteRemoteBranchLeased("origin", "absent", absentSHA) }},
		{"UpdateRemoteBranchLeased", func(repo *gitrepo.Repo) { _ = repo.UpdateRemoteBranchLeased("origin", "absent", absentSHA, absentSHA) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			clone, _ := publishedClone(t)
			repo := gitrepo.New(clone)
			release, err := repo.HoldRemoteRefs()
			if err != nil {
				t.Fatalf("HoldRemoteRefs() error = %v", err)
			}
			released := false
			t.Cleanup(func() {
				if !released {
					_ = release()
				}
			})

			done := make(chan struct{})
			go func() {
				defer close(done)
				tt.call(repo)
			}()
			select {
			case <-done:
				t.Fatalf("%s returned while the remote-refs lock was held; want it to wait", tt.name)
			case <-time.After(remoteRefsHeldWindow):
			}

			released = true
			if err := release(); err != nil {
				t.Fatalf("release() error = %v", err)
			}
			<-done
		})
	}
}

// TestRemoteRefsLock_ConcurrentFetchesAndPushesOfOneRepositoryAllSucceed reproduces two lym processes writing one repository's tracking refs at once.
// Without the lock, git fails the loser with "incorrect old value provided" or "cannot lock ref".
func TestRemoteRefsLock_ConcurrentFetchesAndPushesOfOneRepositoryAllSucceed(t *testing.T) {
	t.Parallel()
	clone, bareRemote := publishedClone(t)
	other := filepath.Join(t.TempDir(), "other")
	gitkit.MustRun(t, filepath.Dir(other), "git", "clone", "-b", "main", bareRemote, other)

	const writers = 4
	var pushers []*gitrepo.Repo
	for i := range writers {
		worktree := filepath.Join(t.TempDir(), fmt.Sprintf("pusher%d", i))
		gitkit.MustRun(t, clone, "git", "worktree", "add", "-b", fmt.Sprintf("branch%d", i), worktree)
		writeFile(t, worktree, "pushed.txt", worktree)
		commitAll(t, worktree, "pushed")
		pushers = append(pushers, gitrepo.New(worktree))
	}
	writeFile(t, other, "moved.txt", "moved")
	commitAll(t, other, "origin moved main")
	if err := gitrepo.New(other).Push(); err != nil {
		t.Fatalf("Push() from the other clone error = %v", err)
	}

	errs := make(chan error, 2*writers)
	var wait sync.WaitGroup
	for i := range writers {
		wait.Add(2)
		go func() {
			defer wait.Done()
			if err := gitrepo.New(clone).Fetch(); err != nil {
				errs <- fmt.Errorf("Fetch(): %w", err)
			}
		}()
		go func() {
			defer wait.Done()
			if err := pushers[i].PushRebaseFree(); err != nil {
				errs <- fmt.Errorf("PushRebaseFree() of branch%d: %w", i, err)
			}
		}()
	}
	wait.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("%v; want every concurrent fetch and push of one repository to succeed", err)
	}

	originMain := gitkit.Git(t, bareRemote, "rev-parse", "main")
	if got := gitkit.Git(t, clone, "rev-parse", "refs/remotes/origin/main"); got != originMain {
		t.Errorf("origin/main after the fetches = %q; want origin's main %q", got, originMain)
	}
}
