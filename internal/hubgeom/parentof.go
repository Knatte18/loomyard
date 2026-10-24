// parentof.go implements ParentOf, the one resolver of the parent every owner reader is handed.

package hubgeom

import (
	"fmt"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

// ParentOf returns the slug of the pair that is the parent of the entry slug, "" meaning the prime.
// A pair's origin record decides, read over the prime's location, and its recorded parent worktree is the answer.
// It reads as the prime when the record names no parent worktree, names the prime's worktree or names the prime's checked-out branch.
// The parent is never derived by matching the recorded branch against the pairs present here,
// so a member whose beam pair is absent on this machine still reads the beam as its parent.
// Only when the slug has no origin record does the local board decide, through the nearest open beam above the entry.
// A read error from the record, the worktree listing or the board is returned.
func ParentOf(prime *lyxcwd.Location, slug string) (string, error) {
	origin, found, err := fabricengine.ReadOriginFor(prime, slug)
	if err != nil {
		return "", fmt.Errorf("hubgeom: read origin record for %q: %w", slug, err)
	}
	if !found {
		tasks, err := boardengine.ReadTasks(fabricengine.BoardDir(prime.HubPath))
		if err != nil {
			return "", fmt.Errorf("hubgeom: read the board for the parent of %q: %w", slug, err)
		}
		return boardengine.ParentBeam(tasks, slug), nil
	}
	if origin.ParentWorktree == "" {
		return "", nil
	}
	entries, err := fabricengine.List(prime.AnchorPath())
	if err != nil {
		return "", fmt.Errorf("hubgeom: list the worktrees for the parent of %q: %w", slug, err)
	}
	for _, entry := range entries {
		if !entry.Main {
			continue
		}
		if origin.ParentBranch == entry.Branch || origin.ParentWorktree == filepath.Base(filepath.Clean(filepath.FromSlash(entry.Path))) {
			return "", nil
		}
	}
	return origin.ParentWorktree, nil
}
