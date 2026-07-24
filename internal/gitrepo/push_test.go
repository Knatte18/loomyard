//go:build integration

// push_test.go covers Push and PushCoalesced against real git repositories.
// Two fixtures are kept deliberately separate, per discussion.md: a bare
// remote with two clones exercises cross-clone rebase-retry recovery (the
// single-pusher lock cannot be exercised there, since two clones have two
// distinct lock files), while a single clone with two concurrent goroutines
// exercises PushCoalesced's lock-blocking/coalescing behavior.

package gitrepo_test

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/lyxtest"
)

// newBareRemote creates a bare git repository at <dir>/remote.git and
// returns its path, ready to be added as an "origin" remote.
func newBareRemote(t *testing.T, dir string) string {
	t.Helper()

	bare := filepath.Join(dir, "remote.git")
	if err := os.Mkdir(bare, 0o755); err != nil {
		t.Fatalf("mkdir bare remote: %v", err)
	}
	lyxtest.MustRun(t, bare, "git", "init", "--bare", "-b", "main")
	return bare
}

// newRepoWithRemote creates a fresh (non-cloned) git repository under
// dir/name on branch main, with bareRemote configured as "origin" but no
// upstream tracking branch yet — the state a repo is in before its very
// first push. It returns both the raw directory (for fixture git calls) and
// the gitrepo.Repo wrapping it (the type under test).
func newRepoWithRemote(t *testing.T, dir, name, bareRemote string) (path string, repo *gitrepo.Repo) {
	t.Helper()

	path = filepath.Join(dir, name)
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", name, err)
	}
	lyxtest.MustRun(t, path, "git", "init", "-b", "main")
	lyxtest.MustRun(t, path, "git", "remote", "add", "origin", bareRemote)
	return path, gitrepo.New(path)
}

// cloneFromBare clones bareRemote into dir/name on branch main — used once
// the bare remote already has history to check out, so the clone comes with
// upstream tracking already established (unlike newRepoWithRemote).
func cloneFromBare(t *testing.T, dir, name, bareRemote string) (path string, repo *gitrepo.Repo) {
	t.Helper()

	path = filepath.Join(dir, name)
	lyxtest.MustRun(t, dir, "git", "clone", "-b", "main", bareRemote, path)
	return path, gitrepo.New(path)
}

// upstreamRef returns the checkout's configured upstream ref name (e.g.
// "origin/main"), or "" if none is configured, used to assert that Push
// establishes tracking on a repo's first push.
func upstreamRef(t *testing.T, dir string) string {
	t.Helper()

	stdout, _, code, err := runGit(t, dir, "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{u}")
	if err != nil {
		t.Fatalf("git rev-parse @{u} error = %v", err)
	}
	if code != 0 {
		return ""
	}
	return strings.TrimSpace(stdout)
}

// TestPush_CrossCloneRebaseRetry exercises fixture (a): a bare remote plus
// two clones. Clone A's very first Push() has no upstream configured yet
// and must both succeed and establish tracking; clone B then pushes ahead,
// putting clone A's next push into a non-fast-forward that Push() must
// recover from via one pull --rebase retry, landing both commits on the
// remote.
func TestPush_CrossCloneRebaseRetry(t *testing.T) {
	container := t.TempDir()
	bareRemote := newBareRemote(t, container)

	cloneAPath, repoA := newRepoWithRemote(t, container, "cloneA", bareRemote)
	writeFile(t, cloneAPath, "a.txt", "from A, commit 1")
	commitAll(t, cloneAPath, "commit from A #1")

	if err := repoA.Push(); err != nil {
		t.Fatalf("Push() (first push, no upstream) error = %v; want nil", err)
	}
	if got := upstreamRef(t, cloneAPath); got == "" {
		t.Fatal("upstream ref after first Push() = \"\"; want Push() to have established tracking")
	}

	// Clone B checks out the bare remote now that it has history, so it
	// starts with upstream tracking already in place from the clone itself.
	cloneBPath, repoB := cloneFromBare(t, container, "cloneB", bareRemote)
	writeFile(t, cloneBPath, "b.txt", "from B")
	commitAll(t, cloneBPath, "commit from B")
	if err := repoB.Push(); err != nil {
		t.Fatalf("Push() from clone B error = %v; want nil", err)
	}

	// Clone A is now behind the remote; a further local commit followed by
	// Push() must hit a non-fast-forward rejection and recover via the
	// rebase-retry rather than failing outright.
	writeFile(t, cloneAPath, "a.txt", "from A, commit 2")
	commitAll(t, cloneAPath, "commit from A #2")
	if err := repoA.Push(); err != nil {
		t.Fatalf("Push() from clone A (behind remote) error = %v; want nil (rebase-retry should recover)", err)
	}

	logOut, stderr, code, err := runGit(t, cloneAPath, "log", "--oneline")
	if err != nil {
		t.Fatalf("git log error = %v", err)
	}
	if code != 0 {
		t.Fatalf("git log exited %d: %s", code, stderr)
	}
	for _, want := range []string{"commit from A #1", "commit from A #2", "commit from B"} {
		if !strings.Contains(logOut, want) {
			t.Errorf("git log --oneline (after rebase-retry) = %q; want it to contain %q", logOut, want)
		}
	}
}

// TestPush_RebaseRetryPrecondition_DirtyTrackedFileAborts asserts the
// documented rebase-retry precondition: pull --rebase aborts when the
// worktree has a dirty tracked file, so Push() surfaces an error instead of
// silently recovering. This is a caller-precondition failure, not a
// gitrepo bug — the caller is responsible for a clean tree of tracked files
// before relying on the rebase-retry to recover a rejected push.
func TestPush_RebaseRetryPrecondition_DirtyTrackedFileAborts(t *testing.T) {
	container := t.TempDir()
	bareRemote := newBareRemote(t, container)

	cloneAPath, repoA := newRepoWithRemote(t, container, "cloneA", bareRemote)
	writeFile(t, cloneAPath, "a.txt", "from A, commit 1")
	commitAll(t, cloneAPath, "commit from A #1")
	if err := repoA.Push(); err != nil {
		t.Fatalf("Push() (first push, no upstream) error = %v; want nil", err)
	}

	cloneBPath, repoB := cloneFromBare(t, container, "cloneB", bareRemote)
	writeFile(t, cloneBPath, "b.txt", "from B")
	commitAll(t, cloneBPath, "commit from B")
	if err := repoB.Push(); err != nil {
		t.Fatalf("Push() from clone B error = %v; want nil", err)
	}

	// Clone A commits again (now behind, so its next Push() will need the
	// rebase-retry) and is then left with a dirty tracked file — the
	// precondition violation under test.
	writeFile(t, cloneAPath, "a.txt", "from A, commit 2")
	commitAll(t, cloneAPath, "commit from A #2")
	writeFile(t, cloneAPath, "a.txt", "dirty uncommitted edit")

	if err := repoA.Push(); err == nil {
		t.Fatal("Push() with a dirty tracked file during rebase-retry = nil error; want an error (rebase must abort, not recover)")
	}
}

// TestPush_RebaseConflict_AbortsToCleanState drives a genuine content
// conflict through the rebase-retry: both clones commit conflicting edits to
// the same file, so clone A's pull --rebase stops mid-rebase. Push must
// surface an error AND leave the repository fully restored — clean worktree,
// no rebase in progress, HEAD back on the local commit — because the
// rebase-retry's contract is to never leave a rebase half-done.
func TestPush_RebaseConflict_AbortsToCleanState(t *testing.T) {
	container := t.TempDir()
	bareRemote := newBareRemote(t, container)

	cloneAPath, repoA := newRepoWithRemote(t, container, "cloneA", bareRemote)
	writeFile(t, cloneAPath, "shared.txt", "base\n")
	commitAll(t, cloneAPath, "init")
	if err := repoA.Push(); err != nil {
		t.Fatalf("Push() (establish upstream) error = %v; want nil", err)
	}

	cloneBPath, repoB := cloneFromBare(t, container, "cloneB", bareRemote)
	writeFile(t, cloneBPath, "shared.txt", "B version\n")
	commitAll(t, cloneBPath, "B edit")
	if err := repoB.Push(); err != nil {
		t.Fatalf("Push() from clone B error = %v; want nil", err)
	}

	writeFile(t, cloneAPath, "shared.txt", "A version\n")
	commitAll(t, cloneAPath, "A conflicting edit")
	localHead, err := repoA.CurrentSHA()
	if err != nil {
		t.Fatalf("CurrentSHA() error = %v", err)
	}

	if err := repoA.Push(); err == nil {
		t.Fatal("Push() with a genuine rebase conflict error = nil; want an error")
	}

	// The abort must have restored a fully clean, non-rebasing state.
	stdout, stderr, code, err := runGitStatus(t, cloneAPath)
	if err != nil {
		t.Fatalf("git status error = %v", err)
	}
	if code != 0 {
		t.Fatalf("git status exited %d: %s", code, stderr)
	}
	if strings.TrimSpace(stdout) != "" {
		t.Errorf("git status --porcelain after aborted rebase = %q; want empty (clean tree)", stdout)
	}
	for _, rebaseDir := range []string{"rebase-merge", "rebase-apply"} {
		if _, statErr := os.Stat(filepath.Join(cloneAPath, ".git", rebaseDir)); statErr == nil {
			t.Errorf(".git/%s exists after Push(); want no rebase left in progress", rebaseDir)
		}
	}
	head, err := repoA.CurrentSHA()
	if err != nil {
		t.Fatalf("CurrentSHA() error = %v", err)
	}
	if head != localHead {
		t.Errorf("HEAD after aborted rebase = %q; want %q (local commit preserved)", head, localHead)
	}
}

// TestPush_NoRemoteConfigured_SurfacesGitError asserts the third fixture
// discussion.md calls out: a repo with zero remotes configured at all (not
// merely no upstream tracking branch — no "origin" either). Push must not
// swallow this into a synthetic message; the wrapped error must still carry
// git's own stderr, matching Push's documented "any other push failure
// returns an error including git's stderr" contract.
func TestPush_NoRemoteConfigured_SurfacesGitError(t *testing.T) {
	dir, repo := newRepo(t)
	writeFile(t, dir, "a.txt", "content")
	commitAll(t, dir, "init")

	err := repo.Push()
	if err == nil {
		t.Fatal("Push() with no remote configured error = nil; want an error")
	}
	if !strings.Contains(err.Error(), "gitrepo: git push:") {
		t.Errorf("Push() error = %q; want it to wrap git's own push error unchanged", err)
	}
}

// TestPushCoalesced_NoRemoteConfigured_SurfacesGitError is PushCoalesced's
// counterpart to TestPush_NoRemoteConfigured_SurfacesGitError: hasUnpushed
// treats the missing upstream as "unpushed" regardless of the missing
// remote, so PushCoalesced proceeds to the same pushWithRebaseRetry path and
// must surface the same unwrapped git error rather than the lock machinery
// masking it.
func TestPushCoalesced_NoRemoteConfigured_SurfacesGitError(t *testing.T) {
	dir, repo := newRepo(t)
	writeFile(t, dir, "a.txt", "content")
	commitAll(t, dir, "init")

	err := repo.PushCoalesced()
	if err == nil {
		t.Fatal("PushCoalesced() with no remote configured error = nil; want an error")
	}
	if !strings.Contains(err.Error(), "gitrepo: git push:") {
		t.Errorf("PushCoalesced() error = %q; want it to wrap git's own push error unchanged", err)
	}
}

// TestPushCoalesced_LockBlocking_Serializes exercises fixture (b): a single
// clone (one worktree, one .gitrepo-push.lock). Several commits are made
// before any push runs, then PushCoalesced is called concurrently from two
// goroutines; they must serialize on the single-pusher lock and both return
// with no error, ending with everything pushed — the second goroutine finds
// nothing unpushed once it acquires the lock and returns immediately.
func TestPushCoalesced_LockBlocking_Serializes(t *testing.T) {
	container := t.TempDir()
	bareRemote := newBareRemote(t, container)

	repoPath, repo := newRepoWithRemote(t, container, "clone", bareRemote)
	writeFile(t, repoPath, "a.txt", "initial")
	commitAll(t, repoPath, "init")
	if err := repo.Push(); err != nil {
		t.Fatalf("Push() (establish upstream) error = %v; want nil", err)
	}

	// Several commits land locally before any push runs, so the two
	// concurrent PushCoalesced calls below must coalesce them rather than
	// each pushing its own subset.
	for i := 0; i < 3; i++ {
		writeFile(t, repoPath, fmt.Sprintf("file%d.txt", i), "content")
		commitAll(t, repoPath, fmt.Sprintf("commit %d", i))
	}

	localHead, err := repo.CurrentSHA()
	if err != nil {
		t.Fatalf("CurrentSHA() error = %v", err)
	}

	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i := range errs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs[i] = repo.PushCoalesced()
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("PushCoalesced() goroutine %d error = %v; want nil", i, err)
		}
	}

	// Nothing should remain unpushed once both goroutines have returned.
	stdout, stderr, code, err := runGit(t, repoPath, "rev-list", "--count", "@{u}..HEAD")
	if err != nil {
		t.Fatalf("git rev-list --count error = %v", err)
	}
	if code != 0 {
		t.Fatalf("git rev-list --count exited %d: %s", code, stderr)
	}
	if got := strings.TrimSpace(stdout); got != "0" {
		t.Errorf("rev-list --count @{u}..HEAD = %q; want \"0\" after both PushCoalesced calls complete", got)
	}

	// The bare remote must have received every commit, landing exactly at
	// the local HEAD captured before the concurrent pushes ran.
	remoteHead, stderr, code, err := runGit(t, bareRemote, "rev-parse", "main")
	if err != nil {
		t.Fatalf("git rev-parse main (bare) error = %v", err)
	}
	if code != 0 {
		t.Fatalf("git rev-parse main (bare) exited %d: %s", code, stderr)
	}
	if got := strings.TrimSpace(remoteHead); got != localHead {
		t.Errorf("bare remote main = %q; want it to match local HEAD %q", got, localHead)
	}
}

// TestPushCoalescedChildProcess is not a standalone test: it is the child
// body TestPushCoalesced_CrossProcess_Serializes re-execs from the test
// binary with GITREPO_TEST_PUSH_DIR set, so the single-pusher lock is
// exercised across genuinely separate OS processes. It skips in a normal
// run (env unset).
func TestPushCoalescedChildProcess(t *testing.T) {
	dir := os.Getenv("GITREPO_TEST_PUSH_DIR")
	if dir == "" {
		t.Skip("child-process body; runs only re-exec'd with GITREPO_TEST_PUSH_DIR set")
	}
	if err := gitrepo.New(dir).PushCoalesced(); err != nil {
		t.Fatalf("PushCoalesced() in child process error = %v; want nil", err)
	}
}

// TestPushCoalesced_CrossProcess_Serializes proves the single-pusher lock
// holds across real OS processes, not just goroutines sharing one process
// (TestPushCoalesced_LockBlocking_Serializes' proxy): several re-exec'd
// child processes call PushCoalesced against the same clone concurrently,
// and every commit must land on the bare remote with all children
// succeeding. Synchronization is the lock itself — no timing assumptions.
func TestPushCoalesced_CrossProcess_Serializes(t *testing.T) {
	container := t.TempDir()
	bareRemote := newBareRemote(t, container)

	repoPath, repo := newRepoWithRemote(t, container, "clone", bareRemote)
	writeFile(t, repoPath, "a.txt", "initial")
	commitAll(t, repoPath, "init")
	if err := repo.Push(); err != nil {
		t.Fatalf("Push() (establish upstream) error = %v; want nil", err)
	}
	for i := 0; i < 3; i++ {
		writeFile(t, repoPath, fmt.Sprintf("file%d.txt", i), "content")
		commitAll(t, repoPath, fmt.Sprintf("commit %d", i))
	}
	localHead, err := repo.CurrentSHA()
	if err != nil {
		t.Fatalf("CurrentSHA() error = %v", err)
	}

	testBin, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable() error = %v", err)
	}

	const procs = 4
	var wg sync.WaitGroup
	outputs := make([]string, procs)
	errs := make([]error, procs)
	for i := 0; i < procs; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			cmd := exec.Command(testBin, "-test.run=^TestPushCoalescedChildProcess$", "-test.v")
			cmd.Env = append(os.Environ(), "GITREPO_TEST_PUSH_DIR="+repoPath)
			out, err := cmd.CombinedOutput()
			outputs[i], errs[i] = string(out), err
		}(i)
	}
	wg.Wait()

	for i := 0; i < procs; i++ {
		if errs[i] != nil {
			t.Errorf("child process %d failed: %v\noutput:\n%s", i, errs[i], outputs[i])
		}
	}
	remoteHead, stderr, code, err := runGit(t, bareRemote, "rev-parse", "main")
	if err != nil {
		t.Fatalf("git rev-parse main (bare) error = %v", err)
	}
	if code != 0 {
		t.Fatalf("git rev-parse main (bare) exited %d: %s", code, stderr)
	}
	if got := strings.TrimSpace(remoteHead); got != localHead {
		t.Errorf("bare remote main = %q; want local HEAD %q after all child processes pushed", got, localHead)
	}
}

// TestLockHolderChildProcess is not a standalone test: it is the child body
// TestPushCoalesced_LockHolderCrash_Recovers re-execs. It acquires the
// repo's push lock, prints a marker so the parent knows the lock is held,
// and blocks until the parent SIGKILLs it. It skips in a normal run.
func TestLockHolderChildProcess(t *testing.T) {
	dir := os.Getenv("GITREPO_TEST_LOCKHOLD_DIR")
	if dir == "" {
		t.Skip("child-process body; runs only re-exec'd with GITREPO_TEST_LOCKHOLD_DIR set")
	}
	held, err := lock.AcquireWriteLock(filepath.Join(dir, ".gitrepo-push.lock"))
	if err != nil {
		t.Fatalf("AcquireWriteLock() in child process error = %v", err)
	}
	fmt.Println("LOCK-HELD")
	// Block until SIGKILLed. KeepAlive prevents the garbage collector from
	// finalizing the lock's file handle, which would silently release it.
	for {
		time.Sleep(time.Hour)
		runtime.KeepAlive(held)
	}
}

// TestPushCoalesced_LockHolderCrash_Recovers proves the push lock does not
// wedge the repo when its holder dies without releasing: a child process
// acquires .gitrepo-push.lock (confirmed genuinely held cross-process via a
// failed TryAcquire), is SIGKILLed mid-hold, and a fresh PushCoalesced must
// then complete — the OS releases a flock on process death — and push the
// pending commit.
func TestPushCoalesced_LockHolderCrash_Recovers(t *testing.T) {
	container := t.TempDir()
	bareRemote := newBareRemote(t, container)

	repoPath, repo := newRepoWithRemote(t, container, "clone", bareRemote)
	writeFile(t, repoPath, "a.txt", "initial")
	commitAll(t, repoPath, "init")
	if err := repo.Push(); err != nil {
		t.Fatalf("Push() (establish upstream) error = %v; want nil", err)
	}
	writeFile(t, repoPath, "b.txt", "unpushed")
	commitAll(t, repoPath, "unpushed commit")

	testBin, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable() error = %v", err)
	}
	cmd := exec.Command(testBin, "-test.run=^TestLockHolderChildProcess$", "-test.v")
	cmd.Env = append(os.Environ(), "GITREPO_TEST_LOCKHOLD_DIR="+repoPath)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("StdoutPipe() error = %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("starting lock-holder child: %v", err)
	}
	t.Cleanup(func() {
		cmd.Process.Kill()
		cmd.Wait()
	})

	// Wait on the child's explicit marker — a real state transition, not a
	// timing guess — before trusting that the lock is held.
	markerSeen := make(chan struct{})
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			if strings.Contains(scanner.Text(), "LOCK-HELD") {
				close(markerSeen)
				return
			}
		}
	}()
	select {
	case <-markerSeen:
	case <-time.After(30 * time.Second):
		t.Fatal("lock-holder child never reported LOCK-HELD")
	}

	// The lock must be genuinely held by the other process before the kill,
	// or the recovery below would prove nothing.
	if _, ok, err := lock.TryAcquireWriteLock(filepath.Join(repoPath, ".gitrepo-push.lock")); err != nil {
		t.Fatalf("TryAcquireWriteLock() error = %v", err)
	} else if ok {
		t.Fatal("TryAcquireWriteLock() ok = true while the child holds the lock; want contention")
	}

	if err := cmd.Process.Kill(); err != nil { // SIGKILL — no graceful release
		t.Fatalf("killing lock-holder child: %v", err)
	}
	cmd.Wait()

	// A fresh PushCoalesced must neither wedge nor fail: the OS released the
	// dead holder's flock, and the pending commit gets pushed.
	done := make(chan error, 1)
	go func() { done <- repo.PushCoalesced() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("PushCoalesced() after lock-holder crash error = %v; want nil", err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("PushCoalesced() wedged after lock-holder crash; want the dead process's lock released")
	}

	localHead, err := repo.CurrentSHA()
	if err != nil {
		t.Fatalf("CurrentSHA() error = %v", err)
	}
	remoteHead, stderr, code, err := runGit(t, bareRemote, "rev-parse", "main")
	if err != nil {
		t.Fatalf("git rev-parse main (bare) error = %v", err)
	}
	if code != 0 {
		t.Fatalf("git rev-parse main (bare) exited %d: %s", code, stderr)
	}
	if got := strings.TrimSpace(remoteHead); got != localHead {
		t.Errorf("bare remote main = %q; want local HEAD %q after crash recovery", got, localHead)
	}
}
