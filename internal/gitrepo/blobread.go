// blobread.go adds the read-only, go-git-only primitives diff-base recovery and history lookups build on: FileAtRevision reads one path's blob contents as of a given revision, FilesInDirAtRevision lists the files directly in one directory as of a revision, PathRevisions walks the commits that touched a path, CommitsWithSubject finds the commits carrying one subject line, and HeadContains asks whether HEAD's history holds a commit.
// No method calls r.run or r.runChecked — all resolve state that is already on disk, which is go-git's side of the package's Client Boundary Invariant.

package gitrepo

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

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

// HeadContains reports whether sha names HEAD's commit or one of its ancestors.
// An unborn HEAD, and a sha whose commit is absent from the local object store, report false with no error;
// an invalid sha is ErrInvalidSHA.
// It reads the object store through go-git and never runs git.
func (r *Repo) HeadContains(sha string) (bool, error) {
	if !validSHA(sha) {
		return false, ErrInvalidSHA
	}

	repo, err := r.goGit()
	if err != nil {
		return false, err
	}

	target, err := lookupObjectRetrying(r, repo, func() (*object.Commit, error) {
		return commitByHash(repo, sha)
	})
	if err != nil {
		if errors.Is(err, plumbing.ErrObjectNotFound) {
			return false, nil
		}
		return false, fmt.Errorf("gitrepo: resolve commit %s: %w", sha, err)
	}

	r.goGitMu.RLock()
	defer r.goGitMu.RUnlock()

	head, err := repo.Head()
	if err != nil {
		if errors.Is(err, plumbing.ErrReferenceNotFound) {
			return false, nil
		}
		return false, fmt.Errorf("gitrepo: read HEAD: %w", err)
	}
	commitIter, err := repo.Log(&git.LogOptions{From: head.Hash()})
	if err != nil {
		return false, fmt.Errorf("gitrepo: log from HEAD: %w", err)
	}
	found := false
	err = commitIter.ForEach(func(commit *object.Commit) error {
		if commit.Hash == target.Hash {
			found = true
			return storer.ErrStop
		}
		return nil
	})
	if err != nil {
		return false, fmt.Errorf("gitrepo: walk HEAD's history: %w", err)
	}
	return found, nil
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

// FilesInDirAtRevision returns the names of the regular files directly in dir in rev's tree, sorted.
// dir is slash-separated and repo-relative;
// "" or "." names the root.
// An absent directory is an empty result, not an error;
// an invalid rev is ErrInvalidSHA.
func (r *Repo) FilesInDirAtRevision(rev, dir string) ([]string, error) {
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

	if dir != "" && dir != "." {
		tree, err = tree.Tree(dir)
		if err != nil {
			if errors.Is(err, object.ErrDirectoryNotFound) {
				return nil, nil
			}
			return nil, fmt.Errorf("gitrepo: find directory %s at %s: %w", dir, rev, err)
		}
	}

	var names []string
	for _, entry := range tree.Entries {
		if entry.Mode.IsRegular() {
			names = append(names, entry.Name)
		}
	}
	sort.Strings(names)
	return names, nil
}

// SubjectCommit is a commit found by its subject line.
type SubjectCommit struct {
	// SHA is the commit's full hex object name.
	SHA string

	// Committed is the commit's committer time.
	Committed time.Time
}

// CommitsWithSubject returns every commit reachable from any branch or tag whose message's first line equals subject, once each however many refs reach it, newest committer time first.
// No match is an empty slice and no error.
func (r *Repo) CommitsWithSubject(subject string) ([]SubjectCommit, error) {
	repo, err := r.goGit()
	if err != nil {
		return nil, err
	}

	r.goGitMu.RLock()
	defer r.goGitMu.RUnlock()

	tips, err := branchAndTagCommits(repo)
	if err != nil {
		return nil, err
	}

	seen := map[plumbing.Hash]bool{}
	var found []SubjectCommit
	for _, tip := range tips {
		commitIter, err := repo.Log(&git.LogOptions{From: tip})
		if err != nil {
			return nil, fmt.Errorf("gitrepo: log from %s: %w", tip, err)
		}
		err = commitIter.ForEach(func(commit *object.Commit) error {
			if seen[commit.Hash] {
				return nil
			}
			seen[commit.Hash] = true
			first, _, _ := strings.Cut(commit.Message, "\n")
			if strings.TrimRight(first, "\r") == subject {
				found = append(found, SubjectCommit{SHA: commit.Hash.String(), Committed: commit.Committer.When})
			}
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("gitrepo: iterate log from %s: %w", tip, err)
		}
	}

	sort.Slice(found, func(i, j int) bool {
		if !found[i].Committed.Equal(found[j].Committed) {
			return found[i].Committed.After(found[j].Committed)
		}
		return found[i].SHA < found[j].SHA
	})
	return found, nil
}

// branchAndTagCommits returns the commit every local branch and every tag points at, an annotated tag peeled to its commit.
func branchAndTagCommits(repo *git.Repository) ([]plumbing.Hash, error) {
	var tips []plumbing.Hash
	collect := func(refs storer.ReferenceIter, peel bool) error {
		return refs.ForEach(func(ref *plumbing.Reference) error {
			hash := ref.Hash()
			if peel {
				if tag, err := repo.TagObject(hash); err == nil {
					commit, err := tag.Commit()
					if err != nil {
						// A tag naming a non-commit object reaches no commit history.
						return nil
					}
					hash = commit.Hash
				}
			}
			tips = append(tips, hash)
			return nil
		})
	}

	branches, err := repo.Branches()
	if err != nil {
		return nil, fmt.Errorf("gitrepo: list branches: %w", err)
	}
	if err := collect(branches, false); err != nil {
		return nil, err
	}
	tags, err := repo.Tags()
	if err != nil {
		return nil, fmt.Errorf("gitrepo: list tags: %w", err)
	}
	if err := collect(tags, true); err != nil {
		return nil, err
	}
	return tips, nil
}
