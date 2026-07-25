//go:build integration

// harness_test.go provides the differential parity harness every operation
// test in read_test.go and write_test.go (batches 2-3) builds on: fixture
// builders that construct real git repositories under t.TempDir(), and
// assert helpers that compare the go-git-backed poc side against the
// gitrepo/git-fixture reference side (see the plan's differential-oracle
// Shared Decision). Fixtures and helpers are exercised for the first time by
// later batches; an unused package-level function here does not break the
// Go build.

package gitnativepoc

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/lyxtest"
)

// runGit is a thin passthrough to gitexec.RunGit scoped to dir, used by
// fixture builders and assertion helpers that need to inspect git's stdout
// directly rather than merely fail-or-succeed (lyxtest.MustRun discards
// output).
func runGit(t *testing.T, dir string, args ...string) (stdout, stderr string, code int, err error) {
	t.Helper()

	return gitexec.RunGit(args, dir)
}

// newRepoFixture builds a repo on branch main with one initial commit
// (a.txt), the baseline fixture most read/write parity cases start from.
func newRepoFixture(t *testing.T) (dir string) {
	t.Helper()

	dir = t.TempDir()
	lyxtest.MustRun(t, dir, "git", "init", "-b", "main")
	lyxtest.MustRun(t, dir, "git", "config", "user.email", "test@test.com")
	lyxtest.MustRun(t, dir, "git", "config", "user.name", "Test")
	writeFixtureFile(t, dir, "a.txt", "initial")
	lyxtest.MustRun(t, dir, "git", "add", ".")
	lyxtest.MustRun(t, dir, "git", "commit", "-m", "init")
	return dir
}

// newEmptyRepoFixture builds a repo initialized via `git init -b main` with
// an unborn HEAD — no commit at all — the fixture exercising CurrentSHA's
// ErrNoCommits path and its poc equivalent.
func newEmptyRepoFixture(t *testing.T) (dir string) {
	t.Helper()

	dir = t.TempDir()
	lyxtest.MustRun(t, dir, "git", "init", "-b", "main")
	lyxtest.MustRun(t, dir, "git", "config", "user.email", "test@test.com")
	lyxtest.MustRun(t, dir, "git", "config", "user.name", "Test")
	return dir
}

// newNonASCIIFixture builds a repo whose second commit adds a filename
// outside ASCII (å.txt), exercising the core.quotePath escaping pitfall
// ChangedFilesSince's -z flag guards against — both sides of the parity
// check must return the literal on-disk name, not a C-quoted escape form.
func newNonASCIIFixture(t *testing.T) (dir, filename string) {
	t.Helper()

	filename = "å.txt"
	dir = newRepoFixture(t)
	writeFixtureFile(t, dir, filename, "berries")
	lyxtest.MustRun(t, dir, "git", "add", ".")
	lyxtest.MustRun(t, dir, "git", "commit", "-m", "add non-ascii filename")
	return dir, filename
}

// newRenameFixture builds a repo where a file is renamed across two
// commits (old.txt -> new.txt with identical content, the pure-rename case
// git's default rename detection folds into one entry), exercising the
// --no-renames flag both sides of a changed-files parity check must apply
// to report both paths rather than only the destination.
func newRenameFixture(t *testing.T) (dir, oldName, newName string) {
	t.Helper()

	oldName, newName = "old.txt", "new.txt"
	dir = t.TempDir()
	lyxtest.MustRun(t, dir, "git", "init", "-b", "main")
	lyxtest.MustRun(t, dir, "git", "config", "user.email", "test@test.com")
	lyxtest.MustRun(t, dir, "git", "config", "user.name", "Test")
	writeFixtureFile(t, dir, oldName, "content that stays identical")
	lyxtest.MustRun(t, dir, "git", "add", ".")
	lyxtest.MustRun(t, dir, "git", "commit", "-m", "init")
	lyxtest.MustRun(t, dir, "git", "mv", oldName, newName)
	lyxtest.MustRun(t, dir, "git", "commit", "-m", "rename")
	return dir, oldName, newName
}

// newSnapshotRefFixture builds a repo on newRepoFixture's baseline with a
// refs/loomyard/snapshot/<key> ref set to HEAD, exercising SnapshotSHA /
// SetSnapshotSHA parity against the snapshot-namespace ref layout
// internal/gitrepo's snapshot methods read and write.
func newSnapshotRefFixture(t *testing.T) (dir, key string) {
	t.Helper()

	key = "mykey"
	dir = newRepoFixture(t)
	stdout, stderr, code, err := runGit(t, dir, "rev-parse", "HEAD")
	if err != nil {
		t.Fatalf("git rev-parse HEAD error = %v", err)
	}
	if code != 0 {
		t.Fatalf("git rev-parse HEAD exited %d: %s", code, stderr)
	}
	head := strings.TrimSpace(stdout)
	lyxtest.MustRun(t, dir, "git", "update-ref", "refs/loomyard/snapshot/"+key, head)
	return dir, key
}

// bareRemoteFixture holds the paths a newBareRemoteFixture builds: the bare
// remote itself plus two clones, both already tracking main, for @{u},
// push-rejection, and rebase-retry parity cases.
type bareRemoteFixture struct {
	Bare   string
	CloneA string
	CloneB string
}

// newBareRemoteFixture builds a bare remote repository plus two clones, both
// checked out on main with upstream tracking already established (clones,
// unlike a fresh `git init` + `git remote add`, carry tracking from the
// clone itself), for @{u}, push-rejection, and rebase-retry parity cases.
func newBareRemoteFixture(t *testing.T) bareRemoteFixture {
	t.Helper()

	container := t.TempDir()
	bare := filepath.Join(container, "remote.git")
	lyxtest.MustRun(t, container, "git", "init", "--bare", "-b", "main", bare)

	seed := filepath.Join(container, "seed")
	lyxtest.MustRun(t, container, "git", "init", "-b", "main", seed)
	lyxtest.MustRun(t, seed, "git", "config", "user.email", "test@test.com")
	lyxtest.MustRun(t, seed, "git", "config", "user.name", "Test")
	writeFixtureFile(t, seed, "a.txt", "initial")
	lyxtest.MustRun(t, seed, "git", "add", ".")
	lyxtest.MustRun(t, seed, "git", "commit", "-m", "init")
	lyxtest.MustRun(t, seed, "git", "remote", "add", "origin", bare)
	lyxtest.MustRun(t, seed, "git", "push", "-u", "origin", "main")

	cloneA := filepath.Join(container, "cloneA")
	lyxtest.MustRun(t, container, "git", "clone", "-b", "main", bare, cloneA)
	cloneB := filepath.Join(container, "cloneB")
	lyxtest.MustRun(t, container, "git", "clone", "-b", "main", bare, cloneB)

	return bareRemoteFixture{Bare: bare, CloneA: cloneA, CloneB: cloneB}
}

// writeFixtureFile creates (or overwrites) name under dir with the given
// content, failing the test on any filesystem error.
func writeFixtureFile(t *testing.T, dir, name, content string) {
	t.Helper()

	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

// assertParitySHA fails the test unless poc and cli — the SHAs returned by
// the go-git-backed poc method and the gitrepo/CLI reference method for the
// same operation and fixture — are identical.
func assertParitySHA(t *testing.T, poc, cli string) {
	t.Helper()

	if poc != cli {
		t.Errorf("SHA parity mismatch: gitnativepoc = %q; gitrepo (CLI reference) = %q", poc, cli)
	}
}

// assertParityFileList fails the test unless poc and cli contain the same
// set of file paths, ignoring order — both sides are sorted before
// comparison, since neither side's ordering is part of the contract under
// test, only the set of changed paths.
func assertParityFileList(t *testing.T, poc, cli []string) {
	t.Helper()

	pocSorted := append([]string(nil), poc...)
	cliSorted := append([]string(nil), cli...)
	sort.Strings(pocSorted)
	sort.Strings(cliSorted)

	if len(pocSorted) != len(cliSorted) {
		t.Errorf("file list parity mismatch: gitnativepoc = %v; gitrepo (CLI reference) = %v", pocSorted, cliSorted)
		return
	}
	for i := range pocSorted {
		if pocSorted[i] != cliSorted[i] {
			t.Errorf("file list parity mismatch: gitnativepoc = %v; gitrepo (CLI reference) = %v", pocSorted, cliSorted)
			return
		}
	}
}

// assertParityBool fails the test unless poc and cli — the boolean results
// returned by the go-git-backed poc method and the gitrepo/CLI reference
// method for the same operation and fixture — agree.
func assertParityBool(t *testing.T, poc, cli bool) {
	t.Helper()

	if poc != cli {
		t.Errorf("bool parity mismatch: gitnativepoc = %v; gitrepo (CLI reference) = %v", poc, cli)
	}
}

// assertParityErrClass fails the test unless poc and cli represent the same
// error class: both nil, or both non-nil with cli satisfying errors.Is(cli,
// target) whenever poc does — the typed-error-class comparison the
// differential-oracle Shared Decision calls for, since the two sides never
// produce byte-identical error strings (one wraps git's CLI stderr, the
// other a go-git error).
func assertParityErrClass(t *testing.T, poc, cli error, target error) {
	t.Helper()

	pocIs := errors.Is(poc, target)
	cliIs := errors.Is(cli, target)
	if pocIs != cliIs {
		t.Errorf("error class parity mismatch for target %v: gitnativepoc errors.Is = %v (err %v); gitrepo (CLI reference) errors.Is = %v (err %v)",
			target, pocIs, poc, cliIs, cli)
	}
}
