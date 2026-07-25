// weftwiring.go implements weft worktree spawn and teardown helpers for paired
// topology operations (add/remove, a later batch).
//
// These unexported helpers handle the weft-side lifecycle: creating weft
// worktrees, pushing to the weft remote, and tearing down both the weft
// worktree and branch. All git operations use gitexec.RunGit with explicit cwd
// (WeftRepoRoot or WeftWorktreePath). Every branch argument here is ALWAYS a
// concrete, already-suffixed weft branch name produced by WeftBranchName — this
// file never derives a branch name itself, so the "-weft" literal never appears
// in this file's Go source (see branchname.go for the single derivation point).
//
// Weft branch model: each weft branch forks from its parent's weft branch
// (non-orphan, shared merge-base), preserving history for future _raddle
// squash-merge-back. _lyx is isolated by pathspec (never merges back), not by
// orphan topology. A detached or unborn host HEAD aborts the spawn before any
// creation, ensuring no partial state.
//
// Adapted from warpengine's weftwiring.go — byte-equivalent behavior, package
// fabricengine. Push honors SkipGit/SkipPush via fabricengine.SyncOptions
// (fabric's own options type, matching warp's AddOptions field shape exactly)
// rather than warp's own AddOptions type.

package fabricengine

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

// weftBranchExists reports whether branch (an already-suffixed weft branch
// name, i.e. the result of WeftBranchName) exists in the weft repo.
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

// createWeftWorktree creates a new weft worktree at the given path on branch
// (an already-suffixed weft branch name, i.e. the result of WeftBranchName).
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

// pushWeftBranch pushes branch (an already-suffixed weft branch name, i.e. the
// result of WeftBranchName) to the origin remote.
//
// When opts.SkipGit or opts.SkipPush is true the push is skipped and nil is
// returned, preserving the same semantics warp's AddOptions gives its
// pushWeftBranch.
//
// Otherwise, runs git push -u origin <branch> from the weft worktree.
// Returns an error if the command fails or exits with non-zero code.
func pushWeftBranch(l *hubgeometry.Layout, slug, branch string, opts SyncOptions) error {
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

// removeWeftWorktree tears down the weft worktree, optionally its branch (an
// already-suffixed weft branch name, i.e. the result of WeftBranchName), and
// related state.
//
// Steps (best-effort, errors collected):
//  1. git worktree remove [--force] <weft-worktree-path>
//  2. git branch -D <branch> (only when deleteBranch is true)
//  3. git worktree prune
//
// deleteBranch exists for Add's rollback of an ADOPTED weft branch: when Add
// merely adopted a pre-existing branch (rather than creating one), rolling
// back must remove only the worktree Add created and leave the branch — and
// any unpushed history it carries — untouched. Deleting it would destroy work
// that predates the failed Add, the exact work Cleanup's raddle-fold-back
// gate exists to protect.
//
// All commands run with cwd = WeftRepoRoot.
// Returns the first error encountered, or nil if all steps succeed.
func removeWeftWorktree(l *hubgeometry.Layout, slug, branch string, force, deleteBranch bool) error {
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

	// Delete branch — skipped when the caller does not own the branch (an
	// adopted, pre-existing weft branch survives its worktree's rollback).
	if deleteBranch {
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
