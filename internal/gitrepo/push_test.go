//go:build integration

// push_test.go covers Push, PushRebaseFree and PushCoalesced against real git repositories.
// Two fixtures are kept deliberately separate, per discussion.md: a bare remote with several clones exercises cross-clone rebase-retry recovery (the single-pusher lock cannot be exercised there, since two clones have two distinct lock files), while a single clone with concurrent goroutines and processes exercises PushCoalesced's lock-blocking/coalescing behavior.

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

// newRepoWithRemote creates a fresh git repository on main with bareRemote
// configured as "origin" but no upstream tracking yet.
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

// cloneFromBare clones bareRemote into dir/name on branch main — used once
// the bare remote already has history to check out, so the clone comes with
// upstream tracking already established (unlike newRepoWithRemote).
func cloneFromBare(t *testing.T, dir, name, bareRemote string) (path string, repo *gitrepo.Repo) {
	t.Helper()

	path = filepath.Join(dir, name)
	gitkit.MustRun(t, dir, "git", "clone", "-b", "main", bareRemote, path)
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

// remoteBranchSHA returns the SHA branch points at in bareRemote.
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

// requireNoRebaseInProgress fails the test when dir carries rebase state, naming call as the operation that must not have left it.
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

// TestPush drives Push and PushRebaseFree through one bare remote and three clones.
// Clone A's very first Push() has no upstream configured yet and must both succeed and establish tracking.
// Clone B then pushes ahead, putting clone A's next push into a non-fast-forward that Push() must recover from via one pull --rebase retry, landing every commit on the remote.
// The rebase-retry's refusals follow: a dirty tracked file aborts the pull --rebase so Push() surfaces an error instead of silently recovering (a caller-precondition failure, not a gitrepo bug — the caller owns a clean tree of tracked files), and a genuine content conflict stops the rebase mid-way, where Push must surface an error AND leave the repository fully restored — clean worktree, no rebase in progress, HEAD back on the local commit — because the rebase-retry's contract is to never leave a rebase half-done.
// PushRebaseFree never recovers a non-fast-forward rejection: it returns an error satisfying errors.Is(err, gitrepo.ErrPushRejected) and leaves the working tree completely untouched, and on a fresh clone it establishes tracking by applying push.autoSetupRemote=true exactly as Push's own first-push path does.
// The steps run serially in that order and share the remote and clones A and B: the dirty and diverged steps each start by returning clone A to the remote's tip, and the steps after the first rely on clone A's upstream and on clone B, which the rebase-retry step clones.
// The top-level test calls t.Parallel; no step does, because the steps share the fixture.
func TestPush(t *testing.T) {
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
			// Clone B checks out the bare remote now that it has history, so it starts with upstream tracking already in place from the clone itself.
			cloneBPath, repoB = cloneFromBare(t, container, "cloneB", bareRemote)
			writeFile(t, cloneBPath, "b.txt", "from B")
			commitAll(t, cloneBPath, "commit from B")
			if err := repoB.Push(); err != nil {
				t.Fatalf("Push() from clone B error = %v; want nil", err)
			}

			// Clone A is now behind the remote; a further local commit followed by Push() must hit a non-fast-forward rejection and recover via the rebase-retry rather than failing outright.
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

			if err := repoA.Push(); err == nil {
				t.Fatal("Push() with a genuine rebase conflict error = nil; want an error")
			}

			// The abort must have restored a fully clean, non-rebasing state.
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

			// Clone A commits again (now behind, so its next Push() will need the rebase-retry) and is then left with a dirty tracked file — the precondition violation under test.
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

			// Clone A is now behind the remote.
			// A further local commit followed by a dirty tracked file (left uncommitted on purpose) sets up the state PushRebaseFree must leave untouched.
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
		// PushRebaseFree's first-push path: a checkout with no upstream tracking branch yet must both succeed and establish tracking, landing the local HEAD on the bare remote.
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
	}
	for _, step := range steps {
		if !t.Run(step.name, step.run) {
			return
		}
	}
}

// TestUpdateRemoteBranchLeased drives UpdateRemoteBranchLeased against one bare remote whose main holds two commits pushed from one clone.
// A leased update naming the remote's current tip moves the branch backwards to the first commit.
// A lease naming the stale second commit then fails with ErrLeaseRejected and leaves the remote branch where it was.
// An invalid SHA returns ErrInvalidSHA before any git spawn, which the not-a-repository clone path would otherwise surface as a different error.
// A pre-receive hook that exits non-zero rejects a correctly leased update with an error that is not ErrLeaseRejected, so a moved remote and a refusing one stay distinguishable.
// The steps run serially in that order and share the remote; the hook step is last because the hook stays installed.
// The top-level test calls t.Parallel; no step does, because the steps share the fixture.
func TestUpdateRemoteBranchLeased(t *testing.T) {
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

// TestPush_NoRemoteConfigured_SurfacesGitError covers a repo with zero remotes configured at all (not merely no upstream tracking branch — no "origin" either).
// Push and PushCoalesced must not swallow this into a synthetic message: the wrapped error must still carry git's own stderr, matching Push's documented "any other push failure returns an error including git's stderr" contract.
// PushCoalesced reaches the same path because HasUnpushed treats the missing upstream as "unpushed" regardless of the missing remote, so it proceeds to the same pushWithRebaseRetry and the lock machinery must not mask the error.
func TestPush_NoRemoteConfigured_SurfacesGitError(t *testing.T) {
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
		})
	}
}

// TestPushCoalescedChildProcess is not a standalone test: it is the child body TestPushCoalesced's cross-process step re-execs from the test binary with GITREPO_TEST_PUSH_DIR set, so the single-pusher lock is exercised across genuinely separate OS processes.
// It skips in a normal run (env unset).
func TestPushCoalescedChildProcess(t *testing.T) {
	dir := os.Getenv("GITREPO_TEST_PUSH_DIR")
	if dir == "" {
		t.Skip("child-process body; runs only re-exec'd with GITREPO_TEST_PUSH_DIR set")
	}
	if err := gitrepo.New(dir).PushCoalesced(); err != nil {
		t.Fatalf("PushCoalesced() in child process error = %v; want nil", err)
	}
}

// TestLockHolderChildProcess is not a standalone test: it is the child body TestPushCoalesced's crashed-lock-holder step re-execs.
// It acquires the repo's push lock, prints a marker so the parent knows the lock is held, and blocks until the parent SIGKILLs it.
// It skips in a normal run.
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

// TestPushCoalesced drives PushCoalesced's single-pusher lock (one clone, one .gitrepo-push.lock) through goroutines, genuinely separate OS processes and a crashed lock holder.
// Several commits land locally before any push runs, so concurrent PushCoalesced calls must coalesce them rather than each pushing its own subset, serializing on the lock: the second finds nothing unpushed once it acquires the lock and returns immediately.
// The cross-process step proves the lock holds across real OS processes, not just goroutines sharing one process: several re-exec'd child processes call PushCoalesced against the same clone concurrently, and every commit must land on the bare remote with all children succeeding;
// synchronization is the lock itself — no timing assumptions.
// The crash step proves the push lock does not wedge the repo when its holder dies without releasing: a child process acquires .gitrepo-push.lock (confirmed genuinely held cross-process via a failed TryAcquire), is SIGKILLed mid-hold, and a fresh PushCoalesced must then complete — the OS releases a flock on process death — and push the pending commit.
// The steps run serially in that order and share the clone and the remote: each commits locally and leaves everything pushed for the next.
// The top-level test calls t.Parallel; no step does, because the steps share the fixture.
func TestPushCoalesced(t *testing.T) {
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

		// The bare remote must have received every commit, landing exactly at the local HEAD captured before the concurrent pushes ran.
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

// pushFromOtherClone clones bareRemote into a fresh directory under container, commits there and pushes, advancing the remote past every other clone.
func pushFromOtherClone(t *testing.T, container, bareRemote string) {
	t.Helper()

	otherPath, other := cloneFromBare(t, container, "other", bareRemote)
	writeFile(t, otherPath, "other.txt", "from other")
	commitAll(t, otherPath, "commit from other")
	if err := other.Push(); err != nil {
		t.Fatalf("Push() (other clone) error = %v; want nil", err)
	}
}
