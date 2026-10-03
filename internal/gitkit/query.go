// query.go holds the git query and commit helpers tests share.
// Every spawn goes through Git and gitExit in gitkit.go, so this file imports no os/exec.

package gitkit

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// RevParse returns `git rev-parse <rev>` in dir.
// dir may be a bare repository.
func RevParse(tb testing.TB, dir, rev string) string {
	tb.Helper()
	return Git(tb, dir, "rev-parse", rev)
}

// CurrentBranch returns the branch HEAD points at in dir.
func CurrentBranch(tb testing.TB, dir string) string {
	tb.Helper()
	return Git(tb, dir, "rev-parse", "--abbrev-ref", "HEAD")
}

// BranchExists reports whether the local branch exists in dir.
// A missing branch answers false rather than failing the test.
func BranchExists(tb testing.TB, dir, branch string) bool {
	tb.Helper()

	_, code, err := gitExit(dir, "show-ref", "--verify", "--quiet", "refs/heads/"+branch)
	if err != nil {
		tb.Fatalf("git show-ref in %s: %v", dir, err)
	}
	switch code {
	case 0:
		return true
	case 1:
		return false
	}
	tb.Fatalf("git show-ref refs/heads/%s in %s: exit %d", branch, dir, code)
	return false
}

// IsAncestor reports whether ancestor is an ancestor of descendant in dir.
// Exit 1 answers false; any other non-zero exit fails the test.
func IsAncestor(tb testing.TB, dir, ancestor, descendant string) bool {
	tb.Helper()

	out, code, err := gitExit(dir, "merge-base", "--is-ancestor", ancestor, descendant)
	if err != nil {
		tb.Fatalf("git merge-base in %s: %v", dir, err)
	}
	switch code {
	case 0:
		return true
	case 1:
		return false
	}
	tb.Fatalf("git merge-base --is-ancestor %s %s in %s: exit %d; output: %s", ancestor, descendant, dir, code, out)
	return false
}

// RevListCount returns the number of commits `git rev-list --count <args>` reports in dir.
func RevListCount(tb testing.TB, dir string, args ...string) int {
	tb.Helper()

	out := Git(tb, dir, append([]string{"rev-list", "--count"}, args...)...)
	n, err := strconv.Atoi(out)
	if err != nil {
		tb.Fatalf("git rev-list --count in %s: unparseable %q: %v", dir, out, err)
	}
	return n
}

// LsFiles returns the lines of `git ls-files <args>` in dir.
func LsFiles(tb testing.TB, dir string, args ...string) []string {
	tb.Helper()
	return nonBlankLines(Git(tb, dir, append([]string{"ls-files"}, args...)...))
}

// ExcludeLines returns the non-blank lines of dir's info/exclude.
// The file is located through git so a linked worktree resolves to its common gitdir.
func ExcludeLines(tb testing.TB, dir string) []string {
	tb.Helper()

	path := Git(tb, dir, "rev-parse", "--git-path", "info/exclude")
	if !filepath.IsAbs(path) {
		path = filepath.Join(dir, path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		tb.Fatalf("read %s: %v", path, err)
	}
	return nonBlankLines(string(data))
}

// CommitFile writes rel under dir with content, stages it alone, commits it with msg and returns
// the new HEAD SHA.
// Parent directories of rel are created.
func CommitFile(tb testing.TB, dir, rel, content, msg string) string {
	tb.Helper()

	path := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		tb.Fatalf("mkdir for %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		tb.Fatalf("write %s: %v", path, err)
	}
	Git(tb, dir, "add", "--", rel)
	Git(tb, dir, "commit", "-m", msg, "--", rel)
	return RevParse(tb, dir, "HEAD")
}

// CommitFileOnBranch checks branch out in dir, then behaves as CommitFile.
func CommitFileOnBranch(tb testing.TB, dir, branch, rel, content, msg string) string {
	tb.Helper()

	Git(tb, dir, "checkout", branch)
	return CommitFile(tb, dir, rel, content, msg)
}

// nonBlankLines splits s into lines and drops the blank ones.
func nonBlankLines(s string) []string {
	var lines []string
	for _, line := range strings.Split(s, "\n") {
		if strings.TrimSpace(line) != "" {
			lines = append(lines, line)
		}
	}
	return lines
}
