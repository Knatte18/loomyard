// reset.go implements ResetHard, the SHA-validated hard reset fabric's
// coordinated history-recovery flows (Fabric.Pull's rebase-reconciliation
// among them) build on: point HEAD (and the working tree) at a
// caller-supplied commit exactly, discarding any local commits or
// uncommitted changes the checkout previously had past that SHA.

package gitrepo

import "fmt"

// ResetHard resets HEAD, index, and working tree to sha via `git reset --hard`.
// sha must be a valid hex object name, or ErrInvalidSHA is returned.
func (r *Repo) ResetHard(sha string) error {
	if !validSHA(sha) {
		return ErrInvalidSHA
	}

	_, _, code, err := r.run("reset", "--hard", sha)
	if err != nil {
		return fmt.Errorf("gitrepo: reset --hard %s in %s: %w", sha, r.path, err)
	}
	if code != 0 {
		return fmt.Errorf("gitrepo: reset --hard %s in %s: git exited %d", sha, r.path, code)
	}
	return nil
}
