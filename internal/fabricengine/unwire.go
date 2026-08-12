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
	"fmt"
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/fslink"
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
	// BoardJunctionRemoved reports whether the operator-convenience _board
	// link was present and removed. It is surfaced separately from
	// JunctionsRemoved because _board is a named special case, not a member
	// of the pathspec-derived names scanOnDiskJunctionNames enumerates: that
	// scan skips every HubReservedNames() entry (_board included), so _board
	// can never appear in JunctionsRemoved.
	BoardJunctionRemoved bool
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

	// Remove the operator-convenience _board junction as an explicitly named
	// case: scanOnDiskJunctionNames above skips every HubReservedNames()
	// entry (_board included), so the generic sweep above can never see or
	// remove it — the same skip that keeps reconcile's stale sweep from
	// touching it (see reconcile.go's scanOnDiskJunctionNames doc).
	boardRemoved, err := unwireBoardLink(rec, l, slug)
	if err != nil {
		return UnwireVerbResult{}, err
	}

	var result UnwireVerbResult
	result.BoardJunctionRemoved = boardRemoved

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

// unwireBoardLink removes the _board junction wireBoardLink creates, if
// present, and its matching .git/info/exclude entry via a standalone
// unseedGitExclude(l, slug, []string{BoardDirName}) call — the unwire
// counterpart to wireBoardLink's own standalone seedGitExclude call, since
// _board has no WarpJunctions mirror-pair record either function can drive
// generically.
//
// Removing an absent link is not an error: it returns (false, nil). A
// present real directory (not a link) is refused, matching
// unseedJunctionRecords' refusal to delete user content sitting where a
// junction belongs.
// rec is the calling verb's own recorder, threaded through to removeLink.
func unwireBoardLink(rec *Mutations, l *lyxcwd.Location, slug string) (removed bool, err error) {
	link := filepath.Join(WorktreePath(l, slug), l.AnchorRel, BoardDirName)

	if _, statErr := os.Lstat(link); statErr == nil {
		isLink, linkErr := fslink.IsLink(link)
		if linkErr != nil {
			return false, fmt.Errorf("islink %s: %w", link, linkErr)
		}
		if !isLink {
			return false, fmt.Errorf(
				"warp repo already contains a real %s at %s; it is not a junction — refusing to remove it",
				filepath.Base(link), link,
			)
		}
		req := pathRequest{
			what:      "remove board junction",
			container: WorktreePath(l, slug),
			target:    link,
			slug:      nil,
			ownership: ownedWiredJunction([]string{link}, BoardDir(l.HubPath)),
			dirtiness: dirtinessNA("a junction holds no content; the weft target it points at is untouched"),
			force:     false,
		}
		if err := removeLink(rec, req); err != nil {
			return false, fmt.Errorf("remove board junction %s: %w", link, err)
		}
		removed = true
	} else if !os.IsNotExist(statErr) {
		return false, fmt.Errorf("lstat %s: %w", link, statErr)
	}

	if _, err := unseedGitExclude(rec, l, slug, []string{BoardDirName}); err != nil {
		return removed, err
	}

	return removed, nil
}
