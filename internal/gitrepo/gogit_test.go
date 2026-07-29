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
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/Knatte18/loomyard/internal/fslink"
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
// built independently because an equivalent package gitrepo_test fixture
// would be unreachable from here (see this file's header comment). It
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

// oracleRemoteName reimplements remoteName directly on `git symbolic-ref` and
// `git config --get branch.<name>.remote`, falling back to "origin" on any
// failure at either step. Duplicated from oracle_test.go's identically-named
// function rather than imported, because that one lives in package
// gitrepo_test (a different Go package, structurally unreachable from this
// file's package gitrepo) — the same package-boundary reason this file
// builds its own linked-worktree fixtures rather than a gitrepo_test one.
func oracleRemoteName(t *testing.T, dir string) string {
	t.Helper()

	stdout, _, code, err := gitexec.RunGit([]string{"symbolic-ref", "--short", "HEAD"}, dir)
	if err != nil || code != 0 {
		return "origin"
	}
	branch := strings.TrimSpace(stdout)

	stdout, _, code, err = gitexec.RunGit([]string{"config", "--get", "branch." + branch + ".remote"}, dir)
	if err != nil || code != 0 {
		return "origin"
	}
	remote := strings.TrimSpace(stdout)
	if remote == "" {
		return "origin"
	}
	return remote
}

// oracleIsStrictDescendant reimplements isStrictDescendant directly on
// `git merge-base --is-ancestor` plus an explicit equality check. Duplicated
// from oracle_test.go for the same package-boundary reason as
// oracleRemoteName above.
func oracleIsStrictDescendant(t *testing.T, dir, ancestor, descendant string) bool {
	t.Helper()

	if ancestor == descendant {
		return false
	}
	_, _, code, err := gitexec.RunGit([]string{"merge-base", "--is-ancestor", ancestor, descendant}, dir)
	return err == nil && code == 0
}

// TestRemoteName_Parity covers remoteName's two paths: the "origin" fallback
// when no branch.<name>.remote is configured, and the configured-remote path
// once it is — asserted directly against a git fixture (via the oracle)
// rather than any gitrepo public method, since remoteName is unexported on
// both sides and has no public gitrepo oracle counterpart.
func TestRemoteName_Parity(t *testing.T) {
	t.Run("OriginFallback", func(t *testing.T) {
		dir, repo := newStandaloneRepo(t)

		oracleGot := oracleRemoteName(t, dir)
		implGot := repo.remoteName()
		if oracleGot != implGot {
			t.Errorf("remoteName() parity mismatch: oracle = %q; gitrepo = %q", oracleGot, implGot)
		}
		if implGot != "origin" {
			t.Errorf("remoteName() = %q, want %q", implGot, "origin")
		}
	})

	t.Run("ConfiguredRemote", func(t *testing.T) {
		// Track a remote deliberately not named "origin" so this subtest
		// actually exercises the branch.<name>.remote config lookup rather
		// than coincidentally matching the fallback value.
		container := t.TempDir()
		bare := filepath.Join(container, "upstream.git")
		lyxtest.MustRun(t, container, "git", "init", "--bare", "-b", "main", bare)

		dir, repo := newStandaloneRepo(t)
		lyxtest.MustRun(t, dir, "git", "remote", "add", "upstream", bare)
		lyxtest.MustRun(t, dir, "git", "push", "-u", "upstream", "main")

		oracleGot := oracleRemoteName(t, dir)
		implGot := repo.remoteName()
		if oracleGot != implGot {
			t.Errorf("remoteName() parity mismatch: oracle = %q; gitrepo = %q", oracleGot, implGot)
		}
		if implGot != "upstream" {
			t.Errorf("remoteName() = %q, want %q (configured tracked remote)", implGot, "upstream")
		}
	})
}

// TestIsStrictDescendant_Parity covers isStrictDescendant across an ancestor
// commit (true), the same commit compared against itself (false — strict),
// and an unrelated orphan-branch commit sharing no history at all (false),
// asserted against both the oracle and the unexported method directly.
func TestIsStrictDescendant_Parity(t *testing.T) {
	dir, repo := newStandaloneRepo(t)
	ancestorSHA, err := repo.CurrentSHA()
	if err != nil {
		t.Fatalf("CurrentSHA() (ancestor) error = %v", err)
	}

	writeAndCommit(t, dir, "b.txt", "second", "second commit")
	descendantSHA, err := repo.CurrentSHA()
	if err != nil {
		t.Fatalf("CurrentSHA() (descendant) error = %v", err)
	}

	// An orphan branch shares no commit history with main at all, giving a
	// genuinely unrelated commit rather than merely an object missing from
	// the object store.
	lyxtest.MustRun(t, dir, "git", "checkout", "--orphan", "unrelated-branch")
	lyxtest.MustRun(t, dir, "git", "rm", "-rf", "--cached", ".")
	writeAndCommit(t, dir, "c.txt", "unrelated", "unrelated root")
	unrelatedSHA, err := repo.CurrentSHA()
	if err != nil {
		t.Fatalf("CurrentSHA() (unrelated) error = %v", err)
	}

	tests := []struct {
		name       string
		ancestor   string
		descendant string
		want       bool
	}{
		{"Ancestor", ancestorSHA, descendantSHA, true},
		{"Equal", descendantSHA, descendantSHA, false},
		{"Unrelated", unrelatedSHA, descendantSHA, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			oracleGot := oracleIsStrictDescendant(t, dir, tt.ancestor, tt.descendant)
			if oracleGot != tt.want {
				t.Errorf("oracle isStrictDescendant(%q, %q) = %v, want %v", tt.ancestor, tt.descendant, oracleGot, tt.want)
			}
			implGot := repo.isStrictDescendant(tt.ancestor, tt.descendant)
			if implGot != tt.want {
				t.Errorf("isStrictDescendant(%q, %q) = %v, want %v", tt.ancestor, tt.descendant, implGot, tt.want)
			}
		})
	}
}

// errOracleNoCommits is the oracle's own "no commits yet" sentinel, local to
// this package for the same package-boundary reason as this file's other
// duplicated oracle helpers — it is never compared for identity against
// oracle_test.go's identically-named sentinel, since the two are never used
// in the same comparison.
var errOracleNoCommits = errors.New("oracle: repository has no commits")

// oracleCurrentSHA reimplements CurrentSHA directly on `git rev-parse HEAD`.
// Duplicated from oracle_test.go for the same package-boundary reason as
// oracleRemoteName above.
func oracleCurrentSHA(t *testing.T, dir string) (string, error) {
	t.Helper()

	stdout, stderr, code, err := gitexec.RunGit([]string{"rev-parse", "HEAD"}, dir)
	if err != nil {
		return "", err
	}
	if code != 0 {
		if strings.Contains(stderr, "ambiguous argument 'HEAD'") || strings.Contains(stderr, "unknown revision") {
			return "", errOracleNoCommits
		}
		return "", fmt.Errorf("oracle: git rev-parse HEAD: %s", stderr)
	}
	return strings.TrimSpace(stdout), nil
}

// oracleCurrentBranch reimplements CurrentBranch directly on
// `git symbolic-ref --short HEAD`. Duplicated from oracle_test.go for the
// same package-boundary reason as oracleRemoteName above.
func oracleCurrentBranch(t *testing.T, dir string) (string, error) {
	t.Helper()

	stdout, stderr, code, err := gitexec.RunGit([]string{"symbolic-ref", "--short", "HEAD"}, dir)
	if err != nil {
		return "", err
	}
	if code != 0 {
		return "", fmt.Errorf("oracle: git symbolic-ref --short HEAD: %s", stderr)
	}
	return strings.TrimSpace(stdout), nil
}

// oracleSHAExists reimplements SHAExists directly on
// `git rev-parse --verify --quiet <sha>^{commit}`. Duplicated from
// oracle_test.go for the same package-boundary reason as oracleRemoteName
// above.
func oracleSHAExists(t *testing.T, dir, sha string) bool {
	t.Helper()

	_, stderr, code, err := gitexec.RunGit([]string{"rev-parse", "--verify", "--quiet", sha + "^{commit}"}, dir)
	if err != nil {
		t.Fatalf("oracle: git rev-parse --verify --quiet %s^{commit} spawn error = %v", sha, err)
	}
	switch code {
	case 0:
		return true
	case 1:
		return false
	default:
		t.Fatalf("oracle: git rev-parse --verify --quiet %s^{commit} exited %d: %s", sha, code, stderr)
		return false
	}
}

// oracleChangedFilesSince reimplements ChangedFilesSince directly on
// `git diff --name-only -z --no-renames <sha>..HEAD`. Duplicated from
// oracle_test.go for the same package-boundary reason as oracleRemoteName
// above.
func oracleChangedFilesSince(t *testing.T, dir, sha string) ([]string, error) {
	t.Helper()

	stdout, stderr, code, err := gitexec.RunGit([]string{"diff", "--name-only", "-z", "--no-renames", sha + "..HEAD"}, dir)
	if err != nil {
		return nil, err
	}
	if code != 0 {
		return nil, fmt.Errorf("oracle: git diff --name-only -z --no-renames %s..HEAD: %s", sha, stderr)
	}

	var files []string
	for _, path := range strings.Split(stdout, "\x00") {
		if path == "" {
			continue
		}
		files = append(files, path)
	}
	return files, nil
}

// oracleSnapshotSHA reimplements SnapshotSHA's local ref read directly on
// `git rev-parse --verify --quiet refs/loomyard/snapshot/<key>`. Duplicated
// from oracle_test.go for the same package-boundary reason as
// oracleRemoteName above.
func oracleSnapshotSHA(t *testing.T, dir, key string) (string, error) {
	t.Helper()

	ref := "refs/loomyard/snapshot/" + key
	stdout, stderr, code, err := gitexec.RunGit([]string{"rev-parse", "--verify", "--quiet", ref}, dir)
	if err != nil {
		return "", err
	}
	switch code {
	case 0:
		return strings.TrimSpace(stdout), nil
	case 1:
		return "", nil
	default:
		return "", fmt.Errorf("oracle: git rev-parse --verify --quiet %s: %s", ref, stderr)
	}
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

// gogitParitySnapshotKey is the snapshot key newLinkedParityFixture writes
// from the main worktree and reads back through the linked worktree (and its
// junction), exercising SnapshotSHA's ref read across the shared common dir.
const gogitParitySnapshotKey = "gogitparity"

// linkedParityFixture holds the paths and fixed SHAs newLinkedParityFixture
// builds: a bare remote, a main worktree, and a linked worktree on branch
// "feature" whose upstream another clone advances past its own tip — the
// shared-common-dir topology needed to exercise every read this batch's
// oracle covers (plus remoteName, hasUnpushed, and isStrictDescendant)
// against a linked worktree rather than a standalone `git init` fixture, per
// the Shared Decision that the linked worktree is the only topology
// production runs in.
type linkedParityFixture struct {
	mainDir   string
	linkedDir string // the worktree's real path — never the junction
	// sharedSHA is the commit before main and feature diverge, common to
	// both worktrees' history and the value refs/loomyard/snapshot/<key> is
	// set to from the main worktree.
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
// a commit the upstream lacks, giving the "strictly behind" hasUnpushed
// state the naive single-hash exclusion gets wrong.
func newLinkedParityFixture(t *testing.T) *linkedParityFixture {
	t.Helper()

	container := t.TempDir()
	bare := filepath.Join(container, "remote.git")
	lyxtest.MustRun(t, container, "git", "init", "--bare", "-b", "main", bare)

	mainDir := filepath.Join(container, "main")
	if err := os.Mkdir(mainDir, 0o755); err != nil {
		t.Fatalf("mkdir main: %v", err)
	}
	lyxtest.MustRun(t, mainDir, "git", "init", "-b", "main")
	writeAndCommit(t, mainDir, "base.txt", "base", "base commit")
	mainRepo := New(mainDir)
	sharedSHA, err := mainRepo.CurrentSHA()
	if err != nil {
		t.Fatalf("CurrentSHA() (shared) error = %v", err)
	}
	lyxtest.MustRun(t, mainDir, "git", "remote", "add", "origin", bare)
	lyxtest.MustRun(t, mainDir, "git", "push", "-u", "origin", "main")
	// refs/loomyard/snapshot/* lives in the shared common dir; setting it
	// from main and reading it back from the linked worktree is exactly the
	// split a wrong open (PlainOpen, without EnableDotGitCommonDir) fails to
	// see, per the probe report.
	lyxtest.MustRun(t, mainDir, "git", "update-ref", "refs/loomyard/snapshot/"+gogitParitySnapshotKey, sharedSHA)

	linkedDir := filepath.Join(container, "linked")
	lyxtest.MustRun(t, mainDir, "git", "worktree", "add", "-b", "feature", linkedDir)
	writeAndCommit(t, linkedDir, "feature-only.txt", "feature advances", "feature commit")
	linkedRepo := New(linkedDir)
	linkedSHA, err := linkedRepo.CurrentSHA()
	if err != nil {
		t.Fatalf("CurrentSHA() (linked) error = %v", err)
	}
	lyxtest.MustRun(t, linkedDir, "git", "push", "-u", "origin", "feature")

	otherClone := filepath.Join(container, "other-clone")
	lyxtest.MustRun(t, container, "git", "clone", "-b", "feature", bare, otherClone)
	writeAndCommit(t, otherClone, "elsewhere.txt", "elsewhere advances", "elsewhere commit")
	lyxtest.MustRun(t, otherClone, "git", "push")
	lyxtest.MustRun(t, linkedDir, "git", "fetch", "origin")

	return &linkedParityFixture{
		mainDir:   mainDir,
		linkedDir: linkedDir,
		sharedSHA: sharedSHA,
		linkedSHA: linkedSHA,
	}
}

// runLinkedWorktreeParityChecks runs the read-side parity checks shared by
// both a direct and a junction-reached run against the linked worktree
// fixture: CurrentSHA, CurrentBranch (on-branch), remoteName, SnapshotSHA's
// ref read, SHAExists, ChangedFilesSince, and isStrictDescendant. hasUnpushed
// is CLI-bound (see push.go's card-21 reversal doc) and carries no go-git
// parity case here for that reason. dir is the path under test — the
// worktree's real path, or a junction pointing at it.
func runLinkedWorktreeParityChecks(t *testing.T, dir string, fx *linkedParityFixture) {
	t.Helper()

	repo := New(dir)

	t.Run("CurrentSHA", func(t *testing.T) {
		oracleGot, oracleErr := oracleCurrentSHA(t, dir)
		if oracleErr != nil {
			t.Fatalf("oracleCurrentSHA() error = %v", oracleErr)
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
		oracleGot, oracleErr := oracleCurrentBranch(t, dir)
		if oracleErr != nil {
			t.Fatalf("oracleCurrentBranch() error = %v", oracleErr)
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

	t.Run("RemoteName", func(t *testing.T) {
		oracleGot := oracleRemoteName(t, dir)
		implGot := repo.remoteName()
		if oracleGot != implGot {
			t.Errorf("remoteName() parity mismatch: oracle = %q; gitrepo = %q", oracleGot, implGot)
		}
		if implGot != "origin" {
			t.Errorf("remoteName() = %q, want %q", implGot, "origin")
		}
	})

	t.Run("SnapshotSHA_CommonRef", func(t *testing.T) {
		oracleGot, oracleErr := oracleSnapshotSHA(t, dir, gogitParitySnapshotKey)
		if oracleErr != nil {
			t.Fatalf("oracleSnapshotSHA() error = %v", oracleErr)
		}
		implGot, implErr := repo.SnapshotSHA(gogitParitySnapshotKey)
		if implErr != nil {
			t.Fatalf("SnapshotSHA() error = %v", implErr)
		}
		if oracleGot != implGot {
			t.Errorf("SnapshotSHA() parity mismatch: oracle = %q; gitrepo = %q", oracleGot, implGot)
		}
		if implGot != fx.sharedSHA {
			t.Errorf("SnapshotSHA() = %q, want %q (set from the OTHER worktree)", implGot, fx.sharedSHA)
		}
	})

	t.Run("SHAExists", func(t *testing.T) {
		oracleGot := oracleSHAExists(t, dir, fx.sharedSHA)
		implGot := repo.SHAExists(fx.sharedSHA)
		if oracleGot != implGot {
			t.Errorf("SHAExists() parity mismatch: oracle = %v; gitrepo = %v", oracleGot, implGot)
		}
		if !implGot {
			t.Errorf("SHAExists(%q) (a commit made in the OTHER worktree) = %v, want true", fx.sharedSHA, implGot)
		}
	})

	t.Run("ChangedFilesSince", func(t *testing.T) {
		oracleFiles, oracleErr := oracleChangedFilesSince(t, dir, fx.sharedSHA)
		if oracleErr != nil {
			t.Fatalf("oracleChangedFilesSince() error = %v", oracleErr)
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

	t.Run("IsStrictDescendant", func(t *testing.T) {
		// isStrictDescendant cannot report a problem (it swallows failure
		// into false), so resolving fx.sharedSHA — a commit made in the
		// OTHER (main) worktree — through the linked worktree's handle is
		// the only way this case would notice a common-dir mishandling: it
		// would surface as a wrong "false" answer, not an error.
		oracleGot := oracleIsStrictDescendant(t, dir, fx.sharedSHA, fx.linkedSHA)
		implGot := repo.isStrictDescendant(fx.sharedSHA, fx.linkedSHA)
		if oracleGot != implGot {
			t.Errorf("isStrictDescendant() parity mismatch: oracle = %v; gitrepo = %v", oracleGot, implGot)
		}
		if !implGot {
			t.Errorf("isStrictDescendant(%q, %q) = %v, want true", fx.sharedSHA, fx.linkedSHA, implGot)
		}
	})
}

// TestLinkedWorktree_Parity runs every read-side parity case this batch
// covers a second time against the linked-worktree fixture — directly, and
// again reached only through a junction (internal/fslink.CreateDirLink),
// since that indirection is how lyx addresses these directories in
// production — plus the CurrentBranch detached-HEAD case, which must run
// last since it mutates the worktree's checked-out ref state. The standalone
// `git init` fixtures used elsewhere in this package cannot substitute for
// any of this: the linked worktree is the only topology production runs in.
func TestLinkedWorktree_Parity(t *testing.T) {
	fx := newLinkedParityFixture(t)
	forceGoGitFinalizersOnCleanup(t)

	t.Run("Direct", func(t *testing.T) {
		runLinkedWorktreeParityChecks(t, fx.linkedDir, fx)
	})

	t.Run("ViaJunction", func(t *testing.T) {
		junctionPath := filepath.Join(t.TempDir(), "via-junction")
		if err := fslink.CreateDirLink(junctionPath, fx.linkedDir); err != nil {
			t.Skipf("directory link creation unavailable on this platform: %v", err)
		}
		runLinkedWorktreeParityChecks(t, junctionPath, fx)
	})

	t.Run("CurrentBranch_Detached", func(t *testing.T) {
		repo := New(fx.linkedDir)
		lyxtest.MustRun(t, fx.linkedDir, "git", "checkout", "--detach", fx.linkedSHA)

		_, oracleErr := oracleCurrentBranch(t, fx.linkedDir)
		_, implErr := repo.CurrentBranch()
		if (oracleErr == nil) != (implErr == nil) {
			t.Errorf("CurrentBranch() parity mismatch on detached linked-worktree HEAD: oracle err = %v; gitrepo err = %v", oracleErr, implErr)
		}
		if implErr == nil {
			t.Error("CurrentBranch() on detached linked-worktree HEAD error = nil, want non-nil")
		}
	})
}

// freezePackIndex forces repo's go-git handle to build (and freeze) its
// internal packfile index against whatever packs exist on disk at the
// moment of the call, by driving one object-lookup miss through SHAExists —
// the internal-package counterpart to parity_test.go's identically-shaped
// forcePackIndexFreeze (unreachable from here; package gitrepo_test, a
// different Go package). Later tests use this to pin the index to a stale,
// pre-repack view before writing and repacking new commits, so the read
// under test can only succeed by going through the fingerprint-gated
// reindex — see gogit.go's lookupObjectRetrying.
func freezePackIndex(t *testing.T, repo *Repo) {
	t.Helper()

	if repo.SHAExists("deadbeefdeadbeefdeadbeefdeadbeefdeadbeef") {
		t.Fatal("SHAExists(fabricated sha) = true; want false (sanity check for the freeze helper)")
	}
}

// TestIsStrictDescendant_MixedBackend_RepackBetweenCommitAndRead is the hard
// variant of isStrictDescendant's mixed-backend coverage: the handle's pack
// index is frozen against a pack-less on-disk state before the descendant
// commit exists, the descendant then lands and is repacked (git gc) before
// isStrictDescendant ever resolves it — the packfile-only-object shape
// SetSnapshotSHA's own adoptSnapshotRef CLI fetch can produce in production,
// and exactly what the fingerprint-gated reindex exists to survive. Without
// it, isStrictDescendant's failure-swallowing posture means this fails
// silently (reports false forever) rather than loudly.
func TestIsStrictDescendant_MixedBackend_RepackBetweenCommitAndRead(t *testing.T) {
	dir, repo := newStandaloneRepo(t)
	ancestorSHA, err := repo.CurrentSHA()
	if err != nil {
		t.Fatalf("CurrentSHA() (ancestor) error = %v", err)
	}

	freezePackIndex(t, repo)

	writeAndCommit(t, dir, "b.txt", "second", "second commit")
	descendantSHA, err := repo.CurrentSHA()
	if err != nil {
		t.Fatalf("CurrentSHA() (descendant) error = %v", err)
	}

	// Force both commits into a packfile the frozen index predates.
	lyxtest.MustRun(t, dir, "git", "gc")

	if !repo.isStrictDescendant(ancestorSHA, descendantSHA) {
		t.Errorf("isStrictDescendant(%q, %q) after repack = false; want true (the fingerprint-gated reindex must recover the now-packed commits)", ancestorSHA, descendantSHA)
	}
}
