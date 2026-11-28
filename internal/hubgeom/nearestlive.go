// nearestlive.go implements NearestLiveOrch, the walk up the beam chain to the nearest worktree whose orch is live.

package hubgeom

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
)

// NearestLiveOrch returns the orch to notify for a run acting from the worktree of from.
// It tries that worktree, then walks upward through the origin records' parent worktrees.
// It returns the nearest worktree whose orch strand is live, or the prime when no worktree on the walk is.
// The answer's Name is that worktree's orch agent name and its Worktree the worktree's name, empty for the prime.
// A worktree with no origin record, an empty parent worktree, or a gone parent worktree ends the walk at the prime.
// A cycle in the records ends it there too.
// A hub with no recorded shortname yields no parent and a warning, since no name can be formed.
// A read error, and an error from live, are returned.
func NearestLiveOrch(from *lymcwd.Location, live func(worktree *lymcwd.Location) (bool, error)) (Parent, error) {
	shortname, found := fabricengine.ReadShortname(fabriclayout.BoltDir(from.HubPath))
	if !found {
		logger.Warn("hubgeom: hub has no recorded shortname; the notice gets no orch", "worktree", from.WorktreeName)
		return Parent{}, nil
	}
	start, err := walkStart(from)
	if err != nil {
		return Parent{}, err
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

// OrchChain returns the orchs on the beam chain at and above the worktree of from, nearest first and the prime's last.
// It walks the chain NearestLiveOrch walks and ends where that walk ends.
// It asks no orch whether it is live, so an orch reading as down keeps its place.
// Each answer's Name is a worktree's orch agent name and its Worktree the worktree's name, empty for the prime.
// A hub with no recorded shortname yields no orch and a warning, since no name can be formed.
// A read error is returned.
func OrchChain(from *lymcwd.Location) ([]Parent, error) {
	shortname, found := fabricengine.ReadShortname(fabriclayout.BoltDir(from.HubPath))
	if !found {
		logger.Warn("hubgeom: hub has no recorded shortname; the beam chain names no orch", "worktree", from.WorktreeName)
		return nil, nil
	}
	start, err := walkStart(from)
	if err != nil {
		return nil, err
	}
	worktrees, err := chainWorktrees(start, func(worktree string) (string, bool, error) { return recordedParentWorktree(from, worktree) })
	if err != nil {
		return nil, err
	}
	chain := make([]Parent, 0, len(worktrees))
	for _, worktree := range worktrees {
		name, err := agentname.Format(shortname, worktree, agentname.RoleOrch)
		if err != nil {
			return nil, fmt.Errorf("hubgeom: form orch agent name for worktree %q: %w", worktree, err)
		}
		chain = append(chain, Parent{Name: name, Worktree: worktree})
	}
	return chain, nil
}

// OrchChainAbove returns the orchs on the beam chain above the worktree of pair, nearest first and the prime's last.
// It leaves out the orch of pair's own worktree and asks none whether it is live.
// A hub with no recorded shortname yields no orch, and a read error is returned.
func OrchChainAbove(pair *lymcwd.Location) ([]Parent, error) {
	chain, err := OrchChain(pair)
	if err != nil {
		return nil, err
	}
	return slices.DeleteFunc(chain, func(orch Parent) bool { return orch.Worktree == pair.WorktreeName }), nil
}

// walkStart returns the worktree a walk up the beam chain from the worktree of from starts at, "" for the prime.
func walkStart(from *lymcwd.Location) (string, error) {
	fromPrime, err := isPrimeWorktree(from.WorktreePath())
	if err != nil {
		return "", err
	}
	if fromPrime {
		return "", nil
	}
	return from.WorktreeName, nil
}

// recordedParentWorktree reads the parent worktree of the named worktree from its origin record, "" meaning the prime.
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

// chainWorktrees walks up from start through parentOf and returns every worktree on the walk, nearest first.
// The walk ends where nearestLive's ends, and the list always closes with "", the prime.
// It reads no filesystem itself.
func chainWorktrees(start string, parentOf func(worktree string) (string, bool, error)) ([]string, error) {
	var chain []string
	visited := map[string]bool{}
	for current := start; current != "" && !visited[current]; {
		visited[current] = true
		chain = append(chain, current)
		parent, found, err := parentOf(current)
		if err != nil {
			return nil, err
		}
		if !found {
			break
		}
		current = parent
	}
	return append(chain, ""), nil
}

// nearestLive walks up from start through parentOf and returns the first worktree that live reports live.
// It returns "" when no worktree on the walk is live, and it reads no filesystem itself.
// parentOf reports a worktree's parent worktree, "" for the prime, and false when the walk ends there;
// start "" is the prime.
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
