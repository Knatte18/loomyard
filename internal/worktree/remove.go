// remove.go implements Remove: it tears down the portal and launchers before the
// target-exists check, so cleanup still runs when the worktree dir is already gone.

package worktree

import (
	"fmt"
	"os"
	"strings"

	"github.com/Knatte18/mhgo/internal/git"
	"github.com/Knatte18/mhgo/internal/paths"
)

// RemoveResult contains the result of successfully removing a worktree.
type RemoveResult struct {
	Slug         string `json:"slug"`
	Path         string `json:"path"`
	LinksRemoved int    `json:"links_removed"`
}

// Remove removes a git worktree and its associated directory, portal, and launchers.
//
// The Layout l provides geometry information; all git operations use l.WorktreeRoot as cwd.
// The slug is the name of the sibling worktree to remove.
// If force is false, the worktree must be clean. If force is true, uncommitted changes
// are allowed and the worktree is forcefully removed.
//
// Steps:
//  1. (EARLY) removePortal(l, slug) and removeLaunchers(l, slug) — best-effort cleanup
//     that runs BEFORE the exists check, so portal/launcher cleanup happens even when
//     the worktree dir is already gone.
//  2. Check if target exists: error if not found (unless cleanup is desired anyway)
//  3. Dirty gate (if !force): check for uncommitted changes; reject if any found
//  4. Link cleanup: call removeLinks to clean up symlinks/junctions before removal
//  5. Git remove: run `git worktree remove [--force] <target>`
//  6. Fallback: if git remove fails, use os.RemoveAll and optionally git worktree prune
//  7. Leave <container>/_launchers/ide-menu.cmd in place
//
// Returns RemoveResult on success or an error if the target doesn't exist or other failures occur.
func (w *Worktree) Remove(l *paths.Layout, slug string, force bool) (RemoveResult, error) {
	// (1) Early teardown: remove portal and launchers BEFORE exists check
	// These are best-effort (errors masked)
	removePortal(l, slug)
	removeLaunchers(l, slug)

	// (2) Locate the target and check if it exists
	target := l.WorktreePath(slug)
	if _, err := os.Stat(target); os.IsNotExist(err) {
		return RemoveResult{}, fmt.Errorf("worktree %q not found", target)
	}

	// (3) Dirty gate (only when !force)
	if !force {
		stdout, _, exitCode, err := git.RunGit([]string{"status", "--porcelain"}, target)
		if err != nil {
			return RemoveResult{}, fmt.Errorf("failed to check worktree status: %v", err)
		}
		if exitCode != 0 {
			return RemoveResult{}, fmt.Errorf("failed to check worktree status")
		}
		if strings.TrimSpace(stdout) != "" {
			return RemoveResult{}, fmt.Errorf("worktree has uncommitted changes; use --force")
		}
	}

	// (4) Link cleanup
	linksRemoved, err := removeLinks(target)
	if err != nil {
		return RemoveResult{}, err
	}

	// (5) Git remove
	args := []string{"worktree", "remove"}
	if force {
		args = append(args, "--force")
	}
	args = append(args, target)

	_, _, exitCode, err := git.RunGit(args, l.WorktreeRoot)
	if err != nil {
		return RemoveResult{}, fmt.Errorf("failed to run git worktree remove: %v", err)
	}

	if exitCode == 0 {
		// Success via git
		return RemoveResult{
			Slug:         slug,
			Path:         target,
			LinksRemoved: linksRemoved,
		}, nil
	}

	// (6) Fallback: use os.RemoveAll
	if err := os.RemoveAll(target); err != nil {
		return RemoveResult{}, fmt.Errorf("fallback removal failed: %w", err)
	}

	// Best-effort prune
	git.RunGit([]string{"worktree", "prune"}, l.WorktreeRoot)

	return RemoveResult{
		Slug:         slug,
		Path:         target,
		LinksRemoved: linksRemoved,
	}, nil
}
