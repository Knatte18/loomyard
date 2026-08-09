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
// The verdict is computed identically in both modes, so a dry run's Protected flags match exactly
// what the same flags plus --apply would do.

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
	// Error is non-empty when this entry was not removed and the operator needs to know why:
	// a removal that failed, or a protected entry force would have removed.
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

			applyStalePairProtection(weftPath, force, &pe)
			if apply && !pe.Protected {
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

		applyStalePairProtection(weftPath, force, &pe)
		if apply && !pe.Protected {
			pe.Removed = removeStalePair(l, warpSlug, weftPath, &pe)
		}

		result.Entries = append(result.Entries, pe)
	}

	return result, nil
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

	stdout, _, exitCode, err := gitexec.RunGit(
		[]string{"status", "--porcelain", "--untracked-files=no"},
		weftPath,
	)
	if err != nil || exitCode != 0 {
		return
	}
	if strings.TrimSpace(stdout) == "" {
		return
	}

	pe.Protected = true
	pe.Error = "weft worktree has uncommitted changes; commit them or re-run with --force to discard them"
}

// removeStalePair removes the stale weft worktree at weftPath (when it exists),
// tears down the dead slug's portal junction and launcher directory, and prunes
// administrative state on both repos. Errors are recorded in pe.Error; it returns
// true only when a weft worktree existed and was removed without error.
func removeStalePair(l *lyxcwd.Location, slug, weftPath string, pe *PruneEntry) bool {
	_ = removePortal(l, slug)
	_ = removeLaunchers(l, slug)

	weftRepoRoot, weftRepoRootErr := WeftRepoRoot(l)
	if weftRepoRootErr != nil {
		pe.Error = fmt.Sprintf("resolve weft repo root: %v", weftRepoRootErr)
		return false
	}

	removed := false

	if _, statErr := os.Stat(weftPath); statErr == nil {
		_, _, exitCode, err := gitexec.RunGit(
			[]string{"worktree", "remove", "--force", weftPath},
			weftRepoRoot,
		)
		if err != nil {
			pe.Error = fmt.Sprintf("git worktree remove: %v", err)
			return false
		}
		if exitCode != 0 {
			if removeErr := os.RemoveAll(weftPath); removeErr != nil {
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
