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
	"fmt"

	"github.com/Knatte18/loomyard/internal/gitexec"
)

// ensureBoardWorktree materializes boardPath as a second worktree of the weft repo,
// checked out on warpBranch. When warpBranch exists locally (ordinary case), the
// worktree adopts it. Otherwise (genuinely empty weft remote), the worktree is created
// as an orphan. Returns any git error.
func ensureBoardWorktree(weftRepoRoot, warpBranch, boardPath string) error {
	_, _, exitCode, err := gitexec.RunGit(
		[]string{"rev-parse", "--verify", "--quiet", "refs/heads/" + warpBranch},
		weftRepoRoot,
	)
	if err != nil {
		return fmt.Errorf("check for local weft branch %q: %w", warpBranch, err)
	}

	if exitCode == 0 {
		_, _, exitCode, err := gitexec.RunGit(
			[]string{"worktree", "add", boardPath, warpBranch},
			weftRepoRoot,
		)
		if err != nil {
			return fmt.Errorf("add _board worktree on existing branch %q: %w", warpBranch, err)
		}
		if exitCode != 0 {
			return fmt.Errorf("git worktree add %q %q failed (git exit %d)", boardPath, warpBranch, exitCode)
		}
		return nil
	}

	_, _, exitCode, err = gitexec.RunGit(
		[]string{"worktree", "add", "--orphan", "-b", warpBranch, boardPath},
		weftRepoRoot,
	)
	if err != nil {
		return fmt.Errorf("add orphan _board worktree on branch %q: %w", warpBranch, err)
	}
	if exitCode != 0 {
		return fmt.Errorf("git worktree add --orphan -b %q %q failed (git exit %d)", warpBranch, boardPath, exitCode)
	}
	return nil
}
