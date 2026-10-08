// reset.go implements ResetHard, the SHA-validated hard reset fabric's coordinated history-recovery
// flows (Fabric.Pull's rebase-reconciliation among them) build on: point HEAD (and the working
// tree) at a caller-supplied commit exactly, discarding any local commits or uncommitted changes
// the checkout previously had past that SHA.
// It also implements ResetKeep, the SHA-validated reset that moves HEAD and keeps uncommitted changes.

package gitrepo

import "fmt"

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
