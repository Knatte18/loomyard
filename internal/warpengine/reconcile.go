// reconcile.go implements the warp repair-and-adopt sweep for paired host↔weft worktrees.
//
// Reconcile walks all host worktrees (never the branch namespace directly) and applies
// the minimal corrective action needed to restore a valid paired topology: it recreates
// a missing weft worktree when the branch still exists, re-points a broken junction, adopts
// a raw (non-lyx) host worktree by creating the weft side dormant, and reports (but does
// not touch) a host worktree on an unmanaged branch.

package warpengine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/paths"
)

// ReconcileAction describes the corrective action applied to one host↔weft pair.
type ReconcileAction string

const (
	// ReconcileActionWeftRecreated means a missing weft worktree was recreated from
	// its existing branch.
	ReconcileActionWeftRecreated ReconcileAction = "weft_recreated"

	// ReconcileActionJunctionRepointed means a broken or dangling host _lyx junction
	// was re-pointed to the correct weft _lyx directory.
	ReconcileActionJunctionRepointed ReconcileAction = "junction_repointed"

	// ReconcileActionRawAdopted means a host worktree created outside lyx had its weft
	// side created (branch + worktree) as a dormant counterpart. No junction is wired;
	// that is lyx init's responsibility.
	ReconcileActionRawAdopted ReconcileAction = "raw_adopted"

	// ReconcileActionUnmanagedReported means a host worktree is on an unmanaged branch
	// with no weft sibling; it was reported but left untouched.
	ReconcileActionUnmanagedReported ReconcileAction = "unmanaged_reported"

	// ReconcileActionAlreadyHealthy means the pair required no corrective action.
	ReconcileActionAlreadyHealthy ReconcileAction = "already_healthy"
)

// ReconcilePairResult describes the outcome for one host↔weft pair.
type ReconcilePairResult struct {
	// HostWorktree is the absolute path to the host worktree.
	HostWorktree string `json:"host_worktree"`
	// WeftWorktree is the absolute path to the expected weft sibling.
	WeftWorktree string `json:"weft_worktree"`
	// Action is the corrective action taken (or reported).
	Action ReconcileAction `json:"action"`
	// Detail provides human-readable context for the action.
	Detail string `json:"detail,omitempty"`
	// Error is non-empty when the reconcile step encountered an error.
	Error string `json:"error,omitempty"`
}

// ReconcileResult is the top-level result returned by Reconcile.
type ReconcileResult struct {
	// Pairs is the ordered list of per-worktree reconcile outcomes.
	Pairs []ReconcilePairResult `json:"pairs"`
}

// Reconcile walks all host worktrees reachable from layout l and applies corrective
// actions to restore a valid paired host↔weft topology.
//
// For each host worktree, Reconcile applies the first matching rule:
//
//  1. Weft worktree missing, weft branch exists → recreate the weft worktree (adopt
//     the branch without creating a new one).
//  2. Weft worktree exists but junction is broken/dangling → re-point the junction
//     via WireJunctions.
//  3. Weft worktree absent and weft branch absent, host worktree is a lyx-managed
//     worktree (has a _lyx entry in its git config) → already handled by rules 1/2.
//  4. Weft worktree absent and weft branch absent, host worktree is a raw (non-lyx)
//     worktree → adopt it by creating the weft branch and worktree dormant (no junction
//     wiring; call `lyx init` to activate).
//  5. Host worktree is on an unmanaged branch (no weft sibling, raw worktree rule does
//     not apply, or branch is not managed) → report, touch nothing.
//
// The layout l provides Hub, Prime, and WeftRepoRoot geometry. Reconcile never walks
// the raw branch namespace — it only acts on worktrees it finds via paths.List.
// Returns an error only on fatal system failures; per-worktree errors are recorded
// inline in ReconcilePairResult.Error.
func (w *Worktree) Reconcile(l *paths.Layout) (ReconcileResult, error) {
	// Enumerate all host worktrees from any worktree in the repository.
	entries, err := paths.List(l.WorktreeRoot)
	if err != nil {
		return ReconcileResult{}, fmt.Errorf("list worktrees: %w", err)
	}

	var result ReconcileResult

	for _, entry := range entries {
		hostPath := filepath.FromSlash(entry.Path)
		hostPath = filepath.Clean(hostPath)

		// Derive the paired weft worktree path from the host worktree base name.
		slug := filepath.Base(hostPath)
		weftPath := filepath.Join(l.Hub, slug+"-weft")

		pr := ReconcilePairResult{
			HostWorktree: hostPath,
			WeftWorktree: weftPath,
		}

		// Build a per-host-worktree layout so junction geometry and branch resolution
		// are rooted at the correct worktree rather than the cwd worktree.
		hostLayout, layoutErr := paths.Resolve(hostPath)
		if layoutErr != nil {
			pr.Error = fmt.Sprintf("resolve layout: %v", layoutErr)
			pr.Action = ReconcileActionUnmanagedReported
			result.Pairs = append(result.Pairs, pr)
			continue
		}

		// Determine whether the weft worktree directory currently exists.
		weftStat, weftStatErr := os.Stat(weftPath)
		weftWorktreeExists := weftStatErr == nil && weftStat.IsDir()

		// Read the host branch to determine which weft branch we expect.
		hostBranch, branchErr := readBranch(hostPath)
		if branchErr != nil {
			pr.Error = fmt.Sprintf("read host branch: %v", branchErr)
			pr.Action = ReconcileActionUnmanagedReported
			result.Pairs = append(result.Pairs, pr)
			continue
		}

		if !weftWorktreeExists {
			// The weft worktree is absent. Decide between recreate, adopt, or report.
			pairedAction := w.reconcileMissingWeft(l, hostLayout, hostPath, weftPath, slug, hostBranch, &pr)
			pr.Action = pairedAction
		} else {
			// The weft worktree exists. Check whether the junction is healthy; if not, re-point it.
			hostLink := hostLayout.HostLyxLinkHere()
			weftLyxDir := hostLayout.WeftLyxDir()
			junctionHealthy, _ := checkJunctionHealth(hostLink, weftLyxDir)

			if !junctionHealthy {
				// Re-point the junction by running WireJunctions. WireJunctions is idempotent
				// and handles both the missing-junction and the wrong-target cases.
				if wireErr := WireJunctions(hostLayout, slug); wireErr != nil {
					pr.Error = fmt.Sprintf("re-point junction: %v", wireErr)
					pr.Action = ReconcileActionJunctionRepointed
				} else {
					pr.Action = ReconcileActionJunctionRepointed
					pr.Detail = fmt.Sprintf("junction re-pointed: %s → %s", hostLink, weftLyxDir)
				}
			} else {
				pr.Action = ReconcileActionAlreadyHealthy
			}
		}

		result.Pairs = append(result.Pairs, pr)
	}

	return result, nil
}

// reconcileMissingWeft determines and applies the corrective action when a weft worktree
// does not exist for the given host worktree.
//
// Rules applied in order:
//  1. If the weft branch exists in the weft repo → recreate the weft worktree from the branch.
//  2. If the host worktree appears to be a raw (non-lyx) worktree → adopt it by creating the
//     weft branch and worktree dormant (no junction).
//  3. Otherwise → report unmanaged and touch nothing.
func (w *Worktree) reconcileMissingWeft(
	l *paths.Layout,
	hostLayout *paths.Layout,
	hostPath, weftPath, slug, hostBranch string,
	pr *ReconcilePairResult,
) ReconcileAction {
	// Rule 1: weft branch exists — recreate the weft worktree from the existing branch.
	// This handles the case where the weft worktree was accidentally removed or is
	// in a git-worktree-prune-eligible state, but the branch (and its history) are intact.
	if weftBranchExists(hostLayout, hostBranch) {
		if err := adoptWeftWorktree(hostLayout, weftPath, hostBranch); err != nil {
			pr.Error = fmt.Sprintf("recreate weft worktree: %v", err)
			return ReconcileActionWeftRecreated
		}
		pr.Detail = fmt.Sprintf("recreated weft worktree at %s (branch %s existed)", weftPath, hostBranch)
		return ReconcileActionWeftRecreated
	}

	// Rule 2: weft branch absent — check whether this is a raw (non-lyx) host worktree.
	// A raw worktree has no weft counterpart at all; we adopt it by creating the weft branch
	// and worktree dormant. The host _lyx junction is NOT wired here; lyx init handles that.
	isRaw := isRawHostWorktree(hostPath)
	if isRaw {
		if err := createDormantWeftForRawHost(hostLayout, l, slug, hostBranch); err != nil {
			pr.Error = fmt.Sprintf("adopt raw host worktree: %v", err)
			return ReconcileActionRawAdopted
		}
		pr.Detail = fmt.Sprintf("adopted raw host worktree at %s; weft branch %s created dormant (run lyx init to activate)", hostPath, hostBranch)
		return ReconcileActionRawAdopted
	}

	// Rule 3: no weft branch and not identifiable as raw — report without touching anything.
	// The caller should run `lyx warp add` or `lyx init` explicitly.
	pr.Detail = fmt.Sprintf(
		"host worktree %s is on branch %s with no weft sibling; run `lyx warp add` or `lyx init`",
		hostPath, hostBranch,
	)
	return ReconcileActionUnmanagedReported
}

// adoptWeftWorktree creates a git worktree at weftPath for the existing branch in the
// weft repo. This is the "adopt" path: the branch already exists so no -b flag is used.
func adoptWeftWorktree(hostLayout *paths.Layout, weftPath, branch string) error {
	// git worktree add <path> <branch> — no -b because the branch already exists.
	_, stderr, exitCode, err := gitexec.RunGit(
		[]string{"worktree", "add", weftPath, branch},
		hostLayout.WeftRepoRoot(),
	)
	if err != nil {
		return fmt.Errorf("git worktree add: %w", err)
	}
	if exitCode != 0 {
		return fmt.Errorf("git worktree add failed: %s", stderr)
	}
	return nil
}

// isRawHostWorktree reports whether the worktree at hostPath lacks any lyx management
// markers. A worktree is considered raw when it has no _lyx junction and no weft branch
// in the weft repo (the branch check is done upstream; here we check the directory state).
//
// Currently, "raw" means: no _lyx path in the worktree root (no junction, no plain dir).
// This is a heuristic: if _lyx exists as a directory or junction the host may already be
// managed by a different lyx instance, which is beyond raw adoption.
func isRawHostWorktree(hostPath string) bool {
	lyxPath := filepath.Join(hostPath, paths.LyxDirName)
	_, err := os.Lstat(lyxPath)
	// A raw host worktree has no _lyx at all.
	return os.IsNotExist(err)
}

// createDormantWeftForRawHost creates a weft branch and worktree for a raw host worktree,
// leaving it dormant (no junction wiring). The weft branch forks from the current weft HEAD
// (parallel to the add adopt-or-create logic). The caller must run lyx init to wire junctions.
func createDormantWeftForRawHost(hostLayout *paths.Layout, l *paths.Layout, slug, hostBranch string) error {
	weftRoot := hostLayout.WeftRepoRoot()

	// Capture the current weft HEAD branch as the fork point for the new weft branch.
	// This preserves the shared merge-base needed for future squash-merge-back operations.
	parentWeftOut, _, exitCode, err := gitexec.RunGit(
		[]string{"rev-parse", "--abbrev-ref", "HEAD"},
		weftRoot,
	)
	if err != nil {
		return fmt.Errorf("capture parent weft branch: %w", err)
	}
	if exitCode != 0 {
		return fmt.Errorf("capture parent weft branch failed with exit code %d", exitCode)
	}
	parentWeftBranch := strings.TrimSpace(parentWeftOut)

	// Create the weft worktree with a new branch forked from the parent weft branch.
	// createWeftWorktree uses git worktree add -b <branch> <path> <startPoint>.
	if err := createWeftWorktree(hostLayout, slug, hostBranch, parentWeftBranch); err != nil {
		return fmt.Errorf("create dormant weft worktree: %w", err)
	}

	// Suppress "unused variable" — l is threaded through but not yet consumed beyond
	// the weft geometry already captured via hostLayout. Keep it for future extension.
	_ = l

	return nil
}
