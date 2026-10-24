// addadopt.go holds the pre-flight reads Add makes when AddOptions.AdoptLocal makes a pair over an existing local warp branch.
// It refuses a branch another worktree holds and finds out whether the branch is the warp's default.

package fabricengine

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

// refuseAdoptLocalCheckedOut refuses an adoption of warpBranch while a worktree holds it, and of weftBranch while a records worktree holds it.
// Each refusal names the worktree and the way forward that releases the branch without tearing any pair down.
// weftExists is whether weftBranch exists locally; a branch that does not has no worktree holding it.
// It reads and mutates nothing.
func refuseAdoptLocalCheckedOut(l *lyxcwd.Location, slug, warpBranch, weftBranch string, weftExists bool) error {
	warpEntries, err := List(l.AnchorPath())
	if err != nil {
		return err
	}
	for _, entry := range warpEntries {
		if !entry.Prunable && entry.Branch == warpBranch {
			path := filepath.FromSlash(entry.Path)
			return fmt.Errorf("--adopt-local: branch %q is checked out in %s; way forward: run `lyx fabric checkout <other branch>` in %s to move it off the branch, then re-run `lyx fabric add %s --adopt-local`", warpBranch, path, path, slug)
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
			return fmt.Errorf("--adopt-local: records branch %q is checked out in the records worktree %s; way forward: in %s switch off it, back to its own pair's records branch with `git switch <records branch>`, or detached with `git switch --detach` when it belongs to no pair, then re-run `lyx fabric add %s --adopt-local`", weftBranch, path, path, slug)
		}
	}
	return nil
}

// warpDefaultBranch returns the branch origin's HEAD points at, read from the local remote-tracking HEAD ref and, when that is unset, from origin.
// Under SkipGit or SkipPush no origin is consulted, so an unset ref reports no default branch ("").
func warpDefaultBranch(l *lyxcwd.Location, opts AddOptions) (string, error) {
	out, err := gitexec.Run([]string{"symbolic-ref", "--short", "refs/remotes/" + originRemoteName + "/HEAD"}, l.WorktreePath())
	if err == nil {
		return strings.TrimPrefix(strings.TrimSpace(out), originRemoteName+"/"), nil
	}
	if opts.SkipGit || opts.SkipPush {
		return "", nil
	}
	return remoteDefaultBranch(l.WorktreePath())
}
