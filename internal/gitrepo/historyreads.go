// historyreads.go adds the go-git-only history reads a report over finished runs builds on: TagRefs lists tags by name prefix, Log walks commits from an arbitrary tip, and SubdirsAtRevision lists the directories directly in one directory as of a revision.
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
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/storer"
)

// TagRef is a tag resolved to the commit it names.
type TagRef struct {
	// Name is the tag's short name.
	Name string

	// Commit is the full hex object name of the commit the tag names, an annotated tag peeled.
	Commit string

	// Committed is that commit's committer time.
	Committed time.Time
}

// TagRefs returns every tag whose short name starts with prefix, sorted by the commit's committer time and then by name.
// An annotated tag is peeled to its commit, and a tag that names no commit is skipped.
func (r *Repo) TagRefs(prefix string) ([]TagRef, error) {
	return readGoGit(r, func(repo *git.Repository) ([]TagRef, error) {
		tags, err := repo.Tags()
		if err != nil {
			return nil, fmt.Errorf("gitrepo: list tags: %w", err)
		}

		var found []TagRef
		err = tags.ForEach(func(ref *plumbing.Reference) error {
			name := ref.Name().Short()
			if !strings.HasPrefix(name, prefix) {
				return nil
			}
			hash := ref.Hash()
			if tag, err := repo.TagObject(hash); err == nil {
				commit, err := tag.Commit()
				if err != nil {
					return nil
				}
				hash = commit.Hash
			}
			commit, err := repo.CommitObject(hash)
			if err != nil {
				return nil
			}
			found = append(found, TagRef{Name: name, Commit: commit.Hash.String(), Committed: commit.Committer.When})
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("gitrepo: iterate tags: %w", err)
		}

		sort.Slice(found, func(i, j int) bool {
			if !found[i].Committed.Equal(found[j].Committed) {
				return found[i].Committed.Before(found[j].Committed)
			}
			return found[i].Name < found[j].Name
		})
		return found, nil
	})
}

// LogQuery shapes a Log walk.
type LogQuery struct {
	// FirstParent follows only each commit's first parent; without it the walk covers every parent in committer-time order.
	FirstParent bool

	// Since, when non-zero, ends a first-parent walk at the first commit committed before it and drops such commits from a full walk.
	Since time.Time

	// PathPrefix, when non-empty, keeps only the commits whose tree changes a slash-separated path equal to or under it against their first parent, every path for a root commit.
	// LogCommit.Paths then holds those changed paths.
	PathPrefix string

	// Limit, when positive, caps the number of commits returned.
	Limit int
}

// LogCommit is one commit a Log walk returned.
type LogCommit struct {
	SHA       string
	Parents   []string
	Tree      string
	Committed time.Time
	Message   string

	// Paths holds the sorted changed paths under LogQuery.PathPrefix, a deleted path included, and is nil when the query names no prefix.
	Paths []string
}

// Log walks the commits reachable from tip, newest first, as q shapes the walk.
// tip must be a valid hex object name, else ErrInvalidSHA.
func (r *Repo) Log(tip string, q LogQuery) ([]LogCommit, error) {
	if !validSHA(tip) {
		return nil, ErrInvalidSHA
	}

	return readGoGit(r, func(repo *git.Repository) ([]LogCommit, error) {
		start, err := commitByHash(repo, tip)
		if err != nil {
			return nil, fmt.Errorf("gitrepo: resolve commit %s: %w", tip, err)
		}

		var commits []LogCommit
		visit := func(commit *object.Commit) error {
			entry, keep, err := logEntry(commit, q)
			if err != nil {
				return err
			}
			if !keep {
				return nil
			}
			commits = append(commits, entry)
			if q.Limit > 0 && len(commits) >= q.Limit {
				return storer.ErrStop
			}
			return nil
		}

		if q.FirstParent {
			for commit := start; ; {
				if !q.Since.IsZero() && commit.Committer.When.Before(q.Since) {
					break
				}
				if err := visit(commit); err != nil {
					if errors.Is(err, storer.ErrStop) {
						break
					}
					return nil, err
				}
				if commit.NumParents() == 0 {
					break
				}
				if commit, err = commit.Parent(0); err != nil {
					return nil, fmt.Errorf("gitrepo: read first parent: %w", err)
				}
			}
			return commits, nil
		}

		commitIter, err := repo.Log(&git.LogOptions{From: start.Hash, Order: git.LogOrderCommitterTime})
		if err != nil {
			return nil, fmt.Errorf("gitrepo: log from %s: %w", tip, err)
		}
		err = commitIter.ForEach(func(commit *object.Commit) error {
			if !q.Since.IsZero() && commit.Committer.When.Before(q.Since) {
				return nil
			}
			return visit(commit)
		})
		if err != nil {
			return nil, fmt.Errorf("gitrepo: walk history from %s: %w", tip, err)
		}
		return commits, nil
	})
}

// logEntry builds the LogCommit for commit and reports whether q keeps it.
func logEntry(commit *object.Commit, q LogQuery) (LogCommit, bool, error) {
	entry := LogCommit{
		SHA:       commit.Hash.String(),
		Tree:      commit.TreeHash.String(),
		Committed: commit.Committer.When,
		Message:   commit.Message,
	}
	for _, parent := range commit.ParentHashes {
		entry.Parents = append(entry.Parents, parent.String())
	}
	if q.PathPrefix == "" {
		return entry, true, nil
	}

	changed, err := changedPaths(commit)
	if err != nil {
		return LogCommit{}, false, err
	}
	for _, path := range changed {
		if path == q.PathPrefix || strings.HasPrefix(path, q.PathPrefix+"/") {
			entry.Paths = append(entry.Paths, path)
		}
	}
	return entry, len(entry.Paths) > 0, nil
}

// SubdirsAtRevision returns the names of the directories directly in dir in rev's tree, sorted.
// dir is slash-separated and repo-relative; "" or "." names the root.
// An absent directory is an empty result, not an error; an invalid rev is ErrInvalidSHA.
func (r *Repo) SubdirsAtRevision(rev, dir string) ([]string, error) {
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
			if entry.Mode == filemode.Dir {
				names = append(names, entry.Name)
			}
		}
		sort.Strings(names)
		return names, nil
	})
}
