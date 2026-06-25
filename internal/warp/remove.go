// remove.go implements Remove: it tears down the portal and launchers before the
// target-exists check, so cleanup still runs when the worktree dir is already gone.

package warp

import (
	"fmt"
	"os"
	"strings"

	"github.com/Knatte18/loomyard/internal/fslink"
	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/paths"
)

// RemoveResult contains the result of successfully removing a worktree.
type RemoveResult struct {
	Slug         string `json:"slug"`
	Path         string `json:"path"`
	LinksRemoved int    `json:"links_removed"`
}

// Remove removes a paired host and weft git worktree with all associated artifacts.
//
// The Layout l provides geometry information; all git operations use the appropriate cwd
// (l.WorktreeRoot for host, l.WeftRepoRoot for weft). The slug is the name of the sibling
// worktree to remove. If force is false, both host and weft worktrees must be clean.
// If force is true, uncommitted changes are allowed in both and they are forcefully removed.
//
// Steps:
//  1. (EARLY) removePortal(l, slug) and removeLaunchers(l, slug) — best-effort cleanup
//     that runs BEFORE the exists check, so portal/launcher cleanup happens even when
//     the worktree dir is already gone.
//  2. Locate the target and check if it exists: error if not found.
//  3. Host dirty gate (if !force): check host worktree for uncommitted changes; reject if any found.
//  4. Weft dirty gate (if !force): check weft worktree for uncommitted changes; reject if any found.
//  5. Explicitly remove host _lyx junction via removeHostJunction (fslink.RemoveLinksIn only scans
//     immediate children and misses nested _lyx at RelPath != "."; this catches subpath junctions).
//  6. Link cleanup: call fslink.RemoveLinksIn as root-level safety net for remaining links.
//  7. Git remove: run `git worktree remove [--force] <target>` on host.
//  8. Fallback: if git remove fails, use os.RemoveAll and optionally git worktree prune.
//  9. Remove weft worktree and branch via removeWeftWorktree.
//
// 10. Leave <container>/_launchers/ide-menu.cmd in place.
//
// Returns RemoveResult on success or an error if the target doesn't exist or other failures occur.
func (w *Worktree) Remove(l *paths.Layout, slug string, force bool) (RemoveResult, error) {
	// Compute weft branch name (mirrored)
	branch := w.cfg.BranchPrefix + slug

	// (1) Early teardown: remove portal and launchers BEFORE exists check
	// These are best-effort (errors masked)
	removePortal(l, slug)
	removeLaunchers(l, slug)

	// (2) Locate the target and check if it exists
	target := l.WorktreePath(slug)
	if _, err := os.Stat(target); os.IsNotExist(err) {
		return RemoveResult{}, fmt.Errorf("worktree %q not found", target)
	}

	// (3) Host dirty gate (only when !force)
	if !force {
		stdout, _, exitCode, err := gitexec.RunGit([]string{"status", "--porcelain"}, target)
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

	// (4) Weft dirty gate (only when !force)
	if !force {
		weftTarget := l.WeftWorktreePath(slug)
		stdout, _, exitCode, err := gitexec.RunGit([]string{"status", "--porcelain"}, weftTarget)
		if err != nil {
			// Weft worktree might not exist or be invalid; skip the check.
			// If it doesn't exist, the weft remove later will handle cleanup.
		} else if exitCode == 0 && strings.TrimSpace(stdout) != "" {
			return RemoveResult{}, fmt.Errorf("weft worktree has uncommitted changes; run \"lyx weft sync\" or use --force")
		}
	}

	// (5) Explicitly remove host _lyx junction (catches nested junctions that fslink.RemoveLinksIn misses)
	removeHostJunction(l, slug)

	// (6) Link cleanup (root-level safety net)
	linksRemoved, err := fslink.RemoveLinksIn(target)
	if err != nil {
		return RemoveResult{}, err
	}

	// (7) Git remove
	args := []string{"worktree", "remove"}
	if force {
		args = append(args, "--force")
	}
	args = append(args, target)

	_, _, exitCode, err := gitexec.RunGit(args, l.WorktreeRoot)
	if err != nil {
		return RemoveResult{}, fmt.Errorf("failed to run git worktree remove: %v", err)
	}

	if exitCode != 0 {
		// (8) Fallback: use os.RemoveAll
		if err := os.RemoveAll(target); err != nil {
			return RemoveResult{}, fmt.Errorf("fallback removal failed: %w", err)
		}

		// Best-effort prune
		gitexec.RunGit([]string{"worktree", "prune"}, l.WorktreeRoot)
	}

	// (9) Remove weft worktree and branch
	removeWeftWorktree(l, slug, branch, force)

	return RemoveResult{
		Slug:         slug,
		Path:         target,
		LinksRemoved: linksRemoved,
	}, nil
}
