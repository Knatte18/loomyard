// checkout.go implements the coordinated host+weft branch switch with rollback.
//
// Checkout switches the host worktree to branch and its weft sibling to
// WeftBranchName(branch) in an all-or-nothing operation. Preconditions are
// checked first; on any weft-side failure the host switch is rolled back to
// the original branch so the pair is never left half-switched. Adapted from
// warpengine's checkout.go — same precondition/rollback discipline, package
// fabricengine. The branch delta: the weft target is always the suffixed
// sibling of the host target, and switchOrForkWeft's fork-from-parent start
// point is the weft branch the worktree was actually on before the switch
// (which, for an in-sync pair, is the suffixed sibling of the previous host
// branch — the same one-hop-over relationship warp's mirrored fork point has).

package fabricengine

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/hubgeometry"
)

// CheckoutResult contains the fields produced by a successful Checkout.
type CheckoutResult struct {
	// Branch is the host branch the host worktree now points to (the weft
	// worktree points to WeftBranchName(Branch)).
	Branch string `json:"branch"`
	// WeftWorktree is the filesystem path to the weft sibling worktree.
	WeftWorktree string `json:"weft_worktree"`
}

// Checkout switches the host worktree to branch and its weft sibling to
// WeftBranchName(branch) in an all-or-nothing operation.
//
// Steps:
//  1. Precondition — refuse if the weft worktree has uncommitted changes (dirty check via
//     git status --porcelain). Git's own refusal propagates naturally when the host switch
//     would clobber uncommitted host changes.
//  2. Capture the original host branch for rollback purposes.
//  3. Switch the host worktree to branch via git switch.
//  4. Resolve the weft sibling branch: if WeftBranchName(branch) exists in the weft repo,
//     switch the weft worktree to it; if it does not (unmanaged target branch), fork a new
//     weft branch from the weft branch the worktree was actually on before the switch,
//     using the same adopt-or-create fork-point logic as Add.
//  5. Re-point junctions via WireJunctions.
//  6. On any failure at steps 4–5, roll back the host switch to the original branch and
//     return the original error untouched; the pair is never left half-switched.
//
// Returns CheckoutResult on success or an error if any step fails.
func (t *Topology) Checkout(l *hubgeometry.Layout, branch string) (CheckoutResult, error) {
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
	_, _, exitCode, err = gitexec.RunGit(
		[]string{"switch", branch},
		l.WorktreeRoot,
	)
	if err != nil {
		return CheckoutResult{}, fmt.Errorf("host switch: %w", err)
	}
	if exitCode != 0 {
		return CheckoutResult{}, fmt.Errorf("host switch to branch %q failed (git exit %d)", branch, exitCode)
	}

	// (4) Resolve the weft sibling branch. On any failure, roll back the host switch.
	// The slug is derived from the current worktree's base name; it identifies which
	// pair of junctions to re-point and which weft worktree path to switch.
	slug := filepath.Base(l.WorktreeRoot)
	if err := t.switchOrForkWeft(l, branch); err != nil {
		// Roll back the host switch to restore the consistent pair state.
		t.rollbackHostSwitch(l, originalBranch)
		return CheckoutResult{}, err
	}

	// (5) Re-point the junction for the current worktree's slug. On failure, roll back.
	if err := WireJunctions(l, slug); err != nil {
		t.rollbackHostSwitch(l, originalBranch)
		return CheckoutResult{}, fmt.Errorf("re-point junctions: %w", err)
	}

	return CheckoutResult{
		Branch:       branch,
		WeftWorktree: weftWorktree,
	}, nil
}

// switchOrForkWeft switches the weft worktree to WeftBranchName(branch), or
// forks it from the weft branch the worktree was actually on before the
// switch when WeftBranchName(branch) does not yet exist in the weft repo.
//
// If the weft branch exists in the weft repo, runs git switch <weftBranch> in the weft
// worktree. If it does not exist, captures the current weft HEAD branch (the weft
// sibling of whatever host branch the pair was on before this call — the suffixed
// sibling of the previous host branch for an in-sync pair) as the fork point and
// creates the new branch in-place via git switch -c. This preserves the shared
// merge-base needed for future squash-merge-back operations, matching Add's
// adopt-or-create fork-point logic one suffix over.
func (t *Topology) switchOrForkWeft(l *hubgeometry.Layout, branch string) error {
	weftWorktree := l.WeftWorktree()
	weftBranch := WeftBranchName(branch)

	if weftBranchExists(l, weftBranch) {
		// Branch already exists in the weft repo: switch the weft worktree to it.
		_, _, exitCode, err := gitexec.RunGit(
			[]string{"switch", weftBranch},
			weftWorktree,
		)
		if err != nil {
			return fmt.Errorf("weft switch: %w", err)
		}
		if exitCode != 0 {
			return fmt.Errorf("weft switch to branch %q failed (git exit %d)", weftBranch, exitCode)
		}
		return nil
	}

	// Branch does not exist in the weft repo: fork it from the current weft HEAD,
	// i.e. the weft branch corresponding to the branch the weft was on before this
	// switch. Using the current weft HEAD preserves the shared merge-base needed for
	// future squash-merge-back operations, matching Add's adopt-or-create fork-point
	// logic one suffix over.
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
	_, _, exitCode, err = gitexec.RunGit(
		[]string{"switch", "-c", weftBranch, parentWeftBranch},
		weftWorktree,
	)
	if err != nil {
		return fmt.Errorf("fork weft branch: %w", err)
	}
	if exitCode != 0 {
		return fmt.Errorf("fork weft branch %q from %q failed (git exit %d)", weftBranch, parentWeftBranch, exitCode)
	}

	return nil
}

// rollbackHostSwitch attempts to switch the host worktree back to originalBranch.
//
// Called only when weft-side or junction operations fail after the host has already
// been switched. Errors from the rollback are silently discarded; the caller already
// has the original error and rollback failures are secondary. The primary invariant is
// that we attempt a best-effort restore rather than leaving the pair half-switched.
//
// Junction invariant: WireJunctions is NOT called here because it was not called
// before the failure point — the junctions still point to the original branch state
// and are therefore consistent with the rolled-back host branch. Rewiring would be
// incorrect here and is not needed.
func (t *Topology) rollbackHostSwitch(l *hubgeometry.Layout, originalBranch string) {
	// Best-effort: silently ignore rollback failure because the caller already holds
	// the original error that triggered this rollback.
	_, _, _, _ = gitexec.RunGit([]string{"switch", originalBranch}, l.WorktreeRoot)
}
