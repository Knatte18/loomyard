// push_test.go covers Push, PushRebaseFree and PushCoalesced against real git repositories.
// A bare remote with several clones exercises rebase-retry recovery across clones.
// The single-pusher lock cannot be exercised across clones, since two clones have two distinct lock files.
// A single clone with concurrent goroutines and processes exercises the lock-blocking and coalescing of PushCoalesced.

package gitrepo_test

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// newBareRemote creates a bare git repository at <dir>/remote.git.
func newBareRemote(t *testing.T, dir string) string {
	t.Helper()

	bare := filepath.Join(dir, "remote.git")
	if err := os.Mkdir(bare, 0o755); err != nil {
		t.Fatalf("mkdir bare remote: %v", err)
	}
	gitkit.MustRun(t, bare, "git", "init", "--bare", "-b", "main")
	return bare
}

// newRepoWithRemote creates a fresh git repository on main with bareRemote configured as "origin".
// The repository has no upstream tracking configured.
func newRepoWithRemote(t *testing.T, dir, name, bareRemote string) (path string, repo *gitrepo.Repo) {
	t.Helper()

	path = filepath.Join(dir, name)
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", name, err)
	}
	gitkit.MustRun(t, path, "git", "init", "-b", "main")
	gitkit.MustRun(t, path, "git", "remote", "add", "origin", bareRemote)
	return path, gitrepo.New(path)
}

// cloneFromBare clones bareRemote into dir/name on branch main.
// The bare remote must already have history on main to check out.
// The clone starts with its upstream tracking for main already set.
func cloneFromBare(t *testing.T, dir, name, bareRemote string) (path string, repo *gitrepo.Repo) {
	t.Helper()

	path = filepath.Join(dir, name)
	gitkit.MustRun(t, dir, "git", "clone", "-b", "main", bareRemote, path)
	return path, gitrepo.New(path)
}

// upstreamRef returns the upstream ref name of the checkout, such as "origin/main", or "" if none is configured.
// Tests use it to check that a first push establishes tracking.
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

// remoteBranchSHA returns the SHA that branch points to in bareRemote.
func remoteBranchSHA(t *testing.T, bareRemote, branch string) string {
	t.Helper()

	stdout, stderr, code, err := runGit(t, bareRemote, "rev-parse", branch)
	if err != nil {
		t.Fatalf("git rev-parse %s (bare) error = %v", branch, err)
	}
	if code != 0 {
		t.Fatalf("git rev-parse %s (bare) exited %d: %s", branch, code, stderr)
	}
	return strings.TrimSpace(stdout)
}

// requireNoRebaseInProgress fails the test when dir carries rebase state.
// The failure names call as the operation that must not have left that state.
func requireNoRebaseInProgress(t *testing.T, dir, call string) {
	t.Helper()

	for _, rebaseDir := range []string{"rebase-merge", "rebase-apply"} {
		if _, statErr := os.Stat(filepath.Join(dir, ".git", rebaseDir)); statErr == nil {
			t.Errorf(".git/%s exists after %s; want no rebase left in progress", rebaseDir, call)
		}
	}
}

// resetToUpstream discards every local commit and edit in dir, returning it to its upstream tip.
func resetToUpstream(t *testing.T, dir string) {
	t.Helper()

	gitkit.MustRun(t, dir, "git", "fetch", "origin")
	gitkit.MustRun(t, dir, "git", "reset", "--hard", "@{u}")
}

// TestPush checks Push and PushRebaseFree against one bare remote and three clones.
// Push establishes upstream tracking on a first push.
// It recovers a non-fast-forward rejection with one pull --rebase retry, so every local commit lands on the remote.
// When that retry meets a dirty tracked file, Push returns an error instead of recovering.
// The caller owns a clean tree of tracked files, so a dirty one is a caller error, not a gitrepo bug.
// When the retry stops on a content conflict, Push returns an error and restores the repository.
// It leaves the worktree clean, with no rebase in progress and HEAD back on the local commit.
// A rebase must never be left half-done.
// PushRebaseFree never recovers a non-fast-forward rejection.
// It returns an error satisfying errors.Is(err, gitrepo.ErrPushRejected) and leaves the working tree untouched.
// A remote that refuses for want of permission makes it return gitrepo.ErrPushAuthRefused after one attempt.
// On a fresh clone, its first push also establishes upstream tracking.
// The steps run serially, in order, on this shared fixture; none of them calls t.Parallel.
// The later steps rely on clone A's upstream, and on clone B, which the rebase-retry step clones.
func TestPush(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	container := t.TempDir()
	bareRemote := newBareRemote(t, container)

	cloneAPath, repoA := newRepoWithRemote(t, container, "cloneA", bareRemote)
	var cloneBPath string
	var repoB *gitrepo.Repo

	steps := []struct {
		name string
		run  func(t *testing.T)
	}{
		{"the first Push establishes upstream tracking", func(t *testing.T) {
			writeFile(t, cloneAPath, "a.txt", "from A, commit 1")
			commitAll(t, cloneAPath, "commit from A #1")

			if err := repoA.Push(); err != nil {
				t.Fatalf("Push() (first push, no upstream) error = %v; want nil", err)
			}
			if got := upstreamRef(t, cloneAPath); got == "" {
				t.Fatal("upstream ref after first Push() = \"\"; want Push() to have established tracking")
			}
		}},
		{"a push behind the remote recovers via one rebase-retry", func(t *testing.T) {
			// Clone B comes from the bare remote, which has history, so it starts with upstream tracking already set.
			cloneBPath, repoB = cloneFromBare(t, container, "cloneB", bareRemote)
			writeFile(t, cloneBPath, "b.txt", "from B")
			commitAll(t, cloneBPath, "commit from B")
			if err := repoB.Push(); err != nil {
				t.Fatalf("Push() from clone B error = %v; want nil", err)
			}

			// Clone A is behind the remote: a further local commit followed by Push() hits a non-fast-forward rejection.
			// Push() must recover via the rebase-retry rather than fail outright.
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
		}},
		{"a rebase conflict aborts to a clean state", func(t *testing.T) {
			writeFile(t, cloneAPath, "shared.txt", "base\n")
			commitAll(t, cloneAPath, "shared base")
			if err := repoA.Push(); err != nil {
				t.Fatalf("Push() (shared base) error = %v; want nil", err)
			}

			if err := repoB.Pull(); err != nil {
				t.Fatalf("Pull() into clone B error = %v; want nil", err)
			}
			writeFile(t, cloneBPath, "shared.txt", "B version\n")
			commitAll(t, cloneBPath, "B edit")
			if err := repoB.Push(); err != nil {
				t.Fatalf("Push() from clone B error = %v; want nil", err)
			}

			writeFile(t, cloneAPath, "shared.txt", "A version\n")
			commitAll(t, cloneAPath, "A conflicting edit")
			localHead := requireCurrentSHA(t, repoA)

			err := repoA.Push()
			if err == nil {
				t.Fatal("Push() with a genuine rebase conflict error = nil; want an error")
			}
			if !errors.Is(err, gitrepo.ErrPullRebaseFailed) {
				t.Errorf("Push() rebase-conflict error = %v; want it to satisfy errors.Is(err, gitrepo.ErrPullRebaseFailed)", err)
			}

			// The abort must have restored a clean, non-rebasing state.
			if status := statusOf(t, cloneAPath); strings.TrimSpace(status) != "" {
				t.Errorf("git status --porcelain after aborted rebase = %q; want empty (clean tree)", status)
			}
			requireNoRebaseInProgress(t, cloneAPath, "Push()")
			if head := requireCurrentSHA(t, repoA); head != localHead {
				t.Errorf("HEAD after aborted rebase = %q; want %q (local commit preserved)", head, localHead)
			}
		}},
		{"a dirty tracked file aborts the rebase-retry", func(t *testing.T) {
			resetToUpstream(t, cloneAPath)

			if err := repoB.Pull(); err != nil {
				t.Fatalf("Pull() into clone B error = %v; want nil", err)
			}
			writeFile(t, cloneBPath, "c.txt", "from B again")
			commitAll(t, cloneBPath, "second commit from B")
			if err := repoB.Push(); err != nil {
				t.Fatalf("Push() from clone B error = %v; want nil", err)
			}

			// Clone A commits again while behind the remote, so its next Push() needs the rebase-retry.
			// It is then left with a dirty tracked file: the precondition violation under test.
			writeFile(t, cloneAPath, "a.txt", "from A, commit 3")
			commitAll(t, cloneAPath, "commit from A #3")
			writeFile(t, cloneAPath, "a.txt", "dirty uncommitted edit")

			if err := repoA.Push(); err == nil {
				t.Fatal("Push() with a dirty tracked file during rebase-retry = nil error; want an error (rebase must abort, not recover)")
			}
		}},
		{"PushRebaseFree against a diverged remote returns ErrPushRejected and touches nothing", func(t *testing.T) {
			resetToUpstream(t, cloneAPath)

			if err := repoB.Pull(); err != nil {
				t.Fatalf("Pull() into clone B error = %v; want nil", err)
			}
			writeFile(t, cloneBPath, "b.txt", "from B, third")
			commitAll(t, cloneBPath, "third commit from B")
			if err := repoB.PushRebaseFree(); err != nil {
				t.Fatalf("PushRebaseFree() from clone B error = %v; want nil", err)
			}

			// Clone A is behind the remote.
			// It makes a further local commit, then leaves a tracked file dirty on purpose:
			// PushRebaseFree must leave that state untouched.
			writeFile(t, cloneAPath, "a.txt", "from A, commit 4")
			commitAll(t, cloneAPath, "commit from A #4")
			const dirtyContent = "dirty uncommitted edit"
			writeFile(t, cloneAPath, "a.txt", dirtyContent)
			localHead := requireCurrentSHA(t, repoA)

			err := repoA.PushRebaseFree()
			if err == nil {
				t.Fatal("PushRebaseFree() against a diverged remote error = nil; want ErrPushRejected")
			}
			if !errors.Is(err, gitrepo.ErrPushRejected) {
				t.Errorf("PushRebaseFree() error = %v; want it to satisfy errors.Is(err, gitrepo.ErrPushRejected)", err)
			}

			// The error carries git's own rejection line and claims no divergence, since a declining hook is rejected the same way.
			if line := strings.TrimPrefix(err.Error(), gitrepo.ErrPushRejected.Error()+": "); strings.Contains(err.Error(), "remote diverged") || line == "" || line == err.Error() {
				t.Errorf("PushRebaseFree() error = %q; want git's rejection line and no \"remote diverged\" claim", err)
			}

			// No pull --rebase ever ran, so the dirty tracked file must be exactly as left, and no rebase state must exist.
			got, readErr := os.ReadFile(filepath.Join(cloneAPath, "a.txt"))
			if readErr != nil {
				t.Fatalf("read a.txt: %v", readErr)
			}
			if string(got) != dirtyContent {
				t.Errorf("a.txt content after rejected PushRebaseFree() = %q; want unchanged %q", got, dirtyContent)
			}
			requireNoRebaseInProgress(t, cloneAPath, "PushRebaseFree()")
			if head := requireCurrentSHA(t, repoA); head != localHead {
				t.Errorf("HEAD after rejected PushRebaseFree() = %q; want unchanged %q", head, localHead)
			}
		}},
		// A checkout with no upstream tracking branch yet must succeed on PushRebaseFree's first push.
		// The first push establishes tracking and lands the local HEAD on the bare remote.
		// The fresh clone pushes a branch of its own, since its unrelated root commit could never fast-forward main.
		{"PushRebaseFree's first push establishes upstream tracking", func(t *testing.T) {
			const branch = "rebasefree-first"
			repoPath, repo := newRepoWithRemote(t, container, "cloneC", bareRemote)
			gitkit.MustRun(t, repoPath, "git", "checkout", "-b", branch)
			writeFile(t, repoPath, "a.txt", "from clone, commit 1")
			commitAll(t, repoPath, "commit 1")
			localHead := requireCurrentSHA(t, repo)

			if err := repo.PushRebaseFree(); err != nil {
				t.Fatalf("PushRebaseFree() (first push, no upstream) error = %v; want nil", err)
			}
			if got := upstreamRef(t, repoPath); got == "" {
				t.Fatal("upstream ref after first PushRebaseFree() = \"\"; want PushRebaseFree() to have established tracking")
			}
			if got := remoteBranchSHA(t, bareRemote, branch); got != localHead {
				t.Errorf("bare remote %s = %q; want it to match local HEAD %q", branch, got, localHead)
			}
		}},
		// The remote's pre-receive hook prints GitHub's 403 wording and declines.
		// Its "rejected" line must not read as a divergence.
		// The refusal is attempted once and never retried.
		{"PushRebaseFree classifies an auth refusal as ErrPushAuthRefused and never retries it", func(t *testing.T) {
			authContainer := t.TempDir()
			authRemote := newBareRemote(t, authContainer)
			hookRuns := filepath.Join(authContainer, "hook-runs")
			hook := "#!/bin/sh\n" +
				"echo ran >> '" + filepath.ToSlash(hookRuns) + "'\n" +
				"echo 'remote: Permission to acme/records.git denied to someone.' >&2\n" +
				"echo \"fatal: unable to access 'https://github.com/acme/records.git/': The requested URL returned error: 403\" >&2\n" +
				"exit 1\n"
			if err := os.WriteFile(filepath.Join(authRemote, "hooks", "pre-receive"), []byte(hook), 0o755); err != nil {
				t.Fatalf("write pre-receive hook: %v", err)
			}
			repoPath, repo := newRepoWithRemote(t, authContainer, "cloneAuth", authRemote)
			writeFile(t, repoPath, "a.txt", "refused")
			commitAll(t, repoPath, "refused commit")

			err := repo.PushRebaseFree()
			if !errors.Is(err, gitrepo.ErrPushAuthRefused) {
				t.Fatalf("PushRebaseFree() error = %v; want errors.Is(err, gitrepo.ErrPushAuthRefused)", err)
			}
			if errors.Is(err, gitrepo.ErrPushRejected) || errors.Is(err, gitrepo.ErrPushRetriesExhausted) {
				t.Errorf("PushRebaseFree() error = %v; want neither ErrPushRejected nor ErrPushRetriesExhausted", err)
			}
			if !strings.Contains(err.Error(), "The requested URL returned error: 403") {
				t.Errorf("PushRebaseFree() error = %q; want it to carry git's refusal line", err)
			}
			runs, readErr := os.ReadFile(hookRuns)
			if readErr != nil {
				t.Fatalf("read hook run log: %v", readErr)
			}
			if got := strings.Count(string(runs), "ran"); got != 1 {
				t.Errorf("pre-receive hook ran %d times; want exactly 1 (an auth refusal is never retried)", got)
			}
		}},
	}
	for _, step := range steps {
		if !t.Run(step.name, step.run) {
			return
		}
	}
}

// TestUpdateRemoteBranchLeased drives UpdateRemoteBranchLeased against one bare remote.
// The remote's main holds two commits, pushed from one clone.
// A leased update naming the remote's current tip moves the branch backwards to the first commit.
// A lease naming the stale second commit then fails with ErrLeaseRejected and leaves the remote branch where it was.
// An invalid SHA returns ErrInvalidSHA before any git spawn;
// without that, the not-a-repository clone path would surface a different error.
// A pre-receive hook that exits non-zero rejects a correctly leased update.
// Its error is not ErrLeaseRejected, so a moved remote and a refusing one stay distinguishable.
// The steps run serially in that order and share the remote; the hook step is last because the hook stays installed.
// The top-level test calls t.Parallel; no step does, because the steps share the fixture.
func TestUpdateRemoteBranchLeased(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	container := t.TempDir()
	bareRemote := newBareRemote(t, container)
	clonePath, repo := newRepoWithRemote(t, container, "clone", bareRemote)

	writeFile(t, clonePath, "a.txt", "one")
	commitAll(t, clonePath, "commit one")
	firstSHA := requireCurrentSHA(t, repo)
	if err := repo.Push(); err != nil {
		t.Fatalf("Push() (first commit) error = %v; want nil", err)
	}
	writeFile(t, clonePath, "a.txt", "two")
	commitAll(t, clonePath, "commit two")
	secondSHA := requireCurrentSHA(t, repo)
	if err := repo.Push(); err != nil {
		t.Fatalf("Push() (second commit) error = %v; want nil", err)
	}

	steps := []struct {
		name string
		run  func(t *testing.T)
	}{
		{"a lease at the remote's tip moves the branch backwards", func(t *testing.T) {
			if err := repo.UpdateRemoteBranchLeased("origin", "main", firstSHA, secondSHA); err != nil {
				t.Fatalf("UpdateRemoteBranchLeased(main, %s, lease %s) error = %v; want nil", firstSHA, secondSHA, err)
			}
			if got := remoteBranchSHA(t, bareRemote, "main"); got != firstSHA {
				t.Errorf("bare remote main = %q; want %q", got, firstSHA)
			}
		}},
		{"a stale lease returns ErrLeaseRejected and leaves the branch", func(t *testing.T) {
			err := repo.UpdateRemoteBranchLeased("origin", "main", secondSHA, secondSHA)
			if !errors.Is(err, gitrepo.ErrLeaseRejected) {
				t.Fatalf("UpdateRemoteBranchLeased() with a stale lease error = %v; want errors.Is(err, ErrLeaseRejected)", err)
			}
			if got := remoteBranchSHA(t, bareRemote, "main"); got != firstSHA {
				t.Errorf("bare remote main after a rejected lease = %q; want unchanged %q", got, firstSHA)
			}
		}},
		{"an invalid SHA returns ErrInvalidSHA", func(t *testing.T) {
			for _, args := range [][2]string{{"not-a-sha", firstSHA}, {firstSHA, "not-a-sha"}} {
				err := repo.UpdateRemoteBranchLeased("origin", "main", args[0], args[1])
				if !errors.Is(err, gitrepo.ErrInvalidSHA) {
					t.Errorf("UpdateRemoteBranchLeased(%q, lease %q) error = %v; want ErrInvalidSHA", args[0], args[1], err)
				}
			}
		}},
		{"a hook rejection is not a lease loss", func(t *testing.T) {
			hook := filepath.Join(bareRemote, "hooks", "pre-receive")
			if err := os.WriteFile(hook, []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
				t.Fatalf("write pre-receive hook: %v", err)
			}

			err := repo.UpdateRemoteBranchLeased("origin", "main", secondSHA, firstSHA)
			if err == nil {
				t.Fatal("UpdateRemoteBranchLeased() against a rejecting hook error = nil; want an error")
			}
			if errors.Is(err, gitrepo.ErrLeaseRejected) {
				t.Errorf("UpdateRemoteBranchLeased() hook rejection error = %v; want it not to satisfy ErrLeaseRejected", err)
			}
			if got := remoteBranchSHA(t, bareRemote, "main"); got != firstSHA {
				t.Errorf("bare remote main after a hook rejection = %q; want unchanged %q", got, firstSHA)
			}
		}},
	}
	for _, step := range steps {
		if !t.Run(step.name, step.run) {
			return
		}
	}
}

// TestPush_NoRemoteConfigured_SurfacesGitError covers a repo with no remotes configured at all, not even "origin".
// Push and PushCoalesced must not swallow this into a synthetic message:
// the wrapped error must still carry git's own stderr.
// PushCoalesced reaches the same path: HasUnpushed reports a missing upstream as unpushed, even with no remote.
// PushCoalesced then proceeds to pushWithRebaseRetry, and the lock must not mask the error.
func TestPush_NoRemoteConfigured_SurfacesGitError(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	dir, repo := newRepo(t)
	writeFile(t, dir, "a.txt", "content")
	commitAll(t, dir, "init")

	tests := []struct {
		name string
		push func() error
	}{
		{"Push", repo.Push},
		{"PushCoalesced", repo.PushCoalesced},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.push()
			if err == nil {
				t.Fatalf("%s() with no remote configured error = nil; want an error", tt.name)
			}
			if !strings.Contains(err.Error(), "gitrepo: git push:") {
				t.Errorf("%s() error = %q; want it to wrap git's own push error unchanged", tt.name, err)
			}
			if errors.Is(err, gitrepo.ErrPullRebaseFailed) {
				t.Errorf("%s() plain push failure satisfies ErrPullRebaseFailed; want only an aborted rebase to", tt.name)
			}
		})
	}
}

// Each variable selects a child-process body that a re-exec'd test binary runs.
// The variable's value is the clone directory that body works on.
const (
	pushChildEnv       = "GITREPO_TEST_PUSH_DIR"
	lockHolderChildEnv = "GITREPO_TEST_LOCKHOLD_DIR"
)

func init() {
	childBodies[pushChildEnv] = pushCoalescedChild
	childBodies[lockHolderChildEnv] = lockHolderChild
}

// pushCoalescedChild runs PushCoalesced against the clone at dir.
// It runs in a separate OS process, so the single-pusher lock is exercised across processes.
func pushCoalescedChild(dir string) int {
	if err := gitrepo.New(dir).PushCoalesced(); err != nil {
		fmt.Fprintf(os.Stderr, "PushCoalesced() in child process error = %v; want nil\n", err)
		return 1
	}
	return 0
}

// lockHolderChild acquires the push lock of the clone at dir.
// It prints a marker so the parent knows the lock is held, then blocks until the parent SIGKILLs it.
func lockHolderChild(dir string) int {
	lockPath, err := gitrepo.New(dir).PushLockPath()
	if err != nil {
		fmt.Fprintf(os.Stderr, "PushLockPath() in child process error = %v\n", err)
		return 1
	}
	held, err := lock.AcquireWriteLock(lockPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "AcquireWriteLock() in child process error = %v\n", err)
		return 1
	}
	fmt.Println("LOCK-HELD")
	// Block until SIGKILLed.
	// KeepAlive prevents the garbage collector from finalizing the lock's file handle, which would silently release it.
	for {
		time.Sleep(time.Hour)
		runtime.KeepAlive(held)
	}
}

// TestPushCoalesced drives PushCoalesced's push lock with goroutines, separate OS processes and a crashed lock holder.
// Several commits land locally before any push runs.
// Concurrent PushCoalesced calls must coalesce them rather than each pushing its own subset.
// The second call to get the lock finds nothing unpushed and returns immediately.
// The cross-process step proves the lock holds across real OS processes, not just goroutines sharing one process.
// Several re-exec'd child processes call PushCoalesced against the same clone concurrently,
// and every commit must land on the bare remote with all children succeeding.
// Synchronization is the lock itself, with no timing assumptions.
// The crash step proves the push lock does not wedge the repo when its holder dies without releasing.
// A child process acquires the push lock; a failed `lock.TryAcquireWriteLock` then confirms it is held cross-process.
// The test then SIGKILLs that child mid-hold.
// A fresh PushCoalesced must complete and push the pending commit, because the OS releases a flock on process death.
// The steps run serially in that order, sharing the clone and the remote.
// Each commits locally and leaves everything pushed for the next.
// The top-level test calls t.Parallel; no step does, because the steps share the fixture.
func TestPushCoalesced(t *testing.T) {
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

	commitFiles := func(prefix string) {
		for i := 0; i < 3; i++ {
			writeFile(t, repoPath, fmt.Sprintf("%s%d.txt", prefix, i), "content")
			commitAll(t, repoPath, fmt.Sprintf("%s commit %d", prefix, i))
		}
	}

	if !t.Run("goroutines serialize on the lock", func(t *testing.T) {
		commitFiles("goroutine")
		localHead := requireCurrentSHA(t, repo)

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

		// The bare remote must have received every commit.
		// Its main lands exactly at the local HEAD captured before the concurrent pushes ran.
		if got := remoteBranchSHA(t, bareRemote, "main"); got != localHead {
			t.Errorf("bare remote main = %q; want it to match local HEAD %q", got, localHead)
		}
	}) {
		return
	}

	if !t.Run("separate processes serialize on the lock", func(t *testing.T) {
		commitFiles("process")
		localHead := requireCurrentSHA(t, repo)

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
				cmd := exec.Command(testBin, "-test.run=^$")
				cmd.Env = append(os.Environ(), pushChildEnv+"="+repoPath)
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
		if got := remoteBranchSHA(t, bareRemote, "main"); got != localHead {
			t.Errorf("bare remote main = %q; want local HEAD %q after all child processes pushed", got, localHead)
		}
	}) {
		return
	}

	t.Run("a crashed lock holder does not wedge the repo", func(t *testing.T) {
		writeFile(t, repoPath, "b.txt", "unpushed")
		commitAll(t, repoPath, "unpushed commit")

		testBin, err := os.Executable()
		if err != nil {
			t.Fatalf("os.Executable() error = %v", err)
		}
		cmd := exec.Command(testBin, "-test.run=^$")
		cmd.Env = append(os.Environ(), lockHolderChildEnv+"="+repoPath)
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

		// Wait on the child's explicit marker before trusting that the lock is held.
		// The marker is a state transition, not a timing guess.
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

		// The lock must be held by the other process before the kill, or the recovery below would prove nothing.
		pushLockPath, err := repo.PushLockPath()
		if err != nil {
			t.Fatalf("PushLockPath() error = %v", err)
		}
		if _, ok, err := lock.TryAcquireWriteLock(pushLockPath); err != nil {
			t.Fatalf("TryAcquireWriteLock() error = %v", err)
		} else if ok {
			t.Fatal("TryAcquireWriteLock() ok = true while the child holds the lock; want contention")
		}

		if err := cmd.Process.Kill(); err != nil { // SIGKILL — no graceful release
			t.Fatalf("killing lock-holder child: %v", err)
		}
		cmd.Wait()

		// A fresh PushCoalesced must neither wedge nor fail:
		// the OS released the dead holder's flock, and the pending commit gets pushed.
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

		localHead := requireCurrentSHA(t, repo)
		if got := remoteBranchSHA(t, bareRemote, "main"); got != localHead {
			t.Errorf("bare remote main = %q; want local HEAD %q after crash recovery", got, localHead)
		}
	})
}

// TestHasUnpulled drives HasUnpulled through a bare remote and a clone of it.
// The clone's upstream equal to HEAD and an upstream that is an ancestor of HEAD (local ahead) answer false;
// an upstream that another clone advanced answers false until this clone fetches it, then true;
// a branch with no upstream answers true.
func TestHasUnpulled(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	tests := []struct {
		name  string
		setup func(t *testing.T, container, bareRemote string, clonePath string)
		want  bool
	}{
		{"upstream equal to HEAD", func(t *testing.T, container, bareRemote, clonePath string) {}, false},
		{"upstream is an ancestor of HEAD", func(t *testing.T, container, bareRemote, clonePath string) {
			writeFile(t, clonePath, "local.txt", "local")
			commitAll(t, clonePath, "local commit")
		}, false},
		{"upstream advanced by another clone but not fetched", func(t *testing.T, container, bareRemote, clonePath string) {
			pushFromOtherClone(t, container, bareRemote)
		}, false},
		{"upstream advanced by another clone and fetched", func(t *testing.T, container, bareRemote, clonePath string) {
			pushFromOtherClone(t, container, bareRemote)
			gitkit.MustRun(t, clonePath, "git", "fetch", "origin")
		}, true},
		{"no upstream configured", func(t *testing.T, container, bareRemote, clonePath string) {
			gitkit.MustRun(t, clonePath, "git", "branch", "--unset-upstream")
		}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			container := t.TempDir()
			bareRemote := newBareRemote(t, container)
			seedPath, seed := newRepoWithRemote(t, container, "seed", bareRemote)
			writeFile(t, seedPath, "a.txt", "initial")
			commitAll(t, seedPath, "init")
			if err := seed.Push(); err != nil {
				t.Fatalf("Push() (seed) error = %v; want nil", err)
			}
			clonePath, clone := cloneFromBare(t, container, "clone", bareRemote)

			tt.setup(t, container, bareRemote, clonePath)

			got, err := clone.HasUnpulled()
			if err != nil {
				t.Fatalf("HasUnpulled() error = %v; want nil", err)
			}
			if got != tt.want {
				t.Errorf("HasUnpulled() = %v; want %v", got, tt.want)
			}
		})
	}
}

// pushFromOtherClone clones bareRemote into a fresh directory under container, commits there and pushes.
// The push advances the remote past every other clone.
func pushFromOtherClone(t *testing.T, container, bareRemote string) {
	t.Helper()

	otherPath, other := cloneFromBare(t, container, "other", bareRemote)
	writeFile(t, otherPath, "other.txt", "from other")
	commitAll(t, otherPath, "commit from other")
	if err := other.Push(); err != nil {
		t.Fatalf("Push() (other clone) error = %v; want nil", err)
	}
}
