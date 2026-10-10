// ancestry.go implements the reachability primitive rebase detection and the nearest-older anchor
// walk both need: IsAncestor answers "is sha an ancestor of ref", CLI-bound via `git merge-base
// --is-ancestor` because SHAExists' object-existence semantics cannot distinguish "this commit is
// still reachable from ref" from "this commit's object merely survived a rebase that walked it off
// history" — see the package's reachability, never object-existence decision.
// MergeBase finds two commits' best common ancestor through go-git, a local read.
// CommitsNotIn lists the commits one tip has over a base, CLI-bound beside it via `git rev-list`.

package gitrepo

import (
	"errors"
	"fmt"
	"strings"

	"github.com/go-git/go-git/v5"

	"github.com/Knatte18/loomyard/internal/gitexec"
)

// IsAncestor reports whether sha is an ancestor of ref via `git merge-base --is-ancestor`,
// returning its tri-state exit code: true if an ancestor, false if not (both with nil error), or an
// error on failure.
// sha and ref are validated before reaching git, returning ErrInvalidSHA.
func (r *Repo) IsAncestor(sha, ref string) (bool, error) {
	if !validSHA(sha) {
		return false, ErrInvalidSHA
	}
	if strings.HasPrefix(ref, "-") {
		return false, ErrInvalidSHA
	}

	_, err := r.runChecked("merge-base", "--is-ancestor", sha, ref)
	var gitErr *gitexec.GitError
	switch {
	case err == nil:
		return true, nil
	case errors.As(err, &gitErr) && gitErr.ExitCode == 1:
		return false, nil
	default:
		return false, fmt.Errorf("gitrepo: merge-base --is-ancestor %s %s in %s: %w", sha, ref, r.path, err)
	}
}

// MergeBase returns the SHA of the best common ancestor of the commits a and b.
// a and b are validated before the object store is read, returning ErrInvalidSHA;
// an object the store does not know, and two commits with no common ancestor, are errors naming both.
// It reads the object store through go-git and never runs git.
func (r *Repo) MergeBase(a, b string) (string, error) {
	if !validSHA(a) || !validSHA(b) {
		return "", ErrInvalidSHA
	}

	return readGoGit(r, func(repo *git.Repository) (string, error) {
		first, err := commitByHash(repo, a)
		if err != nil {
			return "", fmt.Errorf("gitrepo: resolve commit %s in %s: %w", a, r.path, err)
		}
		second, err := commitByHash(repo, b)
		if err != nil {
			return "", fmt.Errorf("gitrepo: resolve commit %s in %s: %w", b, r.path, err)
		}
		bases, err := first.MergeBase(second)
		if err != nil {
			return "", fmt.Errorf("gitrepo: merge base of %s and %s in %s: %w", a, b, r.path, err)
		}
		if len(bases) == 0 {
			return "", fmt.Errorf("gitrepo: %s and %s in %s have no common ancestor", a, b, r.path)
		}
		return bases[0].Hash.String(), nil
	})
}

// CommitsNotIn returns the SHAs of the commits reachable from tip and not from base, newest first, via `git rev-list base..tip`.
// An empty slice means tip adds nothing over base.
// tip and base are validated before reaching git, returning ErrInvalidSHA; an object git does not know is an error naming it.
func (r *Repo) CommitsNotIn(tip, base string) ([]string, error) {
	if !validSHA(tip) || !validSHA(base) {
		return nil, ErrInvalidSHA
	}

	stdout, err := r.runChecked("rev-list", base+".."+tip)
	if err != nil {
		return nil, fmt.Errorf("gitrepo: rev-list %s..%s in %s: %w", base, tip, r.path, err)
	}
	return strings.Fields(stdout), nil
}
