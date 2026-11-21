// boardweft.go materializes the Bolt checkout as a second worktree of the weft repo.
// The Bolt checkout uses the board branch it is handed; other weft worktrees use the suffixed one.
// In a two-repository hub the board branch is the warp prime's unsuffixed branch.
// It is computed from the prime's freshly-cloned checkout.
// In a one-repository hub it is the fixed board branch of the records namespace.
// This file never derives a branch name itself.

package fabricengine

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/gitrepo"
)

// ensureBoardWorktree materializes boardPath as a second worktree of the weft repo, checked out on boardBranch.
// When boardBranch exists locally, the worktree adopts it, which is the ordinary case.
// When only origin carries it, the worktree starts a local branch from origin's with that branch as its upstream.
// Otherwise the weft remote is genuinely empty, and the worktree is created as an orphan.
// It returns any git error.
func ensureBoardWorktree(weftRepoRoot, boardBranch, boardPath string) error {
	repo := gitrepo.New(weftRepoRoot)
	// A missing branch is not an error; it selects the paths below. A failed read is one.
	branchExistsLocally, err := repo.BranchExists(boardBranch)
	if err != nil {
		return fmt.Errorf("check for local weft branch %q: %w", boardBranch, err)
	}
	remoteBranch := originRemoteName + "/" + boardBranch
	_, remoteErr := repo.RefSHA("refs/remotes/" + remoteBranch)
	if remoteErr != nil && !errors.Is(remoteErr, gitrepo.ErrRefNotFound) {
		return fmt.Errorf("check for remote weft branch %q: %w", remoteBranch, remoteErr)
	}

	// Both worktree-add forms route through containedWorktreeAdd.
	// That keeps a symlink standing at boardPath from carrying the Bolt checkout outside the hub.
	// boardPath is the Bolt checkout, a direct child of the hub.
	boardContainer := filepath.Dir(boardPath)
	if branchExistsLocally {
		if err := containedWorktreeAdd(weftRepoRoot, boardContainer, boardPath, func(worktreePath string) []string {
			return []string{"worktree", "add", worktreePath, boardBranch}
		}); err != nil {
			return fmt.Errorf("add %s worktree on existing branch %q: %w", BoltDirName, boardBranch, err)
		}
		return nil
	}

	if remoteErr == nil {
		if err := containedWorktreeAdd(weftRepoRoot, boardContainer, boardPath, func(worktreePath string) []string {
			return []string{"worktree", "add", "--track", "-b", boardBranch, worktreePath, remoteBranch}
		}); err != nil {
			return fmt.Errorf("add %s worktree tracking %q: %w", BoltDirName, remoteBranch, err)
		}
		return nil
	}

	if err := containedWorktreeAdd(weftRepoRoot, boardContainer, boardPath, func(worktreePath string) []string {
		return []string{"worktree", "add", "--orphan", "-b", boardBranch, worktreePath}
	}); err != nil {
		return fmt.Errorf("add orphan %s worktree on branch %q: %w", BoltDirName, boardBranch, err)
	}
	return nil
}
