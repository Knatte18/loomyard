// weftwiring.go implements weft worktree spawn and teardown helpers for paired Add/Remove operations.
//
// These unexported helpers handle the weft-side lifecycle: creating weft worktrees,
// seeding junctions and git excludes, pushing to the weft remote, and tearing down
// both the weft worktree and branch. All git operations use gitexec.RunGit with explicit
// cwd (WeftRepoRoot or WeftWorktreePath).
//
// Weft branch model: each weft branch forks from its parent's weft branch (non-orphan,
// shared merge-base), preserving history for future _raddle squash-merge-back.
// _lyx is isolated by pathspec (never merges back), not by orphan topology. A detached
// or unborn host HEAD aborts the spawn before any creation, ensuring no partial state.

package warpengine

import (
	"fmt"
	"os"

	"github.com/Knatte18/loomyard/internal/fslink"
	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/hubgeometry"
)

// weftRepoExists reports whether a weft repo exists at the expected location.
//
// A weft repo must be a directory that passes the git rev-parse --is-inside-work-tree check.
func weftRepoExists(l *hubgeometry.Layout) bool {
	weftRepoRoot := l.WeftRepoRoot()

	// Check if directory exists
	info, err := os.Stat(weftRepoRoot)
	if err != nil || !info.IsDir() {
		return false
	}

	// Check if it's a valid git repo
	_, _, exitCode, err := gitexec.RunGit([]string{"rev-parse", "--is-inside-work-tree"}, weftRepoRoot)
	if err != nil {
		return false
	}

	return exitCode == 0
}

// weftBranchExists reports whether a branch exists in the weft repo.
//
// It uses git rev-parse --verify to check for the branch.
func weftBranchExists(l *hubgeometry.Layout, branch string) bool {
	_, _, exitCode, err := gitexec.RunGit(
		[]string{"rev-parse", "--verify", "refs/heads/" + branch},
		l.WeftRepoRoot(),
	)
	if err != nil {
		return false
	}
	return exitCode == 0
}

// createWeftWorktree creates a new weft worktree at the given path with the given branch.
//
// The new weft branch forks from startPoint (the parent's weft branch), preserving the
// shared merge-base needed for future squash-merge-back operations. Runs
// git worktree add -b <branch> <path> <startPoint> in the weft repo root.
// Returns an error if the command fails or exits with non-zero code.
func createWeftWorktree(l *hubgeometry.Layout, slug, branch, startPoint string) error {
	weftPath := l.WeftWorktreePath(slug)
	_, _, exitCode, err := gitexec.RunGit(
		[]string{"worktree", "add", "-b", branch, weftPath, startPoint},
		l.WeftRepoRoot(),
	)
	if err != nil {
		return fmt.Errorf("failed to run git worktree add for weft: %w", err)
	}
	if exitCode != 0 {
		return fmt.Errorf("create weft worktree %q for branch %q failed (git exit %d)", weftPath, branch, exitCode)
	}
	return nil
}

// pushWeftBranch pushes the weft branch to the origin remote.
//
// When opts.SkipGit or opts.SkipPush is true the push is skipped and nil is
// returned, preserving the same semantics that the environment variables
// WEFT_SKIP_GIT/WEFT_SKIP_PUSH provided before the env→option migration.
//
// Otherwise, runs git push -u origin <branch> from the weft worktree.
// Returns an error if the command fails or exits with non-zero code.
func pushWeftBranch(l *hubgeometry.Layout, slug, branch string, opts AddOptions) error {
	if opts.SkipGit || opts.SkipPush {
		return nil
	}

	weftPath := l.WeftWorktreePath(slug)
	_, _, exitCode, err := gitexec.RunGit(
		[]string{"push", "-u", "origin", branch},
		weftPath,
	)
	if err != nil {
		return fmt.Errorf("failed to run git push for weft: %w", err)
	}
	if exitCode != 0 {
		return fmt.Errorf("push weft branch %q failed (git exit %d)", branch, exitCode)
	}

	return nil
}

// removeHostJunction removes the host _lyx junction at the given link path.
//
// Uses fslink.Remove to delete the junction/symlink only (idempotent).
// Returns nil if the junction does not exist (idempotent).
// Returns an error if removal fails for reasons other than not-exist.
func removeHostJunction(l *hubgeometry.Layout, slug string) error {
	link := l.HostLyxLink(slug)
	if err := fslink.Remove(link); err != nil {
		return fmt.Errorf("remove host junction %s: %w", link, err)
	}
	return nil
}

// removeWeftWorktree tears down the weft worktree, branch, and related state.
//
// Steps (best-effort, errors collected):
//  1. git worktree remove [--force] <weft-worktree-path>
//  2. git branch -D <branch>
//  3. git worktree prune
//
// All commands run with cwd = WeftRepoRoot.
// Returns the first error encountered, or nil if all steps succeed.
func removeWeftWorktree(l *hubgeometry.Layout, slug, branch string, force bool) error {
	weftPath := l.WeftWorktreePath(slug)
	weftRoot := l.WeftRepoRoot()

	var firstErr error

	// Remove weft worktree
	args := []string{"worktree", "remove"}
	if force {
		args = append(args, "--force")
	}
	args = append(args, weftPath)
	_, _, exitCode, err := gitexec.RunGit(args, weftRoot)
	if err != nil || exitCode != 0 {
		if firstErr == nil {
			if err != nil {
				firstErr = err
			} else {
				firstErr = fmt.Errorf("git worktree remove failed with exit code %d", exitCode)
			}
		}
	}

	// Delete branch
	_, _, exitCode, err = gitexec.RunGit([]string{"branch", "-D", branch}, weftRoot)
	if err != nil || exitCode != 0 {
		if firstErr == nil {
			if err != nil {
				firstErr = err
			} else {
				firstErr = fmt.Errorf("git branch -D failed with exit code %d", exitCode)
			}
		}
	}

	// Prune worktrees
	_, _, exitCode, err = gitexec.RunGit([]string{"worktree", "prune"}, weftRoot)
	if err != nil || exitCode != 0 {
		if firstErr == nil {
			if err != nil {
				firstErr = err
			} else {
				firstErr = fmt.Errorf("git worktree prune failed with exit code %d", exitCode)
			}
		}
	}

	return firstErr
}
