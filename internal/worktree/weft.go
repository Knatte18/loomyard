// weft.go implements weft worktree spawn and teardown helpers for paired Add/Remove operations.
//
// These unexported helpers handle the weft-side lifecycle: creating weft worktrees,
// seeding junctions and git excludes, pushing to the weft remote, and tearing down
// both the weft worktree and branch. All git operations use git.RunGit with explicit
// cwd (WeftRepoRoot or WeftWorktreePath).

package worktree

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/fslink"
	"github.com/Knatte18/loomyard/internal/git"
	"github.com/Knatte18/loomyard/internal/paths"
)

// weftRepoExists reports whether a weft repo exists at the expected location.
//
// A weft repo must be a directory that passes the git rev-parse --is-inside-work-tree check.
func weftRepoExists(l *paths.Layout) bool {
	weftRepoRoot := l.WeftRepoRoot()

	// Check if directory exists
	info, err := os.Stat(weftRepoRoot)
	if err != nil || !info.IsDir() {
		return false
	}

	// Check if it's a valid git repo
	_, _, exitCode, err := git.RunGit([]string{"rev-parse", "--is-inside-work-tree"}, weftRepoRoot)
	if err != nil {
		return false
	}

	return exitCode == 0
}

// weftBranchExists reports whether a branch exists in the weft repo.
//
// It uses git rev-parse --verify to check for the branch.
func weftBranchExists(l *paths.Layout, branch string) bool {
	_, _, exitCode, err := git.RunGit(
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
// Runs git worktree add -b <branch> <path> in the weft repo root.
// Returns an error if the command fails or exits with non-zero code.
func createWeftWorktree(l *paths.Layout, slug, branch string) error {
	weftPath := l.WeftWorktreePath(slug)
	_, stderr, exitCode, err := git.RunGit(
		[]string{"worktree", "add", "-b", branch, weftPath},
		l.WeftRepoRoot(),
	)
	if err != nil {
		return fmt.Errorf("failed to run git worktree add for weft: %w", err)
	}
	if exitCode != 0 {
		return fmt.Errorf("weft worktree add failed: %s", stderr)
	}
	return nil
}

// seedLyxJunction creates or verifies the host junctions pointing to weft directories.
//
// It iterates over the junctions returned by l.HostJunctions(slug), applying the same
// create-or-verify logic per junction using each record's Link and Target.
//
// For each junction, if it already exists:
//   - Validates that it resolves to the correct target via fslink.PointsTo
//   - Checks using fslink.IsLink to determine if it's a link
//   - Returns nil (idempotent)
//
// If os.Lstat fails with not-exist:
//   - Creates the junction via fslink.CreateDirLink
//
// Otherwise:
//   - Returns an error indicating the host repo contains a real directory that predates weft
func seedLyxJunction(l *paths.Layout, slug string) error {
	junctions := l.HostJunctions(slug)

	for _, j := range junctions {
		link := j.Link
		target := j.Target

		_, err := os.Lstat(link)
		if err == nil {
			// Link exists. Resolve the target first; if target doesn't exist, report distinctly.
			targetResolved, errTarget := filepath.EvalSymlinks(target)
			if errTarget != nil {
				return fmt.Errorf("weft directory does not exist at %s; cannot validate junction target", target)
			}

			// Check if link is a link and resolves to the correct target
			isLink, errIsLink := fslink.IsLink(link)
			if errIsLink != nil {
				return fmt.Errorf("islink %s: %w", link, errIsLink)
			}
			if isLink {
				linkResolved, errResolve := fslink.PointsTo(link)
				if errResolve == nil && linkResolved == targetResolved {
					// Idempotent: junction exists and resolves correctly
					continue
				}
			}

			// Not a link or points elsewhere; this is a real directory issue
			return fmt.Errorf(
				"host repo already contains a real %s at %s; it predates weft — migrate via the hub-creator",
				filepath.Base(link),
				link,
			)
		}

		if !os.IsNotExist(err) {
			return fmt.Errorf("lstat %s: %w", link, err)
		}

		// Junction does not exist; create it
		if err := fslink.CreateDirLink(link, target); err != nil {
			return err
		}
	}

	return nil
}

// seedGitExclude adds junction names to the host worktree's .git/info/exclude file if not already present.
//
// It iterates over the junctions returned by l.HostJunctions(slug) and appends each
// junction's Name to the exclude file if not already present. Resolves the exclude
// path via git rev-parse --git-path info/exclude. If the path is relative, joins it
// with the worktree path. Preserves line-exact idempotency per name.
// Idempotent: re-running when all junction names are already present is a no-op.
func seedGitExclude(l *paths.Layout, slug string) error {
	worktreePath := l.WorktreePath(slug)

	// Get the exclude path via git rev-parse --git-path
	stdout, stderr, exitCode, err := git.RunGit(
		[]string{"rev-parse", "--git-path", "info/exclude"},
		worktreePath,
	)
	if err != nil {
		return fmt.Errorf("failed to get git-path for info/exclude: %w", err)
	}
	if exitCode != 0 {
		return fmt.Errorf("git rev-parse --git-path failed: %s", stderr)
	}

	excludePath := strings.TrimSpace(stdout)

	// If path is relative, join with worktree path
	if !filepath.IsAbs(excludePath) {
		excludePath = filepath.Join(worktreePath, excludePath)
	}

	// Create parent directories if needed
	if err := os.MkdirAll(filepath.Dir(excludePath), 0o755); err != nil {
		return fmt.Errorf("mkdir for exclude file: %w", err)
	}

	// Read the file
	content, err := os.ReadFile(excludePath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read exclude file: %w", err)
	}

	contentStr := string(content)

	// Iterate over junction names and append each if not already present.
	junctions := l.HostJunctions(slug)
	for _, j := range junctions {
		name := j.Name

		// Check if name is already present as a line-exact match
		found := false
		for _, line := range strings.Split(contentStr, "\n") {
			if strings.TrimSpace(line) == name {
				found = true
				break
			}
		}

		if found {
			// Already present, skip to next junction
			continue
		}

		// Append name with newline
		if contentStr != "" && !strings.HasSuffix(contentStr, "\n") {
			contentStr += "\n"
		}
		contentStr += name + "\n"
	}

	// Write back
	if err := os.WriteFile(excludePath, []byte(contentStr), 0o644); err != nil {
		return fmt.Errorf("write exclude file: %w", err)
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
func pushWeftBranch(l *paths.Layout, slug, branch string, opts AddOptions) error {
	if opts.SkipGit || opts.SkipPush {
		return nil
	}

	weftPath := l.WeftWorktreePath(slug)
	_, stderr, exitCode, err := git.RunGit(
		[]string{"push", "-u", "origin", branch},
		weftPath,
	)
	if err != nil {
		return fmt.Errorf("failed to run git push for weft: %w", err)
	}
	if exitCode != 0 {
		return fmt.Errorf("weft push failed: %s", stderr)
	}

	return nil
}

// removeHostJunction removes the host _lyx junction at the given link path.
//
// Uses fslink.Remove to delete the junction/symlink only (idempotent).
// Returns nil if the junction does not exist (idempotent).
// Returns an error if removal fails for reasons other than not-exist.
func removeHostJunction(l *paths.Layout, slug string) error {
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
func removeWeftWorktree(l *paths.Layout, slug, branch string, force bool) error {
	weftPath := l.WeftWorktreePath(slug)
	weftRoot := l.WeftRepoRoot()

	var firstErr error

	// Remove weft worktree
	args := []string{"worktree", "remove"}
	if force {
		args = append(args, "--force")
	}
	args = append(args, weftPath)
	_, _, exitCode, err := git.RunGit(args, weftRoot)
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
	_, _, exitCode, err = git.RunGit([]string{"branch", "-D", branch}, weftRoot)
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
	_, _, exitCode, err = git.RunGit([]string{"worktree", "prune"}, weftRoot)
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
