// unwire.go implements the Unwire verb: a per-host-worktree full deactivation
// of fabric wiring, the teardown successor to the deleted `lyx init --undo`.
//
// Unwire is per-worktree and never touches the repo-wide `weft:main` records
// (`.fabric-anchor`, `<BoardDir>/_lyx/config/fabric.yaml`) — those are
// per-repo facts a later `lyx fabric reconcile` re-wire still needs. It is
// distinct from Reconcile: Reconcile converges wiring toward the repo-wide
// pathspec (may add, re-point, or remove junctions), while Unwire always
// removes every fabric junction present on disk, clears the weft-side _lyx
// content, and reverts the managed .gitignore block.

package fabricengine

import (
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/gitignore"
	"github.com/Knatte18/loomyard/internal/hubgeometry"
)

// UnwireVerbResult summarizes what Unwire changed, mirroring the deleted
// initengine.UndoResult shape.
type UnwireVerbResult struct {
	// JunctionsRemoved lists the Name of each host junction that was
	// actually present and removed. Empty when no junction was wired.
	JunctionsRemoved []string
	// WeftContent describes _lyx only — "cleared" or "not_present" — and
	// NEVER _pattern: weft _pattern content is preserved by design (see
	// Unwire's godoc), so its presence or absence never contributes to this
	// field.
	WeftContent string
	GitExclude  string // "reverted" or "unchanged"
	Gitignore   string // "reverted" or "unchanged"
}

// Unwire reverses every host junction wired for the worktree at cwd, clears
// the weft-side _lyx content, and reverts the managed .gitignore block's
// ".lyx/" entry — a full per-host-worktree deactivation, distinct from
// Reconcile's declarative config-convergence.
//
// Unlike the deleted initengine.Undo, the junction name-set enumerated for
// removal comes from a full on-disk scan (scanOnDiskJunctionNames), not a
// config-loaded name-set: this removes every fabric junction present on
// disk, including a stale one absent from the current repo-wide pathspec.
//
// Unwire is per-host-worktree ONLY: it never touches the repo-wide
// `weft:main` records (`.fabric-anchor`, `<BoardDir>/_lyx/config/fabric.yaml`)
// — those are per-repo facts a later `lyx fabric reconcile` re-wire still
// needs to recreate this worktree's wiring.
//
// Steps:
//  1. Resolve cwd and layout; derive slug from the worktree root.
//  2. Enumerate junctions via the on-disk scan and unwire them (and their
//     shared .git/info/exclude entries) via UnwireJunctions. Any junction
//     inconsistency aborts before the weft/gitignore steps run.
//  3. Clear weft-side _lyx content ONLY (never _pattern), then unconditionally
//     commit and push that deletion through fabricengine's weft-git choke
//     point.
//  4. Revert the managed .gitignore block's ".lyx/" entry.
func Unwire(cwd string) (UnwireVerbResult, error) {
	// Resolve layout from cwd (needed for weft sibling derivation and slug).
	l, err := hubgeometry.Resolve(cwd)
	if err != nil {
		return UnwireVerbResult{}, err
	}

	slug := filepath.Base(l.WorktreeRoot)

	// Enumerate every fabric junction actually present on disk — not the
	// config-driven name-set — so a stale junction absent from the current
	// pathspec is removed too, not left behind.
	names, err := scanOnDiskJunctionNames(l.WorktreeRoot, l.RelPath)
	if err != nil {
		return UnwireVerbResult{}, err
	}

	junctionResult, err := UnwireJunctions(l, slug, names)
	if err != nil {
		return UnwireVerbResult{}, err
	}

	var result UnwireVerbResult

	// Weft-side _lyx content ONLY — weft _pattern content is deliberately
	// never touched here or anywhere else in Unwire; see the file-level
	// comment for why. First check whether a weft worktree exists at all;
	// if it doesn't, this is a truly-unpaired host and every remaining part
	// of this step is skipped.
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
			// The weft worktree exists but _lyx was never materialized here
			// — the raw-adopted/dormant pair reconcile.go's
			// createDormantWeftForRawHost leaves behind. Nothing to clear.
			result.WeftContent = "not_present"
		} else {
			if err := os.RemoveAll(weftLyxDir); err != nil {
				return UnwireVerbResult{}, err
			}
			result.WeftContent = "cleared"
		}

		// Regardless of whether weftLyxDir existed this invocation, commit
		// and push once we already know the weft worktree itself exists —
		// this recovers a prior partial run where the deletion committed
		// locally but the push failed.
		opts := EnvSyncOptions()
		f, err := New(l.WorktreeRoot, weftWorktree)
		if err != nil {
			return UnwireVerbResult{}, err
		}
		pathspec := ScopedPathspec(l.RelPath, []string{hubgeometry.LyxDirName})
		if _, _, err := f.commitWeft(pathspec, "lyx fabric unwire: clear _lyx", opts); err != nil {
			return UnwireVerbResult{}, err
		}
		// Push runs unconditionally, never gated on whether commitWeft made
		// a new commit this invocation.
		if err := pushWeftAt(weftWorktree, opts); err != nil {
			return UnwireVerbResult{}, err
		}
	}

	// Revert the managed .gitignore block's ".lyx/" entry.
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
