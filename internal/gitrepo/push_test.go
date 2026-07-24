//go:build integration

// push_test.go covers Push and PushCoalesced against real git repositories.
// Two fixtures are kept deliberately separate, per discussion.md: a bare
// remote with two clones exercises cross-clone rebase-retry recovery (the
// single-pusher lock cannot be exercised there, since two clones have two
// distinct lock files), while a single clone with two concurrent goroutines
// exercises PushCoalesced's lock-blocking/coalescing behavior.

package gitrepo_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitrepo"
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
