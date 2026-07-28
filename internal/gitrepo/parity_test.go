//go:build integration

// parity_test.go carries the differential-parity harness lifted from
// internal/gitnativepoc/harness_test.go into package gitrepo_test: fixture
// builders beyond newRepo/writeFile/commitAll (already defined in
// gitrepo_test.go and reused directly here), repo-shaping helpers the parity
// cases need, and comparison helpers that report an oracle-vs-implementation
// divergence with both values so a failing case is diagnosable without
// re-running it under a debugger. This file itself carries no test cases —
// see gogit_test.go and the exported-method cases populated on top of this
// scaffolding for those.

package gitrepo_test

import (
	"errors"
	"sort"
	"testing"

	"github.com/Knatte18/loomyard/internal/lyxtest"
)

// newEmptyRepoFixture builds a repo initialized via `git init -b main` with
// an unborn HEAD — no commit at all — the fixture CurrentSHA's ErrNoCommits
// path and CurrentBranch's unborn-HEAD path both exercise.
func newEmptyRepoFixture(t *testing.T) (dir string) {
	t.Helper()

	dir = t.TempDir()
	lyxtest.MustRun(t, dir, "git", "init", "-b", "main")
	return dir
}

// newNonASCIIFixture builds a repo whose second commit adds a filename
// outside ASCII (å.txt), exercising the core.quotePath escaping pitfall
// ChangedFilesSince's -z flag guards against — both the oracle and the
// implementation must return the on-disk literal, never a C-quoted escape
// form.
func newNonASCIIFixture(t *testing.T) (dir, filename string) {
	t.Helper()

	filename = "å.txt"
	dir, _ = newRepo(t)
	writeFile(t, dir, "a.txt", "initial")
	commitAll(t, dir, "init")
	writeFile(t, dir, filename, "berries")
	commitAll(t, dir, "add non-ascii filename")
	return dir, filename
}

// newRenameFixture builds a repo where a file is renamed across two commits
// (old.txt -> new.txt with identical content, the pure-rename case git's
// default rename detection folds into one entry), exercising the
// --no-renames convention both the oracle and the implementation must apply
// to report both paths rather than only the destination.
func newRenameFixture(t *testing.T) (dir, oldName, newName string) {
	t.Helper()

	oldName, newName = "old.txt", "new.txt"
	dir, _ = newRepo(t)
	writeFile(t, dir, oldName, "content that stays identical")
	commitAll(t, dir, "init")
	lyxtest.MustRun(t, dir, "git", "mv", oldName, newName)
	lyxtest.MustRun(t, dir, "git", "commit", "-m", "rename")
	return dir, oldName, newName
}

// newSnapshotRefFixture builds a repo on a one-commit baseline with a
// refs/loomyard/snapshot/<key> ref set to HEAD, exercising SnapshotSHA's set-
// ref case against the snapshot-namespace ref layout snapshot.go reads and
// writes.
func newSnapshotRefFixture(t *testing.T) (dir, key string) {
	t.Helper()

	key = "mykey"
	dir, repo := newRepo(t)
	writeFile(t, dir, "a.txt", "initial")
	commitAll(t, dir, "init")
	sha, err := repo.CurrentSHA()
	if err != nil {
		t.Fatalf("CurrentSHA() error = %v", err)
	}
	writeSnapshotRef(t, dir, key, sha)
	return dir, key
}

// commitFile writes name under dir with content and commits it directly via
// the git CLI, bypassing the Repo under test — a repo-shaping helper the
// parity cases use to build history, not something under test itself.
func commitFile(t *testing.T, dir, name, content, message string) {
	t.Helper()

	writeFile(t, dir, name, content)
	lyxtest.MustRun(t, dir, "git", "add", name)
	lyxtest.MustRun(t, dir, "git", "commit", "-m", message)
}

// createBranch creates and checks out a new branch named name, another
// repo-shaping helper for cases that need a real (non-default) branch name —
// e.g. an orphan branch or a second branch to detach from.
func createBranch(t *testing.T, dir, name string) {
	t.Helper()

	lyxtest.MustRun(t, dir, "git", "checkout", "-b", name)
}

// writeSnapshotRef sets refs/loomyard/snapshot/<key> to sha directly via
// `git update-ref`, bypassing SetSnapshotSHA — used to seed a snapshot ref a
// parity case then reads back through both the oracle and SnapshotSHA.
func writeSnapshotRef(t *testing.T, dir, key, sha string) {
	t.Helper()

	lyxtest.MustRun(t, dir, "git", "update-ref", "refs/loomyard/snapshot/"+key, sha)
}

// addRemote adds a remote named name pointing at url via `git remote add` —
// used by remoteName parity cases that need a real, non-"origin" configured
// remote.
func addRemote(t *testing.T, dir, name, url string) {
	t.Helper()

	lyxtest.MustRun(t, dir, "git", "remote", "add", name, url)
}

// assertParitySHA fails the test unless oracle and impl — the SHAs returned
// by the CLI oracle and gitrepo's method for the same operation and fixture —
// are identical.
func assertParitySHA(t *testing.T, oracle, impl string) {
	t.Helper()

	if oracle != impl {
		t.Errorf("SHA parity mismatch: oracle (CLI) = %q; gitrepo = %q", oracle, impl)
	}
}

// assertParityBool fails the test unless oracle and impl — the boolean
// results returned by the CLI oracle and gitrepo's method for the same
// operation and fixture — agree.
func assertParityBool(t *testing.T, oracle, impl bool) {
	t.Helper()

	if oracle != impl {
		t.Errorf("bool parity mismatch: oracle (CLI) = %v; gitrepo = %v", oracle, impl)
	}
}

// assertParityFileList fails the test unless oracle and impl contain the same
// set of file paths, ignoring order — both sides are sorted before
// comparison, since neither ChangedFilesSince's godoc nor any consumer
// contracts on result order, only on the set of changed paths.
func assertParityFileList(t *testing.T, oracle, impl []string) {
	t.Helper()

	oracleSorted := append([]string(nil), oracle...)
	implSorted := append([]string(nil), impl...)
	sort.Strings(oracleSorted)
	sort.Strings(implSorted)

	if len(oracleSorted) != len(implSorted) {
		t.Errorf("file list parity mismatch: oracle (CLI) = %v; gitrepo = %v", oracleSorted, implSorted)
		return
	}
	for i := range oracleSorted {
		if oracleSorted[i] != implSorted[i] {
			t.Errorf("file list parity mismatch: oracle (CLI) = %v; gitrepo = %v", oracleSorted, implSorted)
			return
		}
	}
}

// assertParityErrClass fails the test unless oracleErr and implErr represent
// the same error class: both nil, or both non-nil with each side's error
// satisfying errors.Is against its OWN package's sentinel. The oracle and
// gitrepo define independent sentinel values for the same condition (the
// oracle must not import gitrepo's sentinels — doing so would reintroduce
// exactly the coupling the oracle exists to avoid), so a single shared target
// cannot bridge them; this is the cross-target comparison
// gitnativepoc/read_test.go's assertParityErrClassCrossTarget already
// established for the same reason.
func assertParityErrClass(t *testing.T, oracleErr, oracleTarget, implErr, implTarget error) {
	t.Helper()

	oracleIs := errors.Is(oracleErr, oracleTarget)
	implIs := errors.Is(implErr, implTarget)
	if oracleIs != implIs {
		t.Errorf("error class parity mismatch: oracle errors.Is(%v, %v) = %v; gitrepo errors.Is(%v, %v) = %v",
			oracleErr, oracleTarget, oracleIs, implErr, implTarget, implIs)
	}
}

// assertParityErrPresence fails the test unless oracleErr and implErr are
// either both nil or both non-nil — used where neither side defines a typed
// sentinel to compare against (e.g. CurrentBranch's detached-HEAD failure),
// so only error-vs-no-error agreement is meaningful.
func assertParityErrPresence(t *testing.T, oracleErr, implErr error) {
	t.Helper()

	if (oracleErr == nil) != (implErr == nil) {
		t.Errorf("error presence parity mismatch: oracle err = %v; gitrepo err = %v", oracleErr, implErr)
	}
}
