// boardweft.go materializes <Hub>/_board as a second worktree of the weft
// repo on the host's unsuffixed default branch (never the WeftBranchName-
// suffixed pairing every other weft worktree uses). It never derives a
// branch name itself (hostBranch always arrives pre-computed from
// suffixWeftPrimaryBranch), mirroring weftwiring.go's own stated rule for
// pre-suffixed branch names — _board's deliberately-unsuffixed branch is
// exactly the case that rule exists to keep out of that file.

package fabricengine

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/gitexec"
)

// ensureBoardWorktree materializes boardPath as a second worktree of the weft
// repo rooted at weftRepoRoot, checked out on hostBranch (the host's
// unsuffixed default branch, as read by suffixWeftPrimaryBranch before its
// rename). boardPath must always be supplied by the caller via
// hubgeometry.BoardDir(hub) (per the Hub Geometry Invariant) — this file
// never constructs the "_board" literal itself.
//
// When hostBranch already exists as a local branch in the weft repo (the
// ordinary case: cloneRepo's plain git clone already created and checked out
// a local hostBranch ref, and suffixWeftPrimaryBranch's subsequent
// checkout -b <hostBranch>-weft leaves that ref intact), the worktree adopts
// that existing branch. Otherwise (a genuinely empty weft remote, where clone
// left no local hostBranch ref at all), the worktree is created as an orphan
// on a freshly-named hostBranch. Returns an error wrapping any non-zero exit
// or spawn failure from the underlying git invocations.
func ensureBoardWorktree(weftRepoRoot, hostBranch, boardPath string) error {
	// Mirrors weftwiring.go's weftBranchExists check shape, but inlined rather
	// than calling weftBranchExists directly — that function takes a
	// *hubgeometry.Layout, which CloneHub does not have at this point in
	// bootstrap, since no worktree/config exists yet to resolve one from.
	_, _, exitCode, err := gitexec.RunGit(
		[]string{"rev-parse", "--verify", "--quiet", "refs/heads/" + hostBranch},
		weftRepoRoot,
	)
	if err != nil {
		return fmt.Errorf("check for local weft branch %q: %w", hostBranch, err)
	}

	if exitCode == 0 {
		// Adopt path: hostBranch already exists locally, so _board simply
		// becomes a second worktree checked out on that same branch.
		_, _, exitCode, err := gitexec.RunGit(
			[]string{"worktree", "add", boardPath, hostBranch},
			weftRepoRoot,
		)
		if err != nil {
			return fmt.Errorf("add _board worktree on existing branch %q: %w", hostBranch, err)
		}
		if exitCode != 0 {
			return fmt.Errorf("git worktree add %q %q failed (git exit %d)", boardPath, hostBranch, exitCode)
		}
		return nil
	}

	// Orphan path: the weft remote was genuinely empty, so no local
	// hostBranch ref exists to adopt. --orphan alone names no branch (git's
	// own synopsis is "... [--orphan] [(-b | -B) <new-branch>] <path>
	// [<commit-ish>]", with only <path> [<commit-ish>] as bare positionals);
	// omitting -b <hostBranch> would either bind hostBranch to the <path> slot
	// and error, or silently default the orphan branch's name to
	// basename(<path>) (i.e. literally "_board") instead of the host's actual
	// default branch. Confirmed against `git worktree add --help` on git
	// 2.53.0 during plan review.
	_, _, exitCode, err = gitexec.RunGit(
		[]string{"worktree", "add", "--orphan", "-b", hostBranch, boardPath},
		weftRepoRoot,
	)
	if err != nil {
		return fmt.Errorf("add orphan _board worktree on branch %q: %w", hostBranch, err)
	}
	if exitCode != 0 {
		return fmt.Errorf("git worktree add --orphan -b %q %q failed (git exit %d)", hostBranch, boardPath, exitCode)
	}
	return nil
}
