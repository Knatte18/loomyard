// Package gitoracle is the differential-parity harness's CLI oracle for internal/gitrepo.
// For every gitrepo method that reads through go-git, the package reimplements the same read on gitexec.RunGit.
// It calls no gitrepo method: the duplication is deliberate.
//
// # Why an oracle, not a copy
//
// A parity harness that takes its reference from gitrepo's own methods compares go-git against go-git.
// Such a harness asserts nothing while it stays green, which is worse than no test because it looks like coverage.
// This package keeps a second, independent implementation of the CLI parsing conventions:
// the `-z` NUL-split, the `--verify --quiet` exit-0/1/other convention, and the unborn-HEAD stderr sniff.
// Replacing it with calls into gitrepo would silently turn every parity test into a tautology.
//
// The package imports stdlib and internal/gitexec only, never internal/gitrepo or internal/gitkit;
// leaf_enforcement_test.go enforces that.
// Its callers are tests only.
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

// ErrNoCommits is the sentinel for a repository with no commits yet.
// It is defined here rather than shared, so the oracle does not depend on the code it checks.
var ErrNoCommits = errors.New("oracle: repository has no commits")

// CurrentSHA returns the SHA that HEAD resolves to in dir, from `git rev-parse HEAD`.
// It maps the stderr of an unborn HEAD to ErrNoCommits.
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

// SHAExists reports whether sha names a commit in dir, by running `git rev-parse --verify --quiet <sha>^{commit}`.
// The `^{commit}` peel is contractual, not incidental:
// it makes a tree or blob SHA resolve as absent rather than present.
// Exit 0 means present and exit 1 means absent;
// any other exit is an oracle failure that the calling test must not swallow.
// A silent third case would hide a real oracle bug behind a bool.
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

// ChangedFilesSince lists the paths that differ between sha and HEAD in dir.
// It runs `git diff --name-only -z --no-renames <sha>..HEAD`.
// -z terminates each path with NUL and disables core.quotePath's C-style escaping,
// so a non-ASCII filename comes back verbatim.
// --no-renames keeps a rename reported as a delete plus an add, not folded into one entry.
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

// GitDir returns the git directory of the repository at dir, from `git rev-parse --git-dir`.
// It joins the result onto dir when git prints a relative path.
func GitDir(t testing.TB, dir string) (string, error) {
	t.Helper()
	return revParsePath(dir, "--git-dir")
}

// CommonDir returns the common git directory of the repository at dir, from `git rev-parse --git-common-dir`.
// It joins the result onto dir when git prints a relative path.
func CommonDir(t testing.TB, dir string) (string, error) {
	t.Helper()
	return revParsePath(dir, "--git-common-dir")
}

// BranchExists reports whether refs/heads/<branch> exists in dir.
// It runs `git rev-parse --verify --quiet` on that ref: exit 0 means present, exit 1 absent.
func BranchExists(t testing.TB, dir, branch string) bool {
	t.Helper()

	_, found := verifyQuiet(t, dir, "refs/heads/"+branch)
	return found
}

// RefSHA returns the SHA that ref resolves to in dir, from `git rev-parse --verify --quiet <ref>`.
// The bool is false when the ref does not resolve.
func RefSHA(t testing.TB, dir, ref string) (string, bool) {
	t.Helper()
	return verifyQuiet(t, dir, ref)
}

// HeadRef returns the branch name HEAD points to in dir, and reports whether HEAD is detached.
// `git rev-parse --abbrev-ref HEAD` prints `HEAD` for a detached HEAD.
// An unborn branch makes that command fail, so the name comes from `git branch --show-current` instead.
func HeadRef(t testing.TB, dir string) (branch string, detached bool, err error) {
	t.Helper()

	//gitexec:raw the oracle reads the exit code to tell an unborn HEAD from a branch with a commit
	stdout, stderr, code, runErr := gitexec.RunGit([]string{"rev-parse", "--abbrev-ref", "HEAD"}, dir)
	if runErr != nil {
		return "", false, runErr
	}
	if code == 0 {
		name := strings.TrimSpace(stdout)
		if name == "HEAD" {
			return "", true, nil
		}
		return name, false, nil
	}
	if !strings.Contains(stderr, "ambiguous argument 'HEAD'") && !strings.Contains(stderr, "unknown revision") {
		return "", false, fmt.Errorf("oracle: git rev-parse --abbrev-ref HEAD: %s", stderr)
	}

	current, err := gitexec.Run([]string{"branch", "--show-current"}, dir)
	if err != nil {
		return "", false, fmt.Errorf("oracle: git branch --show-current: %w", err)
	}
	return strings.TrimSpace(current), false, nil
}

// RefTree returns the tree SHA of ref in dir, from `git rev-parse <ref>^{tree}`.
func RefTree(t testing.TB, dir, ref string) (string, error) {
	t.Helper()

	stdout, err := gitexec.Run([]string{"rev-parse", ref + "^{tree}"}, dir)
	if err != nil {
		return "", fmt.Errorf("oracle: git rev-parse %s^{tree}: %w", ref, err)
	}
	return strings.TrimSpace(stdout), nil
}

// Upstream returns the SHA of the branch that branch tracks in dir.
// It resolves `<branch>@{upstream}` with `git rev-parse --verify --quiet`.
// The bool is false when git reports no upstream or one that does not resolve.
func Upstream(t testing.TB, dir, branch string) (string, bool) {
	t.Helper()
	return verifyQuiet(t, dir, branch+"@{upstream}")
}

// verifyQuiet runs `git rev-parse --verify --quiet <rev>` in dir and classifies its exit code.
// Exit 0 prints the SHA, exit 1 means the revision does not resolve, and any other exit fails the test.
func verifyQuiet(t testing.TB, dir, rev string) (string, bool) {
	t.Helper()

	//gitexec:raw the oracle reads the exit code: 0 present, 1 absent, anything else fatal
	stdout, stderr, code, err := gitexec.RunGit([]string{"rev-parse", "--verify", "--quiet", rev}, dir)
	if err != nil {
		t.Fatalf("oracle: git rev-parse --verify --quiet %s spawn error = %v", rev, err)
	}
	switch code {
	case 0:
		return strings.TrimSpace(stdout), true
	case 1:
		return "", false
	default:
		t.Fatalf("oracle: git rev-parse --verify --quiet %s exited %d: %s", rev, code, stderr)
		return "", false
	}
}

// revParsePath runs `git rev-parse <flag>` in dir and returns the path it prints.
// It joins the path onto dir when git prints it relative.
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

// CurrentBranch returns the branch name HEAD points to in dir, from `git symbolic-ref --short HEAD`.
// It fails on a detached HEAD, but it succeeds on an unborn or orphan HEAD and prints the branch name.
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
