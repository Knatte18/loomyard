//go:build integration

// oracle_test.go implements the differential-parity harness's CLI oracle: for
// every gitrepo method batches 3-4 are about to move onto go-git, this file
// reimplements the same read directly on gitexec.RunGit, independent of any
// gitrepo method. This duplication is deliberate, not an oversight — see the
// "why an oracle, not a copy" note below.
//
// # Why this duplication exists
//
// The parity harness being lifted from internal/gitnativepoc used gitrepo's
// own CLI-backed methods as its reference side, because at the time gitrepo
// WAS the CLI implementation. The moment gitrepo's methods start returning
// go-git results (batches 3-4), a harness that still calls gitrepo for its
// "truth" would be comparing go-git against go-git: it would assert nothing
// while continuing to pass green, which is worse than no test at all because
// it looks like coverage. So this file keeps a second, independent
// implementation of every parsing convention production is about to delete —
// the `-z` NUL-split, the `--verify --quiet` exit-0/1/other convention, and
// the unborn-HEAD stderr sniff — precisely so the thing that parsing
// validates (gitrepo's methods) can stop depending on it. Deleting this file
// in favor of calling gitrepo directly would silently turn every parity test
// in this package into a tautology.
//
// Each oracle function takes an explicit worktree directory (never a stored
// path) so a caller can point it at either side of a linked-worktree fixture.

package gitrepo_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitexec"
)

// errOracleNoCommits is the oracle's own sentinel for "no commits yet" —
// intentionally a distinct value from gitrepo.ErrNoCommits, since the whole
// point of an independent oracle is that it shares no code, including error
// sentinels, with the implementation it checks.
var errOracleNoCommits = errors.New("oracle: repository has no commits")

// oracleCurrentSHA reimplements CurrentSHA directly on `git rev-parse HEAD`:
// the unborn-HEAD stderr shapes git prints ("ambiguous argument 'HEAD'" or
// "unknown revision") map to errOracleNoCommits; any other non-zero exit or
// spawn failure is a genuine oracle failure.
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

// oracleSHAExists reimplements SHAExists directly on
// `git rev-parse --verify --quiet <sha>^{commit}`: the `^{commit}` peel is
// contractual, not incidental — it is what makes a tree or blob SHA resolve
// as absent rather than present. Exit 0 means true, exit 1 means false;
// anything else is an oracle failure the calling test must not swallow, since
// a silent third case here would hide a real oracle bug behind a bool.
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
// `git diff --name-only -z --no-renames <sha>..HEAD`: -z terminates each path
// with NUL and disables core.quotePath's C-style escaping, so a non-ASCII
// filename comes back verbatim; --no-renames is what keeps a rename reported
// as delete-plus-add rather than folded into one entry.
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

// oracleCurrentBranch reimplements CurrentBranch directly on
// `git symbolic-ref --short HEAD`, which fails on a detached HEAD and
// succeeds — printing the branch name — even on an unborn or orphan HEAD that
// has never been committed.
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

// oracleSnapshotSHA reimplements SnapshotSHA's local ref read directly on
// `git rev-parse --verify --quiet refs/loomyard/snapshot/<key>` — the ref
// read only, since SnapshotSHA's best-effort fetch is CLI-bound rather than
// migrating (per the Shared Decision) and so needs no oracle. Exit 0 returns
// the SHA, exit 1 returns ("", nil) for a verified-absent ref, and any other
// exit (e.g. "not a git repository") is a genuine failure — the three-way
// distinction gitrepo.SnapshotSHA's own doc calls out.
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

// oracleRemoteName reimplements remoteName directly on `git symbolic-ref`
// and `git config --get branch.<name>.remote`, falling back to "origin" on
// any failure at either step, exactly matching remoteName's fallback posture.
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

// oracleHasUnpushed reimplements hasUnpushed directly on
// `git rev-list --count @{u}..HEAD`: any non-zero exit — no upstream
// configured, or any other rev-list failure — means true, exactly matching
// push.go's documented behaviour, so the first push of a branch still
// happens rather than being skipped as nothing-to-do.
func oracleHasUnpushed(t *testing.T, dir string) (bool, error) {
	t.Helper()

	stdout, _, code, err := gitexec.RunGit([]string{"rev-list", "--count", "@{u}..HEAD"}, dir)
	if err != nil {
		return false, err
	}
	if code != 0 {
		return true, nil
	}
	return strings.TrimSpace(stdout) != "0", nil
}

// oracleIsStrictDescendant reimplements isStrictDescendant directly on
// `git merge-base --is-ancestor` plus an explicit equality check, since
// merge-base --is-ancestor alone would report a commit compared against
// itself as true (ancestor of itself), not the strict inequality the method
// contracts on.
func oracleIsStrictDescendant(t *testing.T, dir, ancestor, descendant string) bool {
	t.Helper()

	if ancestor == descendant {
		return false
	}
	_, _, code, err := gitexec.RunGit([]string{"merge-base", "--is-ancestor", ancestor, descendant}, dir)
	return err == nil && code == 0
}
