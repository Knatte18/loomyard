// addparent.go resolves the parent pair that a new pair forks from, when the caller names one.
// It holds the pre-flight refusals for a parent that cannot be forked from, each naming the way forward.

package fabricengine

import (
	"errors"
	"fmt"
	"path/filepath"
	"slices"

	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/lymcwd"
)

// addParent is the pair that a new pair forks from:
// its worktree location, the branch checked out there, the records branch of that branch, and the commit at its HEAD.
type addParent struct {
	location      *lymcwd.Location
	branch        string
	recordsBranch string
	headSHA       string
}

// resolveAddParent resolves the pair slug parent of l's hub into the fork point of a new pair.
// It refuses a parent that is not a live pair of the hub, or that is the prime.
// It refuses a parent on a detached HEAD or an unborn branch.
// It refuses a parent whose checked-out branch is not its own pair branch.
// A branch is its own pair branch under the configured prefix, as the bare slug, or as any branch with a records branch;
// the last case covers a pair made under another prefix.
// Each refusal names the way forward.
// It reads and mutates nothing.
func resolveAddParent(l *lymcwd.Location, cfg Config, parent string) (addParent, error) {
	entries, err := List(l.AnchorPath())
	if err != nil {
		return addParent{}, err
	}
	parentPath := filepath.Clean(WorktreePath(l, parent))
	found, isPrime := false, false
	for _, entry := range entries {
		if entry.Prunable || filepath.Clean(filepath.FromSlash(entry.Path)) != parentPath {
			continue
		}
		found, isPrime = true, entry.Main
	}
	if !found {
		return addParent{}, fmt.Errorf("--parent %q is not a pair of this hub: no linked worktree at %s; way forward: run `lym fabric pairs` to list the hub's pairs and name one of them", parent, parentPath)
	}
	if isPrime {
		return addParent{}, fmt.Errorf("--parent %q names the prime; way forward: drop --parent and run the add from the prime, since without --parent the add forks from the acting worktree's HEAD", parent)
	}

	pairNames := pairBranchNames(cfg, parent)
	pairBranch := pairNames[0]
	location, err := lymcwd.ResolveWorktree(parentPath)
	if err != nil {
		return addParent{}, fmt.Errorf("resolve the worktree of --parent %q: %w", parent, err)
	}
	repo := gitrepo.New(parentPath)
	branch, detached, err := repo.HeadRef()
	if err != nil {
		return addParent{}, fmt.Errorf("read HEAD of --parent %q: %w", parent, err)
	}
	headSHA, shaErr := repo.CurrentSHA()
	if detached || errors.Is(shaErr, gitrepo.ErrNoCommits) {
		return addParent{}, fmt.Errorf("--parent %q is on a detached HEAD or an unborn branch and has no branch to fork from; way forward: run `lym fabric checkout %s` in %s, then re-run the add", parent, pairBranch, parentPath)
	}
	if shaErr != nil {
		return addParent{}, fmt.Errorf("read HEAD commit of --parent %q: %w", parent, shaErr)
	}
	if !slices.Contains(pairNames, branch) && !weftBranchExists(l, RecordsBranchName(branch)) {
		return addParent{}, fmt.Errorf("--parent %q has %q checked out, not its own pair branch %q; way forward: run `lym fabric checkout %s` in %s, then re-run the add", parent, branch, pairBranch, pairBranch, parentPath)
	}
	return addParent{location: location, branch: branch, recordsBranch: RecordsBranchName(branch), headSHA: headSHA}, nil
}
