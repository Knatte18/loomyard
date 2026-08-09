// remove.go implements Remove: it tears down the portal and launchers before the target-exists
// check, so cleanup still runs when the worktree dir is already gone.
// The weft branch it removes is WeftBranchName(warpBranch).
// Its link sweep is anchored and ownership-filtered — see the sweep's own comment for why reading
// the worktree root and trusting link-ness alone was wrong on both hub geometries.

package fabricengine

import (
	"fmt"
	"os"
	"strings"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

// RemoveResult contains the result of successfully removing a worktree pair.
type RemoveResult struct {
	Slug         string `json:"slug"`
	Path         string `json:"path"`
	LinksRemoved int    `json:"links_removed"`
}

// Remove removes a paired warp and weft git worktree with all associated artifacts.
// If force is false, both worktrees must be clean;
// if force is true, uncommitted changes are forcefully removed.
// Portal and launcher cleanup run before the exists check, ensuring cleanup even if the worktree
// directory is already gone.
func (t *Topology) Remove(l *lyxcwd.Location, slug string, force bool) (RemoveResult, error) {
	warpBranch := t.cfg.BranchPrefix + slug
	weftBranch := WeftBranchName(warpBranch)

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

	// Sweep the ANCHORED directory, and only the links fabric itself created there.
	// The previous sweep read the worktree ROOT and removed every symlink it found: on a
	// subpath-anchored hub that saw none of the pair's junctions (they live at
	// <worktree>/<anchorRel>) and reported LinksRemoved: 0, and at a root anchor it deleted the
	// user's own checked-in symlinks alongside fabric's.
	linksRemoved := 0
	if ownedNames, scanErr := scanOnDiskJunctionNames(l, slug); scanErr == nil {
		if removeErr := removeWarpJunction(l, slug, ownedNames); removeErr == nil {
			linksRemoved = len(ownedNames)
		}
	}
	if boardRemoved, boardErr := unwireBoardLink(l, slug); boardErr == nil && boardRemoved {
		linksRemoved++
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

	// A weft-teardown failure is tolerated only when the weft worktree is actually gone (already
	// absent, or removed with just a branch/prune step failing) — a weft worktree still on disk
	// after a "successful" Remove is a half-torn pair the operator was never told about.
	weftErr := removeWeftWorktree(l, slug, weftBranch, force, true)
	if weftErr != nil {
		weftTarget := WeftWorktreePath(l, slug)
		if _, statErr := os.Stat(weftTarget); statErr == nil {
			return RemoveResult{}, fmt.Errorf(
				"warp worktree removed, but weft teardown failed and the weft worktree remains at %s: %w",
				weftTarget, weftErr)
		}
	}

	return RemoveResult{
		Slug:         slug,
		Path:         target,
		LinksRemoved: linksRemoved,
	}, nil
}
