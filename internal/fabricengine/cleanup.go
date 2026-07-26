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
// A weft branch's host sibling is recovered via hubgeometry.WeftHostSlug(branch) —
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
// and delete the very branch board-weft-storage requires to stay permanent. Comparing
// against live host *branches* protects "main-weft" (the primary host worktree is on
// "main") and every task pair, with no BranchPrefix juggling.
//
// The board repo is excluded entirely — Cleanup only enumerates weft branches
// and compares them against the set of known host worktree slugs.

package fabricengine

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/hubgeometry"
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

// raddleFoldedBack reports whether the weft branch's _raddle content has
// been squash-merged back into the host branch and is therefore safe to delete.
//
// This is the extension point for the real _raddle merge-back check.
// Until that check exists, this function conservatively returns false for any
// branch that looks like a task branch (i.e. all branches), so they are protected
// from deletion unless --force is specified.
//
// When the real merge-back check is implemented, replace the body of this function
// with the actual verification logic (e.g. check whether the branch's _raddle
// commit tree has been merged into the host branch's history).
func raddleFoldedBack(_ string) bool {
	// Conservative: always return false until the real check is implemented.
	// Task branches are therefore always gate-protected unless --force is set.
	return false
}

// Cleanup finds weft branches in the weft repo that have no corresponding host
// worktree sibling and, according to the flag matrix, reports or deletes them.
//
// Orphaned weft branches are identified by comparing all weft branch names against
// the set of host worktree slugs enumerated via hubgeometry.List. The board repo branch
// namespace is excluded — only the weft repo's branches are examined.
//
// The flag matrix governs deletion:
//   - apply == false             → report all orphaned branches; delete nothing.
//   - apply && !force            → delete orphans where raddleFoldedBack returns true;
//     skip (mark protected) those where it returns false.
//   - apply && force             → delete all fabric-managed orphaned branches
//     regardless of the gate (non-fabric-managed branches are never deleted, even
//     with force).
//   - force && !apply            → report only; force does not imply apply.
//
// Independently of the matrix, a weft branch currently checked out at any worktree
// is always reported as protected and never deleted, in every mode: git branch -D
// can never delete a checked-out branch, and a checked-out weft branch means its
// pair is still materialized on disk (e.g. a live pair whose host worktree is on a
// detached HEAD, which the live-host-branch liveness check cannot see).
//
// Returns CleanupResult on success or an error on fatal system failures. Per-branch
// deletion errors are recorded inline in CleanupBranchEntry.Error.
func (t *Topology) Cleanup(l *hubgeometry.Layout, apply, force bool) (CleanupResult, error) {
	// Enumerate host worktrees to build the set of known host slugs.
	// We use hubgeometry.List rather than scanning the hub directory so we only consider
	// git-registered worktrees, not arbitrary directories.
	entries, err := hubgeometry.List(l.WorktreeRoot)
	if err != nil {
		return CleanupResult{}, fmt.Errorf("list host worktrees: %w", err)
	}

	// Build the set of live host branches: the branch each existing host worktree is
	// currently checked out on. A weft branch is a live pair — never an orphan — exactly
	// when its paired host branch is in this set (see the file header for why liveness is
	// judged in branch space rather than against directory names).
	liveHostBranches := make(map[string]bool, len(entries))
	for _, entry := range entries {
		hostPath := filepath.Clean(filepath.FromSlash(entry.Path))
		branch, branchErr := readBranch(hostPath)
		if branchErr != nil {
			// A host worktree whose branch cannot be read — e.g. its directory was
			// deleted, leaving a stale git worktree registration — is not a live pair.
			// Its weft branch, if any, is genuinely orphaned, so skip it here rather
			// than protecting it.
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

		// Recover the host branch by inverting WeftBranchName's suffix. A branch with
		// no "-weft" suffix cannot be fabric-managed by definition — report it, but
		// never delete it, matching Reconcile's report-but-don't-touch rule for
		// unmanaged branches.
		hostBranch, ok := hubgeometry.WeftHostSlug(branch)
		if !ok {
			result.Entries = append(result.Entries, CleanupBranchEntry{
				Branch:    branch,
				Protected: true,
			})
			continue
		}

		if liveHostBranches[hostBranch] {
			// A host worktree is currently on this weft branch's paired host branch;
			// this is a live pair, skip it. This protects both task pairs and the
			// primary pair's weft branch (e.g. main-weft, since the primary host
			// worktree is on "main").
			continue
		}

		entry := CleanupBranchEntry{
			Branch: branch,
		}

		if weftBranch.WorktreePath != "" {
			// The branch is checked out at a worktree, so git branch -D could
			// never delete it — and its being checked out means the pair is
			// still materialized on disk (e.g. a live pair whose host worktree
			// is on a detached HEAD, invisible to the liveness check above).
			// Report it protected in every mode so dry-run, apply, and
			// apply+force all agree instead of attempting a doomed deletion.
			entry.Protected = true
			result.Entries = append(result.Entries, entry)
			continue
		}

		if !apply {
			// Dry-run: report the orphaned branch without deleting it.
			result.Entries = append(result.Entries, entry)
			continue
		}

		// apply is true: decide whether to delete based on gate and force flag.
		folded := raddleFoldedBack(branch)

		if !folded && !force {
			// Gate-protected: _raddle has not been folded back and --force was not set.
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

// weftBranchCheckout pairs one weft repo branch name with the worktree path
// the branch is currently checked out at; WorktreePath is empty when the
// branch is not checked out anywhere.
type weftBranchCheckout struct {
	Branch       string
	WorktreePath string
}

// listWeftBranches returns every branch in the weft repo together with the
// worktree it is checked out at, if any. The checkout location feeds
// Cleanup's checked-out protection: a checked-out branch can never be deleted
// by git branch -D, so Cleanup reports it protected rather than attempting a
// doomed deletion. Returns an error if the git command fails to spawn or
// exits non-zero.
func listWeftBranches(l *hubgeometry.Layout) ([]weftBranchCheckout, error) {
	// \x1f (unit separator) can never appear in a ref name and never in a
	// path git reports, so the field split is unambiguous.
	out, _, exitCode, err := gitexec.RunGit(
		[]string{"branch", "--format=%(refname:short)\x1f%(worktreepath)"},
		l.WeftRepoRoot(),
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

// deleteWeftBranch deletes a single weft branch via git branch -D and records
// any error in entry.Error. Returns true only when the deletion succeeded.
func deleteWeftBranch(l *hubgeometry.Layout, branch string, entry *CleanupBranchEntry) bool {
	_, _, exitCode, err := gitexec.RunGit(
		[]string{"branch", "-D", branch},
		l.WeftRepoRoot(),
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
