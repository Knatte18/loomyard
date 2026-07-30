// worktree.go adds WorktreeChangedFiles, the read-only working-tree scan
// Fabric.Status (a later fabricengine batch) uses to report uncommitted
// changes — as opposed to gitrepo.go's ChangedFilesSince, which compares two
// committed trees and never looks at the working tree at all.

package gitrepo

import (
	"fmt"

	"github.com/go-git/go-git/v5"
)

// WorktreeChangedFiles returns the repo-relative paths of every file with an
// uncommitted change in this Repo's working tree — tracked-and-modified,
// staged, or untracked alike — via go-git's Worktree.Status(). The returned
// set is de-duplicated and its order is not contractual, mirroring
// ChangedFilesSince's set posture: this describes what changed, not a
// sequence.
//
// Status() builds/uses go-git's lazy object index (it walks the whole
// worktree against HEAD's tree to classify every entry), so this call
// follows the write-locked, non-retried "working-tree scan" category gogit.go
// documents: r.goGitMu.Lock is held for the call's whole duration, not
// RLock, and there is no fingerprint-gated reindex-retry, since a scan
// failure here is never the "stale object index" shape lookupObjectRetrying
// exists to heal.
//
// Status() internally resolves ignore patterns via
// plumbing/format/gitignore's ReadPatterns, which reads .git/info/exclude
// before any .gitignore file — so paths this repo git-excludes there (the
// weft lock dir, the push lock file) are already absent from the result;
// WorktreeChangedFiles needs no separate exclude-file filtering of its own.
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
