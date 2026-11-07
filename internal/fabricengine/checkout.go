// checkout.go implements the coordinated switch of the warp and weft worktrees to another branch.
// The switch is all-or-nothing: a failure on either side rolls both sides back, so the pair is never left half-switched.

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

// CheckoutResult describes a completed switch of the warp and weft worktrees.
// It embeds MutationRecord, which carries the mutations recorded over the call.
type CheckoutResult struct {
	MutationRecord
	// Branch is the warp branch the warp worktree now points to,
	// and the weft worktree points to RecordsBranchName(Branch).
	Branch string `json:"branch"`
	// RecordsWorktree is the filesystem path to the weft sibling worktree.
	RecordsWorktree string `json:"records_worktree"`
}

// Checkout switches the warp worktree to branch and its weft sibling to RecordsBranchName(branch), all or nothing.
// Before anything is switched, it refuses when the weft worktree has uncommitted changes or a move record.
// It refuses an empty anchor marker, a half-merged pair, or a target weft branch anchored elsewhere than the worktree.
// The anchor refusal wraps ErrAnchorChange.
// It forks a missing weft branch from its parent, re-points the junctions, and refreshes the correspondence index.
// A failure after the first switch rolls both sides back to their original branches.
func (t *Topology) Checkout(l *lymcwd.Location, branch string) (res CheckoutResult, err error) {
	rec := NewMutations(l.HubPath)
	defer func() { res.Mutations = rec.Snapshot() }()

	weftWorktree := RecordsWorktree(l)

	// The empty-marker refusal comes first: an empty marker would otherwise trip the dirtiness check below,
	// and the refusal names its own way forward.
	if stop := emptyMarkerStop(weftWorktree); stop != nil {
		return CheckoutResult{}, fmt.Errorf("%w: %s", stop, stop.WayForward)
	}

	// A standing move record is refused too:
	// its re-run recovers against the branch the record was written on, and a switch would leave that branch behind.
	moveStop, err := moveRecordStop(weftWorktree)
	if err != nil {
		return CheckoutResult{}, err
	}
	if moveStop != nil {
		return CheckoutResult{}, fmt.Errorf("%w: %s", moveStop, moveStop.WayForward)
	}

	// A coordinated branch switch out of a half-merged pair is refused;
	// the foreign-state disposition belongs to Commit alone, so this check stays record-only.
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

	// A target weft branch anchored elsewhere would leave the pair's wiring and the worktree's anchor apart,
	// so it is refused before anything is switched.
	if err := refuseTargetAnchorChange(l, branch); err != nil {
		return CheckoutResult{}, err
	}

	// Capture both original branches for rollback on later failure.
	originalBranch, err := readBranch(l.WorktreePath())
	if err != nil {
		return CheckoutResult{}, fmt.Errorf("capture warp branch: %w", err)
	}

	// The weft branch capture is best-effort.
	// A detached weft HEAD, which readBranch reports as "HEAD", has no branch to switch back to.
	// rollbackSwitch then skips the weft side, matching the best-effort posture of the rollback as a whole.
	originalWeftBranch := ""
	if b, err := readBranch(weftWorktree); err == nil && b != "HEAD" {
		originalWeftBranch = b
	}

	// The pre-flight refusals have passed: every exit from here, a rollback included, regenerates the hub workspace file.
	defer regenerateAfterVerb(rec, l, "checkout")

	// git's own stderr is carried into the error: the actionable half of a failure is invariably in it,
	// and a bare exit code leaves nothing to act on.
	if _, err := gitexec.Run(
		[]string{"switch", branch},
		l.WorktreePath(),
	); err != nil {
		return CheckoutResult{}, fmt.Errorf("warp switch to branch %q failed: %w", branch, err)
	}
	rec.Append(KindWorktreeSwitched, l.WorktreePath(), branch)

	// The weft side is switched next; a failure there rolls the warp switch back.
	slug := filepath.Base(l.WorktreePath())
	// A branch the call created, by forking or by adopting it from origin, is deleted on rollback.
	// This holds also when the weft switch fails after the adoption.
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

	// Refresh the correspondence index for the new branch; a failure here does not undo the switch.
	if err := refreshCorrIndexAfterSwitch(l.WorktreePath(), weftWorktree); err != nil {
		return CheckoutResult{}, fmt.Errorf("checkout to %q completed, but refreshing the correspondence index failed (re-run `lym fabric checkout` to retry): %w", branch, err)
	}

	return CheckoutResult{
		Branch:          branch,
		RecordsWorktree: weftWorktree,
	}, nil
}

// refuseTargetAnchorChange refuses a checkout of branch when its weft branch carries another anchor than the worktree.
// The target's tip is read from the local branch, or else from origin.
// An origin tip is fetched into its remote-tracking ref, so no local branch is created.
// A target that exists nowhere is forked from the current weft branch and inherits its marker, so it is never refused.
// An unreachable origin is the error the switch itself would hit, raised before anything is switched.
// The refusal wraps ErrAnchorChange and names the commands that settle it, with the cost of each.
// The anchor change commits the new marker on the branch being left, which every machine that checks it out shares.
// An anchor directory the current warp checkout does not track is first brought in by a code commit on that same branch.
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

// targetWeftTip returns a ref naming the tip of weftBranch as a checkout would switch to it.
// It returns "" when the branch exists nowhere, since a checkout would fork it.
// A local branch is that ref.
// Otherwise a branch on origin is fetched into its remote-tracking ref, which is returned,
// and no local branch is created.
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
	if _, err := gitrepo.New(weftRepoRoot).FetchWithArgs("--no-tags", originRemoteName, "refs/heads/"+weftBranch+":"+tracking); err != nil {
		return "", fmt.Errorf("fetch weft branch %q from %q: %w", weftBranch, originRemoteName, err)
	}
	return tracking, nil
}

// switchOrForkWeft switches the weft worktree to the weft branch matching branch.
// It uses the local branch when one exists;
// otherwise it adopts the branch on origin as a local tracking branch;
// otherwise it forks a new branch from the weft worktree's current branch.
// It reports whether this call created the local branch by adopting or forking it, so that a rollback can delete it.
// created stays true alongside an error when the weft switch fails after the branch was adopted from origin.
// A successful switch appends KindWorktreeSwitched to rec at the weft worktree root, with the branch as Detail;
// a forked branch also appends KindBranchCreated, and so does an adoption from origin.
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

	// The branch does not exist, so fork it from the weft worktree's current branch to keep the merge-base.
	parentWeftBranch, err := readBranch(weftWorktree)
	if err != nil {
		return false, fmt.Errorf("capture parent weft branch: %w", err)
	}

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

// rollbackSwitch switches the warp and weft checkouts back to their original branches on failure.
// It discards the errors of both switch calls, since it runs on a path that already failed.
// The junction stays consistent without rewiring, because the worktree directory path does not change.
//
// It deletes forkedWeftBranch through the destructive gate.
// forkedWeftBranch is the weft branch this checkout created, by forking or by adopting it from origin;
// only the local branch is deleted.
// A refusal by the gate is logged with logger.Warn, since this function returns nothing.
// rec collects the mutation records in order:
// one KindWorktreeSwitched per successful switch, then the records of the deletion.
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
			ownership: ownedManagedBranch(l),
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
