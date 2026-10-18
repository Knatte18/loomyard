// branchmark.go implements the per-branch local-only mark: a bool in the branch's own git config section that makes every push primitive refuse to publish the branch.

package gitrepo

import (
	"errors"
	"fmt"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/go-git/go-git/v5"
)

// LocalBranchKey is the last segment of the git config key `branch.<branch>.lyxlocal` that marks a branch local-only.
// The key lives in the branch's own config section, so deleting the branch drops the mark with it.
const LocalBranchKey = "lyxlocal"

// ErrLocalBranch is returned by a push of a branch carrying the local-only mark, before any git push runs.
var ErrLocalBranch = errors.New("gitrepo: branch is local-only, never pushed")

// configKeyNotSetExit is the exit code `git config --unset` answers when the key is absent.
const configKeyNotSetExit = 5

// MarkBranchLocal marks branch local-only, so every push primitive refuses it with ErrLocalBranch.
func (r *Repo) MarkBranchLocal(branch string) error {
	if _, err := r.runChecked("config", "branch."+branch+"."+LocalBranchKey, "true"); err != nil {
		return fmt.Errorf("gitrepo: mark branch %s local: %w", branch, err)
	}
	return nil
}

// ClearBranchLocalMark removes branch's local-only mark and reports whether a mark was there.
func (r *Repo) ClearBranchLocalMark(branch string) (cleared bool, err error) {
	_, err = r.runChecked("config", "--unset", "branch."+branch+"."+LocalBranchKey)
	if err == nil {
		return true, nil
	}
	var gitErr *gitexec.GitError
	if errors.As(err, &gitErr) && gitErr.ExitCode == configKeyNotSetExit {
		return false, nil
	}
	return false, fmt.Errorf("gitrepo: clear local mark of branch %s: %w", branch, err)
}

// BranchMarkedLocal reports whether branch carries the local-only mark.
// An absent key, an absent branch section and a value other than true all read false;
// only a repository config that cannot be read is an error.
func (r *Repo) BranchMarkedLocal(branch string) (bool, error) {
	return readGoGit(r, func(repo *git.Repository) (bool, error) {
		cfg, err := repo.Config()
		if err != nil {
			return false, fmt.Errorf("gitrepo: read repository config: %w", err)
		}
		return cfg.Raw.Section("branch").Subsection(branch).Option(LocalBranchKey) == "true", nil
	})
}

// refuseMarkedCurrentBranch returns ErrLocalBranch when the checked-out branch carries the local-only mark.
// A detached HEAD has no branch to mark and passes; a mark that cannot be read is returned as the error, so nothing is pushed on doubt.
func (r *Repo) refuseMarkedCurrentBranch() error {
	branch, err := r.CurrentBranch()
	if err != nil {
		return nil
	}
	marked, err := r.BranchMarkedLocal(branch)
	if err != nil {
		return err
	}
	if marked {
		return ErrLocalBranch
	}
	return nil
}
