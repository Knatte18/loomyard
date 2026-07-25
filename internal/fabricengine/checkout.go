// checkout.go implements the coordinated host+weft branch switch with rollback.
//
// Checkout switches the host worktree to branch and its weft sibling to
// WeftBranchName(branch) in an all-or-nothing operation. Preconditions are
// checked first; on any weft-side or junction-wiring failure both switches are
// rolled back to their original branches so the pair is never left
// half-switched. Adapted from
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
//  6. On any failure at steps 4–5, roll back BOTH the host and the weft switches to
//     their original branches and return the original error untouched; the pair is
//     never left half-switched. (A step-5 failure happens after the weft has already
//     switched in step 4, so a host-only rollback would strand the weft on the new
//     branch — the very half-switch this contract forbids.)
//  7. Refresh the pair's correspondence index for the new branch (discard + rebuild
//     from the now-current weft branch's Warp-SHA trailers). The index file is
//     per-worktree and would otherwise keep serving entries recorded on the previous
//     branch — entries that still pass SHAExists but that the current branch's
//     trailer history would never produce. A step-7 failure does NOT roll the switch
//     back (the switch itself completed and is consistent); it is reported as an
//     error naming the completed switch, and re-running checkout (including the
//     no-argument in-place form) retries the refresh.
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

	// (2) Capture the original host AND weft branches so a failure at any step
	// after the weft switch rolls BOTH sides back — not just the host. Capturing
	// the weft branch before touching either side is what lets a later junction
	// failure (step 5) restore the pair to a consistent state instead of leaving
	// the host rolled back but the weft stranded on the new branch (a
	// half-switched pair, which this operation must never produce).
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

	// The weft branch capture is best-effort: a detached or unborn weft HEAD
	// (abbrev-ref "HEAD" or empty) has no branch name to switch back to, so
	// rollbackSwitch simply skips the weft side in that abnormal case, matching
	// the best-effort posture of the rollback as a whole.
	originalWeftBranch := ""
	if weftBranchOut, _, code, werr := gitexec.RunGit(
		[]string{"rev-parse", "--abbrev-ref", "HEAD"},
		weftWorktree,
	); werr == nil && code == 0 {
		if b := strings.TrimSpace(weftBranchOut); b != "HEAD" {
			originalWeftBranch = b
		}
	}

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
		// switchOrForkWeft's git switch is atomic, so on failure the weft is
		// still on its original branch; rolling both back restores the pair
		// (the weft side is a no-op here) without special-casing this path.
		t.rollbackSwitch(l, originalBranch, originalWeftBranch)
		return CheckoutResult{}, err
	}

	// (5) Re-point the junction for the current worktree's slug. On failure, roll
	// back BOTH sides: the weft was already switched in step 4, so a host-only
	// rollback would strand it on the new branch and leave a half-switched pair.
	if err := WireJunctions(l, slug); err != nil {
		t.rollbackSwitch(l, originalBranch, originalWeftBranch)
		return CheckoutResult{}, fmt.Errorf("re-point junctions: %w", err)
	}

	// (7) Refresh the pair's correspondence index for the new branch. No rollback
	// on failure — the switch itself is complete and consistent, and rolling back
	// would need the very same refresh; the discard-first refresh leaves lookups
	// missing honestly rather than answering from the previous branch's entries.
	if err := refreshCorrIndexAfterSwitch(l.WorktreeRoot, weftWorktree); err != nil {
		return CheckoutResult{}, fmt.Errorf("checkout to %q completed, but refreshing the correspondence index failed (re-run `lyx fabric checkout` to retry): %w", branch, err)
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

// rollbackSwitch attempts to switch the host worktree back to originalBranch and
// the weft worktree back to originalWeftBranch, restoring the pair to the state
// it was in before Checkout began.
//
// Called when the weft switch (step 4) or junction wiring (step 5) fails after
// the host has already been switched. Both sides are rolled back because a
// step-5 failure occurs AFTER the weft was already switched in step 4: reverting
// only the host would strand the weft on the new branch and leave a half-switched
// pair — exactly the state Checkout's all-or-nothing contract forbids. For the
// step-4 failure path the weft is still on its original branch, so its rollback
// is a harmless no-op. originalWeftBranch is empty when the weft HEAD was
// detached/unborn at capture time; in that abnormal case the weft side is
// skipped (there is no branch name to switch back to).
//
// Errors from the rollback are silently discarded: the caller already holds the
// original error that triggered this rollback, and a best-effort restore is all
// that can be promised. WireJunctions is NOT called here — either it was never
// reached (step-4 path) or it was the step that failed (step-5 path); in both
// cases the junction still points at the pair's own weft worktree directory
// (whose path does not change across a branch switch), so it stays consistent
// with the rolled-back branches without rewiring.
func (t *Topology) rollbackSwitch(l *hubgeometry.Layout, originalBranch, originalWeftBranch string) {
	_, _, _, _ = gitexec.RunGit([]string{"switch", originalBranch}, l.WorktreeRoot)
	if originalWeftBranch != "" {
		_, _, _, _ = gitexec.RunGit([]string{"switch", originalWeftBranch}, l.WeftWorktree())
	}
}
