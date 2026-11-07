// addadopt.go holds the pre-flight reads Add makes when it adopts an existing local warp branch.
// It refuses a branch another worktree holds and finds out whether the branch is the warp's default.

package fabricengine

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/lymcwd"
)

// refuseAdoptLocalCheckedOut refuses an adoption of warpBranch while a worktree holds it.
// It refuses an adoption of weftBranch while a records worktree holds it.
// Each refusal is an *ErrAdoptLocalCheckedOut.
// It names the worktree and the way forward that releases the branch without tearing any pair down.
// weftExists is whether weftBranch exists locally; a branch that does not has no worktree holding it.
// reusedWeft is the records worktree path the add reuses, or empty;
// that worktree does not count as another records worktree holding the branch.
// It reads and mutates nothing.
func refuseAdoptLocalCheckedOut(l *lymcwd.Location, slug, warpBranch, weftBranch string, weftExists bool, reusedWeft string) error {
	warpEntries, err := List(l.AnchorPath())
	if err != nil {
		return err
	}
	for _, entry := range warpEntries {
		if !entry.Prunable && entry.Branch == warpBranch {
			path := filepath.FromSlash(entry.Path)
			return &ErrAdoptLocalCheckedOut{Branch: warpBranch, Path: path, slug: slug}
		}
	}
	if !weftExists {
		return nil
	}
	weftRepoRoot, err := RecordsRepoRoot(l)
	if err != nil {
		return fmt.Errorf("resolve weft repo root: %w", err)
	}
	weftEntries, err := List(weftRepoRoot)
	if err != nil {
		return err
	}
	for _, entry := range weftEntries {
		if !entry.Prunable && entry.Branch == weftBranch {
			path := filepath.FromSlash(entry.Path)
			if reusedWeft != "" && samePath(entry.Path, reusedWeft) {
				continue
			}
			return &ErrAdoptLocalCheckedOut{Branch: weftBranch, Path: path, Records: true, slug: slug}
		}
	}
	return nil
}

// warpDefaultBranch returns the branch origin's HEAD points at.
// It reads the local remote-tracking HEAD ref and falls back to origin when that ref is unset.
// Under SkipGit or SkipPush no origin is consulted, so an unset ref reports no default branch ("").
func warpDefaultBranch(l *lymcwd.Location, opts AddOptions) (string, error) {
	out, err := gitexec.Run([]string{"symbolic-ref", "--short", "refs/remotes/" + originRemoteName + "/HEAD"}, l.WorktreePath())
	if err == nil {
		return strings.TrimPrefix(strings.TrimSpace(out), originRemoteName+"/"), nil
	}
	if opts.SkipGit || opts.SkipPush {
		return "", nil
	}
	return remoteDefaultBranch(l.WorktreePath())
}
