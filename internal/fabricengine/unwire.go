// unwire.go implements the Unwire verb: a per-warp-worktree full deactivation of fabric wiring, the
// teardown successor to the deleted `lym init --undo`.
// Without `apply` the verb only reports what it would remove and changes nothing.
//
// Unwire is per-worktree and never touches the repo-wide `weft:main` records (`<BoardDir>/_lym/config/fabric.yaml`).
// It leaves every worktree's `.lym-anchor` marker at its weft root alone too.
// A later `lym fabric reconcile` re-wire still needs all of them.
// An applied Unwire removes every fabric junction on disk but the root scratch link, which stays with its `/.scratch` exclude line.
// It is distinct from Reconcile: Reconcile converges wiring toward the repo-wide pathspec (may add,
// re-point, or remove junctions), while an applied Unwire always removes every fabric junction present on disk
// and reverses their warp `.git/info/exclude` entries — nothing more. It never deletes weft-side
// content: weft-side `_lym` and `.lym` are preserved, and no committed `.gitignore` block exists to
// revert since `.lym` is excluded through the warp's `.git/info/exclude` alone.

package fabricengine

import (
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/lymcwd"
)

// UnwireVerbResult summarizes what Unwire changed, or on a dry run what it would change.
// It embeds MutationRecord, which carries the mutation record accumulated over the call;
// a dry run records no mutation.
type UnwireVerbResult struct {
	MutationRecord
	// Applied is true only when the junctions and exclude lines were actually removed;
	// false on a dry run.
	Applied bool
	// JunctionsRemoved lists the Name of each warp junction that was actually
	// present and removed. Empty when no junction was wired.
	// On a dry run it lists the junctions that would be removed, all left in place.
	JunctionsRemoved []string
	// WeftContent describes _lym only — "preserved" or "not_present" — weft-side _lym content is preserved by design, never deleted by unwire.
	// The weft-side .lym is never touched by unwire either;
	// it disappears with the weft worktree when Remove tears the pair down, and on Windows an open handle inside it makes that `git worktree remove --force` fail with an OS error that surfaces as-is — remedy: stop the daemons and re-run.
	WeftContent string
	// GitExclude is "reverted" or "unchanged".
	// On a dry run "reverted" means an exclude line would be stripped, and it is left in place.
	GitExclude string
}

// Unwire reverses every warp junction wired for the worktree at cwd and their warp
// `.git/info/exclude` entries — a full per-warp-worktree deactivation.
// With apply false it is a dry run:
// nothing is removed, and the result reports the junctions and exclude lines an applied run would remove, with Applied false.
// A dry run still takes the exclude-file lock, creating the info directory and the lock file when they are absent.
// The junction name-set is enumerated from a full on-disk scan, removing every fabric-owned
// junction present on disk, including stale ones absent from the repo-wide pathspec.
// The root scratch link is the exception: it and its `/.scratch` exclude stay.
// Ownership is decided by where a link resolves (see scanOnDiskJunctionNames): a hand-authored
// symlink checked into the warp repo beside the junctions is never claimed or removed.
// It never deletes weft-side content: every weft-side directory, including `.lym`, is left intact.
// Unwire never touches the repo-wide weft:main records or any worktree's `.lym-anchor` marker at its weft root;
// a later `lym fabric reconcile` re-wire can recreate this worktree's wiring.
func Unwire(cwd string, apply bool) (res UnwireVerbResult, err error) {
	var rec *Mutations
	defer func() { res.Mutations = rec.Snapshot() }()

	l, err := lymcwd.Resolve(cwd)
	if err != nil {
		return UnwireVerbResult{}, err
	}
	rec = NewMutations(l.HubPath)

	slug := filepath.Base(l.WorktreePath())

	names, err := scanOnDiskJunctionNames(l, slug)
	if err != nil {
		return UnwireVerbResult{}, err
	}
	names = withoutRootScratchLink(l, names)

	var junctionsRemoved []string
	var excludeChanged bool
	if apply {
		junctionResult, err := UnwireJunctions(l, slug, names)
		rec.Extend(junctionResult.Mutated())
		if err != nil {
			return UnwireVerbResult{}, err
		}
		junctionsRemoved = junctionResult.JunctionsRemoved
		excludeChanged = junctionResult.ExcludeChanged
	} else {
		junctionsRemoved = names
		excludeChanged, err = unseedGitExclude(rec, l, slug, names, true)
		if err != nil {
			return UnwireVerbResult{}, err
		}
	}

	result := UnwireVerbResult{Applied: apply}

	// A pure observation, never a mutation: the weft-side _lym (and, since it is never
	// touched by unwire either, .lym) is left exactly as it was found. It disappears
	// with the weft worktree only when Remove tears the pair down — on Windows, an
	// open handle inside it makes that `git worktree remove --force` fail with an OS
	// error that surfaces as-is; the remedy is the same as adoption's: stop the
	// daemons and re-run.
	weftWorktree := RecordsWorktree(l)
	if _, statErr := os.Stat(weftWorktree); statErr != nil && !os.IsNotExist(statErr) {
		return UnwireVerbResult{}, statErr
	} else if os.IsNotExist(statErr) {
		result.WeftContent = "not_present"
	} else {
		weftLymDir := WeftLymDirFor(l, slug)
		if _, statErr := os.Stat(weftLymDir); statErr != nil && !os.IsNotExist(statErr) {
			return UnwireVerbResult{}, statErr
		} else if os.IsNotExist(statErr) {
			result.WeftContent = "not_present"
		} else {
			result.WeftContent = "preserved"
		}
	}

	result.JunctionsRemoved = junctionsRemoved
	result.GitExclude = "unchanged"
	if excludeChanged {
		result.GitExclude = "reverted"
	}

	return result, nil
}
