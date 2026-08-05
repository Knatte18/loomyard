// prune.go implements the Prune verb: it identifies and optionally removes
// orphaned or stale host↔weft pairs. A pair is stale when the host worktree
// directory no longer exists; a pair is orphaned when a weft worktree has no
// corresponding host worktree sibling. Prune operates purely on directory
// names (<slug>-weft, a weftname-level invariant); fabric's branch-naming
// scheme does not affect this file.

package fabricengine

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

// PruneEntry describes one stale or orphaned pair that Prune has identified.
type PruneEntry struct {
	// HostWorktree is the absolute path to the (missing or absent) host worktree.
	HostWorktree string `json:"host_worktree"`
	// WeftWorktree is the absolute path to the weft worktree sibling.
	WeftWorktree string `json:"weft_worktree"`
	// Reason describes why this pair was flagged for pruning.
	Reason string `json:"reason"`
	// Removed reports whether the weft worktree was actually deleted.
	// It is false on a dry run and true only when apply is true and removal succeeded.
	Removed bool `json:"removed"`
	// Error is non-empty when apply is true and removal of this entry failed.
	Error string `json:"error,omitempty"`
}

// PruneResult is the top-level result type returned by Prune.
// It lists every stale or orphaned pair, whether or not they were removed.
type PruneResult struct {
	// Entries lists the pairs that were identified (and optionally removed).
	Entries []PruneEntry `json:"entries"`
}

// Prune identifies stale or orphaned host↔weft pairs and removes their stale
// weft worktrees and associated portal/launcher directories when apply is true.
// Per-entry removal errors are recorded in PruneEntry.Error.
func (t *Topology) Prune(l *lyxcwd.Location, apply bool) (PruneResult, error) {
	entries, err := List(l.WorktreePath())
	if err != nil {
		return PruneResult{}, fmt.Errorf("list worktrees: %w", err)
	}

	liveHostSlugs := make(map[string]bool)

	// Track slugs emitted by Pass 1 to avoid re-reporting the same orphaned weft in Pass 2.
	pass1Slugs := make(map[string]bool)

	var result PruneResult
	for _, entry := range entries {
		hostPath := filepath.FromSlash(entry.Path)
		hostPath = filepath.Clean(hostPath)
		slug := filepath.Base(hostPath)

		weftPath := l.WeftWorktreePath(slug)

		_, hostStatErr := os.Stat(hostPath)
		hostMissing := hostStatErr != nil

		if hostMissing {
			pe := PruneEntry{
				HostWorktree: filepath.ToSlash(hostPath),
				WeftWorktree: filepath.ToSlash(weftPath),
				Reason:       "host worktree directory missing",
			}

			if apply {
				pe.Removed = removeStalePair(l, slug, weftPath, &pe)
			}

			pass1Slugs[slug] = true
			result.Entries = append(result.Entries, pe)
		} else {
			liveHostSlugs[slug] = true
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

		hostSlug, ok := lyxcwd.WeftHostSlug(name)
		if !ok {
			continue
		}

		if liveHostSlugs[hostSlug] || pass1Slugs[hostSlug] {
			continue
		}

		weftPath := filepath.Join(l.HubPath, name)
		hostPath := filepath.Join(l.HubPath, hostSlug)

		pe := PruneEntry{
			HostWorktree: filepath.ToSlash(hostPath),
			WeftWorktree: filepath.ToSlash(weftPath),
			Reason:       "weft worktree has no host sibling",
		}

		if apply {
			pe.Removed = removeStalePair(l, hostSlug, weftPath, &pe)
		}

		result.Entries = append(result.Entries, pe)
	}

	return result, nil
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
