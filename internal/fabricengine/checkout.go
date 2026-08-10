// checkout.go implements the coordinated warp+weft branch switch with rollback.
//
// Checkout switches the warp worktree to branch and its weft sibling to WeftBranchName(branch) in
// an all-or-nothing operation.
// Preconditions are checked first;
// on any weft-side or junction-wiring failure both switches are rolled back to their original
// branches so the pair is never left half-switched.
// The weft target is always the suffixed sibling of the warp target,
// and switchOrForkWeft's fork-from-parent start point is the weft branch the worktree was actually
// on before the switch — for an in-sync pair, the suffixed sibling of the previous warp branch.

package fabricengine

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

// CheckoutResult contains the fields produced by a successful Checkout.
type CheckoutResult struct {
	// Branch is the warp branch the warp worktree now points to (the weft
	// worktree points to WeftBranchName(Branch)).
	Branch string `json:"branch"`
	// WeftWorktree is the filesystem path to the weft sibling worktree.
	WeftWorktree string `json:"weft_worktree"`
}

// Checkout switches the warp worktree to branch and its weft sibling to WeftBranchName(branch) in
// an all-or-nothing operation, refusing if the weft worktree has uncommitted changes, forking new
// weft branches when their suffixed siblings don't exist, re-pointing junctions, and refreshing the
// correspondence index — rolling back both sides on failure to preserve all-or-nothing semantics.
func (t *Topology) Checkout(l *lyxcwd.Location, branch string) (CheckoutResult, error) {
	weftWorktree := WeftWorktree(l)

	// Refuse if the weft worktree is dirty to prevent half-switched pairs.
	weftDirty, _, err := worktreeDirty(scopeTracked, weftWorktree)
	if err != nil {
		return CheckoutResult{}, fmt.Errorf("check weft status: %w", err)
	}
	if weftDirty {
		return CheckoutResult{}, fmt.Errorf("weft worktree has uncommitted changes; stash or commit before checkout")
	}

	// Capture both original branches for rollback on later failure.
	origBranchOut, origBranchStderr, exitCode, err := gitexec.RunGit(
		[]string{"rev-parse", "--abbrev-ref", "HEAD"},
		l.WorktreePath(),
	)
	if err != nil {
		return CheckoutResult{}, fmt.Errorf("capture warp branch: %w", err)
	}
	if exitCode != 0 {
		return CheckoutResult{}, fmt.Errorf("capture warp branch failed (git exit %d): %s",
			exitCode, strings.TrimSpace(origBranchStderr))
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

	// Switch the warp worktree to the target branch.
	// git's own stderr is carried into the error: the actionable half of a failure here is
	// invariably in it ("'main' is already used by worktree at ..."), and a bare exit code leaves
	// the operator with nothing to act on.
	_, switchStderr, exitCode, err := gitexec.RunGit(
		[]string{"switch", branch},
		l.WorktreePath(),
	)
	if err != nil {
		return CheckoutResult{}, fmt.Errorf("warp switch: %w", err)
	}
	if exitCode != 0 {
		return CheckoutResult{}, fmt.Errorf("warp switch to branch %q failed (git exit %d): %s", branch, exitCode, strings.TrimSpace(switchStderr))
	}

	// Resolve the weft sibling branch; roll back warp on failure.
	slug := filepath.Base(l.WorktreePath())
	weftForked, err := t.switchOrForkWeft(l, branch)
	if err != nil {
		t.rollbackSwitch(l, originalBranch, originalWeftBranch, "")
		return CheckoutResult{}, err
	}

	// Track any forked weft branch for deletion on rollback.
	forkedWeftBranch := ""
	if weftForked {
		forkedWeftBranch = WeftBranchName(branch)
	}

	// Re-point junctions; roll back both sides on failure (weft already switched).
	names, err := RepoWiredNames(l)
	if err != nil {
		t.rollbackSwitch(l, originalBranch, originalWeftBranch, forkedWeftBranch)
		return CheckoutResult{}, fmt.Errorf("re-point junctions: load fabric config: %w", err)
	}
	if err := WireJunctions(l, slug, names); err != nil {
		t.rollbackSwitch(l, originalBranch, originalWeftBranch, forkedWeftBranch)
		return CheckoutResult{}, fmt.Errorf("re-point junctions: %w", err)
	}

	// Refresh correspondence index for the new branch; switch is complete regardless of failure.
	if err := refreshCorrIndexAfterSwitch(l.WorktreePath(), weftWorktree); err != nil {
		return CheckoutResult{}, fmt.Errorf("checkout to %q completed, but refreshing the correspondence index failed (re-run `lyx fabric checkout` to retry): %w", branch, err)
	}

	return CheckoutResult{
		Branch:       branch,
		WeftWorktree: weftWorktree,
	}, nil
}

// switchOrForkWeft switches or forks the weft branch to match the warp target,
// reporting whether a new branch was created (forked) so rollback can clean it up.
func (t *Topology) switchOrForkWeft(l *lyxcwd.Location, branch string) (forked bool, err error) {
	weftWorktree := WeftWorktree(l)
	weftBranch := WeftBranchName(branch)

	if weftBranchExists(l, weftBranch) {
		// Branch exists: switch to it.
		_, switchStderr, exitCode, err := gitexec.RunGit(
			[]string{"switch", weftBranch},
			weftWorktree,
		)
		if err != nil {
			return false, fmt.Errorf("weft switch: %w", err)
		}
		if exitCode != 0 {
			return false, fmt.Errorf("weft switch to branch %q failed (git exit %d): %s", weftBranch, exitCode, strings.TrimSpace(switchStderr))
		}
		return false, nil
	}

	// Branch does not exist: fork from current weft HEAD to preserve merge-base.
	parentWeftBranchOut, parentWeftStderr, exitCode, err := gitexec.RunGit(
		[]string{"rev-parse", "--abbrev-ref", "HEAD"},
		weftWorktree,
	)
	if err != nil {
		return false, fmt.Errorf("capture parent weft branch: %w", err)
	}
	if exitCode != 0 {
		return false, fmt.Errorf("capture parent weft branch failed (git exit %d): %s",
			exitCode, strings.TrimSpace(parentWeftStderr))
	}
	parentWeftBranch := strings.TrimSpace(parentWeftBranchOut)

	// Create and switch to the new weft branch.
	_, forkStderr, exitCode, err := gitexec.RunGit(
		[]string{"switch", "-c", weftBranch, parentWeftBranch},
		weftWorktree,
	)
	if err != nil {
		return false, fmt.Errorf("fork weft branch: %w", err)
	}
	if exitCode != 0 {
		return false, fmt.Errorf("fork weft branch %q from %q failed (git exit %d): %s", weftBranch, parentWeftBranch, exitCode, strings.TrimSpace(forkStderr))
	}

	return true, nil
}

// rollbackSwitch switches both warp and weft back to their original branches on failure,
// cleaning up any forked weft branch, with errors silently discarded.
// The junction stays consistent without rewiring because the worktree directory path doesn't change.
func (t *Topology) rollbackSwitch(l *lyxcwd.Location, originalBranch, originalWeftBranch, forkedWeftBranch string) {
	_, _, _, _ = gitexec.RunGit([]string{"switch", originalBranch}, l.WorktreePath())
	if originalWeftBranch != "" {
		_, _, _, _ = gitexec.RunGit([]string{"switch", originalWeftBranch}, WeftWorktree(l))
	}
	if forkedWeftBranch != "" {
		_, _, _, _ = gitexec.RunGit([]string{"branch", "-D", forkedWeftBranch}, WeftWorktree(l))
	}
}
