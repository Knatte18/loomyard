// remoterefslock_integration_test.go covers the remote-refs lock against real git repositories: one lock per repository, the pause gate passed before it is taken, and every fetch and pull method waiting on it.
// It also covers every push method transferring outside the lock and writing its tracking ref only under it, and concurrent fetches and pushes of one repository that all succeed.

package gitrepo_test

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
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
	tiergate.Require(t, 1)
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

// TestHoldRemoteRefs_PassesThePauseGateBeforeTheLockAndStaysAdmittedUntilTheRelease is not parallel: it installs the process-wide network gate.
func TestHoldRemoteRefs_PassesThePauseGateBeforeTheLockAndStaysAdmittedUntilTheRelease(t *testing.T) {
	tiergate.Require(t, 1)
	clone, _ := publishedClone(t)
	lockPath := remoteRefsLockPath(t, clone)
	recordPath := filepath.Join(t.TempDir(), pauserecord.RecordFileName)
	pausedAt := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	setLive := func(live ...pauserecord.Entry) {
		t.Helper()
		if _, err := pauserecord.Update(recordPath, pausedAt, func(record *pauserecord.Record) error {
			record.Live = live
			return nil
		}); err != nil {
			t.Fatalf("pauserecord.Update: %v", err)
		}
	}
	setLive(pauserecord.Entry{Scope: pauserecord.ScopeHub, PausedAt: pausedAt})
	waits := 0
	t.Cleanup(pauserecord.InstallGate(pauserecord.Gate{
		Geometry: pauserecord.Geometry{RecordPath: recordPath, Slug: "a"},
		Notify:   func(string) {},
		Sleep: func(context.Context, time.Duration) error {
			waits++
			probe, acquired, err := lock.TryAcquireWriteLock(lockPath)
			if err != nil || !acquired {
				t.Errorf("TryAcquireWriteLock while held at the gate = %v, %v; want the lock free until the gate is passed", acquired, err)
			} else {
				_ = probe.Release()
			}
			setLive()
			return nil
		},
	}))

	release, err := gitrepo.New(clone).HoldRemoteRefs()
	if err != nil {
		t.Fatalf("HoldRemoteRefs() error = %v", err)
	}
	if waits != 1 {
		t.Fatalf("gate waits before the lock = %d; want 1", waits)
	}
	setLive(pauserecord.Entry{Scope: pauserecord.ScopeHub, PausedAt: pausedAt.Add(time.Minute)})
	_, done := pauserecord.Network("fetch")
	done()
	if waits != 1 {
		t.Errorf("gate waits for a call under the lock = %d; want the holder's calls to pass at once", waits-1)
	}

	if err := release(); err != nil {
		t.Fatalf("release() error = %v", err)
	}
	_, done = pauserecord.Network("fetch")
	done()
	if waits != 2 {
		t.Errorf("gate waits after the release = %d; want the next call held again", waits)
	}
}

// remoteRefsHeldWindow is how long a call must stay blocked behind a held remote-refs lock to count as waiting on it.
// A call that ignored the lock finishes a local git operation well inside it; a blocked call can never fail the check, so the window only bounds how fast a missing lock is caught.
const remoteRefsHeldWindow = 500 * time.Millisecond

func TestRemoteRefsLock_EveryFetchAndPullMethodWaitsWhileTheLockIsHeld(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()
	tests := []struct {
		name string
		call func(repo *gitrepo.Repo)
	}{
		{"Fetch", func(repo *gitrepo.Repo) { _ = repo.Fetch() }},
		{"FetchWithArgs", func(repo *gitrepo.Repo) { _, _ = repo.FetchWithArgs("origin") }},
		{"Pull", func(repo *gitrepo.Repo) { _ = repo.Pull() }},
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

// originLandedWithin is how long a push transfer may take to show on the bare remote while the remote-refs lock is held.
const originLandedWithin = 30 * time.Second

// originLandedTick is how often the bare remote is read while waiting for a push transfer to land.
const originLandedTick = 20 * time.Millisecond

// branchSHA returns the commit ref names in the repository at dir, or "" when the ref does not exist.
func branchSHA(t *testing.T, dir, ref string) string {
	t.Helper()
	stdout, _, code, err := runGit(t, dir, "rev-parse", "--verify", "--quiet", ref)
	if err != nil {
		t.Fatalf("git rev-parse %s in %s error = %v", ref, dir, err)
	}
	if code != 0 {
		return ""
	}
	return strings.TrimSpace(stdout)
}

// pushUnderHeldLock holds the clone's remote-refs lock, starts call, and waits until the bare remote's branch reads want, failing if call returns first.
// It returns the function that releases the lock and then waits for call's error.
func pushUnderHeldLock(t *testing.T, clone, bareRemote, branch, want string, call func() error) (releaseAndWait func() error) {
	t.Helper()
	release, err := gitrepo.New(clone).HoldRemoteRefs()
	if err != nil {
		t.Fatalf("HoldRemoteRefs() error = %v", err)
	}
	released := false
	t.Cleanup(func() {
		if !released {
			_ = release()
		}
	})

	var callErr error
	done := make(chan struct{})
	go func() {
		defer close(done)
		callErr = call()
	}()
	deadline := time.Now().Add(originLandedWithin)
	for branchSHA(t, bareRemote, "refs/heads/"+branch) != want {
		select {
		case <-done:
			t.Fatalf("the push returned with error %v before origin's %s read %q; want its transfer to land while the lock is held", callErr, branch, want)
		default:
		}
		if time.Now().After(deadline) {
			t.Fatalf("origin's %s never read %q while the remote-refs lock was held; want the transfer to run outside the lock", branch, want)
		}
		time.Sleep(originLandedTick)
	}
	select {
	case <-done:
		t.Fatalf("the push returned with error %v while the remote-refs lock was held; want its tracking-ref write to wait for the lock", callErr)
	case <-time.After(remoteRefsHeldWindow):
	}

	return func() error {
		released = true
		if err := release(); err != nil {
			t.Fatalf("release() error = %v", err)
		}
		<-done
		return callErr
	}
}

func TestRemoteRefsLock_APushTransfersWhileTheLockIsHeldAndWritesItsTrackingRefOnceReleased(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()
	tests := []struct {
		name   string
		branch string
		// prepare readies the clone and returns the push to run and the commit origin's branch and its tracking ref hold afterwards, empty for a deleted branch.
		prepare      func(t *testing.T, clone string, repo *gitrepo.Repo) (call func() error, want string)
		wantUpstream string
	}{
		{"PushRebaseFree of a first push, which also sets the upstream", "fresh", func(t *testing.T, clone string, repo *gitrepo.Repo) (func() error, string) {
			gitkit.Git(t, clone, "checkout", "-b", "fresh")
			return repo.PushRebaseFree, gitkit.CommitFile(t, clone, "fresh.txt", "fresh", "fresh commit")
		}, "origin/fresh"},
		{"PushBranch", "main", func(t *testing.T, clone string, repo *gitrepo.Repo) (func() error, string) {
			want := gitkit.CommitFile(t, clone, "next.txt", "next", "next commit")
			return func() error { return repo.PushBranch("origin", "main") }, want
		}, "origin/main"},
		{"UpdateRemoteBranchLeased", "main", func(t *testing.T, clone string, repo *gitrepo.Repo) (func() error, string) {
			first := gitkit.RevParse(t, clone, "HEAD")
			second := gitkit.CommitFile(t, clone, "second.txt", "second", "second commit")
			gitkit.Git(t, clone, "push", "origin", "main")
			return func() error { return repo.UpdateRemoteBranchLeased("origin", "main", first, second) }, first
		}, "origin/main"},
		{"DeleteRemoteBranchLeased", "doomed", func(t *testing.T, clone string, repo *gitrepo.Repo) (func() error, string) {
			gitkit.Git(t, clone, "push", "origin", "main:doomed")
			tip := gitkit.RevParse(t, clone, "HEAD")
			return func() error { return repo.DeleteRemoteBranchLeased("origin", "doomed", tip) }, ""
		}, ""},
		{"DeleteRemoteBranch", "doomed", func(t *testing.T, clone string, repo *gitrepo.Repo) (func() error, string) {
			gitkit.Git(t, clone, "push", "origin", "main:doomed")
			return func() error {
				deleted, err := repo.DeleteRemoteBranch("origin", "doomed")
				if err == nil && !deleted {
					return fmt.Errorf("DeleteRemoteBranch() deleted = false; want true")
				}
				return err
			}, ""
		}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			clone, bareRemote := publishedClone(t)
			call, want := tt.prepare(t, clone, gitrepo.New(clone))
			trackingRef := "refs/remotes/origin/" + tt.branch
			trackingBefore := branchSHA(t, clone, trackingRef)
			upstreamBefore := upstreamRef(t, clone)

			releaseAndWait := pushUnderHeldLock(t, clone, bareRemote, tt.branch, want, call)
			if got := branchSHA(t, clone, trackingRef); got != trackingBefore {
				t.Errorf("%s while the remote-refs lock was held = %q; want it unchanged at %q", trackingRef, got, trackingBefore)
			}
			if got := upstreamRef(t, clone); got != upstreamBefore {
				t.Errorf("upstream while the remote-refs lock was held = %q; want it unchanged at %q", got, upstreamBefore)
			}

			if err := releaseAndWait(); err != nil {
				t.Fatalf("%s error = %v; want nil", tt.name, err)
			}
			if got := branchSHA(t, clone, trackingRef); got != want {
				t.Errorf("%s after the release = %q; want %q", trackingRef, got, want)
			}
			if tt.wantUpstream != "" {
				if got := upstreamRef(t, clone); got != tt.wantUpstream {
					t.Errorf("upstream after the release = %q; want %q", got, tt.wantUpstream)
				}
			}
		})
	}
}

// TestRemoteRefsLock_ATrackingRefAFetchMovedPastThePushedCommitStaysThere covers a fetch that read origin after the push's transfer and wrote the tracking ref before the push took the lock.
func TestRemoteRefsLock_ATrackingRefAFetchMovedPastThePushedCommitStaysThere(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()
	clone, bareRemote := publishedClone(t)
	repo := gitrepo.New(clone)
	pushed := gitkit.CommitFile(t, clone, "pushed.txt", "pushed", "pushed commit")

	releaseAndWait := pushUnderHeldLock(t, clone, bareRemote, "main", pushed, func() error { return repo.PushBranch("origin", "main") })
	other := filepath.Join(t.TempDir(), "other")
	gitkit.MustRun(t, filepath.Dir(other), "git", "clone", "-b", "main", bareRemote, other)
	later := gitkit.CommitFile(t, other, "later.txt", "later", "later commit")
	gitkit.Git(t, other, "push", "origin", "main")
	// A raw fetch bypasses the held lock, standing in for a fetch that took the lock before the push did.
	gitkit.Git(t, clone, "fetch", "origin")

	if err := releaseAndWait(); err != nil {
		t.Fatalf("PushBranch() error = %v; want nil", err)
	}
	if got := branchSHA(t, clone, "refs/remotes/origin/main"); got != later {
		t.Errorf("origin/main after the push recorded = %q; want the later fetched commit %q, not the pushed %q", got, later, pushed)
	}
}

// TestRemoteRefsLock_ConcurrentFetchesAndPushesOfOneRepositoryAllSucceed reproduces two lym processes writing one repository's tracking refs at once.
// Without the lock, git fails the loser with "incorrect old value provided" or "cannot lock ref".
func TestRemoteRefsLock_ConcurrentFetchesAndPushesOfOneRepositoryAllSucceed(t *testing.T) {
	tiergate.Require(t, 1)
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
