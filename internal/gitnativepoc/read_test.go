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
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitrepo"
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
