// worktreeseam.go holds fabric's process-wide worktree seam.
// The seam is told when a worktree is made and when it takes in commits.
// Fabric hands the seam paths and names no file of the code that plugs in.

package fabricengine

import (
	"fmt"
	"path/filepath"
	"sync/atomic"

	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/lymdirs"
)

// WorktreeSeam is told when a worktree is made and when it takes in commits.
// An error from either is a warning for the verb that fired it and never fails that verb.
type WorktreeSeam interface {
	// Made reports a worktree that is wired, with its exclude lines seeded and its hub's Bolt checkout present.
	Made(event fabriclayout.WorktreeEvent) error
	// TookIn reports a worktree whose checked-out branch moved to commits that change event.Paths.
	TookIn(event fabriclayout.WorktreeEvent) error
}

// seamWayForward is the repair step every seam warning names.
const seamWayForward = "run `lym heddle render` in that worktree"

var installedWorktreeSeam atomic.Pointer[WorktreeSeam]

// InstallWorktreeSeam stores s as the process's worktree seam and returns a function that reinstates the previous one.
// With no seam installed a worktree event is no event.
func InstallWorktreeSeam(s WorktreeSeam) (restore func()) {
	previous := installedWorktreeSeam.Swap(&s)
	return func() { installedWorktreeSeam.Store(previous) }
}

// EventFor seeds the warp exclude list of l's worktree and returns the event for it carrying paths.
// The seeding runs under the exclude file's lock and comes first,
// so a caller that writes the files the list names writes only after their lines are in place.
func EventFor(l *lymcwd.Location, paths []string) (fabriclayout.WorktreeEvent, error) {
	excluded, err := SeedWarpExclude(l)
	if err != nil {
		return fabriclayout.WorktreeEvent{}, err
	}
	return fabriclayout.WorktreeEvent{
		Root:       l.WorktreePath(),
		Anchor:     l.AnchorPath(),
		ScratchDir: filepath.Join(RecordsWorktreePath(l, l.WorktreeName), l.AnchorRel, lymdirs.DotLymDirName),
		BoltDir:    fabriclayout.BoltDir(l.HubPath),
		Excluded:   excluded,
		Paths:      paths,
	}, nil
}

// fireWorktreeEvent tells the installed seam about l's worktree, "made" when made is set and "took in" over paths otherwise.
// It returns "" with no seam installed or when the seam took the event.
// A failure to build the event or a seam error is logged at Warn and returned as the warning text.
// The text names the worktree and the render verb that repairs it; it is never an error.
// The seam's render takes the Bolt write lock, so the caller holds none.
func fireWorktreeEvent(l *lymcwd.Location, paths []string, made bool) string {
	seam := installedWorktreeSeam.Load()
	if seam == nil {
		return ""
	}
	event, err := EventFor(l, paths)
	if err == nil {
		if made {
			err = (*seam).Made(event)
		} else {
			err = (*seam).TookIn(event)
		}
	}
	if err != nil {
		return seamWarning(l.WorktreePath(), err)
	}
	return ""
}

// seamWarning logs and returns the warning for a worktree event that failed at worktree.
func seamWarning(worktree string, err error) string {
	warning := fmt.Sprintf("the worktree event for %s failed: %v; way forward: %s", worktree, err, seamWayForward)
	logger.Warn("fabricengine: worktree seam failed (non-fatal)", "worktree", worktree, "error", err, "way_forward", seamWayForward)
	return warning
}

// NotifyWorktreeMade tells the installed seam that l's worktree was made.
// It returns the warning text of a failed event, and "" otherwise.
func NotifyWorktreeMade(l *lymcwd.Location) string {
	return fireWorktreeEvent(l, nil, true)
}

// NotifyWorktreeMadeAt tells the installed seam that the worktree at worktreePath was made.
// It resolves the worktree only when a seam is installed.
// It returns the warning text of a failed event or of a path that resolves to no worktree, and "" otherwise.
func NotifyWorktreeMadeAt(worktreePath string) string {
	if installedWorktreeSeam.Load() == nil {
		return ""
	}
	l, err := lymcwd.ResolveWorktree(worktreePath)
	if err != nil {
		return seamWarning(worktreePath, err)
	}
	return NotifyWorktreeMade(l)
}

// fireTookInOver tells the installed seam that l's worktree took in the commits from..to of the repository at dir.
// It returns the warning text of a failed event, and "" with no seam installed.
func fireTookInOver(l *lymcwd.Location, dir, from, to string) string {
	if installedWorktreeSeam.Load() == nil {
		return ""
	}
	paths, err := pathsChangedBetween(dir, from, to)
	if err != nil {
		return seamWarning(l.WorktreePath(), err)
	}
	return fireWorktreeEvent(l, paths, false)
}

// fireTookInAt tells the installed seam that the code worktree at worktree took in the commits from..to.
// It returns the warning text of a failed event, and "" with no seam installed.
func fireTookInAt(worktree, from, to string) string {
	if installedWorktreeSeam.Load() == nil {
		return ""
	}
	l, err := lymcwd.ResolveWorktree(worktree)
	if err != nil {
		return seamWarning(worktree, err)
	}
	return fireTookInOver(l, worktree, from, to)
}

// fireTookIn tells the installed seam that the warp worktree of f took in the commits from..to.
// The worktree is located from f's weft sibling, since f's warp path may lie outside the hub.
// It returns the warning text of a failed event, and "" with no seam installed.
func (f *Fabric) fireTookIn(from, to string) string {
	if installedWorktreeSeam.Load() == nil {
		return ""
	}
	l, err := f.hubLocation()
	if err != nil {
		return seamWarning(f.warpPath, err)
	}
	return fireTookInOver(l, f.warpPath, from, to)
}
