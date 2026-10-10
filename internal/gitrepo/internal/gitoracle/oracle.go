// Package gitoracle is the differential-parity harness's CLI oracle for internal/gitrepo.
// For every gitrepo method that reads through go-git, it reimplements the same read directly on gitexec.RunGit, independent of any gitrepo method.
// The duplication is deliberate.
//
// # Why an oracle, not a copy
//
// The parity harness being lifted from internal/gitnativepoc used gitrepo's own CLI-backed methods as its reference side, because at the time gitrepo WAS the CLI implementation.
// The moment gitrepo's methods return go-git results, a harness that still calls gitrepo for its "truth" compares go-git against go-git:
// it asserts nothing while staying green, which is worse than no test because it looks like coverage.
// This package keeps a second, independent implementation of every parsing convention production deleted — the `-z` NUL-split, the `--verify --quiet` exit-0/1/other convention, and the unborn-HEAD stderr sniff — so the thing that parsing validates (gitrepo's methods) can stop depending on it.
// Replacing it with calls into gitrepo would silently turn every parity test into a tautology.
//
// The package imports stdlib and internal/gitexec only, never internal/gitrepo or internal/gitkit;
// leaf_enforcement_test.go enforces that.
// Its callers are tests alone, in both gitrepo test packages.
//
// Each function takes an explicit worktree directory, never a stored path,
// so a caller can point it at either side of a linked-worktree fixture.
package gitoracle

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitexec"
)

// ErrNoCommits is the oracle's own sentinel for "no commits yet", distinct from gitrepo.ErrNoCommits for independence.
var ErrNoCommits = errors.New("oracle: repository has no commits")

// CurrentSHA reimplements gitrepo's CurrentSHA on `git rev-parse HEAD`, mapping unborn-HEAD stderr to ErrNoCommits.
func CurrentSHA(t testing.TB, dir string) (string, error) {
	t.Helper()

	//gitexec:raw the oracle reads the exit code and stderr to classify unborn HEAD
	stdout, stderr, code, err := gitexec.RunGit([]string{"rev-parse", "HEAD"}, dir)
	if err != nil {
		return "", err
	}
	if code != 0 {
		if strings.Contains(stderr, "ambiguous argument 'HEAD'") || strings.Contains(stderr, "unknown revision") {
			return "", ErrNoCommits
		}
		return "", fmt.Errorf("oracle: git rev-parse HEAD: %s", stderr)
	}
	return strings.TrimSpace(stdout), nil
}

// SHAExists reimplements gitrepo's SHAExists directly on `git rev-parse --verify --quiet <sha>^{commit}`:
// the `^{commit}` peel is contractual, not incidental — it is what makes a tree or blob SHA resolve as absent rather than present.
// Exit 0 means true, exit 1 means false;
// anything else is an oracle failure the calling test must not swallow, since a silent third case here would hide a real oracle bug behind a bool.
func SHAExists(t testing.TB, dir, sha string) bool {
	t.Helper()

	//gitexec:raw the oracle reads the exit code: 0 present, 1 absent, anything else fatal
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

// ChangedFilesSince reimplements gitrepo's ChangedFilesSince directly on `git diff --name-only -z --no-renames <sha>..HEAD`:
// -z terminates each path with NUL and disables core.quotePath's C-style escaping, so a non-ASCII filename comes back verbatim;
// --no-renames is what keeps a rename reported as delete-plus-add rather than folded into one entry.
func ChangedFilesSince(t testing.TB, dir, sha string) ([]string, error) {
	t.Helper()

	//gitexec:raw the oracle reads the exit code and stderr itself
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

// GitDir reimplements gitrepo's GitDir on `git rev-parse --git-dir`, absolutized against dir when git answers a relative path.
func GitDir(t testing.TB, dir string) (string, error) {
	t.Helper()
	return revParsePath(dir, "--git-dir")
}

// CommonDir reimplements gitrepo's CommonDir on `git rev-parse --git-common-dir`, absolutized against dir when git answers a relative path.
func CommonDir(t testing.TB, dir string) (string, error) {
	t.Helper()
	return revParsePath(dir, "--git-common-dir")
}

// Toplevel reimplements gitrepo's Toplevel on `git rev-parse --show-toplevel`.
func Toplevel(t testing.TB, dir string) (string, error) {
	t.Helper()
	return revParsePath(dir, "--show-toplevel")
}

// revParsePath runs `git rev-parse <flag>` in dir and returns the path it prints, joined onto dir when git prints it relative.
func revParsePath(dir, flag string) (string, error) {
	stdout, err := gitexec.Run([]string{"rev-parse", flag}, dir)
	if err != nil {
		return "", fmt.Errorf("oracle: git rev-parse %s: %w", flag, err)
	}
	path := strings.TrimSpace(stdout)
	if !filepath.IsAbs(path) {
		path = filepath.Join(dir, path)
	}
	return path, nil
}

// CurrentBranch reimplements gitrepo's CurrentBranch directly on `git symbolic-ref --short HEAD`, which fails on a detached HEAD and succeeds — printing the branch name — even on an unborn or orphan HEAD that has never been committed.
func CurrentBranch(t testing.TB, dir string) (string, error) {
	t.Helper()

	//gitexec:raw the oracle reads the exit code and stderr itself
	stdout, stderr, code, err := gitexec.RunGit([]string{"symbolic-ref", "--short", "HEAD"}, dir)
	if err != nil {
		return "", err
	}
	if code != 0 {
		return "", fmt.Errorf("oracle: git symbolic-ref --short HEAD: %s", stderr)
	}
	return strings.TrimSpace(stdout), nil
}
