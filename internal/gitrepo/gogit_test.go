//go:build integration

// gogit_test.go covers goGit's open/cache behaviour and
// lookupObjectRetrying's concurrent-safety from inside package gitrepo, so
// it can reach Repo's unexported fields and methods (goGitMu, goGitRepo,
// goGitOK, goGit, lookupObjectRetrying) directly — the package's only other
// internal test file is the untagged keyvalidation_test.go; every
// git-spawning file before this one lived in the external gitrepo_test
// package. It is reached by the existing TestMain in testmain_test.go
// automatically, since one TestMain covers both packages of a test binary.
//
// This file builds its own minimal fixtures rather than reusing
// fixtures_test.go's linkedWorktreeFixture: that type lives in package
// gitrepo_test, a different Go package from this file's package gitrepo,
// and is structurally unreachable from here despite living in the same
// directory.

package gitrepo

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/lyxtest"
)

// forceGoGitFinalizersOnCleanup registers a t.Cleanup (run before
// t.TempDir()'s own removal, since Cleanups run in last-registered-first
// order and TempDir's is always registered earlier, inside the fixture
// builder) that forces Go's garbage collector to run and gives its
// finalizer goroutine a moment to execute. go-git's commondir resolution
// (repository.go's dotGitCommonDirectory) opens the linked worktree's
// "commondir" file and never explicitly closes it; that *os.File is
// unreachable the instant the open call returns, so on Windows — where an
// unclosed file blocks deletion of the same path — it is released as soon
// as the garbage collector finalizes it. Without this, t.TempDir()'s own
// cleanup can fail to remove a fixture that opened a linked-worktree
// handle. See TestGoGit_OpenHandleDoesNotBlockWorktreeRemove's doc for the
// same mechanism spelled out in full.
func forceGoGitFinalizersOnCleanup(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		runtime.GC()
		runtime.GC()
		time.Sleep(50 * time.Millisecond)
	})
}

// writeAndCommit writes name under dir with content and commits it directly
// via the git CLI, bypassing goGit — used only to build fixture history.
func writeAndCommit(t *testing.T, dir, name, content, message string) {
	t.Helper()

	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	lyxtest.MustRun(t, dir, "git", "add", name)
	lyxtest.MustRun(t, dir, "git", "commit", "-m", message)
}

// newStandaloneRepo creates a fresh, non-worktree git repository on branch
// main under a temp directory with one commit, and returns both the raw
// path (for direct git calls) and the *Repo wrapping it.
func newStandaloneRepo(t *testing.T) (dir string, repo *Repo) {
	t.Helper()

	dir = t.TempDir()
	lyxtest.MustRun(t, dir, "git", "init", "-b", "main")
	writeAndCommit(t, dir, "a.txt", "hello", "init")
	return dir, New(dir)
}

// gogitLinkedFixture is this file's own minimal linked-worktree fixture,
// built independently from fixtures_test.go's identically-shaped
// linkedWorktreeFixture (package gitrepo_test, unreachable from here). It
// gives main and linked different branches and different HEAD commits, and
// records a ref set from main so the linked worktree's handle can be
// asserted to see it — refs/loomyard/snapshot/*-style refs live in the
// shared common dir, while HEAD is per-worktree.
type gogitLinkedFixture struct {
	mainDir    string
	linkedDir  string
	linkedRepo *Repo
	// commonRefSHA is the value a ref set from the main worktree resolves
	// to, read back through the linked worktree — the shared-common-dir
	// case a wrong open (PlainOpen, without EnableDotGitCommonDir) reports
	// as absent, per .scratch/gogit-worktree-probe-report.md.
	commonRefSHA string
}

// commonSnapshotRef is the ref name gogitLinkedFixture writes from the main
// worktree and gogit_test.go's linked-worktree coverage reads back.
const commonSnapshotRef = "refs/loomyard/snapshot/gogittest"

func newGogitLinkedFixture(t *testing.T) *gogitLinkedFixture {
	t.Helper()

	container := t.TempDir()
	mainDir := filepath.Join(container, "main")
	if err := os.Mkdir(mainDir, 0o755); err != nil {
		t.Fatalf("mkdir main: %v", err)
	}
	lyxtest.MustRun(t, mainDir, "git", "init", "-b", "main")
	writeAndCommit(t, mainDir, "base.txt", "base", "base commit")

	mainRepo := New(mainDir)
	mainSHA, err := mainRepo.CurrentSHA()
	if err != nil {
		t.Fatalf("CurrentSHA() (main) error = %v", err)
	}
	lyxtest.MustRun(t, mainDir, "git", "update-ref", commonSnapshotRef, mainSHA)

	linkedDir := filepath.Join(container, "linked")
	lyxtest.MustRun(t, mainDir, "git", "worktree", "add", "-b", "feature", linkedDir)
	writeAndCommit(t, linkedDir, "feature.txt", "feature", "feature commit")

	return &gogitLinkedFixture{
		mainDir:      mainDir,
		linkedDir:    linkedDir,
		linkedRepo:   New(linkedDir),
		commonRefSHA: mainSHA,
	}
}

// TestGoGit_SucceedsOnStandaloneRepo asserts goGit opens cleanly on an
// ordinary, non-worktree checkout and the handle can read HEAD.
func TestGoGit_SucceedsOnStandaloneRepo(t *testing.T) {
	_, repo := newStandaloneRepo(t)

	handle, err := repo.goGit()
	if err != nil {
		t.Fatalf("goGit() error = %v; want nil", err)
	}
	if _, err := handle.Head(); err != nil {
		t.Fatalf("Head() on standalone repo error = %v; want nil", err)
	}
}

// TestGoGit_SucceedsOnLinkedWorktree_ReadsCommonDirState is the sharpest
// smoke test the probe report calls for: it resolves an object made in the
// OTHER worktree and reads a ref set from the OTHER worktree, both via the
// linked worktree's own goGit handle. Under a wrong open (plain PlainOpen)
// both fail silently — the commit resolves as "object not found" and the
// ref reads as absent — which is exactly why CurrentBranch (an unresolved,
// per-worktree HEAD read that passes on a broken handle too) must never be
// used as this test.
func TestGoGit_SucceedsOnLinkedWorktree_ReadsCommonDirState(t *testing.T) {
	fx := newGogitLinkedFixture(t)
	forceGoGitFinalizersOnCleanup(t)

	handle, err := fx.linkedRepo.goGit()
	if err != nil {
		t.Fatalf("goGit() on linked worktree error = %v; want nil", err)
	}

	commit, err := handle.CommitObject(plumbing.NewHash(fx.commonRefSHA))
	if err != nil {
		t.Fatalf("CommitObject(%s) (a commit made in the OTHER worktree) via linked handle error = %v; want nil", fx.commonRefSHA, err)
	}
	if got := commit.Hash.String(); got != fx.commonRefSHA {
		t.Errorf("CommitObject().Hash = %s; want %s", got, fx.commonRefSHA)
	}

	ref, err := handle.Reference(plumbing.ReferenceName(commonSnapshotRef), true)
	if err != nil {
		t.Fatalf("Reference(%s) (set from the OTHER worktree) via linked handle error = %v; want nil", commonSnapshotRef, err)
	}
	if got := ref.Hash().String(); got != fx.commonRefSHA {
		t.Errorf("Reference().Hash() = %s; want %s", got, fx.commonRefSHA)
	}
}

// TestGoGit_NonRepoPath_ErrorsWithoutRetargetingParent asserts goGit fails
// on a path that is not itself a repository, rather than silently opening
// an ancestor repository — the DetectDotGit hazard the probe report
// documents (proven there to escape a fixture directory and open this very
// loomyard checkout). notARepo is a real subdirectory of a real repository,
// so a retargeting open would succeed with the PARENT's HEAD; goGit must
// instead fail outright.
func TestGoGit_NonRepoPath_ErrorsWithoutRetargetingParent(t *testing.T) {
	parent := t.TempDir()
	lyxtest.MustRun(t, parent, "git", "init", "-b", "main")
	writeAndCommit(t, parent, "a.txt", "hi", "init")

	notARepo := filepath.Join(parent, "subdir")
	if err := os.Mkdir(notARepo, 0o755); err != nil {
		t.Fatalf("mkdir subdir: %v", err)
	}

	repo := New(notARepo)
	handle, err := repo.goGit()
	if err == nil {
		t.Fatal("goGit() on a non-repository path error = nil; want an error (must not silently retarget the parent repository)")
	}
	if handle != nil {
		t.Errorf("goGit() on failure returned a non-nil handle = %v; want nil", handle)
	}
	if !strings.Contains(err.Error(), "gitrepo: open go-git handle") {
		t.Errorf("goGit() error = %q; want it wrapped with the gitrepo-owned prefix naming this package", err.Error())
	}
}

// TestGoGit_FailedOpen_NotCached asserts a failed open is retried, not
// cached: New's documented posture is that the checkout need not exist yet,
// so a Repo constructed before fabricengine creates the worktree at that
// path must still succeed once the checkout exists.
func TestGoGit_FailedOpen_NotCached(t *testing.T) {
	dir := t.TempDir()
	repo := New(dir) // no checkout at dir yet

	if _, err := repo.goGit(); err == nil {
		t.Fatal("goGit() before the checkout exists error = nil; want an error")
	}

	lyxtest.MustRun(t, dir, "git", "init", "-b", "main")
	writeAndCommit(t, dir, "a.txt", "hi", "init")

	handle, err := repo.goGit()
	if err != nil {
		t.Fatalf("goGit() after the checkout now exists error = %v; want nil (a failed open must not be cached)", err)
	}
	if handle == nil {
		t.Fatal("goGit() after the checkout now exists returned a nil handle; want a real handle")
	}
}

// TestGoGit_SuccessfulOpen_IsCached asserts a successful open is cached:
// two calls on the same Repo return the identical *git.Repository pointer.
func TestGoGit_SuccessfulOpen_IsCached(t *testing.T) {
	_, repo := newStandaloneRepo(t)

	first, err := repo.goGit()
	if err != nil {
		t.Fatalf("goGit() (first call) error = %v; want nil", err)
	}
	second, err := repo.goGit()
	if err != nil {
		t.Fatalf("goGit() (second call) error = %v; want nil", err)
	}
	if first != second {
		t.Errorf("goGit() returned different handles across two calls (%p vs %p); want the same cached pointer", first, second)
	}
}

// TestGoGit_ConcurrentCallers drives several goroutines through goGit and
// lookupObjectRetrying at once against one shared Repo — meaningful only
// under -race, which this batch's verify: always enables. It exercises both
// the found path (a real commit) and the not-found-then-gated-reindex path
// (a fabricated SHA, which must never actually be found and must never
// panic or deadlock the shared lock).
func TestGoGit_ConcurrentCallers(t *testing.T) {
	_, repo := newStandaloneRepo(t)

	head, err := repo.CurrentSHA()
	if err != nil {
		t.Fatalf("CurrentSHA() error = %v", err)
	}
	const fabricatedSHA = "0123456789abcdef0123456789abcdef01234567"

	const goroutines = 8
	var wg sync.WaitGroup
	errs := make([]error, goroutines)
	founds := make([]bool, goroutines)
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()

			handle, err := repo.goGit()
			if err != nil {
				errs[i] = err
				return
			}

			sha := head
			if i%2 == 0 {
				sha = fabricatedSHA
			}
			commit, lookupErr := lookupObjectRetrying(repo, handle, func() (*object.Commit, error) {
				return handle.CommitObject(plumbing.NewHash(sha))
			})
			founds[i] = lookupErr == nil && commit != nil
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("goroutine %d goGit() error = %v; want nil", i, err)
		}
	}
	for i := 0; i < goroutines; i++ {
		want := i%2 != 0
		if founds[i] != want {
			t.Errorf("goroutine %d found = %v; want %v", i, founds[i], want)
		}
	}
}

// TestGoGit_OpenHandleDoesNotBlockWorktreeRemove asserts holding a warmed
// go-git handle open does not permanently block `git worktree remove` —
// measured, per the probe report, to return exit 0 with KeepDescriptors at
// its default (false); a regression here would break fabricengine's
// topology verbs for reasons unrelated to their own code.
//
// go-git's own commondir resolution (repository.go's dotGitCommonDirectory)
// opens the linked worktree's "commondir" file to read the common-dir path
// and never explicitly closes it — a real, narrow go-git resource leak,
// distinct from and much smaller than the KeepDescriptors:true packfile
// hazard the probe report separately measures. That file object is
// unreachable the instant goGit's open call returns (nothing retains it),
// so on Windows — where an unclosed *os.File blocks deletion of the same
// path — it is released as soon as Go's garbage collector finalizes it,
// exactly like any other abandoned *os.File. This test forces that
// collection (runtime.GC, with the finalizer goroutine given a moment to
// run) before removing the worktree, which is what an ordinarily-busy
// long-lived process (fabricengine) gets "for free" from its own memory
// churn; it then asserts removal succeeds outright, with no `--force`
// fallback, matching the probe's own measurement.
func TestGoGit_OpenHandleDoesNotBlockWorktreeRemove(t *testing.T) {
	fx := newGogitLinkedFixture(t)

	handle, err := fx.linkedRepo.goGit()
	if err != nil {
		t.Fatalf("goGit() error = %v; want nil", err)
	}
	// Warm the handle with a real read before removal, matching the probe's
	// methodology.
	if _, err := handle.Head(); err != nil {
		t.Fatalf("Head() error = %v; want nil", err)
	}

	// Give the finalizer goroutine a chance to close goGit's now-unreachable
	// commondir file object before attempting removal — see the doc above.
	runtime.GC()
	runtime.GC()
	time.Sleep(50 * time.Millisecond)

	_, stderr, code, err := gitexec.RunGit([]string{"worktree", "remove", fx.linkedDir}, fx.mainDir)
	if err != nil {
		t.Fatalf("git worktree remove spawn error = %v", err)
	}
	if code != 0 {
		t.Fatalf("git worktree remove exited %d: %s; want 0 (an open go-git handle must not block removal)", code, stderr)
	}

	// Keep the handle alive across everything above so the assertion proves
	// something about a live, still-cached handle, not one goGit's cache
	// had already dropped.
	runtime.KeepAlive(handle)
}
