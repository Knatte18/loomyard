// notices.go resolves where batten's notices about one run go: batten acts from the prime's location, but a notice takes the walk up the beam chain from the run's own pair, never from where batten runs.

package battencli

import (
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/orchcli"
)

// notifyRunParent sends line as creel mail to the nearest live orch above the run slug, signed as batten, and reports whether it was posted.
// The walk starts at the run's pair when it is present here, else at the worktree the pair forks or forked from, else at the prime.
func notifyRunParent(prime *lyxcwd.Location, slug, line string) (bool, error) {
	site, isPair, err := runNoticeSite(prime, slug)
	if err != nil {
		return false, err
	}
	if isPair {
		return orchcli.NotifyParent(site, battenProgram, line)
	}
	return orchcli.NotifyFrom(site, battenProgram, line)
}

// runParentRecorded reports whether the destination notifyRunParent walks to for the run slug records an orch strand, which is what decides a run to be watched or silent.
func runParentRecorded(prime *lyxcwd.Location, slug string) (bool, error) {
	site, isPair, err := runNoticeSite(prime, slug)
	if err != nil {
		return false, err
	}
	if isPair {
		return orchcli.ParentOrchRecorded(site)
	}
	destination, err := hubgeom.NearestLiveOrch(site, orchcli.OrchLive)
	if err != nil {
		return false, err
	}
	worktree := prime
	if destination.Worktree != "" {
		beam, found, err := presentWorktree(prime, destination.Worktree)
		if err != nil {
			return false, err
		}
		if found {
			worktree = beam
		}
	}
	_, recorded, err := orchcli.OrchStrandOf(worktree)
	return recorded, err
}

// runNoticeSite is noticeSite over the hub: the run's pair is looked up under the prime's hub, and its parent is hubgeom's.
func runNoticeSite(prime *lyxcwd.Location, slug string) (site *lyxcwd.Location, isPair bool, err error) {
	return noticeSite(prime, slug,
		func() (*lyxcwd.Location, bool, error) { return presentWorktree(prime, slug) },
		func(slug string) (string, error) { return hubgeom.ParentOf(prime, slug) })
}

// noticeSite picks the worktree a walk for the run slug starts at: the run's pair when pair finds it present here, else the worktree the pair forks or forked from, parentOf's answer, when that worktree is present here, else the prime.
// isPair reports that the site is the run's own pair, whose walk starts at its recorded parent.
func noticeSite(prime *lyxcwd.Location, slug string, pair func() (*lyxcwd.Location, bool, error), parentOf func(slug string) (string, error)) (site *lyxcwd.Location, isPair bool, err error) {
	location, found, err := pair()
	if err != nil {
		return nil, false, err
	}
	if found {
		return location, true, nil
	}
	parent, err := parentOf(slug)
	if err != nil {
		return nil, false, err
	}
	if parent == "" {
		return prime, false, nil
	}
	location, found, err = presentWorktree(prime, parent)
	if err != nil {
		return nil, false, err
	}
	if !found {
		return prime, false, nil
	}
	return location, false, nil
}

// presentWorktree resolves the hub worktree named worktree, and reports false when it is not present here.
func presentWorktree(prime *lyxcwd.Location, worktree string) (*lyxcwd.Location, bool, error) {
	root := fabricengine.WorktreePath(prime, worktree)
	if _, err := os.Stat(filepath.Join(root, ".git")); err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	location, err := lyxcwd.ResolveWorktree(root)
	if err != nil {
		return nil, false, err
	}
	return location, true, nil
}
