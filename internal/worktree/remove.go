package worktree

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Knatte18/mhgo/internal/git"
)

// RemoveResult contains the result of successfully removing a worktree.
type RemoveResult struct {
	Slug         string `json:"slug"`
	Path         string `json:"path"`
	LinksRemoved int    `json:"links_removed"`
}

// Remove removes a git worktree and its associated directory.
//
// The sourceDir is any worktree in the repository (usually the main checkout).
// The slug is the name of the sibling worktree to remove.
// If force is false, the worktree must be clean. If force is true, uncommitted changes
// are allowed and the worktree is forcefully removed.
//
// Steps:
//  1. Locate the target: container := filepath.Dir(sourceDir), target := filepath.Join(container, slug)
//  2. Dirty gate (if !force): check for uncommitted changes; reject if any found
//  3. Link cleanup: call removeLinks to clean up symlinks/junctions before removal
//  4. Git remove: run `git worktree remove [--force] <target>`
//  5. Fallback: if git remove fails, use os.RemoveAll and optionally git worktree prune
//
// Returns RemoveResult on success or an error if the target doesn't exist or other failures occur.
func (w *Worktree) Remove(sourceDir, slug string, force bool) (RemoveResult, error) {
	// (1) Locate the target
	container := filepath.Dir(sourceDir)
	target := filepath.Join(container, slug)

	// Check if target exists
	if _, err := os.Stat(target); os.IsNotExist(err) {
		return RemoveResult{}, fmt.Errorf("worktree %q not found", target)
	}

	// (2) Dirty gate (only when !force)
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

	// (3) Link cleanup
	linksRemoved, err := removeLinks(target)
	if err != nil {
		return RemoveResult{}, err
	}

	// (4) Git remove
	args := []string{"worktree", "remove"}
	if force {
		args = append(args, "--force")
	}
	args = append(args, target)

	_, stderr, exitCode, err := git.RunGit(args, sourceDir)
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

	// (5) Fallback: use os.RemoveAll
	if err := os.RemoveAll(target); err != nil {
		return RemoveResult{}, fmt.Errorf("fallback removal failed: %w", err)
	}

	// Best-effort prune
	git.RunGit([]string{"worktree", "prune"}, sourceDir)

	return RemoveResult{
		Slug:         slug,
		Path:         target,
		LinksRemoved: linksRemoved,
	}, nil
}
