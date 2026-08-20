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
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

// CheckoutResult contains the fields produced by a successful Checkout.
// It embeds MutationRecord, which carries the mutation record accumulated over the call.
type CheckoutResult struct {
	MutationRecord
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
func (t *Topology) Checkout(l *lyxcwd.Location, branch string) (res CheckoutResult, err error) {
	rec := NewMutations(l.HubPath)
	defer func() { res.Mutations = rec.Snapshot() }()

	weftWorktree := WeftWorktree(l)

	// A coordinated branch switch out of a half-merged pair is refused: record-only, since the
	// foreign-state disposition belongs to Commit alone.
	blocked, err := mergeBlocksMutation(l.WorktreePath(), weftWorktree)
	if err != nil {
		return CheckoutResult{}, err
	}
	if blocked {
		return CheckoutResult{}, &ErrMergeInProgress{}
	}

	// Refuse if the weft worktree is dirty to prevent half-switched pairs.
	weftDirty, _, err := worktreeDirty(scopeTracked, weftWorktree)
	if err != nil {
		return CheckoutResult{}, fmt.Errorf("check weft status: %w", err)
	}
	if weftDirty {
		return CheckoutResult{}, fmt.Errorf("weft worktree has uncommitted changes; stash or commit before checkout")
	}

	// Capture both original branches for rollback on later failure.
	origBranchOut, err := gitexec.Run(
		[]string{"rev-parse", "--abbrev-ref", "HEAD"},
		l.WorktreePath(),
	)
	if err != nil {
		return CheckoutResult{}, fmt.Errorf("capture warp branch: %w", err)
	}
	originalBranch := strings.TrimSpace(origBranchOut)

	// The weft branch capture is best-effort: a detached or unborn weft HEAD
	// (abbrev-ref "HEAD" or empty) has no branch name to switch back to, so
	// rollbackSwitch simply skips the weft side in that abnormal case, matching
	// the best-effort posture of the rollback as a whole.
	originalWeftBranch := ""
	if weftBranchOut, err := gitexec.Run(
		[]string{"rev-parse", "--abbrev-ref", "HEAD"},
		weftWorktree,
	); err == nil {
		if b := strings.TrimSpace(weftBranchOut); b != "HEAD" {
			originalWeftBranch = b
		}
	}

	// Switch the warp worktree to the target branch.
	// git's own stderr is carried into the error: the actionable half of a failure here is
	// invariably in it ("'main' is already used by worktree at ..."), and a bare exit code leaves
	// the operator with nothing to act on.
	if _, err := gitexec.Run(
		[]string{"switch", branch},
		l.WorktreePath(),
	); err != nil {
		return CheckoutResult{}, fmt.Errorf("warp switch to branch %q failed: %w", branch, err)
	}
	rec.Append(KindWorktreeSwitched, l.WorktreePath(), branch)

	// Resolve the weft sibling branch; roll back warp on failure.
	slug := filepath.Base(l.WorktreePath())
	weftForked, err := t.switchOrForkWeft(rec, l, branch)
	if err != nil {
		t.rollbackSwitch(rec, l, originalBranch, originalWeftBranch, "")
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
		t.rollbackSwitch(rec, l, originalBranch, originalWeftBranch, forkedWeftBranch)
		return CheckoutResult{}, fmt.Errorf("re-point junctions: load fabric config: %w", err)
	}
	if err := WireJunctionsWith(rec, l, slug, names); err != nil {
		t.rollbackSwitch(rec, l, originalBranch, originalWeftBranch, forkedWeftBranch)
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
// rec is Checkout's own recorder; it records KindWorktreeSwitched at the weft worktree root with the
// branch switched to as Detail on either branch, and additionally records KindBranchCreated for the
// forked branch on the fork branch, since `switch -c` creates it.
func (t *Topology) switchOrForkWeft(rec *Mutations, l *lyxcwd.Location, branch string) (forked bool, err error) {
	weftWorktree := WeftWorktree(l)
	weftBranch := WeftBranchName(branch)

	if weftBranchExists(l, weftBranch) {
		// Branch exists: switch to it.
		if _, err := gitexec.Run(
			[]string{"switch", weftBranch},
			weftWorktree,
		); err != nil {
			return false, fmt.Errorf("weft switch to branch %q failed: %w", weftBranch, err)
		}
		rec.Append(KindWorktreeSwitched, weftWorktree, weftBranch)
		return false, nil
	}

	// Branch does not exist: fork from current weft HEAD to preserve merge-base.
	parentWeftBranchOut, err := gitexec.Run(
		[]string{"rev-parse", "--abbrev-ref", "HEAD"},
		weftWorktree,
	)
	if err != nil {
		return false, fmt.Errorf("capture parent weft branch: %w", err)
	}
	parentWeftBranch := strings.TrimSpace(parentWeftBranchOut)

	// Create and switch to the new weft branch.
	if _, err := gitexec.Run(
		[]string{"switch", "-c", weftBranch, parentWeftBranch},
		weftWorktree,
	); err != nil {
		return false, fmt.Errorf("fork weft branch %q from %q failed: %w", weftBranch, parentWeftBranch, err)
	}
	rec.Append(KindWorktreeSwitched, weftWorktree, weftBranch)
	rec.AppendRef(KindBranchCreated, weftBranch, "")

	return true, nil
}

// rollbackSwitch switches both warp and weft back to their original branches on failure,
// cleaning up any forked weft branch, with errors silently discarded.
// The junction stays consistent without rewiring because the worktree directory path doesn't change.
//
// rollbackSwitch is void and discards every error from its two git switch calls, deliberately — that
// stays unchanged. The forked-branch deletion is different: it now runs through the gate's
// deleteBranch executor, and a gate refusal there is never allowed to vanish silently. Since this
// function cannot return an error without widening its signature (out of scope — it runs on paths
// where Checkout is already failing, and turning a best-effort rollback into a hard failure is a
// behaviour change this slice does not make), a refusal is logged via logger.Warn instead.
// rec is Checkout's own recorder, threaded through to the gate's deleteBranch executor, and also
// records KindWorktreeSwitched for either of this function's own git switch calls that succeeds — a
// rollback switch is a real mutation of the working tree, and the record must carry it, in order,
// which is the whole point of Checkout's both-sides-rollback case.
func (t *Topology) rollbackSwitch(rec *Mutations, l *lyxcwd.Location, originalBranch, originalWeftBranch, forkedWeftBranch string) {
	if _, err := gitexec.Run([]string{"switch", originalBranch}, l.WorktreePath()); err == nil {
		rec.Append(KindWorktreeSwitched, l.WorktreePath(), originalBranch)
	}
	if originalWeftBranch != "" {
		if _, err := gitexec.Run([]string{"switch", originalWeftBranch}, WeftWorktree(l)); err == nil {
			rec.Append(KindWorktreeSwitched, WeftWorktree(l), originalWeftBranch)
		}
	}
	if forkedWeftBranch != "" {
		req := branchRequest{
			what:      "delete forked weft branch",
			repoDir:   WeftWorktree(l),
			branch:    forkedWeftBranch,
			ownership: ownedManagedBranch(l, t.cfg.BranchPrefix),
			dirtiness: dirtyCheckedOutBranch(),
			force:     false,
		}
		if err := deleteBranch(rec, req); err != nil {
			var refusal *destructiveRefusal
			if errors.As(err, &refusal) {
				logger.Warn("fabricengine: rollbackSwitch's branch deletion was refused by the destructive gate", "branch", forkedWeftBranch, "check", string(refusal.Check))
			}
		}
	}
}
