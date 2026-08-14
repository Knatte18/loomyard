// unwire.go implements the Unwire verb: a per-warp-worktree full deactivation of fabric wiring, the
// teardown successor to the deleted `lyx init --undo`.
//
// Unwire is per-worktree and never touches the repo-wide `weft:main` records (`.lyx-anchor`,
// `<BoardDir>/_lyx/config/fabric.yaml`) — those are per-repo facts a later `lyx fabric reconcile`
// re-wire still needs.
// It is distinct from Reconcile: Reconcile converges wiring toward the repo-wide pathspec (may add,
// re-point, or remove junctions), while Unwire always removes every fabric junction present on disk
// and reverses their warp `.git/info/exclude` entries — nothing more. It never deletes weft-side
// content: weft-side `_lyx` and `.lyx` are preserved, and no committed `.gitignore` block exists to
// revert since `.lyx` is excluded through the warp's `.git/info/exclude` alone.

package fabricengine

import (
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

// UnwireVerbResult summarizes what Unwire changed.
// It embeds MutationRecord, which carries the mutation record accumulated over the call.
type UnwireVerbResult struct {
	MutationRecord
	// JunctionsRemoved lists the Name of each warp junction that was actually
	// present and removed. Empty when no junction was wired.
	JunctionsRemoved []string
	// WeftContent describes _lyx only — "preserved" or "not_present" — weft-side
	// _lyx content (including _lyx/PATTERN.md) is preserved by design, never
	// deleted by unwire. The weft-side .lyx is never touched by unwire either;
	// it disappears with the weft worktree when Remove tears the pair down, and on
	// Windows an open handle inside it makes that `git worktree remove --force`
	// fail with an OS error that surfaces as-is — remedy: stop the daemons and
	// re-run.
	WeftContent string
	GitExclude  string // "reverted" or "unchanged"
}

// Unwire reverses every warp junction wired for the worktree at cwd and their warp
// `.git/info/exclude` entries — a full per-warp-worktree deactivation.
// The junction name-set is enumerated from a full on-disk scan, removing every fabric-owned
// junction present on disk, including stale ones absent from the repo-wide pathspec.
// Ownership is decided by where a link resolves (see scanOnDiskJunctionNames): a hand-authored
// symlink checked into the warp repo beside the junctions is never claimed or removed.
// It never deletes weft-side content: every weft-side directory, including `.lyx`, is left intact.
// Unwire never touches the repo-wide weft:main records;
// a later `lyx fabric reconcile` re-wire can recreate this worktree's wiring.
func Unwire(cwd string) (res UnwireVerbResult, err error) {
	var rec *Mutations
	defer func() { res.Mutations = rec.Snapshot() }()

	l, err := lyxcwd.Resolve(cwd)
	if err != nil {
		return UnwireVerbResult{}, err
	}
	rec = NewMutations(l.HubPath)

	slug := filepath.Base(l.WorktreePath())

	names, err := scanOnDiskJunctionNames(l, slug)
	if err != nil {
		return UnwireVerbResult{}, err
	}

	junctionResult, err := UnwireJunctions(l, slug, names)
	rec.Extend(junctionResult.Mutated())
	if err != nil {
		return UnwireVerbResult{}, err
	}

	var result UnwireVerbResult

	// A pure observation, never a mutation: the weft-side _lyx (and, since it is never
	// touched by unwire either, .lyx) is left exactly as it was found. It disappears
	// with the weft worktree only when Remove tears the pair down — on Windows, an
	// open handle inside it makes that `git worktree remove --force` fail with an OS
	// error that surfaces as-is; the remedy is the same as adoption's: stop the
	// daemons and re-run.
	weftWorktree := WeftWorktree(l)
	if _, statErr := os.Stat(weftWorktree); statErr != nil && !os.IsNotExist(statErr) {
		return UnwireVerbResult{}, statErr
	} else if os.IsNotExist(statErr) {
		result.WeftContent = "not_present"
	} else {
		weftLyxDir := WeftLyxDirFor(l, slug)
		if _, statErr := os.Stat(weftLyxDir); statErr != nil && !os.IsNotExist(statErr) {
			return UnwireVerbResult{}, statErr
		} else if os.IsNotExist(statErr) {
			result.WeftContent = "not_present"
		} else {
			result.WeftContent = "preserved"
		}
	}

	result.JunctionsRemoved = junctionResult.JunctionsRemoved
	result.GitExclude = "unchanged"
	if junctionResult.ExcludeChanged {
		result.GitExclude = "reverted"
	}

	return result, nil
}
