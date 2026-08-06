// cleanup.go implements the Cleanup verb: it finds weft branches that have no
// corresponding host worktree sibling and deletes them according to a flag matrix.
//
// Flag matrix:
//   - apply == false                → dry-run/report only; nothing is deleted.
//   - apply == true && !force       → delete non-gate-protected orphan branches;
//     task branches where raddleFoldedBack returns false are skipped (protected).
//   - apply == true && force == true → also delete gate-protected task branches.
//   - force == true && !apply       → report only; force does not imply apply.
//
// A weft branch's host sibling is recovered via WeftHostSlug(branch) —
// inverting WeftBranchName's suffix. The weft repo may also hold non-suffixed weft
// branches inherited from history predating fabric's uniform naming scheme;
// WeftHostSlug rejects those (ok == false), and by definition a non-suffixed weft
// branch is not fabric-managed — it is reported but never deleted, matching the
// report-but-don't-touch rule Reconcile applies to unmanaged branches, rather than
// the raddle-fold-back gate.
//
// Liveness is judged in BRANCH space, not against worktree directory names: a weft
// branch <hostBranch>-weft is a live pair iff some host worktree is currently checked
// out on <hostBranch>. A weft branch that is itself checked out at some worktree is
// additionally always protected, whatever the liveness verdict says: git branch -D
// can never delete a checked-out branch, and a checked-out weft branch means the pair
// is still materialized on disk — e.g. a live pair whose host worktree sits on a
// detached HEAD, which branch-space liveness cannot see. Liveness-in-branch-space is
// the one point where fabric must diverge from warp's original logic. warp compared the weft branch's stripped slug against host worktree
// *directory* base names; that comparison is wrong for the primary pair, whose host
// worktree directory is the repo name (e.g. "lyx-fabric-test") while its branch is
// "main". Under warp the mistake is harmless because warp's primary weft branch is the
// unsuffixed "main" (WeftHostSlug rejects it → protected as unmanaged). Under fabric's
// uniform suffix scheme the primary weft branch is "main-weft", which WeftHostSlug
// accepts — so a directory-name comparison would misclassify it as a deletable orphan
// and delete the very branch board's reserved weft:main branch requires to stay
// permanent, distinct from it. Comparing
// against live host *branches* protects "main-weft" (the primary host worktree is on
// "main") and every task pair, with no BranchPrefix juggling.
//
// Board needs no explicit exclusion: its weft:main branch is the host's own
// unsuffixed default branch (e.g. "main"), which never matches the
// -weft-suffixed pattern WeftHostSlug looks for, so it is never enumerated
// as a Cleanup candidate in the first place — not because of any
// repo-level carve-out, but because Cleanup only ever walks weft branches
// and compares them against the set of known host worktree slugs.

package fabricengine

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

// CleanupBranchEntry describes the fate of one orphaned weft branch under Cleanup.
type CleanupBranchEntry struct {
	// Branch is the weft branch name.
	Branch string `json:"branch"`
	// Deleted reports whether the branch was actually deleted from the weft repo.
	// It is false on a dry run, when the entry was skipped due to gate protection or
	// unmanaged (non-suffixed) status, and when deletion itself failed.
	Deleted bool `json:"deleted"`
	// Protected reports whether the branch was skipped rather than deleted —
	// because raddleFoldedBack returned false and force was not set, because the
	// branch is not fabric-managed (no "-weft" suffix, e.g. inherited from history
	// predating fabric's uniform naming scheme), or because the branch is
	// currently checked out at a worktree (git branch -D could never delete it).
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

// raddleFoldedBack reports whether the weft branch's _raddle has been squash-merged back.
// Currently returns false conservatively; branches are gate-protected unless --force is set.
func raddleFoldedBack(_ string) bool {
	return false
}

// Cleanup finds weft branches with no corresponding host worktree sibling
// and reports or deletes them per the flag matrix: apply gates whether any deletion happens,
// force bypasses the _raddle merge-back gate, checked-out branches are always protected.
func (t *Topology) Cleanup(l *lyxcwd.Location, apply, force bool) (CleanupResult, error) {
	// Enumerate host worktrees using git-registered entries only.
	entries, err := List(l.WorktreePath())
	if err != nil {
		return CleanupResult{}, fmt.Errorf("list host worktrees: %w", err)
	}

	// Build the set of live host branches; unreadable branches (stale registrations) skip.
	liveHostBranches := make(map[string]bool, len(entries))
	for _, entry := range entries {
		hostPath := filepath.Clean(filepath.FromSlash(entry.Path))
		branch, branchErr := readBranch(hostPath)
		if branchErr != nil {
			continue
		}
		liveHostBranches[branch] = true
	}

	// Enumerate all branches in the weft repo to find orphans.
	weftBranches, err := listWeftBranches(l)
	if err != nil {
		return CleanupResult{}, fmt.Errorf("list weft branches: %w", err)
	}

	var result CleanupResult

	for _, weftBranch := range weftBranches {
		branch := weftBranch.Branch

		// Recover the host branch by inverting WeftBranchName's suffix.
		// Non-fabric-managed branches are reported but never deleted.
		hostBranch, ok := WeftHostSlug(branch)
		if !ok {
			result.Entries = append(result.Entries, CleanupBranchEntry{
				Branch:    branch,
				Protected: true,
			})
			continue
		}

		if liveHostBranches[hostBranch] {
			// Live pair: a host worktree is on this weft branch's paired host branch.
			continue
		}

		entry := CleanupBranchEntry{
			Branch: branch,
		}

		if weftBranch.WorktreePath != "" {
			// Checked-out branch: always protected, never deletable.
			entry.Protected = true
			result.Entries = append(result.Entries, entry)
			continue
		}

		if !apply {
			result.Entries = append(result.Entries, entry)
			continue
		}

		// Check gate unless --force is set.
		folded := raddleFoldedBack(branch)
		if !folded && !force {
			entry.Protected = true
			result.Entries = append(result.Entries, entry)
			continue
		}

		entry.Deleted = deleteWeftBranch(l, branch, &entry)
		result.Entries = append(result.Entries, entry)
	}

	return result, nil
}

// weftBranchCheckout pairs a weft branch name with its checked-out worktree path if any.
type weftBranchCheckout struct {
	Branch       string
	WorktreePath string
}

// listWeftBranches returns every branch in the weft repo with its checked-out worktree path if any.
func listWeftBranches(l *lyxcwd.Location) ([]weftBranchCheckout, error) {
	weftRepoRoot, err := WeftRepoRoot(l)
	if err != nil {
		return nil, fmt.Errorf("resolve weft repo root: %w", err)
	}
	out, _, exitCode, err := gitexec.RunGit(
		[]string{"branch", "--format=%(refname:short)\x1f%(worktreepath)"},
		weftRepoRoot,
	)
	if err != nil {
		return nil, fmt.Errorf("git branch: %w", err)
	}
	if exitCode != 0 {
		return nil, fmt.Errorf("list weft branches failed (git exit %d)", exitCode)
	}

	raw := strings.TrimSpace(out)
	if raw == "" {
		return nil, nil
	}

	var branches []weftBranchCheckout
	for _, line := range strings.Split(raw, "\n") {
		name, worktreePath, _ := strings.Cut(strings.TrimSpace(line), "\x1f")
		if name == "" {
			continue
		}
		branches = append(branches, weftBranchCheckout{
			Branch:       name,
			WorktreePath: strings.TrimSpace(worktreePath),
		})
	}
	return branches, nil
}

// deleteWeftBranch deletes a weft branch via git branch -D, recording errors in entry.
func deleteWeftBranch(l *lyxcwd.Location, branch string, entry *CleanupBranchEntry) bool {
	weftRepoRoot, err := WeftRepoRoot(l)
	if err != nil {
		entry.Error = fmt.Sprintf("resolve weft repo root: %v", err)
		return false
	}
	_, _, exitCode, err := gitexec.RunGit(
		[]string{"branch", "-D", branch},
		weftRepoRoot,
	)
	if err != nil {
		entry.Error = fmt.Sprintf("git branch -D %s: %v", branch, err)
		return false
	}
	if exitCode != 0 {
		entry.Error = fmt.Sprintf("delete weft branch %q failed (git exit %d)", branch, exitCode)
		return false
	}
	return true
}
