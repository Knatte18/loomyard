// blobread.go implements the read-only queries that resolve repository state through go-git, without running git.
// They read file contents and directory listings at a revision, and walk commit history.
// They also check whether HEAD's history holds a commit and resolve the current branch's upstream.
// They also read one commit's message and changed paths.

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

// ErrPathNotAtRevision reports that a path is absent from a revision's tree.
// It separates that case from a real read failure.
var ErrPathNotAtRevision = errors.New("gitrepo: path not present at revision")

// FileAtRevision returns relPath's blob contents as stored in rev's tree.
// Later changes to the working-tree copy do not affect the result.
// relPath is slash-separated and relative to the repository root.
// It returns ErrPathNotAtRevision when relPath is absent from that revision's tree.
// It returns ErrInvalidSHA when rev is not a valid hex object name.
func (r *Repo) FileAtRevision(rev, relPath string) ([]byte, error) {
	if !validSHA(rev) {
		return nil, ErrInvalidSHA
	}

	return readGoGit(r, func(repo *git.Repository) ([]byte, error) {
		tree, err := treeForRev(repo, rev)
		if err != nil {
			return nil, fmt.Errorf("gitrepo: resolve tree for %s: %w", rev, err)
		}

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
	})
}

// headContainsCommitterSlack is how far before the target's committer time a history walk stops.
const headContainsCommitterSlack = 24 * time.Hour

// HeadContains reports whether sha names HEAD's commit or one of its ancestors.
// An unborn HEAD, and a sha whose commit is absent from the local object store, report false with no error;
// an invalid sha is ErrInvalidSHA.
// It reads the object store through go-git and never runs git.
// The walk visits commits newest committer time first.
// It stops once a commit's committer time is more than 24 hours older than the target's,
// so a target absent from HEAD's history costs a bounded read.
// Clock skew can leave a descendant of the target more than 24 hours older than the target's committer time.
// The walk then stops before it reaches the target, so HeadContains answers false for a commit that is in HEAD.
func (r *Repo) HeadContains(sha string) (bool, error) {
	if !validSHA(sha) {
		return false, ErrInvalidSHA
	}

	target, err := readGoGit(r, func(repo *git.Repository) (*object.Commit, error) {
		return commitByHash(repo, sha)
	})
	if err != nil {
		if errors.Is(err, plumbing.ErrObjectNotFound) {
			return false, nil
		}
		return false, fmt.Errorf("gitrepo: resolve commit %s: %w", sha, err)
	}

	return readGoGit(r, func(repo *git.Repository) (bool, error) {
		head, err := repo.Head()
		if err != nil {
			if errors.Is(err, plumbing.ErrReferenceNotFound) {
				return false, nil
			}
			return false, fmt.Errorf("gitrepo: read HEAD: %w", err)
		}
		commitIter, err := repo.Log(&git.LogOptions{From: head.Hash(), Order: git.LogOrderCommitterTime})
		if err != nil {
			return false, fmt.Errorf("gitrepo: log from HEAD: %w", err)
		}
		oldestWalked := target.Committer.When.Add(-headContainsCommitterSlack)
		found := false
		err = commitIter.ForEach(func(commit *object.Commit) error {
			if commit.Hash == target.Hash {
				found = true
				return storer.ErrStop
			}
			if commit.Committer.When.Before(oldestWalked) {
				return storer.ErrStop
			}
			return nil
		})
		if err != nil {
			return false, fmt.Errorf("gitrepo: walk HEAD's history: %w", err)
		}
		return found, nil
	})
}

// PathRevisions returns the SHAs of the commits that touched relPath, newest first.
// A limit above zero caps the result at that many SHAs; a limit of zero or less leaves it uncapped.
// A path with no history returns an empty slice and no error, a normal outcome rather than a failure.
func (r *Repo) PathRevisions(relPath string, limit int) ([]string, error) {
	return readGoGit(r, func(repo *git.Repository) ([]string, error) {
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
	})
}

// FilesInDirAtRevision returns the names of the regular files directly in dir in rev's tree, sorted.
// dir is slash-separated and repo-relative;
// "" or "." names the root.
// An absent directory gives an empty result, not an error.
// An invalid rev returns ErrInvalidSHA.
func (r *Repo) FilesInDirAtRevision(rev, dir string) ([]string, error) {
	if !validSHA(rev) {
		return nil, ErrInvalidSHA
	}

	return readGoGit(r, func(repo *git.Repository) ([]string, error) {
		tree, err := dirTreeAtRevision(repo, rev, dir)
		if err != nil || tree == nil {
			return nil, err
		}

		var names []string
		for _, entry := range tree.Entries {
			if entry.Mode.IsRegular() {
				names = append(names, entry.Name)
			}
		}
		sort.Strings(names)
		return names, nil
	})
}

// SubjectCommit identifies a commit by its full SHA and committer time.
type SubjectCommit struct {
	// SHA is the commit's full hex object name.
	SHA string

	// Committed is the commit's committer time.
	Committed time.Time
}

// CommitsWithSubject returns every commit whose message's first line equals subject.
// It searches the history reachable from each local branch and tag.
// A commit reached from several refs is returned once, newest committer time first.
// No match is an empty slice and no error.
func (r *Repo) CommitsWithSubject(subject string) ([]SubjectCommit, error) {
	return readGoGit(r, func(repo *git.Repository) ([]SubjectCommit, error) {
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
	})
}

// branchAndTagCommits returns the commit each local branch and each tag points at.
// An annotated tag is peeled to the commit it names.
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

// ErrNoUpstream reports that the current branch has no configured upstream, or that HEAD is detached.
var ErrNoUpstream = errors.New("gitrepo: no upstream configured")

// UpstreamSHA returns the SHA of the remote-tracking ref that the current branch's upstream names.
// The upstream comes from the branch.<name>.remote and branch.<name>.merge settings.
// A detached HEAD or a branch with no upstream returns ErrNoUpstream bare.
// It reads the ref as last fetched and never contacts the remote.
func (r *Repo) UpstreamSHA() (string, error) {
	return readGoGit(r, func(repo *git.Repository) (string, error) {
		head, err := repo.Head()
		if err != nil {
			if errors.Is(err, plumbing.ErrReferenceNotFound) {
				return "", ErrNoUpstream
			}
			return "", fmt.Errorf("gitrepo: read HEAD: %w", err)
		}
		if !head.Name().IsBranch() {
			return "", ErrNoUpstream
		}

		cfg, err := repo.Config()
		if err != nil {
			return "", fmt.Errorf("gitrepo: read config: %w", err)
		}
		branch, ok := cfg.Branches[head.Name().Short()]
		if !ok || branch.Remote == "" || !branch.Merge.IsBranch() {
			return "", ErrNoUpstream
		}

		tracking, err := repo.Reference(plumbing.NewRemoteReferenceName(branch.Remote, branch.Merge.Short()), true)
		if err != nil {
			return "", fmt.Errorf("gitrepo: resolve upstream %s/%s: %w", branch.Remote, branch.Merge.Short(), err)
		}
		return tracking.Hash().String(), nil
	})
}

// CommitDetail is one commit's full message and the repo-relative, slash-separated paths its tree changes.
type CommitDetail struct {
	Message string
	Paths   []string
}

// CommitDetail returns sha's full message and the sorted paths its tree changes against its first parent.
// For a root commit it returns every path in the tree.
// An invalid sha is ErrInvalidSHA.
// It reads the object store through go-git and never runs git.
func (r *Repo) CommitDetail(sha string) (CommitDetail, error) {
	if !validSHA(sha) {
		return CommitDetail{}, ErrInvalidSHA
	}

	return readGoGit(r, func(repo *git.Repository) (CommitDetail, error) {
		commit, err := commitByHash(repo, sha)
		if err != nil {
			return CommitDetail{}, fmt.Errorf("gitrepo: resolve commit %s: %w", sha, err)
		}
		paths, err := changedPaths(commit)
		if err != nil {
			return CommitDetail{}, err
		}
		return CommitDetail{Message: commit.Message, Paths: paths}, nil
	})
}

// changedPaths returns the sorted paths that commit's tree changes against its first parent.
// A deleted path counts as changed.
// For a root commit it returns every path in its tree.
func changedPaths(commit *object.Commit) ([]string, error) {
	sha := commit.Hash.String()
	tree, err := commit.Tree()
	if err != nil {
		return nil, fmt.Errorf("gitrepo: read tree of %s: %w", sha, err)
	}

	var paths []string
	if commit.NumParents() == 0 {
		err = tree.Files().ForEach(func(file *object.File) error {
			paths = append(paths, file.Name)
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("gitrepo: list files of %s: %w", sha, err)
		}
	} else {
		parent, err := commit.Parent(0)
		if err != nil {
			return nil, fmt.Errorf("gitrepo: read first parent of %s: %w", sha, err)
		}
		parentTree, err := parent.Tree()
		if err != nil {
			return nil, fmt.Errorf("gitrepo: read tree of first parent of %s: %w", sha, err)
		}
		changes, err := parentTree.Diff(tree)
		if err != nil {
			return nil, fmt.Errorf("gitrepo: diff %s against its first parent: %w", sha, err)
		}
		for _, change := range changes {
			name := change.To.Name
			if name == "" {
				name = change.From.Name
			}
			paths = append(paths, name)
		}
	}

	sort.Strings(paths)
	return paths, nil
}

// dirTreeAtRevision returns the tree of dir in rev's tree; "" or "." names the root.
// An absent directory is a nil tree and no error.
func dirTreeAtRevision(repo *git.Repository, rev, dir string) (*object.Tree, error) {
	tree, err := treeForRev(repo, rev)
	if err != nil {
		return nil, fmt.Errorf("gitrepo: resolve tree for %s: %w", rev, err)
	}
	if dir == "" || dir == "." {
		return tree, nil
	}
	tree, err = tree.Tree(dir)
	if err != nil {
		if errors.Is(err, object.ErrDirectoryNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("gitrepo: find directory %s at %s: %w", dir, rev, err)
	}
	return tree, nil
}
