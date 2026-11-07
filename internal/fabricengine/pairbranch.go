// pairbranch.go reads the warp branch a pair stands on from the pair itself:
// its worktrees, then its local branches, then origin.
// This lets the branch prefix change while pairs are live.

package fabricengine

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/lymcwd"
)

// pairBranchNames returns the warp branch names a pair of slug may stand on.
// The names are the current prefix's name, then the bare slug, which a pair made before the prefix was set carries.
// An empty prefix gives the one name.
func pairBranchNames(cfg Config, slug string) []string {
	if cfg.BranchPrefix == "" {
		return []string{slug}
	}
	return []string{cfg.BranchPrefix + slug, slug}
}

// slugOfBranch returns the slug a warp branch names, which is the branch with the current prefix trimmed.
// A branch without the prefix, such as one a pair made before the prefix was set carries, is returned unchanged.
func slugOfBranch(cfg Config, branch string) string {
	return strings.TrimPrefix(branch, cfg.BranchPrefix)
}

// PairTaskBranch returns the task branch of slug's pair.
// It reads the pair itself, not the current branch prefix alone.
// It is the task worktree's branch when that is read.
// Otherwise it is the warp branch paired with the records worktree's branch when that is read.
// Otherwise it is the first of the pair's names that a local task or records branch carries;
// when none does, it is the current prefix's name.
// A task worktree's branch is read when it is one of the pair's names or has a records branch,
// so a pair made under any earlier prefix stays recognised.
// A records worktree's branch is read when its warp branch is one of the pair's names or exists locally.
// A worktree on any other branch is not read,
// so a pair checked out onto an unrelated branch never hands that branch to a teardown.
// It reads locally and makes no network call; a worktree list or branch lookup that fails is returned.
func (t *Topology) PairTaskBranch(l *lymcwd.Location, slug string) (string, error) {
	branch, _, err := t.pairTaskBranchLocal(l, slug)
	return branch, err
}

// PairTaskBranchOrOrigin returns the task branch of slug's pair, read locally first.
// When nothing local names the pair, it returns the first of the pair's names whose task branch origin holds.
// Failing that, it returns the first whose records branch the weft origin holds.
// It returns the current prefix's name when origin holds neither.
// It asks origin only when nothing local names the pair, so a pair the local read names makes no network call.
// A repository with no origin remote is not asked, and a failed lookup on a configured origin is returned.
func (t *Topology) PairTaskBranchOrOrigin(l *lymcwd.Location, slug string) (string, error) {
	branch, found, err := t.pairTaskBranchLocal(l, slug)
	if err != nil || found {
		return branch, err
	}
	onOrigin, err := t.originPairBranch(l, slug)
	if err != nil {
		return "", err
	}
	if onOrigin != "" {
		return onOrigin, nil
	}
	return branch, nil
}

// pairTaskBranchLocal returns the task branch of slug's pair from local worktrees and branches only.
// found is false when nothing local names the pair, and branch is then the current prefix's name.
func (t *Topology) pairTaskBranchLocal(l *lymcwd.Location, slug string) (branch string, found bool, err error) {
	names := pairBranchNames(t.cfg, slug)
	if target := WorktreePath(l, slug); pathPresent(target) {
		branch, err := worktreeBranchAt(l.WorktreePath(), target)
		if err != nil {
			return "", false, err
		}
		if branch != "" && (slices.Contains(names, branch) || weftBranchExists(l, RecordsBranchName(branch))) {
			return branch, true, nil
		}
	}
	if target := RecordsWorktreePath(l, slug); pathPresent(target) {
		// An unresolvable records repository leaves this step to the local-branch step, so a half-torn pair stays readable.
		if weftRoot, rootErr := RecordsRepoRoot(l); rootErr == nil {
			branch, err := worktreeBranchAt(weftRoot, target)
			if err != nil {
				return "", false, err
			}
			if warp, ok := WeftWarpSlug(branch); ok {
				exists, err := localBranchExists(l.WorktreePath(), warp)
				if err != nil {
					return "", false, err
				}
				if exists || slices.Contains(names, warp) {
					return warp, true, nil
				}
			}
		}
	}
	for _, name := range names {
		exists, err := localBranchExists(l.WorktreePath(), name)
		if err != nil {
			return "", false, err
		}
		if exists || weftBranchExists(l, RecordsBranchName(name)) {
			return name, true, nil
		}
	}
	return names[0], false, nil
}

// worktreeBranchAt returns the branch checked out in the linked worktree of repoDir at target.
// It returns "" when target is no linked worktree of repoDir or sits on a detached HEAD.
func worktreeBranchAt(repoDir, target string) (string, error) {
	entries, err := List(repoDir)
	if err != nil {
		return "", err
	}
	cleanTarget := filepath.Clean(target)
	for _, entry := range entries {
		if entry.Main || entry.Prunable || filepath.Clean(filepath.FromSlash(entry.Path)) != cleanTarget {
			continue
		}
		if entry.Branch == detachedBranch {
			return "", nil
		}
		return entry.Branch, nil
	}
	return "", nil
}

// originTaskBranch returns the first of slug's pair names whose task branch origin holds.
// It returns "" when origin holds none.
// A repository with no origin remote gives "", and a failed lookup on a configured origin is returned.
func (t *Topology) originTaskBranch(l *lymcwd.Location, slug string) (string, error) {
	return firstOnOrigin(l.WorktreePath(), pairBranchNames(t.cfg, slug), func(name string) string { return name })
}

// originPairBranch returns the first of slug's pair names whose task branch origin holds.
// Failing that, it returns the first name whose records branch the weft origin holds.
// It returns "" when neither holds.
// A repository with no origin remote is not asked, and a failed lookup on a configured origin is returned.
func (t *Topology) originPairBranch(l *lymcwd.Location, slug string) (string, error) {
	found, err := t.originTaskBranch(l, slug)
	if err != nil || found != "" {
		return found, err
	}
	weftRoot, err := RecordsRepoRoot(l)
	if err != nil {
		return "", fmt.Errorf("resolve weft repo root: %w", err)
	}
	return firstOnOrigin(weftRoot, pairBranchNames(t.cfg, slug), RecordsBranchName)
}

// firstOnOrigin returns the first of names whose branch, as branchOf maps it, exists on origin.
// The origin is that of the repository at repoDir.
// It returns "" when none exists or the repository has no origin remote.
func firstOnOrigin(repoDir string, names []string, branchOf func(string) string) (string, error) {
	if _, err := gitrepo.New(repoDir).RemoteURL(originRemoteName); err != nil {
		return "", nil
	}
	for _, name := range names {
		tip, err := remoteHeadTip(repoDir, branchOf(name))
		if err != nil {
			return "", err
		}
		if tip != "" {
			return name, nil
		}
	}
	return "", nil
}
