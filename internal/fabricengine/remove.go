// remove.go implements Remove: it tears down the portal and launchers before the target-exists check, so cleanup still runs when the worktree dir is already gone.
// The weft branch it removes is WeftBranchName(hostBranch).

package fabricengine

import (
	"fmt"
	"os"
	"strings"

	"github.com/Knatte18/loomyard/internal/fslink"
	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

// RemoveResult contains the result of successfully removing a worktree pair.
type RemoveResult struct {
	Slug         string `json:"slug"`
	Path         string `json:"path"`
	LinksRemoved int    `json:"links_removed"`
}

// Remove removes a paired host and weft git worktree with all associated artifacts.
// If force is false, both worktrees must be clean;
// if force is true, uncommitted changes are forcefully removed.
// Portal and launcher cleanup run before the exists check, ensuring cleanup even if the worktree directory is already gone.
func (t *Topology) Remove(l *lyxcwd.Location, slug string, force bool) (RemoveResult, error) {
	hostBranch := t.cfg.BranchPrefix + slug
	weftBranch := WeftBranchName(hostBranch)

	_ = removePortal(l, slug)
	_ = removeLaunchers(l, slug)

	target := WorktreePath(l, slug)
	if _, err := os.Stat(target); os.IsNotExist(err) {
		return RemoveResult{}, fmt.Errorf("worktree %q not found", target)
	}

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

	if !force {
		weftTarget := WeftWorktreePath(l, slug)
		stdout, _, exitCode, err := gitexec.RunGit([]string{"status", "--porcelain"}, weftTarget)
		if err != nil {
		} else if exitCode == 0 && strings.TrimSpace(stdout) != "" {
			return RemoveResult{}, fmt.Errorf("weft worktree has uncommitted changes; run \"lyx fabric sync\" or use --force")
		}
	}

	names, namesErr := RepoWiredNames(l)
	if namesErr != nil {
		names = nil
	}
	_ = removeHostJunction(l, slug, names)

	linksRemoved, err := fslink.RemoveLinksIn(target)
	if err != nil {
		return RemoveResult{}, err
	}

	args := []string{"worktree", "remove"}
	if force {
		args = append(args, "--force")
	}
	args = append(args, target)

	_, _, exitCode, err := gitexec.RunGit(args, l.WorktreePath())
	if err != nil {
		return RemoveResult{}, fmt.Errorf("failed to run git worktree remove: %v", err)
	}

	if exitCode != 0 {
		if err := os.RemoveAll(target); err != nil {
			return RemoveResult{}, fmt.Errorf("fallback removal failed: %w", err)
		}

		_, _, _, _ = gitexec.RunGit([]string{"worktree", "prune"}, l.WorktreePath())
	}

	_ = removeWeftWorktree(l, slug, weftBranch, force, true)

	return RemoveResult{
		Slug:         slug,
		Path:         target,
		LinksRemoved: linksRemoved,
	}, nil
}
