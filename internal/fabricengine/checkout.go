// checkout.go implements the coordinated warp+weft branch switch with rollback.
//
// Checkout switches the warp worktree to branch and its weft sibling to RecordsBranchName(branch) in
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
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
)

// CheckoutResult contains the fields produced by a successful Checkout.
// It embeds MutationRecord, which carries the mutation record accumulated over the call.
type CheckoutResult struct {
	MutationRecord
	// Branch is the warp branch the warp worktree now points to (the weft
	// worktree points to RecordsBranchName(Branch)).
	Branch string `json:"branch"`
	// RecordsWorktree is the filesystem path to the weft sibling worktree.
	RecordsWorktree string `json:"records_worktree"`
}

// Checkout switches the warp worktree to branch and its weft sibling to RecordsBranchName(branch) in
// an all-or-nothing operation, refusing if the weft worktree has uncommitted changes, forking new
// weft branches when their suffixed siblings don't exist, re-pointing junctions, and refreshing the
// correspondence index — rolling back both sides on failure to preserve all-or-nothing semantics.
// A weft that carries a move record, a weft whose own anchor marker is empty, and a target weft branch anchored elsewhere than the worktree, are refused with an error wrapping ErrAnchorChange before anything is switched.
func (t *Topology) Checkout(l *lymcwd.Location, branch string) (res CheckoutResult, err error) {
	rec := NewMutations(l.HubPath)
	defer func() { res.Mutations = rec.Snapshot() }()

	weftWorktree := RecordsWorktree(l)

	// A blank marker is refused ahead of the dirtiness check it would otherwise trip, so its own way forward is the one named.
	if stop := emptyMarkerStop(weftWorktree); stop != nil {
		return CheckoutResult{}, fmt.Errorf("%w: %s", stop, stop.WayForward)
	}

	// A standing move record is refused too: its re-run recovers against the branch the record was written on, which a switch would leave behind.
	moveStop, err := moveRecordStop(weftWorktree)
	if err != nil {
		return CheckoutResult{}, err
	}
	if moveStop != nil {
		return CheckoutResult{}, fmt.Errorf("%w: %s", moveStop, moveStop.WayForward)
	}

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

	// A target weft branch anchored elsewhere would leave the pair's wiring and its worktree's anchor apart, so it is refused before anything is switched.
	if err := refuseTargetAnchorChange(l, branch); err != nil {
		return CheckoutResult{}, err
	}

	// Capture both original branches for rollback on later failure.
	originalBranch, err := readBranch(l.WorktreePath())
	if err != nil {
		return CheckoutResult{}, fmt.Errorf("capture warp branch: %w", err)
	}

	// The weft branch capture is best-effort: a detached weft HEAD (readBranch's "HEAD") has no branch name to switch back to.
	// rollbackSwitch simply skips the weft side in that abnormal case, matching the best-effort posture of the rollback as a whole.
	originalWeftBranch := ""
	if b, err := readBranch(weftWorktree); err == nil && b != "HEAD" {
		originalWeftBranch = b
	}

	// The pre-flight refusals have passed: every exit from here, a rollback included, regenerates the hub workspace file.
	defer regenerateAfterVerb(rec, l, "checkout")

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
	// A branch the call created (forked, or adopted from origin) is deleted on rollback, also when the weft switch itself failed after the branch was adopted.
	weftCreated, err := t.switchOrForkWeft(rec, l, branch)
	createdWeftBranch := ""
	if weftCreated {
		createdWeftBranch = RecordsBranchName(branch)
	}
	if err != nil {
		t.rollbackSwitch(rec, l, originalBranch, originalWeftBranch, createdWeftBranch)
		return CheckoutResult{}, err
	}

	// Re-point junctions; roll back both sides on failure (weft already switched).
	names, err := RepoWiredNames(l)
	if err != nil {
		t.rollbackSwitch(rec, l, originalBranch, originalWeftBranch, createdWeftBranch)
		return CheckoutResult{}, fmt.Errorf("re-point junctions: load fabric config: %w", err)
	}
	if err := WireJunctionsWith(rec, l, slug, names); err != nil {
		t.rollbackSwitch(rec, l, originalBranch, originalWeftBranch, createdWeftBranch)
		return CheckoutResult{}, fmt.Errorf("re-point junctions: %w", err)
	}

	// Refresh correspondence index for the new branch; switch is complete regardless of failure.
	if err := refreshCorrIndexAfterSwitch(l.WorktreePath(), weftWorktree); err != nil {
		return CheckoutResult{}, fmt.Errorf("checkout to %q completed, but refreshing the correspondence index failed (re-run `lym fabric checkout` to retry): %w", branch, err)
	}

	return CheckoutResult{
		Branch:          branch,
		RecordsWorktree: weftWorktree,
	}, nil
}

// refuseTargetAnchorChange refuses a checkout of branch when the weft branch it would switch to carries another anchor than the worktree's own.
// The target's tip is read from the local branch, else from origin, whose tip is fetched into its remote-tracking ref only so that no local branch is created.
// A target that exists nowhere is forked from the current weft branch and inherits its marker, so it is never refused.
// An unreachable origin is the error the switch itself would hit, raised before anything is switched.
// The refusal wraps ErrAnchorChange and names the commands that settle it with the cost of each:
// the anchor change commits the new marker on the branch being left, which every machine that checks it out shares,
// and an anchor directory the current warp checkout does not track is first brought in by a code commit on that same branch.
func refuseTargetAnchorChange(l *lymcwd.Location, branch string) error {
	weftBranch := RecordsBranchName(branch)
	weftRepoRoot, err := RecordsRepoRoot(l)
	if err != nil {
		return fmt.Errorf("resolve the weft repo to read %q: %w", weftBranch, err)
	}

	tip, err := targetWeftTip(weftRepoRoot, weftBranch)
	if err != nil || tip == "" {
		return err
	}
	names, err := RepoWiredNames(l)
	if err != nil {
		return fmt.Errorf("load fabric config to compare anchors: %w", err)
	}
	targetAnchor, err := treeAnchor(weftRepoRoot, tip, names, l.AnchorRel)
	if err != nil {
		return err
	}
	if targetAnchor == l.AnchorRel {
		return nil
	}

	tracked := targetAnchor == "."
	if !tracked {
		listed, err := gitexec.Run([]string{"ls-tree", "-d", "HEAD", "--", filepath.ToSlash(targetAnchor)}, l.WorktreePath())
		if err != nil {
			return fmt.Errorf("check whether the warp checkout tracks %q: %w", targetAnchor, err)
		}
		tracked = strings.TrimSpace(listed) != ""
	}
	steps := fmt.Sprintf("run `lym fabric anchor --set %s`, which commits the new marker on the weft branch being left, shared by every machine that checks it out, so their pulls and pushes then stop until each integrates it; then run the checkout again", targetAnchor)
	if !tracked {
		steps = fmt.Sprintf("first update this warp checkout by hand to a commit that tracks %s, such as merging the target warp branch %s into the current one or committing the %s directory on it, which is a code commit on the branch being left, shared by every machine that checks it out; then %s",
			targetAnchor, branch, targetAnchor, steps)
	}
	return fmt.Errorf("%w: checkout of %q refused, since its weft branch %s is anchored at %q and this worktree at %q; %s",
		ErrAnchorChange, branch, weftBranch, targetAnchor, l.AnchorRel, steps)
}

// targetWeftTip returns a ref naming the tip of weftBranch as a checkout would switch to it, or "" when it exists nowhere and a checkout would fork it.
// A local branch is that ref.
// Otherwise a branch on origin is fetched into its remote-tracking ref, which is returned, and no local branch is created.
// A repo with no origin remote has no such branch.
func targetWeftTip(weftRepoRoot, weftBranch string) (string, error) {
	local, err := gitrepo.New(weftRepoRoot).BranchExists(weftBranch)
	if err != nil {
		return "", fmt.Errorf("check whether weft branch %q exists: %w", weftBranch, err)
	}
	if local {
		return "refs/heads/" + weftBranch, nil
	}
	if _, urlErr := gitrepo.New(weftRepoRoot).RemoteURL(originRemoteName); urlErr != nil {
		return "", nil
	}
	tip, err := remoteHeadTip(weftRepoRoot, weftBranch)
	if err != nil || tip == "" {
		return "", err
	}
	tracking := "refs/remotes/" + originRemoteName + "/" + weftBranch
	if _, err := gitexec.Run([]string{"fetch", "--no-tags", originRemoteName, "refs/heads/" + weftBranch + ":" + tracking}, weftRepoRoot); err != nil {
		return "", fmt.Errorf("fetch weft branch %q from %q: %w", weftBranch, originRemoteName, err)
	}
	return tracking, nil
}

// switchOrForkWeft switches the weft worktree to the weft branch matching the warp target: the local branch, else the branch on origin adopted as a local tracking branch, else a fork of the current weft branch.
// It reports whether this call created the local branch (adopted from origin or forked),
// so rollback can delete it;
// created stays true alongside an error when the weft switch fails after the branch was adopted from origin.
// rec is Checkout's own recorder;
// it records KindWorktreeSwitched at the weft worktree root with the branch switched to as Detail on every path, and additionally records KindBranchCreated for a created branch: from the resolver when adopted from origin, here when forked, since `switch -c` creates it.
// An unreachable origin is an error and forks nothing.
func (t *Topology) switchOrForkWeft(rec *Mutations, l *lymcwd.Location, branch string) (created bool, err error) {
	weftWorktree := RecordsWorktree(l)
	weftBranch := RecordsBranchName(branch)

	exists, fromOrigin, err := resolveWeftBranch(rec, l, weftBranch, true)
	if err != nil {
		return false, err
	}
	if exists {
		if _, err := gitexec.Run(
			[]string{"switch", weftBranch},
			weftWorktree,
		); err != nil {
			return fromOrigin, fmt.Errorf("weft switch to branch %q failed: %w", weftBranch, err)
		}
		rec.Append(KindWorktreeSwitched, weftWorktree, weftBranch)
		return fromOrigin, nil
	}

	// Branch does not exist: fork from current weft HEAD to preserve merge-base.
	parentWeftBranch, err := readBranch(weftWorktree)
	if err != nil {
		return false, fmt.Errorf("capture parent weft branch: %w", err)
	}

	// Create and switch to the new weft branch.
	if _, err := gitexec.Run(
		[]string{"switch", "-c", weftBranch, parentWeftBranch},
		weftWorktree,
	); err != nil {
		return false, fmt.Errorf("fork weft branch %q from %q failed: %w", weftBranch, parentWeftBranch, err)
	}
	rec.Append(KindWorktreeSwitched, weftWorktree, weftBranch)
	rec.AppendRef(KindBranchCreated, weftBranch, refDetail("records", weftWorktree, ""))

	return true, nil
}

// rollbackSwitch switches both warp and weft back to their original branches on failure, cleaning up any weft branch the checkout created (forked, or adopted from origin; only the local branch is deleted), with errors silently discarded.
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
func (t *Topology) rollbackSwitch(rec *Mutations, l *lymcwd.Location, originalBranch, originalWeftBranch, forkedWeftBranch string) {
	if _, err := gitexec.Run([]string{"switch", originalBranch}, l.WorktreePath()); err == nil {
		rec.Append(KindWorktreeSwitched, l.WorktreePath(), originalBranch)
	}
	if originalWeftBranch != "" {
		if _, err := gitexec.Run([]string{"switch", originalWeftBranch}, RecordsWorktree(l)); err == nil {
			rec.Append(KindWorktreeSwitched, RecordsWorktree(l), originalWeftBranch)
		}
	}
	if forkedWeftBranch != "" {
		req := branchRequest{
			what:      "delete forked weft branch",
			repoDir:   RecordsWorktree(l),
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
