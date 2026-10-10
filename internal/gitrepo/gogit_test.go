//go:build integration

// gogit_test.go covers goGit's open/cache behaviour and readGoGit's concurrent-safety and reindex counting from inside package gitrepo, so
// it can reach Repo's unexported fields and methods (goGitMu, goGitRepo,
// goGitOK, goGit, reindexCount) directly — the package's only other
// internal test file is the untagged keyvalidation_test.go; every
// git-spawning file before this one lived in the external gitrepo_test
// package. It is reached by the existing TestMain in testmain_test.go
// automatically, since one TestMain covers both packages of a test binary.
//
// This file builds its own minimal linked-worktree fixtures rather than a
// package gitrepo_test one: package gitrepo_test is a different Go package
// from this file's package gitrepo, and a gitrepo_test-declared type would be
// structurally unreachable from here despite living in the same directory.
// An earlier gitrepo_test fixture (fixtures_test.go's linkedWorktreeFixture)
// was deleted as dead code once every linked-worktree parity case — exported
// and unexported alike — ended up covered here instead; see
// 01-gogit-handle.md card 3's Round 2 fix note.

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
)

// forceGoGitFinalizersOnCleanup forces the garbage collector to run,
// finalizing unclosed go-git file handles before t.TempDir() cleanup.
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

// commonDirProbeRef is the ref name the linked-worktree fixture writes from the main worktree and the common-dir step reads back through the linked worktree — any ref living in the common dir demonstrates the shared-common-dir read, so the name itself carries no significance.
const commonDirProbeRef = "refs/gitrepo-test/common-dir-probe"

// TestGoGit_NonRepoPath_ErrorsWithoutRetargetingParent asserts goGit fails on a path that is not
// itself a repository, rather than silently opening an ancestor repository — the DetectDotGit
// hazard the probe report documents (proven there to escape a fixture directory and open this very
// loomyard checkout).
// notARepo is a real subdirectory of a real repository, so a retargeting open would succeed with
// the PARENT's HEAD;
// goGit must instead fail outright.
func TestGoGit_NonRepoPath_ErrorsWithoutRetargetingParent(t *testing.T) {
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

// TestGoGit_FailedOpen_NotCached asserts a failed open is retried, not cached: New's documented
// posture is that the checkout need not exist yet, so a Repo constructed before fabricengine
// creates the worktree at that path must still succeed once the checkout exists.
func TestGoGit_FailedOpen_NotCached(t *testing.T) {
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
// Several goroutines first drive SHAExists at once against the shared Repo — meaningful only under -race — exercising both the found path (a real commit) and the not-found path (a fabricated SHA, which must never actually be found and must never panic or deadlock the shared lock).
// A single caller then opens the handle and reads HEAD, and a second call returns the identical cached *git.Repository pointer.
// The last two steps each build their own Repo: two goroutines reading one Repo over a stale pack index after a repack both succeed with exactly one reindex, and repeated reads of an absent sha over an unchanged pack set reindex at most once.
// The first three steps run serially in that order and share the Repo's open cache: the concurrent step runs first so it races the first open.
// The top-level test calls t.Parallel; no step does.
func TestGoGit_StandaloneRepo(t *testing.T) {
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

// containsString reports whether haystack contains needle, used to assert a
// specific path is present in a ChangedFilesSince result without depending on
// list order.
func containsString(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

// linkedParityFixture holds the paths and fixed SHAs newLinkedParityFixture builds: a bare remote, a main worktree whose common dir carries commonDirProbeRef at sharedSHA, and a linked worktree on branch "feature" whose upstream another clone advances past its own tip — the shared-common-dir topology needed to exercise every read this file's oracle covers against a linked worktree rather than a standalone `git init` fixture, per the Shared Decision that the linked worktree is the only topology production runs in.
type linkedParityFixture struct {
	mainDir   string
	linkedDir string // the worktree's real path — never the junction
	// sharedSHA is the commit before main and feature diverge, common to
	// both worktrees' history.
	sharedSHA string
	// linkedSHA is the linked worktree's own HEAD commit on branch "feature",
	// strictly ahead of sharedSHA and, after the fixture's final fetch,
	// strictly behind the upstream another clone has advanced past it.
	linkedSHA string
}

// newLinkedParityFixture builds the fixture linkedParityFixture describes:
// main worktree with one commit, pushed to a bare remote with tracking; a
// linked worktree on branch "feature" with its own commit, also pushed with
// tracking; then a second clone advances "feature" further on the remote and
// the linked worktree fetches (without merging) — its own branch never gains
// a commit the upstream lacks, giving the "strictly behind" HasUnpushed
// state the naive single-hash exclusion gets wrong.
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

// runLinkedWorktreeParityChecks runs the read-side parity checks shared by
// both a direct and a junction-reached run against the linked worktree
// fixture: CurrentSHA, CurrentBranch (on-branch), SHAExists, and
// ChangedFilesSince. HasUnpushed is CLI-bound (see push.go's card-21
// reversal doc) and carries no go-git parity case here for that reason. dir
// is the path under test — the worktree's real path, or a junction pointing
// at it.
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

// TestLinkedWorktree_Parity drives the linked-worktree fixture through every read-side parity case this package covers: goGit's reads of state living in the common dir, then the parity checks directly, and again reached only through a junction (internal/fslink.CreateDirLink), since that indirection is how lyx addresses these directories in production.
// The CurrentBranch detached-HEAD case runs after them because it mutates the worktree's checked-out ref state, and the worktree-remove case runs last because it deletes the worktree.
// The standalone `git init` fixtures used elsewhere in this package cannot substitute for any of this: the linked worktree is the only topology production runs in.
// The steps run serially in that order and share the fixture's worktree.
// The top-level test calls t.Parallel; no step does, because the steps share the fixture.
func TestLinkedWorktree_Parity(t *testing.T) {
	t.Parallel()

	fx := newLinkedParityFixture(t)
	forceGoGitFinalizersOnCleanup(t)

	// The sharpest smoke test the probe report calls for: it resolves an object made in the OTHER worktree and reads a ref set from the OTHER worktree, both via the linked worktree's own goGit handle.
	// Under a wrong open (plain PlainOpen) both fail silently — the commit resolves as "object not found" and the ref reads as absent — which is exactly why CurrentBranch (an unresolved, per-worktree HEAD read that passes on a broken handle too) must never be used as this test.
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

	// Holding a warmed go-git handle open must not permanently block `git worktree remove` — measured, per the probe report, to return exit 0 with KeepDescriptors at its default (false);
	// a regression here would break fabricengine's topology verbs for reasons unrelated to their own code.
	//
	// go-git's own commondir resolution (repository.go's dotGitCommonDirectory) opens the linked worktree's "commondir" file to read the common-dir path and never explicitly closes it — a real, narrow go-git resource leak, distinct from and much smaller than the KeepDescriptors:true packfile hazard the probe report separately measures.
	// That file object is unreachable the instant goGit's open call returns (nothing retains it), so on Windows — where an unclosed *os.File blocks deletion of the same path — it is released as soon as Go's garbage collector finalizes it, exactly like any other abandoned *os.File.
	// This step forces that collection (runtime.GC, with the finalizer goroutine given a moment to run) before removing the worktree, which is what an ordinarily-busy long-lived process (fabricengine) gets "for free" from its own memory churn;
	// it then asserts removal succeeds outright, with no `--force` fallback, matching the probe's own measurement.
	t.Run("OpenHandleDoesNotBlockWorktreeRemove", func(t *testing.T) {
		handle, err := New(fx.linkedDir).goGit()
		if err != nil {
			t.Fatalf("goGit() error = %v; want nil", err)
		}
		// Warm the handle with a real read before removal, matching the probe's methodology.
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

		// Keep the handle alive across everything above so the assertion proves something about a live, still-cached handle, not one goGit's cache had already dropped.
		runtime.KeepAlive(handle)
	})
}
