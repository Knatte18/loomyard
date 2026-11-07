// boardweft.go materializes the Bolt checkout as a second worktree of the weft repo.
// The Bolt checkout uses the unsuffixed warp branch; other weft worktrees use the suffixed one.
// The warp branch arrives already computed from the warp prime's freshly-cloned checkout.
// This file never derives a branch name itself.

package fabricengine

import (
	"fmt"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/gitrepo"
)

// ensureBoardWorktree materializes boardPath as a second worktree of the weft repo, checked out on warpBranch.
// When warpBranch exists locally, the worktree adopts it, which is the ordinary case.
// Otherwise the weft remote is genuinely empty, and the worktree is created as an orphan.
// It returns any git error.
func ensureBoardWorktree(weftRepoRoot, warpBranch, boardPath string) error {
	// A missing branch is not an error; it selects the orphan path below. A failed read is one.
	branchExistsLocally, err := gitrepo.New(weftRepoRoot).BranchExists(warpBranch)
	if err != nil {
		return fmt.Errorf("check for local weft branch %q: %w", warpBranch, err)
	}

	// Both worktree-add forms route through containedWorktreeAdd.
	// That keeps a symlink standing at boardPath from carrying the Bolt checkout outside the hub.
	// boardPath is the Bolt checkout, a direct child of the hub.
	boardContainer := filepath.Dir(boardPath)
	if branchExistsLocally {
		if err := containedWorktreeAdd(weftRepoRoot, boardContainer, boardPath, func(worktreePath string) []string {
			return []string{"worktree", "add", worktreePath, warpBranch}
		}); err != nil {
			return fmt.Errorf("add %s worktree on existing branch %q: %w", BoltDirName, warpBranch, err)
		}
		return nil
	}

	if err := containedWorktreeAdd(weftRepoRoot, boardContainer, boardPath, func(worktreePath string) []string {
		return []string{"worktree", "add", "--orphan", "-b", warpBranch, worktreePath}
	}); err != nil {
		return fmt.Errorf("add orphan %s worktree on branch %q: %w", BoltDirName, warpBranch, err)
	}
	return nil
}
