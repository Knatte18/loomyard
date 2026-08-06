// worktree.go adds WorktreeChangedFiles, the read-only working-tree scan Fabric.Status (a later
// fabricengine batch) uses to report uncommitted changes — as opposed to gitrepo.go's
// ChangedFilesSince, which compares two committed trees and never looks at the working tree at all.

package gitrepo

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/format/gitignore"
	"github.com/go-git/go-git/v5/storage/filesystem"
)

// WorktreeChangedFiles returns repo-relative paths of every file with an uncommitted change
// (tracked-and-modified, staged, or untracked).
// The set is not ordered contractually.
// It manually reads .git/info/exclude since go-git's Worktree.Status() skips it due to path
// chrooting.
func (r *Repo) WorktreeChangedFiles() ([]string, error) {
	repo, err := r.goGit()
	if err != nil {
		return nil, err
	}

	r.goGitMu.Lock()
	defer r.goGitMu.Unlock()

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

	var files []string
	for path, fileStatus := range status {
		if fileStatus.Staging != git.Unmodified || fileStatus.Worktree != git.Unmodified {
			files = append(files, path)
		}
	}
	return files, nil

}

// readGitDirExcludePatterns reads repo's info/exclude file through the git-dir
// filesystem, returning its non-comment, non-blank lines as gitignore.Pattern
// values. A missing file returns empty, nil slice.
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
