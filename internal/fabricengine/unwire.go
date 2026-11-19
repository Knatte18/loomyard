// unwire.go implements the Unwire verb, a full per-worktree deactivation of fabric wiring.
// Without `apply` the verb only reports what it would remove and changes nothing.
//
// Unwire leaves the repo-wide `weft:main` records (`<BoardDir>/_lym/config/fabric.yaml`) in place.
// It also leaves every worktree's `.lym-anchor` marker at its weft root in place.
// A later `lym fabric reconcile` still needs those records and markers to rewire.
// An applied Unwire removes every fabric junction on disk, except the anchor's `.scratch` link and its exclude line.
// It reverses the warp `.git/info/exclude` entries of the junctions it removes.
// It never deletes weft-side content: `_lym` and `.lym` are preserved.
// No committed `.gitignore` block needs reverting: `.lym` is excluded through `.git/info/exclude` alone.

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
	// JunctionsRemoved lists the Name of each warp junction that was actually present and removed.
	// It is empty when no junction was wired.
	// On a dry run it lists the junctions that would be removed, all left in place.
	JunctionsRemoved []string
	// WeftContent is "preserved" when the weft-side _lym exists, and "not_present" otherwise.
	// Unwire never deletes weft-side content, so .lym is never touched either.
	// On Windows an open handle inside it makes the weft worktree's removal fail; stop the daemons and re-run.
	WeftContent string
	// GitExclude is "reverted" or "unchanged".
	// On a dry run "reverted" means an exclude line would be stripped, and it is left in place.
	GitExclude string
}

// Unwire reverses every warp junction wired for the worktree at cwd, and their warp `.git/info/exclude` entries.
// It is a full per-worktree deactivation of fabric wiring.
// With apply false it is a dry run: nothing is removed.
// The result then reports what an applied run would remove, with Applied false.
// A dry run still takes the exclude-file lock, creating the info directory and the lock file when they are absent.
// The junction names come from a full on-disk scan, so stale junctions outside the repo-wide pathspec are removed too.
// The anchor's `.scratch` link and its exclude line are kept.
// Ownership is decided by where a link resolves.
// A hand-authored symlink checked into the warp repo is never claimed or removed.
// It never deletes weft-side content: every weft-side directory, including `.lym`, is left intact.
// It never touches the repo-wide `weft:main` records or any worktree's `.lym-anchor` marker at its weft root.
// A later `lym fabric reconcile` can rewire this worktree.
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
	names = withoutScratchLink(names)

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

	// The weft-side _lym is only observed, never mutated, and .lym is left alone too.
	// It disappears with the weft worktree when the pair is torn down, and on Windows an open handle makes that fail.
	// The remedy is to stop the daemons and re-run.
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
