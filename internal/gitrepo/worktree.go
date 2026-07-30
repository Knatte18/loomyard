// worktree.go adds WorktreeChangedFiles, the read-only working-tree scan
// Fabric.Status (a later fabricengine batch) uses to report uncommitted
// changes — as opposed to gitrepo.go's ChangedFilesSince, which compares two
// committed trees and never looks at the working tree at all.

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
// # Why WorktreeChangedFiles reads .git/info/exclude itself
//
// Status()'s own ignore-pattern resolution (plumbing/format/gitignore's
// ReadPatterns, called with the WORKING TREE filesystem) reads .gitignore
// files fine, but silently fails to read .git/info/exclude: go-git's
// worktree filesystem is chrooted at the worktree root and refuses to open
// any path with a ".git" component ("invalid path component"), and
// ReadPatterns swallows that open error rather than surfacing it — so
// Status() ends up applying NO exclude-file patterns at all, despite the
// source tracing as if it did. Confirmed empirically against the vendored
// go-git v5.19.1 (not merely traced from source): a file matched only by an
// entry in .git/info/exclude, never by any .gitignore, still comes back
// from Worktree.Status() as untracked. readGitDirExcludePatterns below reads
// the same info/exclude file through the git-DIR filesystem instead (never
// chrooted away from ".git", since that filesystem's root already IS the
// git dir), and this function assigns the result to wt.Excludes before
// calling Status() — excludeIgnoredChanges appends w.Excludes to whatever
// ReadPatterns found, so this restores exactly the exclude-file behavior
// the doc comment on gitrepo's callers (fabricengine's weft lock dir, push
// lock file, and cross-module artifact excludes) depends on, without
// touching go-git's own broken ReadPatterns path. .gitignore files
// elsewhere in the tree are unaffected and still resolve through Status()'s
// normal path.
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

// readGitDirExcludePatterns reads repo's info/exclude file — the same file
// `git rev-parse --git-path info/exclude` names — through the git-dir
// filesystem (repo.Storer's *filesystem.Storage, opened with
// EnableDotGitCommonDir so a linked worktree resolves the shared common
// dir's copy, matching plain git's per-repository, not per-worktree,
// exclude semantics), returning its non-comment, non-blank lines as parsed
// gitignore.Pattern values. A missing file (no exclude entries have ever
// been written) is not an error — it returns an empty, nil slice, the same
// "normal, valid state" posture packFingerprint takes on a missing
// objects/pack directory. See WorktreeChangedFiles' doc comment for why this
// exists instead of relying on Worktree.Status()'s own ignore-pattern
// resolution.
func readGitDirExcludePatterns(repo *git.Repository) ([]gitignore.Pattern, error) {
	storer, ok := repo.Storer.(*filesystem.Storage)
	if !ok {
		// Not a filesystem-backed storer (never true for a handle goGit
		// opened) — nothing to read.
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
