// remove.go implements Remove: it tears down the portal and launchers after the slug and
// target-exists checks, so a refused slug never loses another pair's launchers, and the teardown
// still runs when the worktree dir itself is already gone.
// The weft branch it removes is WeftBranchName(warpBranch).
// Its link sweep is anchored and ownership-filtered — see the sweep's own comment for why reading
// the worktree root and trusting link-ness alone was wrong on both hub geometries.
// It never deletes a directory git declined to remove unless that directory is a registered LINKED
// worktree of this repo — see removeWarpWorktreeDir for the data-loss this rule exists to prevent.

package fabricengine

import (
	"fmt"
	"os"
	"path/filepath"
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
// It validates slug through the same validator Add uses (slug.go), so hub geometry — `_board`,
// `_portals`, `_launchers`, `_lyx`, `.lyx`, and every weft sibling — can never be handed to a
// teardown verb as if it were a pair.
// It refuses the hub's prime worktree outright too: the prime is the warp repository itself, not a
// pair this verb can tear down, and git's own refusal to remove a main working tree is not a
// licence to delete the clone.
// Portal and launcher cleanup run after those checks but before the git removal, so they still run
// when the worktree directory is already gone.
func (t *Topology) Remove(l *lyxcwd.Location, slug string, force bool) (RemoveResult, error) {
	warpBranch := t.cfg.BranchPrefix + slug
	weftBranch := WeftBranchName(warpBranch)

	if err := validateWorktreeSlug(slug, t.cfg.Dirs()); err != nil {
		return RemoveResult{}, err
	}

	if err := refusePrimeSlug(l, slug); err != nil {
		return RemoveResult{}, err
	}

	target := WorktreePath(l, slug)
	if _, err := os.Stat(target); os.IsNotExist(err) {
		return RemoveResult{}, fmt.Errorf("worktree %q not found", target)
	}

	_ = removePortal(l, slug)
	_ = removeLaunchers(l, slug)

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
		if err := refuseDirtyWeftWorktree(WeftWorktreePath(l, slug)); err != nil {
			return RemoveResult{}, err
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

	if err := removeWarpWorktreeDir(l, target, force); err != nil {
		return RemoveResult{}, err
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

// refuseDirtyWeftWorktree returns an error when the weft worktree at weftTarget carries
// uncommitted changes, or when its status could not be read at all.
//
// An ABSENT weft worktree is not a refusal: there is no uncommitted work to lose, and tearing down
// a half-present pair is exactly what Remove is for.
// An unreadable one IS a refusal, and that is the whole point of this helper: the probe used to
// swallow its own spawn error in an empty if-branch, so a git that failed to run silently reported
// the weft side clean and the no-force gate simply disappeared.
func refuseDirtyWeftWorktree(weftTarget string) error {
	if _, statErr := os.Stat(weftTarget); os.IsNotExist(statErr) {
		return nil
	}

	stdout, stderr, exitCode, err := gitexec.RunGit([]string{"status", "--porcelain"}, weftTarget)
	if err != nil {
		return fmt.Errorf("check weft worktree status at %s: %w", weftTarget, err)
	}
	if exitCode != 0 {
		return fmt.Errorf("check weft worktree status at %s (git exit %d): %s",
			weftTarget, exitCode, strings.TrimSpace(stderr))
	}
	if strings.TrimSpace(stdout) != "" {
		return fmt.Errorf("weft worktree has uncommitted changes; run \"lyx fabric sync\" or use --force")
	}
	return nil
}

// refusePrimeSlug returns an error when slug names the hub's prime (main) warp worktree.
// The prime is the warp repository itself rather than a pair Remove can tear down, and git refuses
// to remove a main working tree — so without this guard the removal reaches the directory-removal
// fallback and deletes the whole clone, gitdir included.
// A prime-name resolution failure is not fatal here: it means the hub geometry is already broken,
// and removeWarpWorktreeDir's own registered-worktree rule still refuses to delete anything git
// declined to remove.
func refusePrimeSlug(l *lyxcwd.Location, slug string) error {
	primeName, err := PrimeName(l)
	if err != nil {
		return nil
	}
	if slug != primeName {
		return nil
	}
	return fmt.Errorf(
		"refusing to remove %q: it is this hub's prime worktree — the warp repository itself, not a pair; remove the whole hub directory instead if that is what you meant",
		slug)
}

// removeWarpWorktreeDir removes the warp worktree at target via git, falling back to a direct
// directory removal ONLY when target is a registered LINKED worktree of this repo.
//
// The narrow fallback is the whole point of this helper.
// `git worktree remove` refuses a main working tree, a path that is not a worktree of this repo at
// all, and a worktree carrying state it will not discard;
// treating every one of those refusals as licence to delete the directory turned an ordinary typo
// (`lyx fabric remove <prime>`, `lyx fabric remove _board`) into the loss of a whole git clone.
// A registered linked worktree is fabric's own pair member and nothing else, so deleting it after a
// git refusal is recoverable bookkeeping rather than data loss.
func removeWarpWorktreeDir(l *lyxcwd.Location, target string, force bool) error {
	args := []string{"worktree", "remove"}
	if force {
		args = append(args, "--force")
	}
	args = append(args, target)

	_, stderr, exitCode, err := gitexec.RunGit(args, l.WorktreePath())
	if err != nil {
		return fmt.Errorf("failed to run git worktree remove: %v", err)
	}
	if exitCode == 0 {
		return nil
	}

	if !isRegisteredLinkedWorktree(l, target) {
		return fmt.Errorf(
			"git refused to remove worktree %s (git exit %d): %s; it is not a linked worktree of this repo, so fabric will not delete the directory itself",
			target, exitCode, strings.TrimSpace(stderr))
	}

	if removeErr := os.RemoveAll(target); removeErr != nil {
		return fmt.Errorf("fallback removal failed: %w", removeErr)
	}
	_, _, _, _ = gitexec.RunGit([]string{"worktree", "prune"}, l.WorktreePath())
	return nil
}

// isRegisteredLinkedWorktree reports whether target is registered in this repo's worktree list as a
// worktree OTHER than the main one.
// A failure to enumerate answers false, the conservative direction: an unenumerable repo is exactly
// where a blind directory removal is least defensible.
func isRegisteredLinkedWorktree(l *lyxcwd.Location, target string) bool {
	entries, err := List(l.WorktreePath())
	if err != nil {
		return false
	}
	cleanTarget := filepath.Clean(target)
	for _, entry := range entries {
		if entry.Main {
			continue
		}
		if filepath.Clean(filepath.FromSlash(entry.Path)) == cleanTarget {
			return true
		}
	}
	return false
}
