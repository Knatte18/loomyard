// boardweft.go materializes <Hub>/_board as a second worktree of the weft repo on the weft
// primary's unsuffixed default branch — the same name the warp repo uses in the common case,
// and never the WeftBranchName-suffixed pairing every other weft worktree uses.
// It never derives a branch name itself (warpBranch always arrives pre-computed from
// suffixWeftPrimaryBranch, which read it from the weft primary's freshly-cloned checkout before
// renaming that primary onto its -weft pairing), mirroring weftwiring.go's own stated rule for
// pre-suffixed branch names — _board's deliberately-unsuffixed branch is exactly the case that rule
// exists to keep out of that file.

package fabricengine

import (
	"errors"
	"fmt"

	"github.com/Knatte18/loomyard/internal/gitexec"
)

// ensureBoardWorktree materializes boardPath as a second worktree of the weft repo,
// checked out on warpBranch. When warpBranch exists locally (ordinary case), the
// worktree adopts it. Otherwise (genuinely empty weft remote), the worktree is created
// as an orphan. Returns any git error.
func ensureBoardWorktree(weftRepoRoot, warpBranch, boardPath string) error {
	// Mixed probe: the exit path answers "the branch is not there yet", the orphan-create path
	// this function exists to support, so it is recovered via errors.As rather than merged into a
	// single message.
	_, err := gitexec.Run(
		[]string{"rev-parse", "--verify", "--quiet", "refs/heads/" + warpBranch},
		weftRepoRoot,
	)
	branchExistsLocally := err == nil
	if err != nil {
		var gitErr *gitexec.GitError
		if !errors.As(err, &gitErr) {
			return fmt.Errorf("check for local weft branch %q: %w", warpBranch, err)
		}
	}

	if branchExistsLocally {
		if _, err := gitexec.Run(
			[]string{"worktree", "add", boardPath, warpBranch},
			weftRepoRoot,
		); err != nil {
			return fmt.Errorf("add _board worktree on existing branch %q: %w", warpBranch, err)
		}
		return nil
	}

	if _, err := gitexec.Run(
		[]string{"worktree", "add", "--orphan", "-b", warpBranch, boardPath},
		weftRepoRoot,
	); err != nil {
		return fmt.Errorf("add orphan _board worktree on branch %q: %w", warpBranch, err)
	}
	return nil
}
