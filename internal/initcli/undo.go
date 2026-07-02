// undo.go implements the lyx init --undo reversal path.
//
// runUndo reverses exactly what runInit wires: the host _lyx junction, the
// weft-side _lyx content, the managed .gitignore block, and the
// .git/info/exclude entry. Each step independently no-ops if its own target
// is already absent, and a junction inconsistency aborts the whole run before
// any weft-content or .gitignore step runs (see warpengine.UnwireJunctions).

package initcli

import (
	"io"
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/gitignore"
	"github.com/Knatte18/loomyard/internal/hubgeometry"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/warpengine"
	"github.com/Knatte18/loomyard/internal/weftengine"
)

// runUndo is the package-private handler for lyx init --undo.
//
// It reverses runInit's scaffolding in this order:
//  1. Resolve cwd and layout (identical error handling to runInit; unlike
//     runInit there is no "no weft pairing" pre-gate — each step below
//     independently no-ops when its own target is absent).
//  2. Derive slug from the worktree root (identical to runInit).
//  3. Unwire the host junction and its .git/info/exclude entry via
//     warpengine.UnwireJunctions. Any error here aborts immediately: no
//     weft-content clearing or .gitignore revert runs.
//  4. Clear weft-side _lyx content, if any weft worktree exists at all, then
//     unconditionally commit and push that deletion through weftengine.
//  5. Revert the managed .gitignore block's ".lyx/" entry.
//  6. Emit a JSON envelope summarizing what changed.
//
// Returns exit code 0 on success, 1 on error.
func runUndo(out io.Writer, args []string) int {
	// Resolve current working directory.
	cwd, err := hubgeometry.Getwd()
	if err != nil {
		return output.Err(out, "failed to get working directory: "+err.Error())
	}

	// Resolve layout from cwd (needed for weft sibling derivation and slug).
	l, err := hubgeometry.Resolve(cwd)
	if err != nil {
		// hubgeometry.Resolve's error is already self-describing; pass it
		// through bare rather than restating it with a redundant prefix.
		return output.Err(out, err.Error())
	}

	slug := filepath.Base(l.WorktreeRoot)

	// Step 3: unwire the host junction and its exclude entry. Per the "any
	// junction inconsistency is a hard error" Shared Decision, any error here
	// aborts the whole run: no weft-content or .gitignore step runs.
	result, err := warpengine.UnwireJunctions(l, slug)
	if err != nil {
		return output.Err(out, err.Error())
	}

	// Step 4: weft-side content. First check whether a weft worktree exists
	// at all; if it doesn't, this is a truly-unpaired host (the same
	// condition runInit hard-gates on) and every remaining part of this step
	// is skipped — in particular, weftengine.Commit must never be called
	// against a nonexistent weft worktree, since its ensureLockDir would
	// unconditionally create a stray <slug>-weft/.weft/ directory tree.
	var weftContentStatus string
	weftWorktree := l.WeftWorktree()
	if _, statErr := os.Stat(weftWorktree); statErr != nil && !os.IsNotExist(statErr) {
		return output.Err(out, statErr.Error())
	} else if os.IsNotExist(statErr) {
		weftContentStatus = "not_present"
	} else {
		weftLyxDir := l.WeftLyxDirFor(slug)
		if _, statErr := os.Stat(weftLyxDir); statErr != nil && !os.IsNotExist(statErr) {
			return output.Err(out, statErr.Error())
		} else if os.IsNotExist(statErr) {
			weftContentStatus = "not_present"
		} else {
			if err := os.RemoveAll(weftLyxDir); err != nil {
				return output.Err(out, err.Error())
			}
			weftContentStatus = "cleared"
		}

		// Regardless of whether weftLyxDir existed this invocation, commit
		// and push once we already know the weft worktree itself exists —
		// this recovers a prior partial run where the deletion committed
		// locally but the push failed.
		opts := weftengine.EnvSyncOptions()
		if _, err := weftengine.Commit(weftWorktree, weftengine.ScopedPathspec(l.RelPath, []string{hubgeometry.LyxDirName}), "lyx init --undo: clear _lyx", opts); err != nil {
			return output.Err(out, err.Error())
		}
		// Push runs unconditionally, never gated on whether Commit made a new
		// commit this invocation — see the "Push runs unconditionally" Shared
		// Decision.
		if err := weftengine.Push(weftWorktree, opts); err != nil {
			return output.Err(out, err.Error())
		}
	}

	// Step 5: revert the managed .gitignore block's ".lyx/" entry.
	changed, err := gitignore.Remove(cwd, ".lyx/")
	if err != nil {
		return output.Err(out, err.Error())
	}
	gitignoreStatus := "unchanged"
	if changed {
		gitignoreStatus = "reverted"
	}

	// Step 6: emit success JSON summarizing what changed.
	lyxJunctionStatus := "not_present"
	if result.JunctionRemoved {
		lyxJunctionStatus = "removed"
	}
	gitExcludeStatus := "unchanged"
	if result.ExcludeChanged {
		gitExcludeStatus = "reverted"
	}

	return output.Ok(out, map[string]any{
		"lyx_junction": lyxJunctionStatus,
		"weft_content": weftContentStatus,
		"git_exclude":  gitExcludeStatus,
		"gitignore":    gitignoreStatus,
	})
}
