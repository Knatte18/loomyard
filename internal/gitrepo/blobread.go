// blobread.go adds the two read-only, go-git-only primitives diff-base recovery builds on:
// FileAtRevision reads one path's blob contents as of a given revision, and PathRevisions walks the
// commits that touched a path. Neither method calls r.run or r.runChecked — both resolve state that
// is already on disk, which is go-git's side of the package's Client Boundary Invariant.

package gitrepo

import (
	"errors"
	"fmt"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/storer"
)

// ErrPathNotAtRevision is returned by FileAtRevision when relPath is absent from rev's tree,
// distinguishing "not there yet" from a real read failure.
var ErrPathNotAtRevision = errors.New("gitrepo: path not present at revision")

// FileAtRevision returns relPath's blob contents as stored in rev's tree, unaffected by any later
// change to the working-tree copy. relPath is slash-separated and repo-relative, matching what
// ChangedFilesSince returns. Returns ErrPathNotAtRevision when relPath is absent from that
// revision's tree, or ErrInvalidSHA when rev is not a valid hex object name.
func (r *Repo) FileAtRevision(rev, relPath string) ([]byte, error) {
	if !validSHA(rev) {
		return nil, ErrInvalidSHA
	}

	repo, err := r.goGit()
	if err != nil {
		return nil, err
	}

	tree, err := lookupObjectRetrying(r, repo, func() (*object.Tree, error) {
		return treeForRev(repo, rev)
	})
	if err != nil {
		return nil, fmt.Errorf("gitrepo: resolve tree for %s: %w", rev, err)
	}

	r.goGitMu.RLock()
	defer r.goGitMu.RUnlock()

	file, err := tree.File(relPath)
	if err != nil {
		if errors.Is(err, object.ErrFileNotFound) {
			return nil, ErrPathNotAtRevision
		}
		return nil, fmt.Errorf("gitrepo: find %s at %s: %w", relPath, rev, err)
	}

	contents, err := file.Contents()
	if err != nil {
		return nil, fmt.Errorf("gitrepo: read %s at %s: %w", relPath, rev, err)
	}
	return []byte(contents), nil
}

// PathRevisions returns the SHAs of the commits that touched relPath, newest first, capped at limit
// when limit > 0 and uncapped when limit <= 0. Returns an empty slice and no error when relPath has
// no history — an unrecoverable base is a normal outcome the caller reports explicitly, not an error
// condition.
func (r *Repo) PathRevisions(relPath string, limit int) ([]string, error) {
	repo, err := r.goGit()
	if err != nil {
		return nil, err
	}

	r.goGitMu.RLock()
	defer r.goGitMu.RUnlock()

	commitIter, err := repo.Log(&git.LogOptions{
		FileName: &relPath,
		Order:    git.LogOrderCommitterTime,
	})
	if err != nil {
		if errors.Is(err, plumbing.ErrReferenceNotFound) {
			// Unborn HEAD: no commits exist yet, so the path has no history.
			return nil, nil
		}
		return nil, fmt.Errorf("gitrepo: log for %s: %w", relPath, err)
	}

	var revisions []string
	err = commitIter.ForEach(func(commit *object.Commit) error {
		if limit > 0 && len(revisions) >= limit {
			return storer.ErrStop
		}
		revisions = append(revisions, commit.Hash.String())
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("gitrepo: iterate log for %s: %w", relPath, err)
	}
	return revisions, nil
}
