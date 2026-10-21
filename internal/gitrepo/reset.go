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

// IsLocalChangeRefusal reports whether err is the refusal git raises when `reset --keep` would overwrite an uncommitted change:
// a checked *gitexec.GitError whose stderr says an entry is not up to date or a local change would be overwritten.
// Any other failure of the reset, such as a held index lock, is not a refusal.
func IsLocalChangeRefusal(err error) bool {
	var gitErr *gitexec.GitError
	if !errors.As(err, &gitErr) {
		return false
	}
	stderr := strings.ToLower(gitErr.Stderr)
	return strings.Contains(stderr, "not uptodate") || strings.Contains(stderr, "would be overwritten")
}

// CherryPick applies sha's change onto HEAD via `git cherry-pick`.
// A commit whose change the branch already carries is skipped with HEAD unmoved, as `pull --rebase` drops it.
// A commit that is empty from the start is refused with an error naming it, and the cherry-pick is left in progress for CherryPickAbort.
// sha must be a valid hex object name, or ErrInvalidSHA is returned.
// A conflict or any other failed pick returns the wrapped *gitexec.GitError and leaves the cherry-pick in progress, when one started, for CherryPickAbort.
func (r *Repo) CherryPick(sha string) error {
	if !validSHA(sha) {
		return ErrInvalidSHA
	}

	_, pickErr := r.runChecked("cherry-pick", sha)
	if pickErr == nil {
		return nil
	}
	wrapped := fmt.Errorf("gitrepo: cherry-pick %s in %s: %w", sha, r.path, pickErr)

	// A pick stops empty when a cherry-pick is in progress, nothing is unmerged and the index equals HEAD.
	var gitErr *gitexec.GitError
	_, err := r.runChecked("rev-parse", "--verify", "--quiet", "CHERRY_PICK_HEAD")
	switch {
	case err == nil:
	case errors.As(err, &gitErr) && gitErr.ExitCode == 1:
		return wrapped
	default:
		return fmt.Errorf("gitrepo: rev-parse --verify --quiet CHERRY_PICK_HEAD in %s: %w", r.path, err)
	}

	unmerged, err := r.runChecked("ls-files", "--unmerged")
	if err != nil {
		return fmt.Errorf("gitrepo: ls-files --unmerged in %s: %w", r.path, err)
	}
	if strings.TrimSpace(unmerged) != "" {
		return wrapped
	}

	_, err = r.runChecked("diff", "--cached", "--quiet", "HEAD")
	switch {
	case err == nil:
	case errors.As(err, &gitErr) && gitErr.ExitCode == 1:
		return wrapped
	default:
		return fmt.Errorf("gitrepo: diff --cached --quiet HEAD in %s: %w", r.path, err)
	}

	_, err = r.runChecked("diff-tree", "--quiet", "--root", sha)
	switch {
	case err == nil:
		return fmt.Errorf("gitrepo: cherry-pick %s in %s: the commit is empty from the start", sha, r.path)
	case errors.As(err, &gitErr) && gitErr.ExitCode == 1:
		// The commit has a change HEAD already carries.
		if _, err := r.runChecked("cherry-pick", "--skip"); err != nil {
			return fmt.Errorf("gitrepo: cherry-pick --skip %s in %s: %w", sha, r.path, err)
		}
		return nil
	default:
		return fmt.Errorf("gitrepo: diff-tree --quiet --root %s in %s: %w", sha, r.path, err)
	}
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
