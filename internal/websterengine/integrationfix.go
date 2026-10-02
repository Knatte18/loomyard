// integrationfix.go is the home of the integration-fix attempt: the one-shot strand that tries to repair an integration regression.
// checkFixCommits decides whether Go accepts the commits that strand left behind.
//
// The path rule binds commits only.
// When the plan directory or `_lyx` lies outside the task repository, as in the hub geometry where `_lyx` is a directory link to the weft worktree,
// no commit can reach a file there, and only a commit adding or retargeting the `_lyx` entry itself is visible.
// An on-disk plan write is caught by the plan-fingerprint compare around the strand;
// any other on-disk write under `_lyx` outside the repository is bounded by the strand's prompt alone.

package websterengine

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/lyxdirs"
)

// checkFixCommits returns an error, making the fix attempt count as failed, unless the strand's commits are acceptable:
// reportHead is preFixHead or descends from it, every commit in preFixHead..reportHead has exactly one parent,
// none changes a path under planDir, the `_lyx` entry itself or a path under it, HEAD reconciles against reportHead under the merge-only rule,
// and the worktree carries no uncommitted or untracked change, since the post-fix verify runs over the working tree and must judge only the commits.
// planDir may be absolute or relative to worktree; the plan rule is skipped when it lies outside worktree.
// The warning is reconcileHead's moved-HEAD notice, passed through.
func checkFixCommits(worktree, preFixHead, reportHead, planDir string, parentBranch ParentBranchFunc) (warning string, err error) {
	if reportHead != preFixHead {
		descends, err := isAncestor(worktree, preFixHead, reportHead)
		if err != nil {
			return "", err
		}
		if !descends {
			return "", fmt.Errorf("webster: integration fix: reported head %q does not descend from the pre-fix head %q", reportHead, preFixHead)
		}
	}

	planRel, err := planDirRel(worktree, planDir)
	if err != nil {
		return "", err
	}
	commits, err := commitsBetween(worktree, preFixHead, reportHead)
	if err != nil {
		return "", err
	}
	for _, commit := range commits {
		parents, err := commitParentCount(worktree, commit)
		if err != nil {
			return "", err
		}
		if parents != 1 {
			return "", fmt.Errorf("webster: integration fix: commit %s has %d parents; only non-merge commits are accepted", commit, parents)
		}
		paths, err := commitChangedPaths(worktree, commit)
		if err != nil {
			return "", err
		}
		for _, path := range paths {
			if fixForbiddenPath(path, planRel) {
				return "", fmt.Errorf("webster: integration fix: commit %s changes %s, which is under the plan or %s", commit, path, lyxdirs.LyxDirName)
			}
		}
	}

	warning, err = reconcileHead(worktree, reportHead, "integration fix", parentBranch, integrationFixHeadRefusal)
	if err != nil {
		return "", err
	}

	isDirty, err := dirty(worktree)
	if err != nil {
		return "", err
	}
	if isDirty {
		return "", fmt.Errorf("webster: integration fix: worktree %s has uncommitted or untracked changes; the post-fix verify must judge only commits", worktree)
	}
	return warning, nil
}

// planDirRel returns planDir as a slash-separated path relative to worktree, or "" when it lies outside worktree or is worktree itself.
func planDirRel(worktree, planDir string) (string, error) {
	if !filepath.IsAbs(planDir) {
		return filepath.ToSlash(filepath.Clean(planDir)), nil
	}
	root, err := canonicalPath(worktree)
	if err != nil {
		return "", err
	}
	dir, err := canonicalPath(planDir)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(root, dir)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", nil
	}
	return filepath.ToSlash(rel), nil
}

// fixForbiddenPath reports whether a fix commit may not change path: the `_lyx` entry itself, anything under it, or anything under planRel.
func fixForbiddenPath(path, planRel string) bool {
	if path == lyxdirs.LyxDirName || strings.HasPrefix(path, lyxdirs.LyxDirName+"/") {
		return true
	}
	return planRel != "" && (path == planRel || strings.HasPrefix(path, planRel+"/"))
}
