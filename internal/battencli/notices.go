// notices.go resolves where batten's notices about one run go: batten acts from the prime's location,
// but a notice takes the walk up the beam chain from the run's own pair, never from where batten runs.

package battencli

import (
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/creelengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/orchcli"
)

// notifyRunParent sends line as creel mail to the nearest live orch above the run slug, signed as batten.
// The mail is tagged with slug as its run, so the run's teardown drops it while it is unread.
// It reports whether the mail was posted.
// The walk starts at the run's pair when it is present here.
// Otherwise it starts at the worktree the pair forks or forked from, if that worktree is present here.
// Failing that, it starts at the prime.
func notifyRunParent(prime *lymcwd.Location, slug, line string) (bool, error) {
	site, isPair, err := runNoticeSite(prime, slug)
	if err != nil {
		return false, err
	}
	if isPair {
		return orchcli.NotifyParent(site, battenProgram, line)
	}
	return orchcli.NotifyRunFrom(site, "", battenProgram, runNotifyTags(slug), line)
}

// runNotifyTags returns the creel tags of batten's notice about the run slug.
// The run lets the run's teardown drop the mail while it is unread.
func runNotifyTags(slug string) creelengine.PostTags {
	return creelengine.PostTags{Run: slug}
}

// runParentRecorded reports whether the live orch above the run slug records an orch strand.
// The answer decides whether the run is watched or silent.
func runParentRecorded(prime *lymcwd.Location, slug string) (bool, error) {
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

// runNoticeSite resolves the site a notice about the run slug starts from, under the prime's hub.
// The run's pair is looked up under that hub, and its parent comes from `hubgeom`.
func runNoticeSite(prime *lymcwd.Location, slug string) (site *lymcwd.Location, isPair bool, err error) {
	return noticeSite(prime, slug,
		func() (*lymcwd.Location, bool, error) { return presentWorktree(prime, slug) },
		func(slug string) (string, error) { return hubgeom.ParentOf(prime, slug) })
}

// noticeSite picks the worktree a walk for the run slug starts at.
// It is the run's pair when pair finds it present here.
// Otherwise it is the worktree parentOf names, when that worktree is present here.
// If neither applies, it is the prime.
// isPair reports that the site is the run's own pair, whose walk starts at its recorded parent.
func noticeSite(prime *lymcwd.Location, slug string, pair func() (*lymcwd.Location, bool, error), parentOf func(slug string) (string, error)) (site *lymcwd.Location, isPair bool, err error) {
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
func presentWorktree(prime *lymcwd.Location, worktree string) (*lymcwd.Location, bool, error) {
	root := fabricengine.WorktreePath(prime, worktree)
	if _, err := os.Stat(filepath.Join(root, ".git")); err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	location, err := lymcwd.ResolveWorktree(root)
	if err != nil {
		return nil, false, err
	}
	return location, true, nil
}
