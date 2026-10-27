// nearestlive.go implements NearestLiveOrch, the walk up the beam chain to the nearest worktree whose orch is live.

package hubgeom

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
)

// NearestLiveOrch returns the orch to notify for a run acting from the worktree from names.
// It tries that worktree, then walks upward through the origin records, each worktree's ParentWorktree.
// It returns the nearest worktree whose orch strand live reports live, and the prime whatever its state when none is.
// The answer's Name is that worktree's orch agent name and its Worktree the worktree's name, empty for the prime.
// A worktree with no origin record, an empty parent worktree or a parent worktree that is gone ends the walk at the prime.
// A cycle in the records ends it there too.
// A hub with no recorded shortname yields no parent and a warning, since no name can be formed.
// A read error, and an error from live, are returned.
func NearestLiveOrch(from *lymcwd.Location, live func(worktree *lymcwd.Location) (bool, error)) (Parent, error) {
	shortname, found := fabricengine.ReadShortname(fabricengine.BoardDir(from.HubPath))
	if !found {
		logger.Warn("hubgeom: hub has no recorded shortname; the notice gets no orch", "worktree", from.WorktreeName)
		return Parent{}, nil
	}
	start := from.WorktreeName
	fromPrime, err := isPrimeWorktree(from.WorktreePath())
	if err != nil {
		return Parent{}, err
	}
	if fromPrime {
		start = ""
	}
	worktree, err := nearestLive(start,
		func(worktree string) (string, bool, error) { return recordedParentWorktree(from, worktree) },
		func(worktree string) (bool, error) {
			location, err := lymcwd.ResolveWorktree(filepath.Join(from.HubPath, worktree))
			if err != nil {
				return false, fmt.Errorf("hubgeom: resolve worktree %q: %w", worktree, err)
			}
			return live(location)
		})
	if err != nil {
		return Parent{}, err
	}
	name, err := agentname.Format(shortname, worktree, agentname.RoleOrch)
	if err != nil {
		return Parent{}, fmt.Errorf("hubgeom: form orch agent name for worktree %q: %w", worktree, err)
	}
	return Parent{Name: name, Worktree: worktree}, nil
}

// recordedParentWorktree reads the parent worktree of the worktree named worktree from its origin record, "" meaning the prime.
// It reports false when the worktree has no record, the record names no parent worktree or the parent worktree is gone.
func recordedParentWorktree(from *lymcwd.Location, worktree string) (string, bool, error) {
	origin, found, err := fabricengine.ReadOriginFor(from, worktree)
	if err != nil {
		return "", false, fmt.Errorf("hubgeom: read origin record for %q: %w", worktree, err)
	}
	if !found || origin.ParentWorktree == "" {
		return "", false, nil
	}
	parentRoot := filepath.Join(from.HubPath, origin.ParentWorktree)
	if _, err := os.Stat(filepath.Join(parentRoot, gitEntryName)); errors.Is(err, fs.ErrNotExist) {
		return "", false, nil
	}
	parentIsPrime, err := isPrimeWorktree(parentRoot)
	if err != nil {
		return "", false, err
	}
	if parentIsPrime {
		return "", true, nil
	}
	return origin.ParentWorktree, true, nil
}

// nearestLive is NearestLiveOrch's walk over told reads, so its table needs no filesystem.
// It returns start when live reports it live, else its parent's answer, and "" for the prime.
// parentOf reports a worktree's parent worktree, "" for the prime, and false when the walk ends there; start "" is the prime.
// The prime is never asked of live, and a worktree seen twice ends the walk at the prime.
func nearestLive(start string, parentOf func(worktree string) (string, bool, error), live func(worktree string) (bool, error)) (string, error) {
	visited := map[string]bool{}
	for current := start; current != ""; {
		if visited[current] {
			return "", nil
		}
		visited[current] = true
		isLive, err := live(current)
		if err != nil {
			return "", err
		}
		if isLive {
			return current, nil
		}
		parent, found, err := parentOf(current)
		if err != nil {
			return "", err
		}
		if !found {
			return "", nil
		}
		current = parent
	}
	return "", nil
}
