// worktree.go holds the read-only scans of the working tree and the index.
// WorktreeChangedFiles reports every file with an uncommitted change.
// UntrackedFiles lists the untracked, non-ignored files.
// TrackedRootEntries lists the top-level names the index tracks.

package gitrepo

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/format/gitignore"
	"github.com/go-git/go-git/v5/storage/filesystem"
)

// WorktreeChangedFiles returns the repo-relative paths of every file with an uncommitted change:
// tracked-and-modified, staged, or untracked.
// The set is not ordered contractually.
// It reads .git/info/exclude itself, since go-git's Worktree.Status() skips that file because of path chrooting.
func (r *Repo) WorktreeChangedFiles() ([]string, error) {
	status, err := r.worktreeStatus()
	if err != nil {
		return nil, err
	}

	var files []string
	for path, fileStatus := range status {
		if fileStatus.Staging != git.Unmodified || fileStatus.Worktree != git.Unmodified {
			files = append(files, path)
		}
	}
	return files, nil
}

// UntrackedFiles returns the repo-relative paths of every untracked, non-ignored file, sorted.
// It honors .git/info/exclude, so a junctioned `_lym` or `.lym` listed there never appears.
// It returns an empty, never nil, slice when there are none.
func (r *Repo) UntrackedFiles() ([]string, error) {
	status, err := r.worktreeStatus()
	if err != nil {
		return nil, err
	}

	files := []string{}
	for path, fileStatus := range status {
		if fileStatus.Worktree == git.Untracked {
			files = append(files, path)
		}
	}
	sort.Strings(files)
	return files, nil
}

// worktreeStatus returns go-git's worktree status, with .git/info/exclude honored and read in one readGoGit call.
func (r *Repo) worktreeStatus() (git.Status, error) {
	return readGoGit(r, func(repo *git.Repository) (git.Status, error) {
		wt, err := repo.Worktree()
		if err != nil {
			return nil, fmt.Errorf("gitrepo: resolve worktree: %w", err)
		}

		excludes, err := readGitDirExcludePatterns(repo)
		if err != nil {
			return nil, fmt.Errorf("gitrepo: read git-dir exclude patterns: %w", err)
		}
		wt.Excludes = excludes

		status, err := wt.Status()
		if err != nil {
			return nil, fmt.Errorf("gitrepo: worktree status: %w", err)
		}
		return status, nil
	})
}

// readGitDirExcludePatterns returns the patterns of repo's info/exclude file as gitignore.Pattern values.
// Comment and blank lines are skipped.
// A missing file yields a nil slice and no error.
func readGitDirExcludePatterns(repo *git.Repository) ([]gitignore.Pattern, error) {
	storer, ok := repo.Storer.(*filesystem.Storage)
	if !ok {
		return nil, nil
	}

	f, err := storer.Filesystem().Open("info/exclude")
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	var patterns []gitignore.Pattern
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "#") || strings.TrimSpace(line) == "" {
			continue
		}
		patterns = append(patterns, gitignore.ParsePattern(line, nil))
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("gitrepo: scan info/exclude: %w", err)
	}
	return patterns, nil
}

// TrackedRootEntries returns, sorted and deduplicated, the first path segment of every entry of the repository's index.
// A root entry staged and never committed counts as tracked, and an untracked one does not.
// An absent index yields an empty, never nil, result.
func (r *Repo) TrackedRootEntries() ([]string, error) {
	return readGoGit(r, func(repo *git.Repository) ([]string, error) {
		idx, err := repo.Storer.Index()
		if err != nil {
			return nil, fmt.Errorf("gitrepo: read index: %w", err)
		}
		seen := map[string]bool{}
		entries := []string{}
		for _, entry := range idx.Entries {
			root, _, _ := strings.Cut(entry.Name, "/")
			if !seen[root] {
				seen[root] = true
				entries = append(entries, root)
			}
		}
		sort.Strings(entries)
		return entries, nil
	})
}
