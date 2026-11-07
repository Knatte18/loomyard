// gogit_test.go tests the go-git backed reads of Repo from inside package gitrepo.
// The gitrepo_test package is separate, so this file cannot reach its types even though both share the directory.

package gitrepo

import (
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-git/go-git/v5/plumbing"

	"github.com/Knatte18/loomyard/internal/fslink"
	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/gitrepo/internal/gitoracle"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// forceGoGitFinalizersOnCleanup forces the garbage collector to run at cleanup.
// It finalizes unclosed go-git file handles before t.TempDir() cleanup runs.
func forceGoGitFinalizersOnCleanup(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		runtime.GC()
		runtime.GC()
		time.Sleep(50 * time.Millisecond)
	})
}

// newStandaloneRepo creates a fresh git repository on main with one commit.
func newStandaloneRepo(t *testing.T) (dir string, repo *Repo) {
	t.Helper()

	dir = t.TempDir()
	gitkit.MustRun(t, dir, "git", "init", "-b", "main")
	gitkit.CommitFile(t, dir, "a.txt", "hello", "init")
	return dir, New(dir)
}

// commonDirProbeRef is the ref that newLinkedParityFixture writes from the main worktree.
// The common-dir step reads it back through the linked worktree.
// Any ref in the common directory shows the shared-common-dir read, so its name carries no significance.
const commonDirProbeRef = "refs/gitrepo-test/common-dir-probe"

// TestGoGit_NonRepoPath_ErrorsWithoutRetargetingParent asserts goGit fails on a path that is not itself a repository.
// It must not silently open an ancestor repository, the DetectDotGit hazard.
// DetectDotGit can let a fixture directory escape and open the enclosing loomyard checkout.
// `notARepo` is a real subdirectory of a real repository, so a retargeting open would succeed with the parent's HEAD.
// goGit must instead fail outright.
func TestGoGit_NonRepoPath_ErrorsWithoutRetargetingParent(t *testing.T) {
	tiergate.Require(t, 1)
	parent := t.TempDir()
	gitkit.MustRun(t, parent, "git", "init", "-b", "main")
	gitkit.CommitFile(t, parent, "a.txt", "hi", "init")

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

// TestGoGit_FailedOpen_NotCached asserts a failed open is retried, not cached.
// New does not require the checkout to exist yet,
// so a Repo built before fabricengine creates the worktree must still open once the checkout exists.
func TestGoGit_FailedOpen_NotCached(t *testing.T) {
	tiergate.Require(t, 1)
	dir := t.TempDir()
	repo := New(dir) // no checkout at dir yet

	if _, err := repo.goGit(); err == nil {
		t.Fatal("goGit() before the checkout exists error = nil; want an error")
	}

	gitkit.MustRun(t, dir, "git", "init", "-b", "main")
	gitkit.CommitFile(t, dir, "a.txt", "hi", "init")

	handle, err := repo.goGit()
	if err != nil {
		t.Fatalf("goGit() after the checkout now exists error = %v; want nil (a failed open must not be cached)", err)
	}
	if handle == nil {
		t.Fatal("goGit() after the checkout now exists returned a nil handle; want a real handle")
	}
}

// TestGoGit_StandaloneRepo drives goGit through one ordinary, non-worktree checkout.
// The first three steps share the Repo's open cache and run serially, in order.
// The concurrent step runs first, so it races the first open, and it is meaningful only under -race.
// A fabricated SHA must never be found, and it must never panic or deadlock the shared lock.
// The last two steps each build their own Repo and check that reindexing happens at most once.
// The top-level test calls t.Parallel; no step does.
func TestGoGit_StandaloneRepo(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	dir, repo := newStandaloneRepo(t)

	if !t.Run("concurrent callers", func(t *testing.T) {
		head := gitkit.RevParse(t, dir, "HEAD")
		const fabricatedSHA = "0123456789abcdef0123456789abcdef01234567"

		const goroutines = 8
		var wg sync.WaitGroup
		founds := make([]bool, goroutines)
		for i := 0; i < goroutines; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()

				sha := head
				if i%2 == 0 {
					sha = fabricatedSHA
				}
				founds[i] = repo.SHAExists(sha)
			}(i)
		}
		wg.Wait()

		for i := 0; i < goroutines; i++ {
			want := i%2 != 0
			if founds[i] != want {
				t.Errorf("goroutine %d found = %v; want %v", i, founds[i], want)
			}
		}
	}) {
		return
	}

	if !t.Run("opens cleanly and reads HEAD", func(t *testing.T) {
		handle, err := repo.goGit()
		if err != nil {
			t.Fatalf("goGit() error = %v; want nil", err)
		}
		if _, err := handle.Head(); err != nil {
			t.Fatalf("Head() on standalone repo error = %v; want nil", err)
		}
	}) {
		return
	}

	if !t.Run("successful open is cached", func(t *testing.T) {
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
	}) {
		return
	}

	if !t.Run("two readers over a stale pack index share one reindex", func(t *testing.T) {
		dir, staleRepo := newStandaloneRepo(t)
		if staleRepo.SHAExists("deadbeefdeadbeefdeadbeefdeadbeefdeadbeef") {
			t.Fatal("SHAExists(fabricated sha) = true; want false")
		}
		gitkit.CommitFile(t, dir, "b.txt", "second", "second commit")
		sha := gitkit.RevParse(t, dir, "HEAD")
		gitkit.MustRun(t, dir, "git", "repack", "-d")
		gitkit.MustRun(t, dir, "git", "prune-packed")

		const readers = 2
		var wg sync.WaitGroup
		errs := make([]error, readers)
		for i := 0; i < readers; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				_, errs[i] = staleRepo.CommitParents(sha)
			}(i)
		}
		wg.Wait()

		for i, err := range errs {
			if err != nil {
				t.Errorf("reader %d CommitParents() error = %v; want nil (the retry must recover the packed commit)", i, err)
			}
		}
		if staleRepo.reindexCount != 1 {
			t.Errorf("reindexCount = %d; want 1 (concurrent readers of one stale index share a single reindex)", staleRepo.reindexCount)
		}
	}) {
		return
	}

	t.Run("repeated reads of an absent sha reindex at most once", func(t *testing.T) {
		dir, absentRepo := newStandaloneRepo(t)
		gitkit.MustRun(t, dir, "git", "repack", "-d")
		gitkit.MustRun(t, dir, "git", "prune-packed")

		for i := 0; i < 3; i++ {
			if absentRepo.SHAExists("deadbeefdeadbeefdeadbeefdeadbeefdeadbeef") {
				t.Fatal("SHAExists(fabricated sha) = true; want false")
			}
		}
		if absentRepo.reindexCount > 1 {
			t.Errorf("reindexCount = %d after repeated absent reads; want at most 1 while the pack set is unchanged", absentRepo.reindexCount)
		}
	})
}

// containsString reports whether haystack contains needle.
// Tests use it to find a path in a `ChangedFilesSince` result without depending on list order.
func containsString(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

// linkedParityFixture holds the paths and fixed SHAs that newLinkedParityFixture builds.
// The fixture has a bare remote; its main worktree's common dir carries commonDirProbeRef at sharedSHA.
// Its linked worktree is on branch "feature", whose upstream another clone advances past its tip.
// Production runs only in linked worktrees,
// so the parity reads run against one rather than against a standalone `git init` repository.
type linkedParityFixture struct {
	mainDir   string
	linkedDir string // the worktree's real path — never the junction
	// sharedSHA is the commit before main and feature diverge, in the history of both worktrees.
	sharedSHA string
	// linkedSHA is the linked worktree's own HEAD commit on branch "feature".
	// It is strictly ahead of sharedSHA, and after the fixture's final fetch it is strictly behind its upstream.
	linkedSHA string
}

// newLinkedParityFixture builds the linked-worktree fixture that the parity checks run against:
// the main worktree has one commit, pushed to a bare remote with tracking;
// a linked worktree on branch "feature" has its own commit, also pushed with tracking;
// then a second clone advances "feature" on the remote,
// and the linked worktree fetches it without merging.
// Its own branch therefore never gains a commit the upstream lacks.
// That gives the "strictly behind" state that HasUnpushed must report.
func newLinkedParityFixture(t *testing.T) *linkedParityFixture {
	t.Helper()

	container := t.TempDir()
	bare := filepath.Join(container, "remote.git")
	gitkit.MustRun(t, container, "git", "init", "--bare", "-b", "main", bare)

	mainDir := filepath.Join(container, "main")
	if err := os.Mkdir(mainDir, 0o755); err != nil {
		t.Fatalf("mkdir main: %v", err)
	}
	gitkit.MustRun(t, mainDir, "git", "init", "-b", "main")
	gitkit.CommitFile(t, mainDir, "base.txt", "base", "base commit")
	mainRepo := New(mainDir)
	sharedSHA, err := mainRepo.CurrentSHA()
	if err != nil {
		t.Fatalf("CurrentSHA() (shared) error = %v", err)
	}
	gitkit.MustRun(t, mainDir, "git", "update-ref", commonDirProbeRef, sharedSHA)
	gitkit.MustRun(t, mainDir, "git", "remote", "add", "origin", bare)
	gitkit.MustRun(t, mainDir, "git", "push", "-u", "origin", "main")

	linkedDir := filepath.Join(container, "linked")
	gitkit.MustRun(t, mainDir, "git", "worktree", "add", "-b", "feature", linkedDir)
	gitkit.CommitFile(t, linkedDir, "feature-only.txt", "feature advances", "feature commit")
	linkedRepo := New(linkedDir)
	linkedSHA, err := linkedRepo.CurrentSHA()
	if err != nil {
		t.Fatalf("CurrentSHA() (linked) error = %v", err)
	}
	gitkit.MustRun(t, linkedDir, "git", "push", "-u", "origin", "feature")

	otherClone := filepath.Join(container, "other-clone")
	gitkit.MustRun(t, container, "git", "clone", "-b", "feature", bare, otherClone)
	gitkit.CommitFile(t, otherClone, "elsewhere.txt", "elsewhere advances", "elsewhere commit")
	gitkit.MustRun(t, otherClone, "git", "push")
	gitkit.MustRun(t, linkedDir, "git", "fetch", "origin")

	return &linkedParityFixture{
		mainDir:   mainDir,
		linkedDir: linkedDir,
		sharedSHA: sharedSHA,
		linkedSHA: linkedSHA,
	}
}

// runLinkedWorktreeParityChecks runs the read-side parity checks against the linked worktree fixture.
// The checks are shared by a direct run and a junction-reached run.
// They cover CurrentSHA, CurrentBranch (on-branch), SHAExists, and ChangedFilesSince.
// HasUnpushed is CLI-bound and carries no go-git parity case here for that reason.
// dir is the path under test: the worktree's real path, or a junction pointing at it.
func runLinkedWorktreeParityChecks(t *testing.T, dir string, fx *linkedParityFixture) {
	t.Helper()

	repo := New(dir)

	t.Run("CurrentSHA", func(t *testing.T) {
		oracleGot, oracleErr := gitoracle.CurrentSHA(t, dir)
		if oracleErr != nil {
			t.Fatalf("gitoracle.CurrentSHA() error = %v", oracleErr)
		}
		implGot, implErr := repo.CurrentSHA()
		if implErr != nil {
			t.Fatalf("CurrentSHA() error = %v", implErr)
		}
		if oracleGot != implGot {
			t.Errorf("CurrentSHA() parity mismatch: oracle = %q; gitrepo = %q", oracleGot, implGot)
		}
		if implGot != fx.linkedSHA {
			t.Errorf("CurrentSHA() = %q, want %q", implGot, fx.linkedSHA)
		}
	})

	t.Run("CurrentBranch_OnBranch", func(t *testing.T) {
		oracleGot, oracleErr := gitoracle.CurrentBranch(t, dir)
		if oracleErr != nil {
			t.Fatalf("gitoracle.CurrentBranch() error = %v", oracleErr)
		}
		implGot, implErr := repo.CurrentBranch()
		if implErr != nil {
			t.Fatalf("CurrentBranch() error = %v", implErr)
		}
		if oracleGot != implGot {
			t.Errorf("CurrentBranch() parity mismatch: oracle = %q; gitrepo = %q", oracleGot, implGot)
		}
		if implGot != "feature" {
			t.Errorf("CurrentBranch() = %q, want %q", implGot, "feature")
		}
	})

	t.Run("SHAExists", func(t *testing.T) {
		oracleGot := gitoracle.SHAExists(t, dir, fx.sharedSHA)
		implGot := repo.SHAExists(fx.sharedSHA)
		if oracleGot != implGot {
			t.Errorf("SHAExists() parity mismatch: oracle = %v; gitrepo = %v", oracleGot, implGot)
		}
		if !implGot {
			t.Errorf("SHAExists(%q) (a commit made in the OTHER worktree) = %v, want true", fx.sharedSHA, implGot)
		}
	})

	t.Run("ChangedFilesSince", func(t *testing.T) {
		oracleFiles, oracleErr := gitoracle.ChangedFilesSince(t, dir, fx.sharedSHA)
		if oracleErr != nil {
			t.Fatalf("gitoracle.ChangedFilesSince() error = %v", oracleErr)
		}
		implFiles, implErr := repo.ChangedFilesSince(fx.sharedSHA)
		if implErr != nil {
			t.Fatalf("ChangedFilesSince() error = %v", implErr)
		}

		oracleSorted := append([]string(nil), oracleFiles...)
		implSorted := append([]string(nil), implFiles...)
		sort.Strings(oracleSorted)
		sort.Strings(implSorted)
		if len(oracleSorted) != len(implSorted) {
			t.Fatalf("ChangedFilesSince() parity mismatch: oracle = %v; gitrepo = %v", oracleSorted, implSorted)
		}
		for i := range oracleSorted {
			if oracleSorted[i] != implSorted[i] {
				t.Errorf("ChangedFilesSince() parity mismatch: oracle = %v; gitrepo = %v", oracleSorted, implSorted)
			}
		}
		if !containsString(implSorted, "feature-only.txt") {
			t.Errorf("ChangedFilesSince() = %v, want it to contain %q", implSorted, "feature-only.txt")
		}
	})

}

// TestLinkedWorktree_Parity runs every read-side parity case this package covers.
// It first checks goGit's reads of state that lives in the common dir.
// It then runs the parity checks directly.
// It runs them again, reached only through a junction (`internal/fslink.CreateDirLink`).
// That indirection is how lym addresses these directories in production.
// The CurrentBranch detached-HEAD case runs after them because it mutates the worktree's checked-out ref state.
// The worktree-remove case runs last because it deletes the worktree.
// The standalone `git init` fixtures used elsewhere in this package cannot substitute for any of this:
// the linked worktree is the only topology production runs in.
// The steps run serially in that order.
// The top-level test calls t.Parallel; no step does, because the steps share the fixture's worktree.
func TestLinkedWorktree_Parity(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	fx := newLinkedParityFixture(t)
	forceGoGitFinalizersOnCleanup(t)

	// This step is the sharpest smoke test.
	// It resolves an object made in the other worktree, and it reads a ref set from that worktree.
	// Both go through the linked worktree's own goGit handle.
	// A wrong open (plain PlainOpen) fails both silently.
	// The commit resolves as "object not found", and the ref reads as absent.
	// That is why CurrentBranch must not stand in for this check.
	// It is a per-worktree HEAD read, so it passes on a broken handle too.
	if !t.Run("ReadsCommonDirState", func(t *testing.T) {
		handle, err := New(fx.linkedDir).goGit()
		if err != nil {
			t.Fatalf("goGit() on linked worktree error = %v; want nil", err)
		}

		commit, err := handle.CommitObject(plumbing.NewHash(fx.sharedSHA))
		if err != nil {
			t.Fatalf("CommitObject(%s) (a commit made in the OTHER worktree) via linked handle error = %v; want nil", fx.sharedSHA, err)
		}
		if got := commit.Hash.String(); got != fx.sharedSHA {
			t.Errorf("CommitObject().Hash = %s; want %s", got, fx.sharedSHA)
		}

		ref, err := handle.Reference(plumbing.ReferenceName(commonDirProbeRef), true)
		if err != nil {
			t.Fatalf("Reference(%s) (set from the OTHER worktree) via linked handle error = %v; want nil", commonDirProbeRef, err)
		}
		if got := ref.Hash().String(); got != fx.sharedSHA {
			t.Errorf("Reference().Hash() = %s; want %s", got, fx.sharedSHA)
		}
	}) {
		return
	}

	if !t.Run("Direct", func(t *testing.T) {
		runLinkedWorktreeParityChecks(t, fx.linkedDir, fx)
	}) {
		return
	}

	if !t.Run("ViaJunction", func(t *testing.T) {
		junctionPath := filepath.Join(t.TempDir(), "via-junction")
		if err := fslink.CreateDirLink(junctionPath, fx.linkedDir); err != nil {
			t.Skipf("directory link creation unavailable on this platform: %v", err)
		}
		runLinkedWorktreeParityChecks(t, junctionPath, fx)
	}) {
		return
	}

	if !t.Run("CurrentBranch_Detached", func(t *testing.T) {
		repo := New(fx.linkedDir)
		gitkit.MustRun(t, fx.linkedDir, "git", "checkout", "--detach", fx.linkedSHA)

		_, oracleErr := gitoracle.CurrentBranch(t, fx.linkedDir)
		_, implErr := repo.CurrentBranch()
		if (oracleErr == nil) != (implErr == nil) {
			t.Errorf("CurrentBranch() parity mismatch on detached linked-worktree HEAD: oracle err = %v; gitrepo err = %v", oracleErr, implErr)
		}
		if implErr == nil {
			t.Error("CurrentBranch() on detached linked-worktree HEAD error = nil, want non-nil")
		}
	}) {
		return
	}

	// Holding a warmed go-git handle open must not permanently block `git worktree remove`.
	// Measured, `git worktree remove` exits 0 with KeepDescriptors at its default (false).
	// A regression here would break fabricengine's topology verbs for reasons unrelated to their own code.
	//
	// go-git's commondir resolution (repository.go's dotGitCommonDirectory) opens the linked worktree's "commondir" file.
	// It reads the common-dir path from that file and never explicitly closes it.
	// This is a real, narrow go-git resource leak.
	// It is distinct from, and much smaller than, the packfile hazard of `KeepDescriptors:true`.
	// That file object is unreachable the instant goGit's open call returns, since nothing retains it.
	// On Windows, an unclosed *os.File blocks deletion of the same path,
	// so the object is released as soon as Go's garbage collector finalizes it, exactly like any other abandoned *os.File.
	// This step forces that collection before removing the worktree:
	// it calls runtime.GC and gives the finalizer goroutine a moment to run.
	// An ordinarily-busy long-lived process (fabricengine) gets that collection "for free" from its own memory churn.
	// The step then asserts removal succeeds outright, with no `--force` fallback.
	t.Run("OpenHandleDoesNotBlockWorktreeRemove", func(t *testing.T) {
		handle, err := New(fx.linkedDir).goGit()
		if err != nil {
			t.Fatalf("goGit() error = %v; want nil", err)
		}
		// Warm the handle with a real read before removal.
		if _, err := handle.Head(); err != nil {
			t.Fatalf("Head() error = %v; want nil", err)
		}

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

		// Keep the handle alive across everything above.
		// The assertion then covers a live, still-cached handle that goGit's cache has not already dropped.
		runtime.KeepAlive(handle)
	})
}
