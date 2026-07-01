// prune.go implements the Prune verb: it identifies and optionally removes
// orphaned or stale host↔weft pairs. A pair is stale when the host worktree
// directory no longer exists; a pair is orphaned when a weft worktree has no
// corresponding host worktree sibling.

package warpengine

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/hubgeometry"
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

// Prune identifies stale or orphaned host↔weft pairs and, when apply is true,
// removes the stale weft worktrees.
//
// A pair is considered stale/orphaned when either:
//   - A registered host worktree's directory no longer exists on disk.
//   - A weft worktree (named <slug>-weft) exists in the hub without a corresponding
//     host worktree sibling (<slug>).
//
// When apply is false, Prune reports the pairs that would be pruned without making
// any changes. When apply is true, Prune removes each stale weft worktree directory
// via git worktree remove --force and then runs git worktree prune on the weft repo
// to clean up stale administrative refs.
//
// Live pairs (both host and weft directories exist and are registered) are never
// touched. The board repo is excluded entirely — Prune only considers host worktrees
// discovered via hubgeometry.List.
//
// Returns PruneResult on success or an error on fatal system failures. Per-entry
// removal errors are recorded inline in PruneEntry.Error.
func (w *Worktree) Prune(l *hubgeometry.Layout, apply bool) (PruneResult, error) {
	// Enumerate all registered host worktrees from the repository.
	entries, err := hubgeometry.List(l.WorktreeRoot)
	if err != nil {
		return PruneResult{}, fmt.Errorf("list worktrees: %w", err)
	}

	// Build a set of host slugs that currently have an existing host directory.
	// We use this below when scanning for orphaned weft worktrees.
	liveHostSlugs := make(map[string]bool)

	var result PruneResult

	// Pass 1: find registered host worktrees whose directory is missing.
	// These correspond to stale git worktree registrations where the actual
	// directory was deleted outside of `git worktree remove`.
	for _, entry := range entries {
		hostPath := filepath.FromSlash(entry.Path)
		hostPath = filepath.Clean(hostPath)
		slug := filepath.Base(hostPath)

		weftPath := l.WeftWorktreePath(slug)

		// Check whether the host worktree directory actually exists on disk.
		_, hostStatErr := os.Stat(hostPath)
		hostMissing := hostStatErr != nil

		if hostMissing {
			// The host worktree is registered in git but the directory is gone.
			// Its paired weft worktree (if any) is now orphaned.
			// Emit forward-slash paths in the JSON-tagged fields only; hostPath/weftPath
			// stay OS-native below for os.Stat and the removeStalePair git operations.
			pe := PruneEntry{
				HostWorktree: filepath.ToSlash(hostPath),
				WeftWorktree: filepath.ToSlash(weftPath),
				Reason:       "host worktree directory missing",
			}

			if apply {
				pe.Removed = removeStalePair(l, weftPath, &pe)
			}

			result.Entries = append(result.Entries, pe)
		} else {
			// Record this slug as having a live host directory so we can skip it
			// in the orphaned-weft scan below.
			liveHostSlugs[slug] = true
		}
	}

	// Pass 2: scan the hub for weft worktrees that have no corresponding host sibling.
	// A weft worktree is named <slug>-weft; its host sibling would be <slug>.
	// If <slug> is not in liveHostSlugs (i.e. the host directory is absent), it is orphaned.
	hubEntries, err := os.ReadDir(l.Hub)
	if err != nil {
		// A missing or unreadable hub is a fatal error; we cannot scan for orphans.
		return PruneResult{}, fmt.Errorf("read hub directory: %w", err)
	}

	for _, dirEntry := range hubEntries {
		if !dirEntry.IsDir() {
			continue
		}

		name := dirEntry.Name()

		// Only consider directories that follow the <slug>-weft naming convention.
		// WeftHostSlug rejects entries that are not valid weft names (wrong suffix or
		// empty slug after stripping), matching the skip semantics of the old guard.
		hostSlug, ok := hubgeometry.WeftHostSlug(name)
		if !ok {
			continue
		}

		// Skip if a live host worktree exists for this slug — the pair is healthy.
		if liveHostSlugs[hostSlug] {
			continue
		}

		// The weft worktree exists but there is no live host sibling; it is orphaned.
		weftPath := filepath.Join(l.Hub, name)
		hostPath := filepath.Join(l.Hub, hostSlug)

		// Emit forward-slash paths in the JSON-tagged fields only; hostPath/weftPath
		// stay OS-native below for the removeStalePair git operations.
		pe := PruneEntry{
			HostWorktree: filepath.ToSlash(hostPath),
			WeftWorktree: filepath.ToSlash(weftPath),
			Reason:       "weft worktree has no host sibling",
		}

		if apply {
			pe.Removed = removeStalePair(l, weftPath, &pe)
		}

		result.Entries = append(result.Entries, pe)
	}

	return result, nil
}

// removeStalePair removes the stale weft worktree at weftPath and runs
// git worktree prune on the weft repo to clean up administrative state.
//
// It writes any removal error into pe.Error and returns true only when
// the removal completed without error. The caller has already set pe fields
// other than Removed and Error.
func removeStalePair(l *hubgeometry.Layout, weftPath string, pe *PruneEntry) bool {
	// Attempt to remove via git worktree remove --force. We use --force because
	// the host is already gone so the weft may have been left in a dirty state.
	_, _, exitCode, err := gitexec.RunGit(
		[]string{"worktree", "remove", "--force", weftPath},
		l.WeftRepoRoot(),
	)
	if err != nil {
		pe.Error = fmt.Sprintf("git worktree remove: %v", err)
		return false
	}
	if exitCode != 0 {
		// git worktree remove --force failed; fall back to os.RemoveAll.
		if removeErr := os.RemoveAll(weftPath); removeErr != nil {
			pe.Error = fmt.Sprintf("remove weft worktree %q failed (git exit %d); fallback cleanup also failed: %v", weftPath, exitCode, removeErr)
			return false
		}
	}

	// Prune stale administrative refs in the weft repo so git worktree list is clean.
	// A non-zero exit here is non-fatal — the worktree directory is already gone.
	gitexec.RunGit([]string{"worktree", "prune"}, l.WeftRepoRoot()) //nolint:errcheck

	// Also prune on the host repo so the host's worktree refs are cleaned up.
	gitexec.RunGit([]string{"worktree", "prune"}, l.WorktreeRoot) //nolint:errcheck

	return true
}
