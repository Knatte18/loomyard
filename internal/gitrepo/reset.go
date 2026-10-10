// reset.go implements ResetHard, the SHA-validated hard reset fabric's coordinated history-recovery
// flows (Fabric.Pull's rebase-reconciliation among them) build on: point HEAD (and the working
// tree) at a caller-supplied commit exactly, discarding any local commits or uncommitted changes
// the checkout previously had past that SHA.
// It also implements ResetKeep, the SHA-validated reset that moves HEAD and keeps uncommitted changes, and CherryPick with its CherryPickAbort, the tree-mutating replay of one commit onto HEAD.

package gitrepo

import (
	"errors"
	"fmt"
	"strings"

	"github.com/Knatte18/loomyard/internal/gitexec"
)

// ResetHard resets HEAD, index, and working tree to sha via `git reset --hard`.
// sha must be a valid hex object name,
// or ErrInvalidSHA is returned.
func (r *Repo) ResetHard(sha string) error {
	if !validSHA(sha) {
		return ErrInvalidSHA
	}

	_, err := r.runChecked("reset", "--hard", sha)
	if err != nil {
		return fmt.Errorf("gitrepo: reset --hard %s in %s: %w", sha, r.path, err)
	}
	return nil
}

// ResetKeep moves HEAD and the index to sha via `git reset --keep`, carrying every uncommitted change across.
// It refuses, with nothing changed, when an uncommitted change sits in a path that differs between HEAD and sha;
// that refusal is the checked *gitexec.GitError, wrapped, so a caller can surface git's message.
// sha must be a valid hex object name,
// or ErrInvalidSHA is returned.
func (r *Repo) ResetKeep(sha string) error {
	if !validSHA(sha) {
		return ErrInvalidSHA
	}

	_, err := r.runChecked("reset", "--keep", sha)
	if err != nil {
		return fmt.Errorf("gitrepo: reset --keep %s in %s: %w", sha, r.path, err)
	}
	return nil
}

// CherryPick applies sha's change onto HEAD via `git cherry-pick --empty=drop`, so a commit whose change the branch already carries is dropped with HEAD unmoved, as `pull --rebase` drops it.
// sha must be a valid hex object name, or ErrInvalidSHA is returned.
// A conflict returns the wrapped *gitexec.GitError and leaves the cherry-pick in progress for CherryPickAbort.
func (r *Repo) CherryPick(sha string) error {
	if !validSHA(sha) {
		return ErrInvalidSHA
	}

	_, err := r.runChecked("cherry-pick", "--empty=drop", sha)
	if err != nil {
		return fmt.Errorf("gitrepo: cherry-pick %s in %s: %w", sha, r.path, err)
	}
	return nil
}

// CherryPickAbort abandons a cherry-pick in progress via `git cherry-pick --abort`, restoring HEAD and the working tree.
// With no cherry-pick in progress it succeeds.
func (r *Repo) CherryPickAbort() error {
	_, err := r.runChecked("cherry-pick", "--abort")
	if err == nil {
		return nil
	}

	var gitErr *gitexec.GitError
	if errors.As(err, &gitErr) && strings.Contains(strings.ToLower(gitErr.Stderr), "no cherry-pick or revert in progress") {
		return nil
	}
	return fmt.Errorf("gitrepo: cherry-pick --abort in %s: %w", r.path, err)
}
