// pairbranch.go reads the warp branch a pair stands on from the pair itself — its worktrees, then its local branches, then origin — so the branch prefix can change while pairs are live.

package fabricengine

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/lymcwd"
)

// pairBranchNames returns the warp branch names a pair of slug may stand on: the current prefix's, then the bare slug a pair made before the prefix was set carries.
// An empty prefix gives the one name.
func pairBranchNames(cfg Config, slug string) []string {
	if cfg.BranchPrefix == "" {
		return []string{slug}
	}
	return []string{cfg.BranchPrefix + slug, slug}
}

// slugOfBranch returns the slug a warp branch names: the branch without the current prefix when it carries it, else the bare branch of a pair made before the prefix was set.
func slugOfBranch(cfg Config, branch string) string {
	return strings.TrimPrefix(branch, cfg.BranchPrefix)
}

// PairTaskBranch returns the task branch of slug's pair, read from the pair itself rather than from the current branch prefix alone.
// It is the branch the task worktree has checked out, else the warp branch paired with the branch the records worktree has checked out, else the first of the pair's names (the current prefix's, then the bare slug) that a local task or records branch carries, else the current prefix's name.
// A task worktree's branch is read when it is one of the pair's names or has a records branch, so a pair made under any earlier prefix stays recognised.
// A records worktree's branch is read when its warp branch is one of the pair's names or exists locally.
// A worktree on any other branch is not read, so a pair checked out onto an unrelated branch never hands that branch to a teardown.
// It reads locally and makes no network call; a worktree list or branch lookup that fails is returned.
func (t *Topology) PairTaskBranch(l *lymcwd.Location, slug string) (string, error) {
	branch, _, err := t.pairTaskBranchLocal(l, slug)
	return branch, err
}

// PairTaskBranchOrOrigin returns the task branch of slug's pair as PairTaskBranch reads it, except that a pair of which nothing is left locally is named by the first of its names whose task branch, or else records branch, origin holds.
// It asks origin only when no local worktree or branch names the pair, so a pair the local read names makes no network call.
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

// pairTaskBranchLocal is PairTaskBranch's local read; found is false when nothing local names the pair and the result is the current prefix's name.
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

// worktreeBranchAt returns the branch the linked worktree of repoDir at target has checked out, or "" when target is no linked worktree of it or sits on a detached HEAD.
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

// originTaskBranch returns the first of slug's pair names whose task branch the task repository's origin holds, or "" when it holds none.
// A repository with no origin remote reads "", and a failed lookup on a configured origin is returned.
func (t *Topology) originTaskBranch(l *lymcwd.Location, slug string) (string, error) {
	return firstOnOrigin(l.WorktreePath(), pairBranchNames(t.cfg, slug), func(name string) string { return name })
}

// originPairBranch returns the first of slug's pair names whose task branch, or else whose records branch, the respective origin holds, or "" when origin holds neither for any name.
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

// firstOnOrigin returns the first of names whose branch, as branchOf names it, the origin of the repository at repoDir holds, or "" when it holds none or has no origin remote.
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
