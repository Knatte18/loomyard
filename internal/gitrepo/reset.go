// reset.go implements ResetHard, the SHA-validated hard reset that
// RevertWithWeft's history-recovery flow builds on: point HEAD (and the
// working tree) at a caller-supplied commit exactly, discarding any local
// commits or uncommitted changes the checkout previously had past that SHA.

package gitrepo

import "fmt"

// ResetHard resets this Repo's HEAD, index, and working tree to sha via
// `git reset --hard`, discarding any local commits or uncommitted changes
// past that point. sha must be a plain hex object name — validSHA rejects it
// before any git spawn (exactly like ChangedFilesSince), returning
// ErrInvalidSHA (checkable via errors.Is) so an option-shaped string (e.g.
// "--hard") can never be parsed as a reset flag instead of a target commit.
// On a spawn failure the underlying error is wrapped and returned; on any
// other non-zero exit (a well-formed sha not present in this repo's history)
// the returned error names the repo path and git's exit code without
// including raw stderr, matching Pull's no-stderr-leak error style.
// ResetHard is the primitive RevertWithWeft's history recovery builds on.
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
