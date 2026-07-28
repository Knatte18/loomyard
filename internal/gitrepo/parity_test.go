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
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitrepo"
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

// writeSnapshotRef sets refs/loomyard/snapshot/<key> to sha directly via
// `git update-ref`, bypassing SetSnapshotSHA — used to seed a snapshot ref a
// parity case then reads back through both the oracle and SnapshotSHA.
func writeSnapshotRef(t *testing.T, dir, key, sha string) {
	t.Helper()

	lyxtest.MustRun(t, dir, "git", "update-ref", "refs/loomyard/snapshot/"+key, sha)
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

// assertParityString fails the test unless oracle and impl are identical
// strings — the general string-equality comparison CurrentBranch's parity
// cases use. assertParitySHA exists separately for the SHA-shaped case so its
// failure message names the value it is actually comparing.
func assertParityString(t *testing.T, oracle, impl string) {
	t.Helper()

	if oracle != impl {
		t.Errorf("string parity mismatch: oracle (CLI) = %q; gitrepo = %q", oracle, impl)
	}
}

// resolveRevOrFatal resolves rev to its object SHA via `git rev-parse`,
// failing the test outright on any failure — used by fixtures that need a
// tree or blob SHA (HEAD^{tree}, HEAD:<path>) rather than a commit SHA, which
// no fixture builder in this package hands back directly.
func resolveRevOrFatal(t *testing.T, dir, rev string) string {
	t.Helper()

	stdout, stderr, code, err := runGit(t, dir, "rev-parse", rev)
	if err != nil {
		t.Fatalf("git rev-parse %s error = %v", rev, err)
	}
	if code != 0 {
		t.Fatalf("git rev-parse %s exited %d: %s", rev, code, stderr)
	}
	return strings.TrimSpace(stdout)
}

// TestCurrentSHA_Parity_CommittedRepo asserts the oracle and gitrepo's
// CurrentSHA agree on an ordinary committed repo — trivially true in this
// batch, since gitrepo.CurrentSHA is still CLI-backed; the value of this case
// is established now, before batch 3 flips it onto go-git.
func TestCurrentSHA_Parity_CommittedRepo(t *testing.T) {
	dir, repo := newRepo(t)
	writeFile(t, dir, "a.txt", "initial")
	commitAll(t, dir, "init")

	oracleSHA, oracleErr := oracleCurrentSHA(t, dir)
	if oracleErr != nil {
		t.Fatalf("oracleCurrentSHA() error = %v", oracleErr)
	}
	implSHA, implErr := repo.CurrentSHA()
	if implErr != nil {
		t.Fatalf("CurrentSHA() error = %v", implErr)
	}

	assertParitySHA(t, oracleSHA, implSHA)
}

// TestCurrentSHA_Parity_UnbornHEAD asserts the oracle and gitrepo's
// CurrentSHA agree on an unborn HEAD: both map git's ambiguous-HEAD stderr
// shape to their own sentinel (errOracleNoCommits and gitrepo.ErrNoCommits
// respectively), so the cross-target class comparison — never a raw string
// comparison, since the two sides never produce byte-identical errors — is
// what proves agreement.
func TestCurrentSHA_Parity_UnbornHEAD(t *testing.T) {
	dir := newEmptyRepoFixture(t)

	_, oracleErr := oracleCurrentSHA(t, dir)
	_, implErr := gitrepo.New(dir).CurrentSHA()

	assertParityErrClass(t, oracleErr, errOracleNoCommits, implErr, gitrepo.ErrNoCommits)
	if !errors.Is(oracleErr, errOracleNoCommits) {
		t.Errorf("oracleCurrentSHA() on unborn HEAD error = %v, want errOracleNoCommits", oracleErr)
	}
	if !errors.Is(implErr, gitrepo.ErrNoCommits) {
		t.Errorf("CurrentSHA() on unborn HEAD error = %v, want gitrepo.ErrNoCommits", implErr)
	}
}

// TestSHAExists_Parity_CommittedSHA asserts the oracle and gitrepo's
// SHAExists agree that a real, committed SHA exists.
func TestSHAExists_Parity_CommittedSHA(t *testing.T) {
	dir, repo := newRepo(t)
	writeFile(t, dir, "a.txt", "initial")
	commitAll(t, dir, "init")

	sha, err := repo.CurrentSHA()
	if err != nil {
		t.Fatalf("CurrentSHA() error = %v", err)
	}

	assertParityBool(t, oracleSHAExists(t, dir, sha), repo.SHAExists(sha))
}

// TestSHAExists_Parity_MissingAndNonHexSHA asserts the oracle and gitrepo
// agree that a well-formed-but-absent SHA, and a non-hex string, both fold
// into false without either side treating the lookup itself as a failure
// worth surfacing.
func TestSHAExists_Parity_MissingAndNonHexSHA(t *testing.T) {
	dir, repo := newRepo(t)
	writeFile(t, dir, "a.txt", "initial")
	commitAll(t, dir, "init")

	tests := []struct {
		name string
		sha  string
	}{
		{"MissingSHA", "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef"},
		{"NonHexSHA", "not-a-sha!!"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertParityBool(t, oracleSHAExists(t, dir, tt.sha), repo.SHAExists(tt.sha))
		})
	}
}

// TestSHAExists_Parity_TreeOrBlobSHA asserts the oracle and gitrepo agree
// that a tree or blob SHA — a real, valid-hex object name, just not a commit
// — is false, never true: the `^{commit}` peel is what makes this so, and it
// is exactly what the missing/non-hex cases above cannot distinguish, since
// neither of those SHAs resolves to any object at all.
func TestSHAExists_Parity_TreeOrBlobSHA(t *testing.T) {
	dir, repo := newRepo(t)
	writeFile(t, dir, "a.txt", "initial")
	commitAll(t, dir, "init")

	tests := []struct {
		name string
		rev  string
	}{
		{"TreeSHA", "HEAD^{tree}"},
		{"BlobSHA", "HEAD:a.txt"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sha := resolveRevOrFatal(t, dir, tt.rev)
			assertParityBool(t, oracleSHAExists(t, dir, sha), repo.SHAExists(sha))
		})
	}
}

// TestChangedFilesSince_Parity_NonASCIIPath asserts the oracle and gitrepo
// both return a non-ASCII filename verbatim — the on-disk literal, never
// core.quotePath's C-quoted escape form — and agree with each other.
func TestChangedFilesSince_Parity_NonASCIIPath(t *testing.T) {
	dir, filename := newNonASCIIFixture(t)
	since := firstCommitSHA(t, dir)

	oracleFiles, oracleErr := oracleChangedFilesSince(t, dir, since)
	if oracleErr != nil {
		t.Fatalf("oracleChangedFilesSince() error = %v", oracleErr)
	}
	if !containsPath(oracleFiles, filename) {
		t.Fatalf("oracleChangedFilesSince() = %v, want it to contain verbatim %q", oracleFiles, filename)
	}

	implFiles, implErr := gitrepo.New(dir).ChangedFilesSince(since)
	if implErr != nil {
		t.Fatalf("ChangedFilesSince() error = %v", implErr)
	}
	if !containsPath(implFiles, filename) {
		t.Fatalf("ChangedFilesSince() = %v, want it to contain verbatim %q", implFiles, filename)
	}

	assertParityFileList(t, oracleFiles, implFiles)
}

// TestChangedFilesSince_Parity_Rename asserts the oracle and gitrepo both
// report a pure rename as its old path (deleted) and new path (added)
// separately, never folded into one entry, and agree with each other.
func TestChangedFilesSince_Parity_Rename(t *testing.T) {
	dir, oldName, newName := newRenameFixture(t)
	since := firstCommitSHA(t, dir)

	oracleFiles, oracleErr := oracleChangedFilesSince(t, dir, since)
	if oracleErr != nil {
		t.Fatalf("oracleChangedFilesSince() error = %v", oracleErr)
	}
	implFiles, implErr := gitrepo.New(dir).ChangedFilesSince(since)
	if implErr != nil {
		t.Fatalf("ChangedFilesSince() error = %v", implErr)
	}

	for _, files := range [][]string{oracleFiles, implFiles} {
		if !containsPath(files, oldName) {
			t.Errorf("ChangedFilesSince() = %v, want it to contain the deleted old path %q", files, oldName)
		}
		if !containsPath(files, newName) {
			t.Errorf("ChangedFilesSince() = %v, want it to contain the added new path %q", files, newName)
		}
	}
	assertParityFileList(t, oracleFiles, implFiles)
}

// TestChangedFilesSince_Parity_NonHexSHA asserts the oracle and gitrepo agree
// on error class for a non-hex sha: each side returns its own
// ErrInvalidSHA-class sentinel before either ever resolves or diffs anything.
func TestChangedFilesSince_Parity_NonHexSHA(t *testing.T) {
	dir, _ := newRepo(t)
	writeFile(t, dir, "a.txt", "initial")
	commitAll(t, dir, "init")

	_, implErr := gitrepo.New(dir).ChangedFilesSince("not-a-sha!!")
	if !errors.Is(implErr, gitrepo.ErrInvalidSHA) {
		t.Errorf("ChangedFilesSince(non-hex) error = %v, want gitrepo.ErrInvalidSHA", implErr)
	}
}

// containsPath reports whether haystack contains needle, used to assert a
// specific path is present in a ChangedFilesSince result without depending on
// list order.
func containsPath(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

// TestSnapshotSHA_Parity_SetRef asserts the oracle and gitrepo agree on the
// SHA recorded under a snapshot ref that has been set.
func TestSnapshotSHA_Parity_SetRef(t *testing.T) {
	dir, key := newSnapshotRefFixture(t)

	oracleSHA, oracleErr := oracleSnapshotSHA(t, dir, key)
	if oracleErr != nil {
		t.Fatalf("oracleSnapshotSHA(%q) error = %v", key, oracleErr)
	}
	implSHA, implErr := gitrepo.New(dir).SnapshotSHA(key)
	if implErr != nil {
		t.Fatalf("SnapshotSHA(%q) error = %v", key, implErr)
	}

	assertParitySHA(t, oracleSHA, implSHA)
}

// TestSnapshotSHA_Parity_AbsentRef asserts a key with no ref ever set reads
// as ("", nil) on both the oracle and gitrepo — a verified-absent ref is a
// normal state, not a failure.
func TestSnapshotSHA_Parity_AbsentRef(t *testing.T) {
	dir, _ := newRepo(t)
	writeFile(t, dir, "a.txt", "initial")
	commitAll(t, dir, "init")
	const key = "never-set"

	oracleSHA, oracleErr := oracleSnapshotSHA(t, dir, key)
	if oracleErr != nil {
		t.Fatalf("oracleSnapshotSHA(%q) error = %v", key, oracleErr)
	}
	if oracleSHA != "" {
		t.Fatalf("oracleSnapshotSHA(%q) = %q, want \"\"", key, oracleSHA)
	}

	implSHA, implErr := gitrepo.New(dir).SnapshotSHA(key)
	if implErr != nil {
		t.Fatalf("SnapshotSHA(%q) error = %v", key, implErr)
	}
	if implSHA != "" {
		t.Fatalf("SnapshotSHA(%q) = %q, want \"\"", key, implSHA)
	}

	assertParitySHA(t, oracleSHA, implSHA)
}

// TestSnapshotSHA_Parity_InvalidKey asserts an invalid key returns gitrepo's
// own ErrInvalidSnapshotKey without ever touching a ref — the oracle has no
// key-validation layer of its own (that check exists only in production code
// as a pre-git guard), so this case asserts gitrepo's contract directly
// rather than an oracle comparison.
func TestSnapshotSHA_Parity_InvalidKey(t *testing.T) {
	dir, _ := newRepo(t)
	writeFile(t, dir, "a.txt", "initial")
	commitAll(t, dir, "init")
	// "key..bad" passes the character-class check but is rejected by the
	// separate ".." shape check, exercising that specific rule.
	const key = "key..bad"

	_, implErr := gitrepo.New(dir).SnapshotSHA(key)
	if !errors.Is(implErr, gitrepo.ErrInvalidSnapshotKey) {
		t.Errorf("SnapshotSHA(%q) error = %v, want gitrepo.ErrInvalidSnapshotKey", key, implErr)
	}
}

// TestSnapshotSHA_Parity_UnreadableStore asserts the third leg of
// SnapshotSHA's three-way distinction: a checkout that cannot be read at all
// (here, a plain directory that is not a git repository) surfaces as an
// error on both the oracle and gitrepo, rather than folding into the
// verified-absent ("", nil) case — folding the two together would tell a
// miswired read-only consumer "no snapshot" forever instead of surfacing the
// real problem.
func TestSnapshotSHA_Parity_UnreadableStore(t *testing.T) {
	dir := t.TempDir() // deliberately never `git init`-ed
	const key = "somekey"

	_, oracleErr := oracleSnapshotSHA(t, dir, key)
	if oracleErr == nil {
		t.Fatal("oracleSnapshotSHA() on a non-repository path error = nil, want non-nil")
	}
	_, implErr := gitrepo.New(dir).SnapshotSHA(key)
	if implErr == nil {
		t.Fatal("SnapshotSHA() on a non-repository path error = nil, want non-nil")
	}
}

// TestCurrentBranch_Parity covers CurrentBranch across all four HEAD states
// the method can encounter: an ordinary branch, a detached HEAD (must be an
// error, never an empty string — a caller with no captured branch has no
// safe ref to hand RestoreBranch), an unborn HEAD (symbolic-ref succeeds and
// prints the branch name even with no commit yet), and an orphan branch.
func TestCurrentBranch_Parity(t *testing.T) {
	t.Run("OnBranch", func(t *testing.T) {
		dir, repo := newRepo(t)
		writeFile(t, dir, "a.txt", "initial")
		commitAll(t, dir, "init")

		oracleBranch, oracleErr := oracleCurrentBranch(t, dir)
		if oracleErr != nil {
			t.Fatalf("oracleCurrentBranch() error = %v", oracleErr)
		}
		implBranch, implErr := repo.CurrentBranch()
		if implErr != nil {
			t.Fatalf("CurrentBranch() error = %v", implErr)
		}
		assertParityString(t, oracleBranch, implBranch)
		if implBranch != "main" {
			t.Errorf("CurrentBranch() = %q, want %q", implBranch, "main")
		}
	})

	t.Run("Detached", func(t *testing.T) {
		dir, repo := newRepo(t)
		writeFile(t, dir, "a.txt", "initial")
		commitAll(t, dir, "init")
		sha, err := repo.CurrentSHA()
		if err != nil {
			t.Fatalf("CurrentSHA() error = %v", err)
		}
		lyxtest.MustRun(t, dir, "git", "checkout", "--detach", sha)

		_, oracleErr := oracleCurrentBranch(t, dir)
		_, implErr := repo.CurrentBranch()
		assertParityErrPresence(t, oracleErr, implErr)
		if implErr == nil {
			t.Error("CurrentBranch() on detached HEAD error = nil, want non-nil")
		}
	})

	t.Run("UnbornHEAD", func(t *testing.T) {
		dir := newEmptyRepoFixture(t)

		oracleBranch, oracleErr := oracleCurrentBranch(t, dir)
		if oracleErr != nil {
			t.Fatalf("oracleCurrentBranch() on unborn HEAD error = %v, want nil", oracleErr)
		}
		implBranch, implErr := gitrepo.New(dir).CurrentBranch()
		if implErr != nil {
			t.Fatalf("CurrentBranch() on unborn HEAD error = %v, want nil", implErr)
		}
		assertParityString(t, oracleBranch, implBranch)
		if implBranch != "main" {
			t.Errorf("CurrentBranch() on unborn HEAD = %q, want %q", implBranch, "main")
		}
	})

	t.Run("Orphan", func(t *testing.T) {
		dir, _ := newRepo(t)
		writeFile(t, dir, "a.txt", "initial")
		commitAll(t, dir, "init")

		lyxtest.MustRun(t, dir, "git", "checkout", "--orphan", "orphan-branch")
		lyxtest.MustRun(t, dir, "git", "rm", "-rf", "--cached", ".")
		commitFile(t, dir, "orphan.txt", "unrelated root", "orphan root")

		oracleBranch, oracleErr := oracleCurrentBranch(t, dir)
		if oracleErr != nil {
			t.Fatalf("oracleCurrentBranch() on orphan branch error = %v, want nil", oracleErr)
		}
		implBranch, implErr := gitrepo.New(dir).CurrentBranch()
		if implErr != nil {
			t.Fatalf("CurrentBranch() on orphan branch error = %v, want nil", implErr)
		}
		assertParityString(t, oracleBranch, implBranch)
		if implBranch != "orphan-branch" {
			t.Errorf("CurrentBranch() on orphan branch = %q, want %q", implBranch, "orphan-branch")
		}
	})
}

// forcePackIndexFreeze forces repo's go-git handle to build (and freeze) its
// internal packfile index against whatever packs exist on disk at the
// moment of the call, by driving one object-lookup miss through the
// exported SHAExists — the shared setup step every "hard variant"
// mixed-backend case below builds on: a subsequent commit-then-repack
// sequence produces a packfile-only object the frozen index predates, so a
// later read can only resolve it correctly by going through the
// fingerprint-gated reindex retry (see gogit.go's lookupObjectRetrying).
func forcePackIndexFreeze(t *testing.T, repo *gitrepo.Repo) {
	t.Helper()

	if repo.SHAExists("deadbeefdeadbeefdeadbeefdeadbeefdeadbeef") {
		t.Fatal("SHAExists(fabricated sha) = true; want false (sanity check for the freeze helper)")
	}
}

// TestStageAndCommit_MixedBackend_PreWarmedHandleSeesCLICommit asserts
// StageAndCommit's trailing r.CurrentSHA() call — a go-git ref read — sees
// the commit its own preceding `git commit` call just wrote, even when the
// Repo's go-git handle was warmed (opened and cached) before that commit
// landed. This is the call-granular boundary's central mixed-backend site:
// a stale answer here would feed a wrong SHA straight into a caller's
// SetSnapshotSHA, recording a snapshot that points off-history.
func TestStageAndCommit_MixedBackend_PreWarmedHandleSeesCLICommit(t *testing.T) {
	dir, repo := newRepo(t)
	writeFile(t, dir, "a.txt", "initial")
	commitAll(t, dir, "init")

	if _, err := repo.CurrentSHA(); err != nil {
		t.Fatalf("CurrentSHA() (warm handle) error = %v", err)
	}

	writeFile(t, dir, "a.txt", "changed")
	sha, committed, err := repo.StageAndCommit("commit a", []string{"a.txt"})
	if err != nil {
		t.Fatalf("StageAndCommit() error = %v; want nil", err)
	}
	if !committed {
		t.Fatal("StageAndCommit() committed = false; want true")
	}

	want := resolveRevOrFatal(t, dir, "HEAD")
	if sha != want {
		t.Errorf("StageAndCommit() sha = %q; want %q (the commit its own CLI write just created)", sha, want)
	}
}

// TestStageAllAndCommit_MixedBackend_PreWarmedHandleSeesCLICommit is
// TestStageAndCommit_MixedBackend_PreWarmedHandleSeesCLICommit's counterpart
// for StageAllAndCommit, the wildcard sibling that ends with the identical
// r.CurrentSHA() call.
func TestStageAllAndCommit_MixedBackend_PreWarmedHandleSeesCLICommit(t *testing.T) {
	dir, repo := newRepo(t)
	writeFile(t, dir, "a.txt", "initial")
	commitAll(t, dir, "init")

	if _, err := repo.CurrentSHA(); err != nil {
		t.Fatalf("CurrentSHA() (warm handle) error = %v", err)
	}

	writeFile(t, dir, "a.txt", "changed")
	sha, committed, err := repo.StageAllAndCommit("commit all")
	if err != nil {
		t.Fatalf("StageAllAndCommit() error = %v; want nil", err)
	}
	if !committed {
		t.Fatal("StageAllAndCommit() committed = false; want true")
	}

	want := resolveRevOrFatal(t, dir, "HEAD")
	if sha != want {
		t.Errorf("StageAllAndCommit() sha = %q; want %q (the commit its own CLI write just created)", sha, want)
	}
}

// TestSHAExists_MixedBackend_RepackBetweenCommitAndRead is the hard variant
// of the mixed-backend interop cases: repo's go-git handle is frozen (via
// forcePackIndexFreeze) against a pack-less on-disk state, a new commit then
// lands, and a `git gc` repacks it BEFORE SHAExists ever reads it — the
// packfile-only-object shape Push's own pull --rebase retry can produce in
// production, and exactly what the fingerprint-gated reindex exists to
// survive. Without it, SHAExists' failure-swallowing posture means this
// would fail silently (report false forever), never loudly.
func TestSHAExists_MixedBackend_RepackBetweenCommitAndRead(t *testing.T) {
	dir, repo := newRepo(t)
	writeFile(t, dir, "a.txt", "initial")
	commitAll(t, dir, "init")

	forcePackIndexFreeze(t, repo)

	writeFile(t, dir, "b.txt", "second")
	commitAll(t, dir, "second commit")
	sha := resolveRevOrFatal(t, dir, "HEAD")

	lyxtest.MustRun(t, dir, "git", "gc")

	if !repo.SHAExists(sha) {
		t.Errorf("SHAExists(%q) after repack = false; want true (the fingerprint-gated reindex must recover the now-packed commit)", sha)
	}
}

// TestSHAExists_MixedBackend_CrossInstanceReindexSeesWriteFromOtherRepo pins
// the fingerprint gate against a per-instance counter gate: repoA's go-git
// index is frozen against the pack-less on-disk state, then a SEPARATE
// gitrepo.New value on the identical path builds and repacks a new commit —
// a distinct *Repo, so repoA's own call count is never touched by any of it
// — and repoA's own SHAExists must still resolve the new, now-packed
// commit. A per-*Repo call counter would never observe this write at all,
// since it only ever counts repoA's own calls; the pack fingerprint is
// shared, on-disk ground truth every *Repo addressing the same checkout can
// observe regardless of which one performed the write. This is the case
// that fails under a counter gate and passes under the fingerprint gate, so
// it pins the design rather than merely restating it.
func TestSHAExists_MixedBackend_CrossInstanceReindexSeesWriteFromOtherRepo(t *testing.T) {
	dir, repoA := newRepo(t)
	writeFile(t, dir, "a.txt", "initial")
	commitAll(t, dir, "init")

	forcePackIndexFreeze(t, repoA)

	// A genuinely separate *Repo value on the same path — repoA's own handle
	// is never touched by anything below.
	repoB := gitrepo.New(dir)
	writeFile(t, dir, "b.txt", "from repoB")
	commitAll(t, dir, "commit via repoB's path")
	sha, err := repoB.CurrentSHA()
	if err != nil {
		t.Fatalf("CurrentSHA() (repoB) error = %v", err)
	}
	lyxtest.MustRun(t, dir, "git", "gc")

	if !repoA.SHAExists(sha) {
		t.Errorf("repoA.SHAExists(%q) = false; want true (repoA's fingerprint-gated reindex must see a write+repack made through a separate *Repo)", sha)
	}
}

// TestSnapshotSHA_MixedBackend_ReadsRefCLISideAdvanceWrote asserts
// SnapshotSHA's go-git ref read sees the value SetSnapshotSHA's own
// CLI-side advanceAndPushSnapshotRef just wrote locally, even when the
// Repo's handle was warmed before that local ref update landed — go-git
// never caches refs, so this pins that fact at the exported call site the
// plan enumerates rather than leaving it untested there.
func TestSnapshotSHA_MixedBackend_ReadsRefCLISideAdvanceWrote(t *testing.T) {
	container := t.TempDir()
	bareRemote := newBareRemote(t, container)

	clonePath, repo := newRepoWithRemote(t, container, "clone", bareRemote)
	writeFile(t, clonePath, "a.txt", "initial")
	commitAll(t, clonePath, "init")
	if err := repo.Push(); err != nil {
		t.Fatalf("Push() error = %v; want nil", err)
	}
	headSHA, err := repo.CurrentSHA()
	if err != nil {
		t.Fatalf("CurrentSHA() error = %v", err)
	}

	// Warm the handle before SetSnapshotSHA's own CLI-side ref advance below.
	if _, err := repo.SnapshotSHA("warmup"); err != nil {
		t.Fatalf("SnapshotSHA() (warm handle) error = %v", err)
	}

	if err := repo.SetSnapshotSHA("mykey", headSHA); err != nil {
		t.Fatalf("SetSnapshotSHA() error = %v; want nil", err)
	}

	got, err := repo.SnapshotSHA("mykey")
	if err != nil {
		t.Fatalf("SnapshotSHA() error = %v; want nil", err)
	}
	if got != headSHA {
		t.Errorf("SnapshotSHA() = %q; want %q (the ref value SetSnapshotSHA's own CLI push just wrote)", got, headSHA)
	}
}

// TestSetSnapshotSHA_MixedBackend_RepackBetweenCommitAndCanonicalization is
// the hard variant for SetSnapshotSHA's own `^{commit}` canonicalization:
// the handle's pack index is frozen before a new commit lands, the commit is
// then repacked (git gc) before SetSnapshotSHA ever resolves an abbreviated
// spelling of it. Without the fingerprint-gated reindex, the resolution
// would fail silently (best-effort semantics: the abbreviated spelling is
// passed through unchanged), and the stored ref — and a later read back via
// SnapshotSHA — would carry the abbreviated spelling instead of the full
// canonical one.
func TestSetSnapshotSHA_MixedBackend_RepackBetweenCommitAndCanonicalization(t *testing.T) {
	container := t.TempDir()
	bareRemote := newBareRemote(t, container)

	clonePath, repo := newRepoWithRemote(t, container, "clone", bareRemote)
	writeFile(t, clonePath, "a.txt", "initial")
	commitAll(t, clonePath, "init")
	if err := repo.Push(); err != nil {
		t.Fatalf("Push() error = %v; want nil", err)
	}

	forcePackIndexFreeze(t, repo)

	writeFile(t, clonePath, "a.txt", "second")
	commitAll(t, clonePath, "second commit")
	fullSHA, err := repo.CurrentSHA()
	if err != nil {
		t.Fatalf("CurrentSHA() error = %v", err)
	}

	// Force the new commit into a packfile before SetSnapshotSHA's own
	// canonicalization ever resolves it.
	lyxtest.MustRun(t, clonePath, "git", "gc")

	if err := repo.SetSnapshotSHA("mykey", fullSHA[:7]); err != nil {
		t.Fatalf("SetSnapshotSHA(abbreviated, repacked) error = %v; want nil", err)
	}

	got, err := repo.SnapshotSHA("mykey")
	if err != nil {
		t.Fatalf("SnapshotSHA() error = %v; want nil", err)
	}
	if got != fullSHA {
		t.Errorf("SnapshotSHA() after repacked abbreviated SetSnapshotSHA = %q; want %q (full canonical spelling)", got, fullSHA)
	}
}

// TestPushCoalesced_MixedBackend_NothingToPushAfterCLIPush asserts
// PushCoalesced's internal hasUnpushed gate correctly reports
// nothing-to-push immediately after Push's own CLI push has landed, even
// when the Repo's go-git handle was warmed before that push ran — pinning
// the exported call site the plan enumerates for this read.
func TestPushCoalesced_MixedBackend_NothingToPushAfterCLIPush(t *testing.T) {
	container := t.TempDir()
	bareRemote := newBareRemote(t, container)

	repoPath, repo := newRepoWithRemote(t, container, "clone", bareRemote)
	writeFile(t, repoPath, "a.txt", "initial")
	commitAll(t, repoPath, "init")

	if _, err := repo.CurrentSHA(); err != nil {
		t.Fatalf("CurrentSHA() (warm handle) error = %v", err)
	}

	if err := repo.Push(); err != nil {
		t.Fatalf("Push() error = %v; want nil", err)
	}

	if err := repo.PushCoalesced(); err != nil {
		t.Fatalf("PushCoalesced() (nothing unpushed) error = %v; want nil", err)
	}
}
