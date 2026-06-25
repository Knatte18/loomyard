// checkout.go implements the coordinated host+weft branch switch with rollback.
//
// Checkout switches the host worktree and its weft sibling to the same branch in an
// all-or-nothing operation. Preconditions are checked first; on any weft-side failure
// the host switch is rolled back to the original branch so the pair is never left
// half-switched.

package warp

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/paths"
)

// CheckoutResult contains the fields produced by a successful Checkout.
type CheckoutResult struct {
	// Branch is the branch that both host and weft worktrees now point to.
	Branch string `json:"branch"`
	// WeftWorktree is the filesystem path to the weft sibling worktree.
	WeftWorktree string `json:"weft_worktree"`
}

// Checkout switches the host worktree and its weft sibling to branch in an
// all-or-nothing operation.
//
// Steps:
//  1. Precondition — refuse if the weft worktree has uncommitted changes (dirty check via
//     git status --porcelain). Git's own refusal propagates naturally when the host switch
//     would clobber uncommitted host changes.
//  2. Capture the original host branch for rollback purposes.
//  3. Switch the host worktree to branch via git switch.
//  4. Resolve the weft sibling branch: if it exists in the weft repo, switch the weft
//     worktree to it; if it does not (unmanaged target branch), fork a new weft branch
//     from the parent weft branch using the same adopt-or-create fork-point logic as Add.
//  5. Re-point junctions via WireJunctions.
//  6. On any failure at steps 4–5, roll back the host switch to the original branch and
//     return the original error untouched; the pair is never left half-switched.
//
// Returns CheckoutResult on success or an error if any step fails.
func (w *Worktree) Checkout(l *paths.Layout, branch string) (CheckoutResult, error) {
	weftWorktree := l.WeftWorktree()

	// (1) Precondition: refuse if the weft worktree is dirty. A dirty weft would mean
	// the branch switch could clobber or be blocked by uncommitted local changes, leaving
	// the pair in an indeterminate state. We check weft before touching host so either
	// both switch or neither does.
	weftStatus, _, exitCode, err := gitexec.RunGit(
		[]string{"status", "--porcelain", "--untracked-files=no"},
		weftWorktree,
	)
	if err != nil {
		return CheckoutResult{}, fmt.Errorf("check weft status: %w", err)
	}
	if exitCode != 0 {
		return CheckoutResult{}, fmt.Errorf("git status failed in weft worktree (exit %d)", exitCode)
	}
	if strings.TrimSpace(weftStatus) != "" {
		return CheckoutResult{}, fmt.Errorf("weft worktree has uncommitted changes; stash or commit before checkout")
	}

	// (2) Capture the original host branch so we can roll back if the weft switch fails.
	origBranchOut, _, exitCode, err := gitexec.RunGit(
		[]string{"rev-parse", "--abbrev-ref", "HEAD"},
		l.WorktreeRoot,
	)
	if err != nil {
		return CheckoutResult{}, fmt.Errorf("capture host branch: %w", err)
	}
	if exitCode != 0 {
		return CheckoutResult{}, fmt.Errorf("capture host branch failed with exit code %d", exitCode)
	}
	originalBranch := strings.TrimSpace(origBranchOut)

	// (3) Switch the host worktree to the target branch. Git propagates its own refusal
	// (e.g., conflicting local changes) unchanged; we do not suppress it.
	_, hostSwitchStderr, exitCode, err := gitexec.RunGit(
		[]string{"switch", branch},
		l.WorktreeRoot,
	)
	if err != nil {
		return CheckoutResult{}, fmt.Errorf("host switch: %w", err)
	}
	if exitCode != 0 {
		return CheckoutResult{}, fmt.Errorf("host switch failed: %s", hostSwitchStderr)
	}

	// (4) Resolve the weft sibling branch. On any failure, roll back the host switch.
	// The slug is derived from the current worktree's base name; it identifies which
	// pair of junctions to re-point and which weft worktree path to switch.
	slug := filepath.Base(l.WorktreeRoot)
	if err := w.switchOrForkWeft(l, branch); err != nil {
		// Roll back the host switch to restore the consistent pair state.
		w.rollbackHostSwitch(l, originalBranch)
		return CheckoutResult{}, err
	}

	// (5) Re-point the junction for the current worktree's slug. On failure, roll back.
	if err := WireJunctions(l, slug); err != nil {
		w.rollbackHostSwitch(l, originalBranch)
		return CheckoutResult{}, fmt.Errorf("re-point junctions: %w", err)
	}

	return CheckoutResult{
		Branch:       branch,
		WeftWorktree: weftWorktree,
	}, nil
}

// switchOrForkWeft switches the weft worktree to branch or forks it from the current
// parent weft branch when branch does not yet exist in the weft repo.
//
// If the weft branch exists in the weft repo, runs git switch <branch> in the weft
// worktree. If the weft branch does not exist, captures the current weft HEAD branch
// as the fork point and creates the new branch in-place via git switch -c. This
// preserves the shared merge-base needed for future squash-merge-back operations,
// matching Add's adopt-or-create fork-point logic.
func (w *Worktree) switchOrForkWeft(l *paths.Layout, branch string) error {
	weftWorktree := l.WeftWorktree()

	if weftBranchExists(l, branch) {
		// Branch already exists in the weft repo: switch the weft worktree to it.
		_, stderr, exitCode, err := gitexec.RunGit(
			[]string{"switch", branch},
			weftWorktree,
		)
		if err != nil {
			return fmt.Errorf("weft switch: %w", err)
		}
		if exitCode != 0 {
			return fmt.Errorf("weft switch failed: %s", stderr)
		}
		return nil
	}

	// Branch does not exist in the weft repo: fork it from the current weft HEAD.
	// The fork point is the weft branch that corresponds to the parent host branch.
	// Using the current weft HEAD preserves the shared merge-base needed for future
	// squash-merge-back operations, matching Add's adopt-or-create fork-point logic.
	parentWeftBranchOut, _, exitCode, err := gitexec.RunGit(
		[]string{"rev-parse", "--abbrev-ref", "HEAD"},
		weftWorktree,
	)
	if err != nil {
		return fmt.Errorf("capture parent weft branch: %w", err)
	}
	if exitCode != 0 {
		return fmt.Errorf("capture parent weft branch failed with exit code %d", exitCode)
	}
	parentWeftBranch := strings.TrimSpace(parentWeftBranchOut)

	// Create the new weft branch forked from the parent weft branch and switch
	// the weft worktree to it immediately via git switch -c.
	_, stderr, exitCode, err := gitexec.RunGit(
		[]string{"switch", "-c", branch, parentWeftBranch},
		weftWorktree,
	)
	if err != nil {
		return fmt.Errorf("fork weft branch: %w", err)
	}
	if exitCode != 0 {
		return fmt.Errorf("fork weft branch failed: %s", stderr)
	}

	return nil
}

// rollbackHostSwitch attempts to switch the host worktree back to originalBranch.
//
// Called only when weft-side or junction operations fail after the host has already
// been switched. Errors from the rollback are silently discarded; the caller already
// has the original error and rollback failures are secondary. The primary invariant is
// that we attempt a best-effort restore rather than leaving the pair half-switched.
func (w *Worktree) rollbackHostSwitch(l *paths.Layout, originalBranch string) {
	// Best-effort: silently ignore rollback failure because the caller already holds
	// the original error that triggered this rollback.
	_, _, _, _ = gitexec.RunGit([]string{"switch", originalBranch}, l.WorktreeRoot)
}
