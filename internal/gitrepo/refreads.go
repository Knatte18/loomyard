// refreads.go implements the ref reads fabric asks of a checkout through go-git:
// BranchExists, RefSHA, HeadRef, RefTree, Upstream and BranchNames.

package gitrepo

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
)

// ErrRefNotFound is returned, wrapped with the ref, when a ref name resolves to nothing.
var ErrRefNotFound = errors.New("gitrepo: ref not found")

// BranchExists reports whether `refs/heads/<branch>` exists, loose or packed.
func (r *Repo) BranchExists(branch string) (bool, error) {
	return readGoGit(r, func(repo *git.Repository) (bool, error) {
		_, err := repo.Reference(plumbing.NewBranchReferenceName(branch), false)
		if errors.Is(err, plumbing.ErrReferenceNotFound) {
			return false, nil
		}
		if err != nil {
			return false, fmt.Errorf("gitrepo: read branch %s in %s: %w", branch, r.path, err)
		}
		return true, nil
	})
}

// RefSHA resolves a full ref name such as `refs/remotes/origin/main` to the hash it points at.
// An absent ref is ErrRefNotFound wrapped with the ref.
func (r *Repo) RefSHA(ref string) (string, error) {
	return readGoGit(r, func(repo *git.Repository) (string, error) {
		resolved, err := repo.Reference(plumbing.ReferenceName(ref), true)
		if errors.Is(err, plumbing.ErrReferenceNotFound) {
			return "", fmt.Errorf("%w: %s", ErrRefNotFound, ref)
		}
		if err != nil {
			return "", fmt.Errorf("gitrepo: read ref %s in %s: %w", ref, r.path, err)
		}
		return resolved.Hash().String(), nil
	})
}

// HeadRef reports the branch that HEAD names and whether HEAD is detached.
// A symbolic HEAD answers its short branch name with detached false, whether or not the branch has a commit yet.
// A HEAD holding a hash answers ("", true, nil).
func (r *Repo) HeadRef() (branch string, detached bool, err error) {
	type headRead struct {
		branch   string
		detached bool
	}

	read, err := readGoGit(r, func(repo *git.Repository) (headRead, error) {
		head, err := repo.Reference(plumbing.HEAD, false)
		if err != nil {
			return headRead{}, fmt.Errorf("gitrepo: read HEAD reference in %s: %w", r.path, err)
		}
		if head.Type() != plumbing.SymbolicReference {
			return headRead{detached: true}, nil
		}
		return headRead{branch: head.Target().Short()}, nil
	})
	return read.branch, read.detached, err
}

// RefTree resolves a ref name or hash to a commit and returns the hash of that commit's tree.
// A name that resolves to nothing is ErrRefNotFound wrapped with the ref.
func (r *Repo) RefTree(ref string) (string, error) {
	return readGoGit(r, func(repo *git.Repository) (string, error) {
		commit, err := commitByHash(repo, ref)
		if errors.Is(err, plumbing.ErrObjectNotFound) {
			return "", fmt.Errorf("%w: %s", ErrRefNotFound, ref)
		}
		if err != nil {
			return "", fmt.Errorf("gitrepo: resolve tree of %s in %s: %w", ref, r.path, err)
		}
		return commit.TreeHash.String(), nil
	})
}

// Upstream returns the hash of the remote-tracking ref that branch is configured to track.
// The ref is named by `branch.<name>.remote` and `branch.<name>.merge`.
// ok is false when either key is unset or the remote-tracking ref does not exist.
func (r *Repo) Upstream(branch string) (sha string, ok bool, err error) {
	type upstreamRead struct {
		sha string
		ok  bool
	}

	read, err := readGoGit(r, func(repo *git.Repository) (upstreamRead, error) {
		cfg, err := repo.Config()
		if err != nil {
			return upstreamRead{}, fmt.Errorf("gitrepo: read config in %s: %w", r.path, err)
		}
		tracked, found := cfg.Branches[branch]
		if !found || tracked.Remote == "" || tracked.Merge == "" {
			return upstreamRead{}, nil
		}

		trackingRef := plumbing.NewRemoteReferenceName(tracked.Remote, tracked.Merge.Short())
		resolved, err := repo.Reference(trackingRef, true)
		if errors.Is(err, plumbing.ErrReferenceNotFound) {
			return upstreamRead{}, nil
		}
		if err != nil {
			return upstreamRead{}, fmt.Errorf("gitrepo: read ref %s in %s: %w", trackingRef, r.path, err)
		}
		return upstreamRead{sha: resolved.Hash().String(), ok: true}, nil
	})
	return read.sha, read.ok, err
}

// BranchNames lists, sorted, the short names of the branches under refs/heads when remote is empty.
// Otherwise it lists the remote-tracking branches of remote, without the remote in each name.
// The symbolic HEAD ref a clone leaves under the remote is skipped.
func (r *Repo) BranchNames(remote string) ([]string, error) {
	prefix := "refs/heads/"
	if remote != "" {
		prefix = "refs/remotes/" + remote + "/"
	}
	return readGoGit(r, func(repo *git.Repository) ([]string, error) {
		refs, err := repo.References()
		if err != nil {
			return nil, fmt.Errorf("gitrepo: list refs in %s: %w", r.path, err)
		}
		var names []string
		err = refs.ForEach(func(ref *plumbing.Reference) error {
			name := string(ref.Name())
			if !strings.HasPrefix(name, prefix) || ref.Type() == plumbing.SymbolicReference {
				return nil
			}
			names = append(names, strings.TrimPrefix(name, prefix))
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("gitrepo: iterate refs in %s: %w", r.path, err)
		}
		sort.Strings(names)
		return names, nil
	})
}
