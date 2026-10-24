// addparent.go resolves the parent pair that Add forks a new pair from when AddOptions.Parent names one.
// It holds the pre-flight refusals for a parent that cannot be forked from, each naming the way forward.

package fabricengine

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

// addParent is the pair Add forks a new pair from: its worktree location, the branch checked out there, the records branch of that branch and the commit its HEAD stands at.
type addParent struct {
	location      *lyxcwd.Location
	branch        string
	recordsBranch string
	headSHA       string
}

// resolveAddParent resolves the pair slug parent of l's hub into the fork point of a new pair.
// It refuses a parent that is not a live pair of the hub, the prime, a parent on a detached HEAD or unborn branch,
// and a parent whose checked-out branch is not its own pair branch, each naming the way forward.
// It reads and mutates nothing.
func resolveAddParent(l *lyxcwd.Location, cfg Config, parent string) (addParent, error) {
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
		return addParent{}, fmt.Errorf("--parent %q is not a pair of this hub: no linked worktree at %s; way forward: run `lyx fabric pairs` to list the hub's pairs and name one of them", parent, parentPath)
	}
	if isPrime {
		return addParent{}, fmt.Errorf("--parent %q names the prime; way forward: drop --parent and run the add from the prime, since without --parent the add forks from the acting worktree's HEAD", parent)
	}

	pairBranch := cfg.BranchPrefix + parent
	location, err := lyxcwd.ResolveWorktree(parentPath)
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
		return addParent{}, fmt.Errorf("--parent %q is on a detached HEAD or an unborn branch and has no branch to fork from; way forward: run `lyx fabric checkout %s` in %s, then re-run the add", parent, pairBranch, parentPath)
	}
	if shaErr != nil {
		return addParent{}, fmt.Errorf("read HEAD commit of --parent %q: %w", parent, shaErr)
	}
	if branch != pairBranch {
		return addParent{}, fmt.Errorf("--parent %q has %q checked out, not its own pair branch %q; way forward: run `lyx fabric checkout %s` in %s, then re-run the add", parent, branch, pairBranch, pairBranch, parentPath)
	}
	return addParent{location: location, branch: branch, recordsBranch: RecordsBranchName(branch), headSHA: headSHA}, nil
}
