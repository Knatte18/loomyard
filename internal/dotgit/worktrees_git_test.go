// worktrees_git_test.go checks ListWorktrees against `git worktree list --porcelain` on repositories git itself wrote.

package dotgit_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/Knatte18/loomyard/internal/dotgit"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// porcelainWorktrees parses `git worktree list --porcelain`, run in dir, into the entries ListWorktrees returns.
func porcelainWorktrees(t *testing.T, dir string) []dotgit.Worktree {
	t.Helper()
	var worktrees []dotgit.Worktree
	for _, block := range strings.Split(gitkit.Git(t, dir, "worktree", "list", "--porcelain"), "\n\n") {
		var worktree dotgit.Worktree
		for _, line := range strings.Split(strings.TrimSpace(block), "\n") {
			key, value, _ := strings.Cut(line, " ")
			switch key {
			case "worktree":
				worktree.Path = value
			case "HEAD":
				worktree.Head = value
			case "branch":
				worktree.Branch = value
			case "locked":
				worktree.Locked = true
			case "prunable":
				worktree.Prunable = true
			case "bare":
				t.Fatalf("git worktree list in %s reports a bare repository", dir)
			}
		}
		worktrees = append(worktrees, worktree)
	}
	return worktrees
}

// newRepository initialises a repository at root/repo with one commit and returns its path.
// root is resolved through symlinks first, so the paths git prints and the ones the test spells agree.
func newRepository(t *testing.T) (root, repo string) {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("resolve temp dir: %v", err)
	}
	repo = filepath.Join(root, "repo")
	gitkit.Git(t, root, "init", "repo")
	gitkit.CommitFile(t, repo, "a.txt", "one", "first")
	return root, repo
}

// TestListWorktrees_MatchesGitPorcelain lists one repository from its main worktree, a symlink to it,
// and a subdirectory of a linked worktree, each time equal to git's own list.
// The repository holds a branch worktree whose ref is only in packed-refs and one with a loose ref.
// It holds a detached one, a locked one with a reason and its directory gone, and a locked one without.
// It holds a prunable one, one whose gitdir file is removed and one whose HEAD file is removed.
func TestListWorktrees_MatchesGitPorcelain(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	root, repo := newRepository(t)
	worktree := func(name string) string { return filepath.Join(root, name) }
	gitkit.Git(t, repo, "worktree", "add", "-b", "packed", worktree("packed"))
	gitkit.Git(t, repo, "pack-refs", "--all")
	gitkit.Git(t, repo, "worktree", "add", "-b", "loose", worktree("loose"))
	gitkit.CommitFile(t, worktree("loose"), "b.txt", "two", "second")
	for _, name := range []string{"detached", "locked-reason", "locked", "prunable", "no-gitdir", "no-head"} {
		gitkit.Git(t, repo, "worktree", "add", "--detach", worktree(name))
	}
	gitkit.Git(t, repo, "worktree", "lock", "--reason", "moved away", worktree("locked-reason"))
	gitkit.Git(t, repo, "worktree", "lock", worktree("locked"))
	for _, path := range []string{
		worktree("locked-reason"),
		worktree("prunable"),
		filepath.Join(repo, ".git", "worktrees", "no-gitdir", "gitdir"),
		filepath.Join(repo, ".git", "worktrees", "no-head", "HEAD"),
	} {
		if err := os.RemoveAll(path); err != nil {
			t.Fatalf("remove %s: %v", path, err)
		}
	}
	subdirectory := filepath.Join(worktree("loose"), "sub")
	if err := os.MkdirAll(subdirectory, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", subdirectory, err)
	}

	starts := []string{repo, subdirectory}
	if runtime.GOOS != "windows" {
		link := filepath.Join(root, "link")
		if err := os.Symlink(repo, link); err != nil {
			t.Fatalf("symlink %s: %v", link, err)
		}
		starts = append(starts, link)
	}

	for _, start := range starts {
		got, err := dotgit.ListWorktrees(start)
		if err != nil {
			t.Fatalf("ListWorktrees(%s): %v", start, err)
		}
		if diff := cmp.Diff(porcelainWorktrees(t, start), got); diff != "" {
			t.Errorf("ListWorktrees(%s) mismatch with git (-git +got):\n%s", start, diff)
		}
	}
}

// TestListWorktrees_RelativeGitdirMatchesGit lists a linked worktree whose gitdir file holds a relative path.
// Only git 2.48 and later write one.
// The test expects the path git reports.
func TestListWorktrees_RelativeGitdirMatchesGit(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()
	gitkit.SkipBelowGitVersion(t, "2.48", "a relative worktree gitdir")

	root, repo := newRepository(t)
	gitkit.Git(t, repo, "worktree", "add", "--relative-paths", "--detach", filepath.Join(root, "relative"))
	gitdirFile := filepath.Join(repo, ".git", "worktrees", "relative", "gitdir")
	content, err := os.ReadFile(gitdirFile)
	if err != nil {
		t.Fatalf("read %s: %v", gitdirFile, err)
	}
	if filepath.IsAbs(strings.TrimSpace(string(content))) {
		t.Fatalf("%s holds the absolute path %q; want a relative one", gitdirFile, content)
	}

	got, err := dotgit.ListWorktrees(repo)
	if err != nil {
		t.Fatalf("ListWorktrees(%s): %v", repo, err)
	}
	if diff := cmp.Diff(porcelainWorktrees(t, repo), got); diff != "" {
		t.Errorf("ListWorktrees(%s) mismatch with git (-git +got):\n%s", repo, diff)
	}
}
