// unwire.go implements the Unwire verb: a per-host-worktree full deactivation
// of fabric wiring, the teardown successor to the deleted `lyx init --undo`.
//
// Unwire is per-worktree and never touches the repo-wide `weft:main` records
// (`.lyx-anchor`, `<BoardDir>/_lyx/config/fabric.yaml`) — those are
// per-repo facts a later `lyx fabric reconcile` re-wire still needs. It is
// distinct from Reconcile: Reconcile converges wiring toward the repo-wide
// pathspec (may add, re-point, or remove junctions), while Unwire always
// removes every fabric junction present on disk, clears the weft-side _lyx
// content, and reverts the managed .gitignore block.

package fabricengine

import (
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/gitignore"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

// UnwireVerbResult summarizes what Unwire changed.
type UnwireVerbResult struct {
	// JunctionsRemoved lists the Name of each host junction that was actually
	// present and removed. Empty when no junction was wired.
	JunctionsRemoved []string
	// WeftContent describes _lyx only — "cleared" or "not_present" — and never
	// _pattern: weft _pattern content is preserved by design.
	WeftContent string
	GitExclude  string // "reverted" or "unchanged"
	Gitignore   string // "reverted" or "unchanged"
}

// Unwire reverses every host junction wired for the worktree at cwd, clears
// the weft-side _lyx content, and reverts the managed .gitignore block's
// ".lyx/" entry — a full per-host-worktree deactivation. The junction name-set
// is enumerated from a full on-disk scan, removing every fabric junction present
// on disk, including stale ones absent from the repo-wide pathspec. Unwire never
// touches the repo-wide weft:main records; a later `lyx fabric reconcile` re-wire
// can recreate this worktree's wiring.
func Unwire(cwd string) (UnwireVerbResult, error) {
	l, err := lyxcwd.Resolve(cwd)
	if err != nil {
		return UnwireVerbResult{}, err
	}

	slug := filepath.Base(l.WorktreePath())

	names, err := scanOnDiskJunctionNames(l.WorktreePath(), l.AnchorRel)
	if err != nil {
		return UnwireVerbResult{}, err
	}

	junctionResult, err := UnwireJunctions(l, slug, names)
	if err != nil {
		return UnwireVerbResult{}, err
	}

	var result UnwireVerbResult

	weftWorktree := l.WeftWorktree()
	if _, statErr := os.Stat(weftWorktree); statErr != nil && !os.IsNotExist(statErr) {
		return UnwireVerbResult{}, statErr
	} else if os.IsNotExist(statErr) {
		result.WeftContent = "not_present"
	} else {
		weftLyxDir := l.WeftLyxDirFor(slug)
		if _, statErr := os.Stat(weftLyxDir); statErr != nil && !os.IsNotExist(statErr) {
			return UnwireVerbResult{}, statErr
		} else if os.IsNotExist(statErr) {
			result.WeftContent = "not_present"
		} else {
			if err := os.RemoveAll(weftLyxDir); err != nil {
				return UnwireVerbResult{}, err
			}
			result.WeftContent = "cleared"
		}

		opts := EnvSyncOptions()
		f, err := New(l.WorktreePath(), weftWorktree)
		if err != nil {
			return UnwireVerbResult{}, err
		}
		pathspec := ScopedPathspec(l.AnchorRel, []string{configengine.LyxDirName})
		if _, _, err := f.commitWeft(pathspec, "lyx fabric unwire: clear _lyx", opts); err != nil {
			return UnwireVerbResult{}, err
		}
		if err := pushWeftAt(weftWorktree, opts); err != nil {
			return UnwireVerbResult{}, err
		}
	}

	changed, err := gitignore.Remove(cwd, ".lyx/")
	if err != nil {
		return UnwireVerbResult{}, err
	}
	if changed {
		result.Gitignore = "reverted"
	} else {
		result.Gitignore = "unchanged"
	}

	result.JunctionsRemoved = junctionResult.JunctionsRemoved
	result.GitExclude = "unchanged"
	if junctionResult.ExcludeChanged {
		result.GitExclude = "reverted"
	}

	return result, nil
}
