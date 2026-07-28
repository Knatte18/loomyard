// undo.go implements the core logic for lyx init --undo — the reversal of Init.
//
// Undo reverses exactly what Init wires: every host junction, the weft-side
// _lyx content, the managed .gitignore block, and the .git/info/exclude
// entries. Each step independently no-ops if its own target is already absent,
// and a junction inconsistency aborts the whole run before any weft-content or
// .gitignore step runs (see fabricengine.UnwireJunctions).
//
// Weft _pattern content is deliberately NEVER touched by Undo — see Undo's
// godoc for why.

package initengine

import (
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitignore"
	"github.com/Knatte18/loomyard/internal/hubgeometry"
)

// UndoResult summarizes what Undo changed.
type UndoResult struct {
	// JunctionsRemoved lists the Name of each host junction that was actually
	// present and removed, carrying fabricengine.UnwireResult.JunctionsRemoved
	// through unchanged. Empty when no junction was wired.
	JunctionsRemoved []string
	// WeftContent describes _lyx only — "cleared" or "not_present" — and NEVER
	// _pattern: weft _pattern content is preserved by design (see Undo's
	// godoc), so its presence or absence never contributes to this field.
	WeftContent string
	GitExclude  string // "reverted" or "unchanged"
	Gitignore   string // "reverted" or "unchanged"
}

// Undo reverses Init's scaffolding in this order:
//  1. Resolve cwd and layout (identical error handling to Init; unlike
//     Init there is no "no weft pairing" pre-gate — each step below
//     independently no-ops when its own target is absent).
//  2. Derive slug from the worktree root (identical to Init).
//  3. Unwire every host junction (both _lyx and _pattern) and their shared
//     .git/info/exclude entries via fabricengine.UnwireJunctions. Any error
//     here aborts immediately: no weft-content clearing or .gitignore
//     revert runs.
//  4. Clear weft-side _lyx content ONLY, if any weft worktree exists at all,
//     then unconditionally commit and push that deletion through
//     fabricengine. Weft _pattern content is deliberately NEVER cleared,
//     committed, or pushed by this step, or by any other step of Undo.
//  5. Revert the managed .gitignore block's ".lyx/" entry.
//
// The _lyx/_pattern asymmetry in step 4 is deliberate, not an oversight: step
// 4 does os.RemoveAll, commits, and PUSHES the deletion, which is correct for
// _lyx — lyx's own runtime state, owned entirely by fabric — and would be
// badly wrong for _pattern, which holds the host repo's own hand-authored
// constraint-injection content (PATTERN.md). Deactivating lyx must not
// destroy the repo's own invariants and push that destruction to the remote,
// where it cannot be casually undone. So while step 3 unwires BOTH junctions
// (a junction is fabric-owned wiring metadata, never user content — removing
// it is always safe, for either directory), step 4's RemoveAll/commit/push
// sequence names only hubgeometry.LyxDirName: the os.RemoveAll target stays
// l.WeftLyxDirFor(slug), the commit pathspec stays
// fabricengine.ScopedPathspec(l.RelPath, []string{hubgeometry.LyxDirName}),
// and the commit message stays _lyx-scoped. No _pattern equivalent exists,
// and none should be added.
func Undo(cwd string) (UndoResult, error) {
	// Resolve layout from cwd (needed for weft sibling derivation and slug).
	l, err := hubgeometry.Resolve(cwd)
	if err != nil {
		// hubgeometry.Resolve's error is already self-describing; pass it
		// through bare rather than restating it with a redundant prefix.
		return UndoResult{}, err
	}

	slug := filepath.Base(l.WorktreeRoot)

	// Step 3: unwire every host junction (both _lyx and _pattern) and their
	// exclude entries. Per the "any junction inconsistency is a hard error"
	// Shared Decision, any error here aborts the whole run: no weft-content
	// or .gitignore step runs.
	junctionResult, err := fabricengine.UnwireJunctions(l, slug)
	if err != nil {
		return UndoResult{}, err
	}

	var result UndoResult

	// Step 4: weft-side _lyx content ONLY — weft _pattern content is
	// deliberately never touched here or anywhere else in Undo; see Undo's
	// godoc for why. First check whether a weft worktree exists at all; if
	// it doesn't, this is a truly-unpaired host (the same condition Init
	// hard-gates on) and every remaining part of this step is skipped — in
	// particular, fabricengine's CommitWeft must never be called against a
	// nonexistent weft worktree, since its ensureLockDir would
	// unconditionally create a stray <slug>-weft/.weft/ directory tree.
	weftWorktree := l.WeftWorktree()
	if _, statErr := os.Stat(weftWorktree); statErr != nil && !os.IsNotExist(statErr) {
		return UndoResult{}, statErr
	} else if os.IsNotExist(statErr) {
		result.WeftContent = "not_present"
	} else {
		weftLyxDir := l.WeftLyxDirFor(slug)
		if _, statErr := os.Stat(weftLyxDir); statErr != nil && !os.IsNotExist(statErr) {
			return UndoResult{}, statErr
		} else if os.IsNotExist(statErr) {
			result.WeftContent = "not_present"
		} else {
			if err := os.RemoveAll(weftLyxDir); err != nil {
				return UndoResult{}, err
			}
			result.WeftContent = "cleared"
		}

		// Regardless of whether weftLyxDir existed this invocation, commit
		// and push once we already know the weft worktree itself exists —
		// this recovers a prior partial run where the deletion committed
		// locally but the push failed.
		opts := fabricengine.EnvSyncOptions()
		f, err := fabricengine.New(l.WorktreeRoot, weftWorktree)
		if err != nil {
			return UndoResult{}, err
		}
		pathspec := fabricengine.ScopedPathspec(l.RelPath, []string{hubgeometry.LyxDirName})
		if _, _, err := f.CommitWeft(pathspec, "lyx init --undo: clear _lyx", opts); err != nil {
			return UndoResult{}, err
		}
		// Push runs unconditionally, never gated on whether CommitWeft made a
		// new commit this invocation — see the "Push runs unconditionally"
		// Shared Decision.
		if err := fabricengine.PushWeftAt(weftWorktree, opts); err != nil {
			return UndoResult{}, err
		}
	}

	// Step 5: revert the managed .gitignore block's ".lyx/" entry.
	changed, err := gitignore.Remove(cwd, ".lyx/")
	if err != nil {
		return UndoResult{}, err
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
