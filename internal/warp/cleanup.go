// cleanup.go implements the Cleanup verb: it finds weft branches that have no
// corresponding host worktree sibling and deletes them according to a flag matrix.
//
// Flag matrix:
//   - apply == false                → dry-run/report only; nothing is deleted.
//   - apply == true && !force       → delete non-gate-protected orphan branches;
//     task branches where codeguideFoldedBack returns false are skipped (protected).
//   - apply == true && force == true → also delete gate-protected task branches.
//   - force == true && !apply       → report only (force does not imply apply).
//
// The board repo is excluded entirely — Cleanup only enumerates weft branches
// and compares them against the set of known host worktree slugs.

package warp

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/paths"
)

// CleanupBranchEntry describes the fate of one orphaned weft branch under Cleanup.
type CleanupBranchEntry struct {
	// Branch is the weft branch name.
	Branch string `json:"branch"`
	// Deleted reports whether the branch was actually deleted from the weft repo.
	// It is false on a dry run, when the entry was skipped due to gate protection,
	// and when deletion itself failed.
	Deleted bool `json:"deleted"`
	// Protected reports whether the branch was skipped because codeguideFoldedBack
	// returned false and force was not set.
	Protected bool `json:"protected,omitempty"`
	// Error is non-empty when apply is true and branch deletion failed.
	Error string `json:"error,omitempty"`
}

// CleanupResult is the top-level result type returned by Cleanup.
// It lists every orphaned weft branch, whether deleted, protected, or reported only.
type CleanupResult struct {
	// Entries lists the orphaned weft branches and their dispositions.
	Entries []CleanupBranchEntry `json:"entries"`
}

// codeguideFoldedBack reports whether the weft branch's _codeguide content has
// been squash-merged back into the host branch and is therefore safe to delete.
//
// This is the extension point for the real _codeguide merge-back check.
// Until that check exists, this function conservatively returns false for any
// branch that looks like a task branch (i.e. all branches), so they are protected
// from deletion unless --force is specified.
//
// When the real merge-back check is implemented, replace the body of this function
// with the actual verification logic (e.g. check whether the branch's _codeguide
// commit tree has been merged into the host branch's history).
func codeguideFoldedBack(_ string) bool {
	// Conservative: always return false until the real check is implemented.
	// Task branches are therefore always gate-protected unless --force is set.
	return false
}

// Cleanup finds weft branches in the weft repo that have no corresponding host
// worktree sibling and, according to the flag matrix, reports or deletes them.
//
// Orphaned weft branches are identified by comparing all weft branch names against
// the set of host worktree slugs enumerated via paths.List. The board repo branch
// namespace is excluded — only the weft repo's branches are examined.
//
// The flag matrix governs deletion:
//   - apply == false             → report all orphaned branches; delete nothing.
//   - apply && !force            → delete orphans where codeguideFoldedBack returns true;
//     skip (mark protected) those where it returns false.
//   - apply && force             → delete all orphaned branches regardless of the gate.
//   - force && !apply            → report only; force does not imply apply.
//
// Returns CleanupResult on success or an error on fatal system failures. Per-branch
// deletion errors are recorded inline in CleanupBranchEntry.Error.
func (w *Worktree) Cleanup(l *paths.Layout, apply, force bool) (CleanupResult, error) {
	// Enumerate host worktrees to build the set of known host slugs.
	// We use paths.List rather than scanning the hub directory so we only consider
	// git-registered worktrees, not arbitrary directories.
	entries, err := paths.List(l.WorktreeRoot)
	if err != nil {
		return CleanupResult{}, fmt.Errorf("list host worktrees: %w", err)
	}

	// Build a set of slug names for all currently registered host worktrees.
	// The slug is the base name of the worktree path (e.g. "my-task" for /hub/my-task).
	hostSlugs := make(map[string]bool, len(entries))
	for _, entry := range entries {
		hostPath := filepath.FromSlash(entry.Path)
		hostPath = filepath.Clean(hostPath)
		hostSlugs[filepath.Base(hostPath)] = true
	}

	// Enumerate all branches in the weft repo to find orphans.
	weftBranches, err := listWeftBranches(l)
	if err != nil {
		return CleanupResult{}, fmt.Errorf("list weft branches: %w", err)
	}

	var result CleanupResult

	for _, branch := range weftBranches {
		// A weft branch is orphaned when there is no host worktree with a matching slug.
		// The weft branch name is BranchPrefix+slug (set during warp add), so we must
		// strip the prefix before the hostSlugs lookup; without trimming, any non-empty
		// BranchPrefix makes every live weft branch appear as an orphan.
		slug := strings.TrimPrefix(branch, w.cfg.BranchPrefix)
		if hostSlugs[slug] {
			// The host worktree exists for this branch; this is a live pair, skip it.
			continue
		}

		entry := CleanupBranchEntry{
			Branch: branch,
		}

		if !apply {
			// Dry-run: report the orphaned branch without deleting it.
			result.Entries = append(result.Entries, entry)
			continue
		}

		// apply is true: decide whether to delete based on gate and force flag.
		folded := codeguideFoldedBack(branch)

		if !folded && !force {
			// Gate-protected: _codeguide has not been folded back and --force was not set.
			// Skip deletion and mark as protected.
			entry.Protected = true
			result.Entries = append(result.Entries, entry)
			continue
		}

		// Either the gate passed or --force was set; delete the branch.
		entry.Deleted = deleteWeftBranch(l, branch, &entry)
		result.Entries = append(result.Entries, entry)
	}

	return result, nil
}

// listWeftBranches returns all branch names in the weft repo.
//
// It runs git branch --format=%(refname:short) in the weft repo root to get a
// clean, newline-delimited list of branch names with no decoration. Returns an
// error if the git command fails to spawn or exits non-zero.
func listWeftBranches(l *paths.Layout) ([]string, error) {
	out, stderr, exitCode, err := gitexec.RunGit(
		[]string{"branch", "--format=%(refname:short)"},
		l.WeftRepoRoot(),
	)
	if err != nil {
		return nil, fmt.Errorf("git branch: %w", err)
	}
	if exitCode != 0 {
		return nil, fmt.Errorf("git branch exited %d: %s", exitCode, stderr)
	}

	raw := strings.TrimSpace(out)
	if raw == "" {
		return nil, nil
	}

	return strings.Split(raw, "\n"), nil
}

// deleteWeftBranch deletes a single weft branch via git branch -D and records
// any error in entry.Error. Returns true only when the deletion succeeded.
func deleteWeftBranch(l *paths.Layout, branch string, entry *CleanupBranchEntry) bool {
	_, stderr, exitCode, err := gitexec.RunGit(
		[]string{"branch", "-D", branch},
		l.WeftRepoRoot(),
	)
	if err != nil {
		entry.Error = fmt.Sprintf("git branch -D %s: %v", branch, err)
		return false
	}
	if exitCode != 0 {
		entry.Error = fmt.Sprintf("git branch -D %s failed: %s", branch, stderr)
		return false
	}
	return true
}
