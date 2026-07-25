//go:build integration

// read_test.go holds the differential parity tests for gitnativepoc's
// read-surface methods in read.go: for each operation and fixture built by
// harness_test.go's builders, it runs the go-git-backed poc method alongside
// gitrepo.Repo's CLI-backed reference method (or, for gitrepo's unexported
// helpers, a direct git-fixture assertion) and asserts the two agree, per
// the plan's differential-oracle Shared Decision. Each test carries its
// MIGRATE/CLI-BOUND verdict as a comment, per the
// cli-bound-is-a-recorded-outcome Shared Decision.

package gitnativepoc

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/lyxtest"
)

// assertParityErrClassCrossTarget fails the test unless poc and cli agree on
// whether they represent the same semantic error class, checking each side
// against its own package's typed sentinel. gitnativepoc and gitrepo define
// independent sentinel values for the same condition — the
// differential-oracle Shared Decision restricts internal/gitrepo imports to
// test files, so read.go can never alias gitrepo's sentinel directly — so a
// single shared target cannot bridge them the way harness_test.go's
// assertParityErrClass does for genuinely interchangeable error values.
func assertParityErrClassCrossTarget(t *testing.T, poc error, pocTarget error, cli error, cliTarget error) {
	t.Helper()

	pocIs := errors.Is(poc, pocTarget)
	cliIs := errors.Is(cli, cliTarget)
	if pocIs != cliIs {
		t.Errorf("error class parity mismatch: gitnativepoc errors.Is(%v, %v) = %v; gitrepo errors.Is(%v, %v) = %v",
			poc, pocTarget, pocIs, cli, cliTarget, cliIs)
	}
}

// TestCurrentSHA_CommittedRepo is MIGRATE: go-git's Repository.Head returns
// the same SHA as `git rev-parse HEAD` for an ordinary committed repo.
func TestCurrentSHA_CommittedRepo(t *testing.T) {
	dir := newRepoFixture(t)

	poc, err := OpenRepo(dir)
	if err != nil {
		t.Fatalf("OpenRepo(%q) error = %v", dir, err)
	}
	pocSHA, pocErr := poc.CurrentSHA()
	if pocErr != nil {
		t.Fatalf("gitnativepoc CurrentSHA() error = %v", pocErr)
	}

	cliSHA, cliErr := gitrepo.New(dir).CurrentSHA()
	if cliErr != nil {
		t.Fatalf("gitrepo CurrentSHA() error = %v", cliErr)
	}

	assertParitySHA(t, pocSHA, cliSHA)
}

// TestCurrentSHA_UnbornHEAD is MIGRATE: go-git's Head() reports the
// unborn-HEAD case (a freshly-initialized repo with no commits) as
// plumbing.ErrReferenceNotFound, which gitnativepoc.CurrentSHA translates to
// its own ErrNoCommits — the same error class gitrepo.CurrentSHA reports, as
// gitrepo.ErrNoCommits, via its stderr-substring sniff of
// `git rev-parse HEAD`'s failure.
func TestCurrentSHA_UnbornHEAD(t *testing.T) {
	dir := newEmptyRepoFixture(t)

	poc, err := OpenRepo(dir)
	if err != nil {
		t.Fatalf("OpenRepo(%q) error = %v", dir, err)
	}
	_, pocErr := poc.CurrentSHA()

	_, cliErr := gitrepo.New(dir).CurrentSHA()

	assertParityErrClassCrossTarget(t, pocErr, ErrNoCommits, cliErr, gitrepo.ErrNoCommits)
	// The cross-target helper above only proves the two sides agree on
	// whether they matched their respective target; it cannot by itself
	// distinguish "both genuinely hit ErrNoCommits" from "both happened to
	// return some other error", so assert each side's class directly too.
	if !errors.Is(pocErr, ErrNoCommits) {
		t.Errorf("gitnativepoc CurrentSHA() on unborn HEAD error = %v, want ErrNoCommits", pocErr)
	}
	if !errors.Is(cliErr, gitrepo.ErrNoCommits) {
		t.Errorf("gitrepo CurrentSHA() on unborn HEAD error = %v, want gitrepo.ErrNoCommits", cliErr)
	}
}

// TestSHAExists_CommittedSHA is MIGRATE: go-git's ResolveRevision resolves a
// real commit SHA exactly as `git rev-parse --verify --quiet` does.
func TestSHAExists_CommittedSHA(t *testing.T) {
	dir := newRepoFixture(t)

	cliRepo := gitrepo.New(dir)
	sha, cliErr := cliRepo.CurrentSHA()
	if cliErr != nil {
		t.Fatalf("gitrepo CurrentSHA() error = %v", cliErr)
	}

	poc, err := OpenRepo(dir)
	if err != nil {
		t.Fatalf("OpenRepo(%q) error = %v", dir, err)
	}

	assertParityBool(t, poc.SHAExists(sha), cliRepo.SHAExists(sha))
}

// TestSHAExists_MissingAndNonHexSHA is MIGRATE: both a well-formed but
// absent SHA and a non-hex string fold into false on both sides, without
// either side treating the lookup itself as a failure worth surfacing.
func TestSHAExists_MissingAndNonHexSHA(t *testing.T) {
	dir := newRepoFixture(t)

	poc, err := OpenRepo(dir)
	if err != nil {
		t.Fatalf("OpenRepo(%q) error = %v", dir, err)
	}
	cliRepo := gitrepo.New(dir)

	tests := []struct {
		name string
		sha  string
	}{
		{"MissingSHA", "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef"},
		{"NonHexSHA", "not-a-sha!!"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertParityBool(t, poc.SHAExists(tt.sha), cliRepo.SHAExists(tt.sha))
		})
	}
}

// TestChangedFilesSince_NonASCIIPath is MIGRATE: go-git's tree entries are
// raw UTF-8, so a non-ASCII filename comes back verbatim on both sides
// instead of git CLI's core.quotePath-escaped form.
func TestChangedFilesSince_NonASCIIPath(t *testing.T) {
	dir, filename := newNonASCIIFixture(t)

	cliRepo := gitrepo.New(dir)
	files, cliErr := cliRepo.ChangedFilesSince(firstCommitSHA(t, dir))
	if cliErr != nil {
		t.Fatalf("gitrepo ChangedFilesSince() error = %v", cliErr)
	}
	if !containsString(files, filename) {
		t.Fatalf("gitrepo ChangedFilesSince() = %v, want it to contain verbatim %q", files, filename)
	}

	poc, err := OpenRepo(dir)
	if err != nil {
		t.Fatalf("OpenRepo(%q) error = %v", dir, err)
	}
	pocFiles, pocErr := poc.ChangedFilesSince(firstCommitSHA(t, dir))
	if pocErr != nil {
		t.Fatalf("gitnativepoc ChangedFilesSince() error = %v", pocErr)
	}
	if !containsString(pocFiles, filename) {
		t.Fatalf("gitnativepoc ChangedFilesSince() = %v, want it to contain verbatim %q", pocFiles, filename)
	}

	assertParityFileList(t, pocFiles, files)
}

// TestChangedFilesSince_Rename is MIGRATE: object.DiffTree (unlike
// Tree.Diff's default rename detection) reports a pure rename as a plain
// delete-plus-add pair, matching gitrepo's --no-renames CLI invocation —
// both sides must report the old path as deleted and the new path as added,
// never folded into one entry.
func TestChangedFilesSince_Rename(t *testing.T) {
	dir, oldName, newName := newRenameFixture(t)
	sinceSHA := firstCommitSHA(t, dir)

	cliRepo := gitrepo.New(dir)
	cliFiles, cliErr := cliRepo.ChangedFilesSince(sinceSHA)
	if cliErr != nil {
		t.Fatalf("gitrepo ChangedFilesSince() error = %v", cliErr)
	}

	poc, err := OpenRepo(dir)
	if err != nil {
		t.Fatalf("OpenRepo(%q) error = %v", dir, err)
	}
	pocFiles, pocErr := poc.ChangedFilesSince(sinceSHA)
	if pocErr != nil {
		t.Fatalf("gitnativepoc ChangedFilesSince() error = %v", pocErr)
	}

	for _, files := range [][]string{cliFiles, pocFiles} {
		if !containsString(files, oldName) {
			t.Errorf("ChangedFilesSince() = %v, want it to contain the deleted old path %q", files, oldName)
		}
		if !containsString(files, newName) {
			t.Errorf("ChangedFilesSince() = %v, want it to contain the added new path %q", files, newName)
		}
	}
	assertParityFileList(t, pocFiles, cliFiles)
}

// TestChangedFilesSince_NonHexSHA is MIGRATE: a non-hex sha returns each
// package's own ErrInvalidSHA before either side ever resolves or diffs
// anything.
func TestChangedFilesSince_NonHexSHA(t *testing.T) {
	dir := newRepoFixture(t)

	poc, err := OpenRepo(dir)
	if err != nil {
		t.Fatalf("OpenRepo(%q) error = %v", dir, err)
	}
	_, pocErr := poc.ChangedFilesSince("not-a-sha!!")

	_, cliErr := gitrepo.New(dir).ChangedFilesSince("not-a-sha!!")

	assertParityErrClassCrossTarget(t, pocErr, ErrInvalidSHA, cliErr, gitrepo.ErrInvalidSHA)
	if !errors.Is(pocErr, ErrInvalidSHA) {
		t.Errorf("gitnativepoc ChangedFilesSince(non-hex) error = %v, want ErrInvalidSHA", pocErr)
	}
	if !errors.Is(cliErr, gitrepo.ErrInvalidSHA) {
		t.Errorf("gitrepo ChangedFilesSince(non-hex) error = %v, want gitrepo.ErrInvalidSHA", cliErr)
	}
}

// firstCommitSHA returns the SHA of dir's very first commit — the "since"
// point every ChangedFilesSince fixture in this file diffs HEAD against —
// found by walking the log to its root via `git rev-list --max-parents=0`,
// since none of the harness fixtures expose their initial commit's SHA
// directly.
func firstCommitSHA(t *testing.T, dir string) string {
	t.Helper()

	stdout, stderr, code, err := runGit(t, dir, "rev-list", "--max-parents=0", "HEAD")
	if err != nil {
		t.Fatalf("git rev-list --max-parents=0 HEAD error = %v", err)
	}
	if code != 0 {
		t.Fatalf("git rev-list --max-parents=0 HEAD exited %d: %s", code, stderr)
	}
	return strings.TrimSpace(stdout)
}

// containsString reports whether haystack contains needle, used to assert a
// specific path is present in a ChangedFilesSince result without depending
// on the list's order.
func containsString(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

// TestSnapshotSHA_SetRef is MIGRATE: go-git's Reference lookup under
// refs/loomyard/snapshot/<key> returns the same SHA as gitrepo's
// `git rev-parse --verify --quiet` once the ref has been set.
func TestSnapshotSHA_SetRef(t *testing.T) {
	dir, key := newSnapshotRefFixture(t)

	cliSHA, cliErr := gitrepo.New(dir).SnapshotSHA(key)
	if cliErr != nil {
		t.Fatalf("gitrepo SnapshotSHA(%q) error = %v", key, cliErr)
	}

	poc, err := OpenRepo(dir)
	if err != nil {
		t.Fatalf("OpenRepo(%q) error = %v", dir, err)
	}
	pocSHA, pocErr := poc.SnapshotSHA(key)
	if pocErr != nil {
		t.Fatalf("gitnativepoc SnapshotSHA(%q) error = %v", key, pocErr)
	}

	assertParitySHA(t, pocSHA, cliSHA)
}

// TestSnapshotSHA_AbsentRef is MIGRATE: a key with no ref ever set reads as
// ("", nil) on both sides — an absent snapshot is a normal state, not a
// failure to surface.
func TestSnapshotSHA_AbsentRef(t *testing.T) {
	dir := newRepoFixture(t)
	const key = "never-set"

	cliSHA, cliErr := gitrepo.New(dir).SnapshotSHA(key)
	if cliErr != nil {
		t.Fatalf("gitrepo SnapshotSHA(%q) error = %v", key, cliErr)
	}
	if cliSHA != "" {
		t.Fatalf("gitrepo SnapshotSHA(%q) = %q, want \"\"", key, cliSHA)
	}

	poc, err := OpenRepo(dir)
	if err != nil {
		t.Fatalf("OpenRepo(%q) error = %v", dir, err)
	}
	pocSHA, pocErr := poc.SnapshotSHA(key)
	if pocErr != nil {
		t.Fatalf("gitnativepoc SnapshotSHA(%q) error = %v", key, pocErr)
	}

	assertParitySHA(t, pocSHA, cliSHA)
}

// TestRemoteName_OriginFallback is MIGRATE, asserted directly against a git
// fixture rather than gitrepo.Repo since remoteName is unexported on both
// sides and has no public gitrepo oracle: with no branch.<b>.remote
// configured, both go-git and the CLI fall back to "origin"; once
// branch.<b>.remote is set, both report the configured remote's name.
func TestRemoteName_OriginFallback(t *testing.T) {
	t.Run("NoRemoteConfigured", func(t *testing.T) {
		dir := newRepoFixture(t)

		poc, err := OpenRepo(dir)
		if err != nil {
			t.Fatalf("OpenRepo(%q) error = %v", dir, err)
		}
		if got := poc.remoteName(); got != "origin" {
			t.Errorf("remoteName() = %q, want %q", got, "origin")
		}
	})

	t.Run("RemoteConfigured", func(t *testing.T) {
		// Track a remote deliberately not named "origin" so this subtest
		// actually exercises the branch.<b>.remote config lookup rather than
		// coincidentally matching the same fallback value the other subtest
		// checks.
		container := t.TempDir()
		bare := filepath.Join(container, "upstream.git")
		lyxtest.MustRun(t, container, "git", "init", "--bare", "-b", "main", bare)
		dir := newRepoFixture(t)
		lyxtest.MustRun(t, dir, "git", "remote", "add", "upstream", bare)
		lyxtest.MustRun(t, dir, "git", "push", "-u", "upstream", "main")

		poc, err := OpenRepo(dir)
		if err != nil {
			t.Fatalf("OpenRepo(%q) error = %v", dir, err)
		}
		if got := poc.remoteName(); got != "upstream" {
			t.Errorf("remoteName() = %q, want %q (configured tracked remote)", got, "upstream")
		}
	})
}

// TestSnapshotSHA_InvalidKey is MIGRATE: a key rejected by validSnapshotKey
// returns each package's own ErrInvalidSnapshotKey without ever touching a
// ref.
func TestSnapshotSHA_InvalidKey(t *testing.T) {
	dir := newRepoFixture(t)
	// "key..bad" passes the character-class check (only alphanumerics, '.',
	// '_', '-' are involved) but is rejected by the separate ".." shape
	// check, exercising that specific rule rather than the character class.
	const key = "key..bad"

	poc, err := OpenRepo(dir)
	if err != nil {
		t.Fatalf("OpenRepo(%q) error = %v", dir, err)
	}
	_, pocErr := poc.SnapshotSHA(key)

	_, cliErr := gitrepo.New(dir).SnapshotSHA(key)

	assertParityErrClassCrossTarget(t, pocErr, ErrInvalidSnapshotKey, cliErr, gitrepo.ErrInvalidSnapshotKey)
	if !errors.Is(pocErr, ErrInvalidSnapshotKey) {
		t.Errorf("gitnativepoc SnapshotSHA(%q) error = %v, want ErrInvalidSnapshotKey", key, pocErr)
	}
	if !errors.Is(cliErr, gitrepo.ErrInvalidSnapshotKey) {
		t.Errorf("gitrepo SnapshotSHA(%q) error = %v, want gitrepo.ErrInvalidSnapshotKey", key, cliErr)
	}
}

// TestHasUnpushed is MIGRATE, asserted directly against fixture-derived
// expectations rather than gitrepo.Repo since hasUnpushed is unexported on
// both sides and has no public gitrepo oracle: HEAD ahead of its upstream
// reports true, an up-to-date checkout reports false, and no upstream
// configured at all reports true (never an error) — matching
// gitrepo.hasUnpushed's contract.
func TestHasUnpushed(t *testing.T) {
	t.Run("AheadOfUpstream", func(t *testing.T) {
		fixture := newBareRemoteFixture(t)
		writeFixtureFile(t, fixture.CloneA, "b.txt", "more")
		lyxtest.MustRun(t, fixture.CloneA, "git", "add", ".")
		lyxtest.MustRun(t, fixture.CloneA, "git", "commit", "-m", "second")

		poc, err := OpenRepo(fixture.CloneA)
		if err != nil {
			t.Fatalf("OpenRepo(%q) error = %v", fixture.CloneA, err)
		}
		got, err := poc.hasUnpushed()
		if err != nil {
			t.Fatalf("hasUnpushed() error = %v", err)
		}
		if !got {
			t.Errorf("hasUnpushed() = %v, want true (ahead of upstream)", got)
		}
	})

	t.Run("UpToDate", func(t *testing.T) {
		fixture := newBareRemoteFixture(t)

		poc, err := OpenRepo(fixture.CloneA)
		if err != nil {
			t.Fatalf("OpenRepo(%q) error = %v", fixture.CloneA, err)
		}
		got, err := poc.hasUnpushed()
		if err != nil {
			t.Fatalf("hasUnpushed() error = %v", err)
		}
		if got {
			t.Errorf("hasUnpushed() = %v, want false (up to date)", got)
		}
	})

	t.Run("NoUpstreamConfigured", func(t *testing.T) {
		dir := newRepoFixture(t)

		poc, err := OpenRepo(dir)
		if err != nil {
			t.Fatalf("OpenRepo(%q) error = %v", dir, err)
		}
		got, err := poc.hasUnpushed()
		if err != nil {
			t.Fatalf("hasUnpushed() error = %v", err)
		}
		if !got {
			t.Errorf("hasUnpushed() = %v, want true (no upstream configured)", got)
		}
	})
}

// TestIsStrictDescendant is MIGRATE, asserted directly against fixture SHAs
// rather than gitrepo.Repo since isStrictDescendant is unexported on both
// sides and has no public gitrepo oracle: an ancestor commit reports true,
// the same commit compared against itself reports false, and an unrelated
// commit — an orphan-branch root sharing no history with descendant —
// reports false.
func TestIsStrictDescendant(t *testing.T) {
	dir := newRepoFixture(t)
	ancestorSHA := firstCommitSHA(t, dir)

	writeFixtureFile(t, dir, "b.txt", "second")
	lyxtest.MustRun(t, dir, "git", "add", ".")
	lyxtest.MustRun(t, dir, "git", "commit", "-m", "second")
	descendantSHA := headSHA(t, dir)

	// An orphan branch shares no commit history with main at all, giving a
	// genuinely unrelated commit rather than merely an object missing from
	// the object store.
	lyxtest.MustRun(t, dir, "git", "checkout", "--orphan", "unrelated-branch")
	lyxtest.MustRun(t, dir, "git", "rm", "-rf", "--cached", ".")
	writeFixtureFile(t, dir, "c.txt", "unrelated")
	lyxtest.MustRun(t, dir, "git", "add", ".")
	lyxtest.MustRun(t, dir, "git", "commit", "-m", "unrelated root")
	unrelatedSHA := headSHA(t, dir)

	poc, err := OpenRepo(dir)
	if err != nil {
		t.Fatalf("OpenRepo(%q) error = %v", dir, err)
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
			if got := poc.isStrictDescendant(tt.ancestor, tt.descendant); got != tt.want {
				t.Errorf("isStrictDescendant(%q, %q) = %v, want %v", tt.ancestor, tt.descendant, got, tt.want)
			}
		})
	}
}

// headSHA returns the SHA dir's HEAD currently points at.
func headSHA(t *testing.T, dir string) string {
	t.Helper()

	stdout, stderr, code, err := runGit(t, dir, "rev-parse", "HEAD")
	if err != nil {
		t.Fatalf("git rev-parse HEAD error = %v", err)
	}
	if code != 0 {
		t.Fatalf("git rev-parse HEAD exited %d: %s", code, stderr)
	}
	return strings.TrimSpace(stdout)
}
