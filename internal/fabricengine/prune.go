// prune.go implements the Prune verb: it identifies and optionally removes orphaned or stale
// warp↔weft pairs.
// A pair is stale when the warp worktree directory no longer exists;
// a pair is orphaned when a weft worktree has no corresponding warp worktree sibling.
// Prune operates purely on directory names (<slug>-weft, a weftname-level invariant);
// fabric's branch-naming scheme does not affect this file.
//
// The weft removal is a `git worktree remove --force`, so a weft worktree carrying uncommitted
// TRACKED changes is protected unless the caller passes force — the same refuse-then-offer-force
// posture Remove takes, rather than discarding the work silently.
//
// Ownership is checked BEFORE either mode acts, and force does not bypass it.
// Prune's orphan pass enumerates by directory NAME alone — any hub child ending in the weft suffix —
// so the set it reports is not the set it owns: an ordinary `<hub>/notes-weft/` directory, or a
// wholly unrelated git clone parked at `<hub>/proj-weft/`, both land in it.
// A path fabric may delete is one the hub's weft repo registers as a LINKED worktree, and nothing
// else;
// everything else is reported Unowned and left alone, in both modes.
// This is the same rule removeWarpWorktreeDir applies on the warp side, and it exists for the same
// reason: `git worktree remove` refusing a path is not licence to delete it.
//
// The verdict is computed identically in both modes, so a dry run's Protected and Unowned flags
// match exactly what the same flags plus --apply would do.

package fabricengine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

// PruneEntry describes one stale or orphaned pair that Prune has identified.
type PruneEntry struct {
	// WarpWorktree is the absolute path to the (missing or absent) warp worktree.
	WarpWorktree string `json:"warp_worktree"`
	// WeftWorktree is the absolute path to the weft worktree sibling.
	WeftWorktree string `json:"weft_worktree"`
	// Reason describes why this pair was flagged for pruning.
	Reason string `json:"reason"`
	// Removed reports whether the weft worktree was actually deleted.
	// It is false on a dry run and true only when apply is true and removal succeeded.
	Removed bool `json:"removed"`
	// Protected reports whether this entry's weft worktree carries uncommitted tracked changes
	// that force was not given to discard.
	// It is computed in both modes, so a dry run answers the question --apply would act on.
	Protected bool `json:"protected,omitempty"`
	// Unowned reports that the enumerated path is not a linked worktree of this hub's weft repo,
	// so fabric will not remove it in any mode — force included.
	// The orphan pass enumerates on directory NAME alone, so an ordinary user directory or an
	// unrelated git clone whose name happens to end in the weft suffix is reported here rather
	// than deleted.
	// Like Protected, it is computed in both modes.
	Unowned bool `json:"unowned,omitempty"`
	// Error is non-empty when this entry was not removed and the operator needs to know why:
	// a removal that failed, a protected entry force would have removed, or an unowned path
	// fabric refuses to touch.
	Error string `json:"error,omitempty"`
}

// PruneResult is the top-level result type returned by Prune.
// It lists every stale or orphaned pair, whether or not they were removed.
type PruneResult struct {
	// Entries lists the pairs that were identified (and optionally removed).
	Entries []PruneEntry `json:"entries"`
}

// Prune identifies stale or orphaned warp↔weft pairs and removes their stale weft worktrees and
// associated portal/launcher directories when apply is true.
// A weft worktree carrying uncommitted tracked changes is protected unless force is true, since the
// removal is a forced one that would discard them without a trace.
// Per-entry removal errors and protection reasons are recorded in PruneEntry.Error.
func (t *Topology) Prune(l *lyxcwd.Location, apply, force bool) (PruneResult, error) {
	entries, err := List(l.WorktreePath())
	if err != nil {
		return PruneResult{}, fmt.Errorf("list worktrees: %w", err)
	}

	liveWarpSlugs := make(map[string]bool)

	// Track slugs emitted by Pass 1 to avoid re-reporting the same orphaned weft in Pass 2.
	pass1Slugs := make(map[string]bool)

	var result PruneResult
	for _, entry := range entries {
		warpPath := filepath.FromSlash(entry.Path)
		warpPath = filepath.Clean(warpPath)
		slug := filepath.Base(warpPath)

		weftPath := WeftWorktreePath(l, slug)

		_, warpStatErr := os.Stat(warpPath)
		warpMissing := warpStatErr != nil

		if warpMissing {
			pe := PruneEntry{
				WarpWorktree: filepath.ToSlash(warpPath),
				WeftWorktree: filepath.ToSlash(weftPath),
				Reason:       "warp worktree directory missing",
			}

			applyStalePairOwnership(l, weftPath, &pe)
			applyStalePairProtection(weftPath, force, &pe)
			if apply && !pe.Protected && !pe.Unowned {
				pe.Removed = removeStalePair(l, slug, weftPath, &pe)
			}

			pass1Slugs[slug] = true
			result.Entries = append(result.Entries, pe)
		} else {
			liveWarpSlugs[slug] = true
		}
	}
	hubEntries, err := os.ReadDir(l.HubPath)
	if err != nil {
		// A missing or unreadable hub is a fatal error; we cannot scan for orphans.
		return PruneResult{}, fmt.Errorf("read hub directory: %w", err)
	}

	for _, dirEntry := range hubEntries {
		if !dirEntry.IsDir() {
			continue
		}

		name := dirEntry.Name()

		warpSlug, ok := WeftWarpSlug(name)
		if !ok {
			continue
		}

		if liveWarpSlugs[warpSlug] || pass1Slugs[warpSlug] {
			continue
		}

		weftPath := filepath.Join(l.HubPath, name)
		warpPath := filepath.Join(l.HubPath, warpSlug)

		pe := PruneEntry{
			WarpWorktree: filepath.ToSlash(warpPath),
			WeftWorktree: filepath.ToSlash(weftPath),
			Reason:       "weft worktree has no warp sibling",
		}

		applyStalePairOwnership(l, weftPath, &pe)
		applyStalePairProtection(weftPath, force, &pe)
		if apply && !pe.Protected && !pe.Unowned {
			pe.Removed = removeStalePair(l, warpSlug, weftPath, &pe)
		}

		result.Entries = append(result.Entries, pe)
	}

	return result, nil
}

// applyStalePairOwnership marks pe unowned unless weftPath is registered as a LINKED worktree of
// this hub's weft repo.
//
// It runs in BOTH modes and force does NOT bypass it, because it does not answer "is there work
// here worth keeping" — it answers "is this fabric's to delete at all", and no flag can make a
// directory fabric never created become fabric's.
// The orphan pass reaches this function with a path chosen by directory name alone
// (WeftWarpSlug over every hub child), so `<hub>/notes-weft/` holding an operator's own notes, and
// a separate git clone parked at `<hub>/proj-weft/`, both arrive here indistinguishable from a real
// stale pair until this check tells them apart.
//
// An absent path is NOT unowned: there is nothing to delete, and removeStalePair still has portal,
// launcher, and worktree-registration debris to clear for a pair whose weft worktree is already
// gone.
func applyStalePairOwnership(l *lyxcwd.Location, weftPath string, pe *PruneEntry) {
	if _, statErr := os.Stat(weftPath); os.IsNotExist(statErr) {
		return
	}

	weftRepoRoot, err := WeftRepoRoot(l)
	if err != nil {
		pe.Unowned = true
		pe.Error = fmt.Sprintf("cannot resolve this hub's weft repo to confirm %q is one of its worktrees (%v); refusing to remove it", weftPath, err)
		return
	}

	if isRegisteredLinkedWorktreeIn(weftRepoRoot, weftPath) {
		return
	}

	pe.Unowned = true
	pe.Error = fmt.Sprintf(
		"%q is not a linked worktree of this hub's weft repo at %s; its name merely ends in the weft suffix, so fabric will not remove it — delete it yourself if it really is debris",
		weftPath, weftRepoRoot)
}

// applyStalePairProtection marks pe protected when its weft worktree carries uncommitted TRACKED
// changes and force was not given.
//
// The probe is tracked-only on purpose. `git worktree remove --force` discards tracked
// modifications with no trace, which is the data loss this gate exists to stop;
// untracked files are the ordinary residue of an abandoned pair, and refusing on them would protect
// nothing while making prune useless on exactly the debris it exists to clear.
// A probe that cannot run at all leaves the entry unprotected: the weft worktree is then not a
// readable git checkout, so there is no tracked work in it to lose.
func applyStalePairProtection(weftPath string, force bool, pe *PruneEntry) {
	if force {
		return
	}
	if _, statErr := os.Stat(weftPath); statErr != nil {
		return
	}

	dirty, _, err := worktreeDirty(scopeTracked, weftPath)
	if err != nil {
		return
	}
	if !dirty {
		return
	}

	pe.Protected = true
	pe.Error = "weft worktree has uncommitted changes; commit them or re-run with --force to discard them"
}

// removeStalePair removes the stale weft worktree at weftPath (when it exists),
// tears down the dead slug's portal junction and launcher directory, and prunes
// administrative state on both repos. Errors are recorded in pe.Error; it returns
// true only when a weft worktree existed and was removed without error.
//
// It is reached only for an entry applyStalePairOwnership has already cleared, so weftPath is
// either absent or a registered linked worktree of this hub's weft repo.
// The directory-removal fallback below rests on exactly that: it fires only after git declined to
// remove a path git itself still registers as a worktree, which is recoverable bookkeeping rather
// than data loss — the identical rule, and the identical justification, as
// removeWarpWorktreeDir's.
//
// Portal and launcher teardown runs AFTER the ownership gate rather than before it, because the
// slug they are keyed on is derived from a directory name the orphan pass chose: tearing them down
// first meant a stray `<hub>/my-task-weft/` directory removed the LIVE `my-task` pair's portal
// junction and launcher directory before anything had established the entry was fabric's at all.
func removeStalePair(l *lyxcwd.Location, slug, weftPath string, pe *PruneEntry) bool {
	weftRepoRoot, weftRepoRootErr := WeftRepoRoot(l)
	if weftRepoRootErr != nil {
		pe.Error = fmt.Sprintf("resolve weft repo root: %v", weftRepoRootErr)
		return false
	}

	// The portal and launcher teardown here is keyed on a slug the orphan pass derived from a
	// directory name — precisely the input a refusal is most likely to be about — so a refusal must
	// be recorded rather than swallowed alongside an ordinary operational failure.
	if err := surfaceRefusal(removePortal(l, slug)); err != nil {
		pe.Error = err.Error()
		return false
	}
	if err := surfaceRefusal(removeLaunchers(l, slug)); err != nil {
		pe.Error = err.Error()
		return false
	}

	removed := false

	if _, statErr := os.Stat(weftPath); statErr == nil {
		req := pathRequest{
			what:      "remove weft worktree",
			container: l.HubPath,
			target:    weftPath,
			ownership: ownedRegisteredLinkedWorktree(weftRepoRoot),
			dirtiness: dirtyScopeTracked(),
			force:     true,
		}
		exitCode, stderr, err := removeGitWorktree(req, weftRepoRoot)
		if err != nil {
			pe.Error = fmt.Sprintf("git worktree remove: %v", err)
			return false
		}
		if exitCode != 0 {
			if !isRegisteredLinkedWorktreeIn(weftRepoRoot, weftPath) {
				// The registration vanished between the ownership gate and here (a concurrent
				// prune, an external `git worktree prune`). Report git's own reason and delete
				// nothing: an unregistered path is never fabric's to remove.
				pe.Error = fmt.Sprintf(
					"git refused to remove weft worktree %q (git exit %d): %s; it is no longer a linked worktree of %s, so fabric will not delete the directory itself",
					weftPath, exitCode, strings.TrimSpace(stderr), weftRepoRoot)
				return false
			}
			fallbackReq := pathRequest{
				what:      "remove weft worktree",
				container: l.HubPath,
				target:    weftPath,
				ownership: ownedRegisteredLinkedWorktree(weftRepoRoot),
				dirtiness: dirtyScopeTracked(),
			}
			if removeErr := removePath(fallbackReq); removeErr != nil {
				pe.Error = fmt.Sprintf("remove weft worktree %q failed (git exit %d); fallback cleanup also failed: %v", weftPath, exitCode, removeErr)
				return false
			}
		}
		removed = true
	}

	gitexec.RunGit([]string{"worktree", "prune"}, weftRepoRoot)     //nolint:errcheck
	gitexec.RunGit([]string{"worktree", "prune"}, l.WorktreePath()) //nolint:errcheck

	return removed
}
