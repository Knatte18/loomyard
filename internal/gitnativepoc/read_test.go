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
