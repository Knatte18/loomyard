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
// Push honors SkipGit/SkipPush via fabricengine.SyncOptions, fabric's own
// options type.

package fabricengine

import (
	"errors"
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

// removeHostJunction removes every host junction for slug — every entry in
// l.HostJunctions(slug) — via fslink.Remove. It is a thin wrapper over
// removeJunctionRecords, which owns the actual best-effort loop; the split
// exists purely so the loop's continue-past-failure contract is directly
// testable against a synthetic junction slice, since l.HostJunctions always
// returns exactly one entry today and cannot itself produce the
// multi-junction scenario the contract is about (mirroring
// unseedLyxJunction/unseedJunctionRecords in junction.go).
//
// Returns nil if every junction is already absent (idempotent). See
// removeJunctionRecords for the error case.
func removeHostJunction(l *hubgeometry.Layout, slug string) error {
	return removeJunctionRecords(l.HostJunctions(slug))
}

// removeJunctionRecords removes each junction in junctions via fslink.Remove.
//
// It is best-effort and deliberately continues past a per-junction failure,
// accumulating every error via errors.Join rather than aborting on the first —
// the opposite of unseedJunctionRecords' abort-on-first-junction-error rule in
// junction.go, and deliberately so. A future reader must not "fix" one loop to
// match the other; they intentionally disagree. Remove's call site is
// `_ = removeHostJunction(l, slug)`, discarding the return value exactly as the
// adjacent removePortal and removeLaunchers calls in the same teardown do, so
// aborting on the first junction's failure would silently leave every later
// junction in place — defeating the whole point of this step.
//
// This step exists because Remove's later link-cleanup step,
// fslink.RemoveLinksIn(target), scans only the immediate children of the
// worktree root and misses a nested junction whenever RelPath != "." — the
// reason step (5) of Remove removes junctions explicitly, before that safety
// net runs. Leaving this _lyx-only would reintroduce exactly that documented
// bug for every junction after the first.
//
// Returns nil if junctions is empty or every entry is already absent
// (idempotent). Returns a joined error naming every junction whose removal
// failed; a non-nil error does NOT mean no junction was removed — the loop
// still attempted (and may have succeeded for) every other junction.
func removeJunctionRecords(junctions []hubgeometry.HostJunction) error {
	var errs []error
	for _, j := range junctions {
		if err := fslink.Remove(j.Link); err != nil {
			errs = append(errs, fmt.Errorf("remove host junction %s: %w", j.Link, err))
		}
	}
	return errors.Join(errs...)
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
