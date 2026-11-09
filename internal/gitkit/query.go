// query.go holds the git query and commit helpers tests share.
// Every spawn goes through Git and gitExit in gitkit.go, so this file imports no os/exec.

package gitkit

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/dotgit"
)

// RevParse returns `git rev-parse <rev>` in dir.
// dir may be a bare repository.
// HEAD in a worktree whose HEAD names a commit is read from the repository's files with no git process;
// every other rev is answered by git.
func RevParse(tb testing.TB, dir, rev string) string {
	tb.Helper()
	if rev == "HEAD" {
		if sha, ok := headCommit(dir); ok {
			return sha
		}
	}
	return Git(tb, dir, "rev-parse", rev)
}

// headCommit returns the commit HEAD resolves to in the worktree holding dir, as `git rev-parse HEAD` prints it.
// ok is false where the files cannot answer:
// dir in no worktree, a bare repository, refs that are not files, and a HEAD that names no commit yet.
func headCommit(dir string) (sha string, ok bool) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", false
	}
	root, _, err := dotgit.FindRoot(abs)
	if err != nil {
		return "", false
	}
	worktrees, err := dotgit.ListWorktrees(abs)
	if err != nil {
		return "", false
	}
	want := resolvedPath(root)
	for _, worktree := range worktrees {
		if resolvedPath(worktree.Path) != want {
			continue
		}
		if strings.Trim(worktree.Head, "0") == "" {
			return "", false
		}
		return worktree.Head, true
	}
	return "", false
}

// resolvedPath returns path with its symlinks resolved, or cleaned when it cannot be resolved.
func resolvedPath(path string) string {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return resolved
	}
	return filepath.Clean(path)
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

// CommitFile writes rel under dir with content, stages it alone, commits it with msg and returns the new HEAD SHA.
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

// CommitFileOnBranch checks out branch in dir, then writes rel with content, stages it alone, and commits it with msg.
// It returns the new HEAD SHA.
func CommitFileOnBranch(tb testing.TB, dir, branch, rel, content, msg string) string {
	tb.Helper()

	Git(tb, dir, "checkout", branch)
	return CommitFile(tb, dir, rel, content, msg)
}

// SkipBelowGitVersion skips tb when the git on PATH is older than minimum, a dotted version such as "2.48".
// The skip names the version found, minimum and needs, the feature that wants it.
func SkipBelowGitVersion(tb testing.TB, minimum, needs string) {
	tb.Helper()

	found := parseGitVersion(Git(tb, tb.TempDir(), "version"))
	if compareDottedVersions(found, minimum) < 0 {
		tb.Skipf("git %s is below %s, which %s needs", found, minimum, needs)
	}
}

// parseGitVersion returns the dotted numeric version in `git version` output, without any vendor suffix.
// "git version 2.47.1.windows.1" yields "2.47.1" and "git version 2.39.5 (Apple Git-154)" yields "2.39.5".
func parseGitVersion(output string) string {
	found := strings.TrimPrefix(strings.TrimSpace(output), "git version ")
	if end := strings.IndexFunc(found, func(r rune) bool { return r != '.' && (r < '0' || r > '9') }); end >= 0 {
		found = found[:end]
	}
	return strings.TrimRight(found, ".")
}

// compareDottedVersions compares two dotted numeric versions component by component, a missing component counting as 0.
func compareDottedVersions(a, b string) int {
	as, bs := strings.Split(a, "."), strings.Split(b, ".")
	for i := range max(len(as), len(bs)) {
		if diff := dottedComponent(as, i) - dottedComponent(bs, i); diff != 0 {
			return diff
		}
	}
	return 0
}

func dottedComponent(parts []string, i int) int {
	if i >= len(parts) {
		return 0
	}
	n, _ := strconv.Atoi(parts[i])
	return n
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
