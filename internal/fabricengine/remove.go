// remove.go implements Remove: every refusal — slug, prime, target-exists, merge-in-progress and the no-force dirtiness checks with their status probes — runs first, through the read-only probe RemoveRefusal, and leaves the hub and the weft origin untouched.
// Then the sibling's pending records are committed (commitPendingRecords) and the weft tip is tagged locally for its archive.
// Only then are the launchers and worktrees torn down, so a refused call never loses a launcher or tags a tip that misses uncommitted records.
// The pushes of the weft tip and the tag, and every delete on origin, run last in the detached teardown child.
// The weft branch it removes is RecordsBranchName(warpBranch).
// After both worktrees are gone it also deletes the pair's local warp branch, but only when the
// destructive gate proves no work is lost: every commit is on another ref or already landed on the
// pair's recorded parent — see deleteWarpBranch. A branch that fails the gate is kept with a reason.
// Its link sweep is anchored and ownership-filtered — see the sweep's own comment for why reading
// the worktree root and trusting link-ness alone was wrong on both hub geometries.
// It never deletes a directory git declined to remove unless that directory is a registered LINKED
// worktree of this repo — see removeWarpWorktreeDir for the data-loss this rule exists to prevent.

package fabricengine

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
)

// ErrPairSiblingDirty is the sentinel Remove's no-force refusal wraps when the pair's other worktree carries uncommitted changes outside the record pathspec.
// Changes inside the pathspec never raise it: Remove commits them before the archive.
// It is worded without naming either side of the pair so callers outside the fabric vocabulary owner set can match it with errors.Is and offer their own remedy.
var ErrPairSiblingDirty = errors.New("the pair's sibling worktree has uncommitted changes")

// siblingDirtyRefusal carries the refusal text unchanged while unwrapping to ErrPairSiblingDirty.
type siblingDirtyRefusal struct{ msg string }

func (e siblingDirtyRefusal) Error() string { return e.msg }

func (e siblingDirtyRefusal) Unwrap() error { return ErrPairSiblingDirty }

// ErrPairNotFound is the sentinel Remove's error wraps when nothing of the pair remains: no worktree on either side, no branch locally or on origin, and no launcher entry.
// It is worded without naming either side of the pair so callers outside the fabric vocabulary owner set can match it with errors.Is.
var ErrPairNotFound = errors.New("pair not found")

// ErrStrayPathKept is the sentinel an error wraps when a directory that is no worktree stays at a pair's task worktree path after the pair's removal, because deleting it failed.
// The pair itself is gone; the directory blocks the slug's next pair until it is deleted.
var ErrStrayPathKept = errors.New("a directory that is no worktree stays at the pair's task worktree path")

// The steps a Remove call reports in RemoveResult.Steps, a closed set; each is appended only after it changed state.
// The two origin steps are the teardown child's, so they are listed only when its steps ran in process.
const (
	// RemoveStepRecordCommit is the commit of the sibling's pending records.
	RemoveStepRecordCommit = "record_commit"
	// RemoveStepArchive is the archive tag made on the local weft tip before the teardown; its push to origin is the teardown child's.
	RemoveStepArchive = "archive"
	// RemoveStepLaunchers is the removal of the pair's launcher directory.
	RemoveStepLaunchers = "launchers"
	// RemoveStepTaskWorktree is the removal of the task worktree.
	RemoveStepTaskWorktree = "task_worktree"
	// RemoveStepSiblingWorktree is the removal of the sibling worktree.
	RemoveStepSiblingWorktree = "sibling_worktree"
	// RemoveStepSiblingBranch is the deletion of the sibling's local branch.
	RemoveStepSiblingBranch = "sibling_branch"
	// RemoveStepTaskBranch is the deletion of the task's local branch.
	RemoveStepTaskBranch = "task_branch"
	// RemoveStepSiblingBranchOnOrigin is the deletion of the sibling's branch on origin.
	RemoveStepSiblingBranchOnOrigin = "sibling_branch_on_origin"
	// RemoveStepTaskBranchOnOrigin is the deletion of the task's branch on origin.
	RemoveStepTaskBranchOnOrigin = "task_branch_on_origin"
)

// RemoveResult contains the result of successfully removing a worktree pair.
// It embeds MutationRecord, which carries the mutation record accumulated over the call.
type RemoveResult struct {
	MutationRecord
	Slug         string `json:"slug"`
	Path         string `json:"path"`
	LinksRemoved int    `json:"links_removed"`
	// Steps lists the steps this call performed, in order, from the RemoveStep constants; always an array.
	// A teardown child's steps are listed only when they ran in process, so RemoveStepArchive then stands for the local tag alone.
	Steps []string `json:"steps"`
	// Finished is true when the task worktree was already gone at entry, so the call completed a half-removed pair.
	Finished bool `json:"finished"`
	// StrayPath names a path at the task worktree's location that is not a registered linked worktree:
	// one found there at entry, which Remove never deletes, or a directory a late writer refilled after the task worktree's removal and that a last removal could not delete, which Remove also returns as an error wrapping ErrStrayPathKept.
	StrayPath string `json:"stray_path,omitempty"`
	// StrayPathDeleted is true when a caller's separate delete of a directory that is no worktree at the task worktree's path removed one; Remove itself never sets it.
	StrayPathDeleted bool `json:"stray_path_deleted,omitempty"`
	// RemoteBranchDeleted reports whether the pair's weft branch was observably removed from the
	// remote. It is true only when the remote deletion was attempted and actually removed a ref.
	RemoteBranchDeleted bool `json:"remote_branch_deleted,omitempty"`
	// RemoteBranchError is non-empty when the remote deletion was attempted and did not succeed. Its
	// text always names the layer that said no — the gate's own refusal, or the remote deletion
	// itself.
	RemoteBranchError string `json:"remote_branch_error,omitempty"`
	// RemoteBranchKeptReason is non-empty when the pair's weft branch on the remote was left in place because it moved after the archive read its tip.
	// A kept branch is another process's live branch, not a failure of Remove.
	RemoteBranchKeptReason string `json:"remote_branch_kept_reason,omitempty"`
	// RemoteSkippedReason carries a once-per-verb reason the sibling branch's remote copy was not deleted and no deletion was attempted:
	// with remote, a weft repo with no origin remote configured;
	// without remote, a sibling branch left only on origin, which is kept.
	RemoteSkippedReason string `json:"remote_skipped_reason,omitempty"`
	// CodeBranchDeleted reports whether the pair's local warp branch was deleted after the teardown.
	CodeBranchDeleted bool `json:"code_branch_deleted"`
	// CodeBranchKeptReason is non-empty when the warp branch was left in place, naming why: the
	// destructive gate's refusal, or a failure to delete it. A kept branch is not a failure of Remove.
	CodeBranchKeptReason string `json:"code_branch_kept_reason,omitempty"`
	// RemoteCodeBranchDeleted reports whether the pair's task branch was observably removed from the warp repo's origin.
	// It is true only with remote, when the branch's work was landed.
	RemoteCodeBranchDeleted bool `json:"remote_code_branch_deleted"`
	// RemoteCodeBranchKeptReason is non-empty when remote was set and the task branch on origin was left in place, naming why: the gate's refusal or a failure to look it up or delete it.
	// A kept branch is not a failure of Remove.
	RemoteCodeBranchKeptReason string `json:"remote_code_branch_kept_reason,omitempty"`
	// ArchiveTag names the archive/<slug>/<tip> tag made on the local weft tip before the teardown, whose push to the weft origin is the teardown child's;
	// empty when none was made.
	ArchiveTag string `json:"archive_tag,omitempty"`
	// OriginArchiveTag names the second archive tag, at origin's weft tip, pushed when that tip is neither the local tip nor an ancestor of it; empty when none was needed.
	OriginArchiveTag string `json:"origin_archive_tag,omitempty"`
	// ArchiveSkippedReason is non-empty when no archive was attempted — today only a weft repo with no origin remote configured.
	ArchiveSkippedReason string `json:"archive_skipped_reason,omitempty"`
	// RemoteHandedOff is true when the remote steps went to the detached teardown child, which runs them after Remove returns; the remote outcome fields then stay empty.
	RemoteHandedOff bool `json:"remote_handed_off,omitempty"`
	// SetAsideTag names the local set-aside/<slug>/<tip> tag a local removal put on the weft tip in place of an archive tag; empty when none was put.
	SetAsideTag string `json:"set_aside_tag,omitempty"`
	// WorkspaceSkipped is true when a local removal found the hub workspace lock held and left the workspace file stale.
	WorkspaceSkipped bool `json:"workspace_skipped,omitempty"`
	// SnapshotTag names the snapshot tag that kept the pair's uncommitted paths before a forced local removal; empty when none was taken.
	SnapshotTag string `json:"snapshot_tag,omitempty"`
	// SnapshotPaths lists the paths the snapshot tag holds.
	SnapshotPaths []string `json:"snapshot_paths,omitempty"`
}

// Remove removes a paired warp and weft git worktree with all associated artifacts.
// If force is false, both worktrees must be clean;
// if force is true, uncommitted changes are forcefully removed.
// It validates slug through the same validator Add uses (slug.go), so hub geometry — the Bolt checkout, `_portals`, `_launchers`, `_lym`, `.lym`, and every weft sibling — can never be handed to a teardown verb as if it were a pair.
// It refuses the hub's prime worktree outright too: the prime is the warp repository itself, not a
// pair this verb can tear down, and git's own refusal to remove a main working tree is not a
// licence to delete the clone.
// Its no-force dirtiness checks, status probes included, are refusals like the rest: a refusal leaves the hub and the weft origin untouched, with no tag pushed and nothing torn down.
// The sibling's dirtiness inside the record pathspec is no refusal: Remove commits it first, with or without force, so the archive covers it.
// After every refusal and before its first teardown mutation, Remove tags the pair's local weft tip archive/<slug>/<tip>, reported in ArchiveTag, so the run records on the branch outlive its deletion.
// A failed tag returns its error, wrapping ErrArchiveFailed, with everything still in place, so a plain re-run retries it.
// The tag is taken whatever remote says, and force does not skip it: force answers dirtiness only.
// A weft repo with no origin proceeds untagged, with ArchiveSkippedReason set on the result.
// Launcher cleanup runs after the tag but before the git removal.
// Once both worktrees are removed, Remove deletes the local warp branch (BranchPrefix + slug) through
// the destructive gate, which refuses unless the branch's work is on another ref or landed on the
// parent recorded in the pair's origin record; a refusal fills CodeBranchKeptReason and Remove still
// succeeds. force never answers that check.
// The remote steps run last, in a detached child that holds the pair's teardown lock, and Remove returns without waiting on them, reporting RemoteHandedOff.
// In order, the child pushes the local weft tip to origin when origin lacks it, a refusal only logged;
// pushes the archive tag, and a second tag at origin's tip when that tip is neither the local tip nor an ancestor of it;
// with remote, deletes the weft branch on origin, leased on the origin tip it read or on the local tip once its push landed, keeping a branch origin moved since;
// and with remote, deletes the task branch on origin, through the landed-work gate and a lease on the observed remote tip.
// A failed remote step is logged and mailed to the hub's orch with its way forward; a kept branch is no failure.
// In a test binary the remote steps run in process before Remove returns, and fill ArchiveTag, OriginArchiveTag, the RemoteBranch and RemoteCodeBranch fields and their steps, an archive failure returned as the error.
// The recorded parent is read before any teardown, since the record lives in the weft worktree the
// teardown deletes.
// A pair whose task worktree is already gone is finished rather than refused: Remove performs whatever teardown remains, in the same order and through the same gates, and reports it in Steps with Finished set.
// With nothing of the pair left it returns an error wrapping ErrPairNotFound.
// A sibling branch present only on origin is archived from there by the child and, with remote, deleted there; without remote it stays.
// A path at the task worktree's location that is not a registered linked worktree is reported in StrayPath and never deleted.
// A directory a late writer, such as a build server or a session's exit hooks, refills at the task worktree's path after its removal is removed once more after the sibling's teardown;
// one that cannot be is reported in StrayPath, and Remove finishes the teardown and then returns an error wrapping ErrStrayPathKept that names the directory.
// After the refusals and the pair inspection, and before its first mutation, Remove has the teardown child take the pair's teardown lock exclusive, waiting at most the topology's teardown wait for a push of the pair to let go of it;
// when the wait runs out it returns, before any mutation, an error wrapping ErrPairPushRunning.
// The child holds the lock past the remote steps, and a failure after taking it that leaves a worktree of the pair in place releases the lock and respawns the pair's push.
// After a successful teardown, and before it returns, Remove disables auto gc in both hub stores and runs a best-effort foreground `git gc --auto` in each;
// a failure there is logged and never fails the Remove, and a refused or failed Remove runs none of it.
func (t *Topology) Remove(l *lymcwd.Location, slug string, force, remote bool) (res RemoveResult, err error) {
	return t.removePair(l, slug, force, remote, false)
}

// RemoveLocal removes a paired warp and weft git worktree the way Remove does, without any origin step and without waiting on another writer.
// It makes the same refusals as Remove, with force answering dirtiness only.
// A held teardown lock or weft write lock returns an error wrapping ErrPairBusy with nothing changed.
// It commits the sibling's pending records, then puts a local set-aside/<slug>/<sha> tag on the weft tip, reported in SetAsideTag, in place of Remove's pushed archive tag.
// It runs no ls-remote, respawns no push after a failure, skips the store housekeeping and regenerates the hub workspace file by one attempt at its lock, reporting WorkspaceSkipped when the lock is held.
// The commits the pushed archive tag would have carried are kept under the local tag, and the removal's branch deletions keep their gates.
func (t *Topology) RemoveLocal(l *lymcwd.Location, slug string, force bool) (res RemoveResult, err error) {
	return t.removePair(l, slug, force, false, true)
}

// removePair is Remove's body, with localMode switching off every origin step and every wait on another writer; see RemoveLocal for the mode's contract.
func (t *Topology) removePair(l *lymcwd.Location, slug string, force, remote, localMode bool) (res RemoveResult, err error) {
	rec := NewMutations(l.HubPath)
	defer func() { res.Mutations = rec.Snapshot() }()

	warpBranch := t.cfg.BranchPrefix + slug
	weftBranch := RecordsBranchName(warpBranch)

	// Every refusal runs here, through the same probe a caller can ask without removing anything, so the two can never drift.
	if err := t.RemoveRefusal(l, slug, force); err != nil {
		return RemoveResult{}, err
	}

	target := WorktreePath(l, slug)
	pair, err := t.inspectPair(l, slug)
	if err != nil {
		return RemoveResult{}, err
	}
	// Read before the teardown deletes the records worktree that holds the pair's own marker.
	pairLoc, err := lymcwd.NamedWorktree(l.HubPath, slug)
	if err != nil {
		return RemoveResult{}, fmt.Errorf("read the anchor of %q: %w", slug, err)
	}
	steps := []string{}

	// Registered before the teardown lock, so it runs after the lock's release on every exit past the refusals.
	if localMode {
		defer func() { res.WorkspaceSkipped = regenerateHubWorkspaceNow(rec, l) }()
	} else {
		defer regenerateAfterVerb(rec, l, "remove")
	}

	// The pair's detached pushes hold this lock shared, so no push runs against a worktree this call deletes.
	// A failure after this point that leaves a worktree behind respawns the pair's push once the lock is released, since a child may have exited on the held lock.
	// A local removal holds the lock itself and respawns nothing: the surviving worktree's commits stay local for the next reconcile.
	hold, err := t.holdTeardown(rec, rec, l, slug, localMode, fmt.Sprintf("re-run `lym fabric remove %s`", slug), !localMode)
	if err != nil {
		return RemoveResult{}, err
	}
	handedOver := false
	defer func() {
		if !handedOver {
			hold.finish(nil)
		}
		// A directory kept at the task worktree's path is no worktree, so it leaves nothing to push.
		if err != nil && !errors.Is(err, ErrStrayPathKept) && !localMode && (pathPresent(target) || pathPresent(RecordsWorktreePath(l, slug))) {
			respawnPairPush(l, slug)
		}
	}()

	if removeAfterLockHook != nil {
		if err := removeAfterLockHook(); err != nil {
			return RemoveResult{}, err
		}
	}

	// Read the recorded parent now: the record lives in the weft worktree the teardown deletes. A
	// missing or unreadable record leaves parentBranch empty, which asks the gate for the stricter
	// reachability check alone.
	parentBranch := ""
	if origin, found, originErr := ReadOriginFor(l, slug); originErr == nil && found {
		parentBranch = origin.ParentBranch
	}

	weftTip, archiveTag, archiveSkippedReason, setAsideTagName := "", "", "", ""
	// Commit the sibling's pending records so the archive tag covers them; force answers dirtiness only, so it never skips this step either.
	committed, err := commitPendingRecords(rec, l, slug, warpBranch, localMode)
	if err != nil {
		return RemoveResult{}, err
	}
	if committed {
		steps = append(steps, RemoveStepRecordCommit)
	}

	if localMode {
		// The weft tip is kept under a local tag, since no origin step may run; the tag covers what the pushed archive tag would have.
		setAsideTagName, err = setAsideWeftTip(rec, l, slug, weftBranch, pair.siblingBranch)
		if err != nil {
			return RemoveResult{}, err
		}
	} else {
		// Tag the weft tip after every refusal and before the first teardown mutation: a failed tag leaves the worktrees, launchers and both branches untouched, so a plain re-run retries it.
		// force answers dirtiness only, so it never skips this step; the tag's push is the teardown child's.
		weftTip, archiveTag, archiveSkippedReason, err = tagWeftTipForArchive(l, slug, weftBranch)
		if err != nil {
			return RemoveResult{}, err
		}
		if archiveTag != "" {
			steps = append(steps, RemoveStepArchive)
		}
	}

	// removeLaunchers is best-effort: an operational failure is discarded exactly as
	// before, but a gate refusal must surface rather than vanish at the verb the slice's worst defect
	// came from.
	if err := surfaceRefusal(removeLaunchers(rec, pairLoc, slug)); err != nil {
		return RemoveResult{}, err
	}
	if pair.launchers && !pathPresent(LauncherDir(pairLoc, slug)) {
		steps = append(steps, RemoveStepLaunchers)
	}

	// Sweep the ANCHORED directory, and only the links fabric itself created there.
	// The previous sweep read the worktree ROOT and removed every symlink it found: on a
	// subpath-anchored hub that saw none of the pair's junctions (they live at
	// <worktree>/<anchorRel>) and reported LinksRemoved: 0, and at a root anchor it deleted the
	// user's own checked-in symlinks alongside fabric's.
	// The root scratch link is excepted from the sweep: it is not an anchored junction, so unlinkScratchLink removes it before the worktree directory removal.
	// A task worktree that is gone, or a path there that is not a registered linked worktree, has no junctions of fabric's to sweep and nothing for the directory removal to take.
	linksRemoved := 0
	if pair.taskWorktree {
		if scannedNames, scanErr := scanOnDiskJunctionNames(pairLoc, slug); scanErr == nil {
			ownedNames := withoutRootScratchLink(pairLoc, scannedNames)
			removeErr := removeWarpJunction(rec, pairLoc, slug, ownedNames)
			if err := surfaceRefusal(removeErr); err != nil {
				return RemoveResult{}, err
			}
			if removeErr == nil {
				linksRemoved = len(ownedNames)
			}
		}
		if err := surfaceRefusal(unlinkScratchLink(rec, pairLoc, slug)); err != nil {
			return RemoveResult{}, err
		}
		if err := removeWarpWorktreeDir(rec, l, target, force, t.leftover); err != nil {
			return RemoveResult{}, err
		}
		if !pathPresent(target) {
			steps = append(steps, RemoveStepTaskWorktree)
		}
	} else {
		// A worktree removed by hand stays registered, and git refuses to delete a branch checked out at a registered worktree.
		// Best-effort, like the prune after the fallback removal: a failed prune surfaces as the branch gate's own refusal.
		_, _ = gitexec.Run([]string{"worktree", "prune"}, l.WorktreePath())
	}

	// A sibling with neither worktree nor branch could only be found on origin, which only the teardown child asks.
	siblingSteps, err := t.removeSibling(rec, l, slug, weftBranch, pair, force)
	if err != nil {
		return RemoveResult{}, err
	}
	steps = append(steps, siblingSteps...)

	if removeAfterTeardownHook != nil {
		removeAfterTeardownHook()
	}

	// A late writer can refill the task worktree's directory after its removal finished, and a directory left there blocks the slug's next pair.
	// This call's own removal of the registered worktree is the ownership proof, as it is for the finish right after git.
	var strayErr error
	if pair.taskWorktree && pathPresent(target) && !isRegisteredLinkedWorktree(l, target) {
		if leftoverErr := removeWorktreeLeftover(rec, l.HubPath, target, t.leftover); leftoverErr != nil {
			logger.Warn("fabricengine: a directory refilled at a removed task worktree's path was not deleted", "slug", slug, "path", target, "error", leftoverErr)
			strayErr = fmt.Errorf("%w: the pair of %q is removed, but %s was refilled after its removal and deleting it failed: %w; way forward: stop whatever writes into it, such as a build or language server, then delete it",
				ErrStrayPathKept, slug, target, leftoverErr)
		}
	}

	warpDeleted, warpKeptReason := deleteWarpBranch(rec, l, warpBranch, parentBranch)
	if warpDeleted {
		steps = append(steps, RemoveStepTaskBranch)
	}

	for _, side := range []struct{ repo, branch string }{{failedPushSideCode, warpBranch}, {failedPushSideRecords, weftBranch}} {
		if err := ClearPushFailure(l.HubPath, side.repo, side.branch); err != nil {
			logger.Warn("fabricengine: failed-push record of a removed pair not cleared", "slug", slug, "side", side.repo, "error", err)
		}
	}

	// Both worktrees are gone: reclaim the stores' unreferenced objects while lym is already doing slow work.
	// A refused or failed Remove returned above and runs no housekeeping, and a local removal runs none either.
	if !localMode {
		if housekeepHook != nil {
			housekeepHook(slug)
		}
		housekeepStores(l, slug, t.inFlight)
	}

	strayPath := ""
	if pair.strayPath || strayErr != nil {
		strayPath = target
	}
	res = RemoveResult{
		Slug:                 slug,
		Path:                 target,
		LinksRemoved:         linksRemoved,
		CodeBranchDeleted:    warpDeleted,
		CodeBranchKeptReason: warpKeptReason,
		ArchiveTag:           archiveTag,
		ArchiveSkippedReason: archiveSkippedReason,
		Steps:                steps,
		Finished:             !pair.taskWorktree,
		StrayPath:            strayPath,
		SetAsideTag:          setAsideTagName,
	}
	if remote && archiveSkippedReason != "" {
		res.RemoteSkippedReason = "no remote deletion attempted: " + strings.TrimPrefix(archiveSkippedReason, "no archive tag pushed: ")
	}
	if localMode {
		return res, strayErr
	}

	// The remote steps go last, to the holder of the teardown lock, which releases it once they end.
	handedOver = true
	out := hold.finish(&remoteTeardownJob{
		Kind:               remoteTeardownRemove,
		Slug:               slug,
		Worktree:           l.WorktreePath(),
		BranchPrefix:       t.cfg.BranchPrefix,
		WeftBranch:         weftBranch,
		WeftTip:            weftTip,
		DeleteWeftOnOrigin: remote && !weftBranchExists(l, weftBranch),
		WarpBranch:         warpBranch,
		ParentBranch:       parentBranch,
		DeleteTaskOnOrigin: remote,
	})
	res.RemoteHandedOff = out.handedOff
	if !out.ran {
		return res, strayErr
	}
	res.ArchiveTag, res.OriginArchiveTag = out.archive.localTag, out.archive.originTag
	// A sibling found only on origin had no local tip to tag, so its archive is the child's alone.
	if res.ArchiveTag != "" && !slices.Contains(res.Steps, RemoveStepArchive) {
		res.Steps = append(res.Steps, RemoveStepArchive)
	}
	res.RemoteBranchDeleted, res.RemoteBranchError, res.RemoteBranchKeptReason = out.weft.RemoteDeleted, out.weft.RemoteError, out.weft.RemoteKeptReason
	res.RemoteCodeBranchDeleted, res.RemoteCodeBranchKeptReason = out.taskDeleted, out.taskKept
	if res.RemoteSkippedReason == "" {
		res.RemoteSkippedReason = out.weftKeptOnOrigin
	}
	if out.weft.RemoteDeleted {
		res.Steps = append(res.Steps, RemoveStepSiblingBranchOnOrigin)
	}
	if out.taskDeleted {
		res.Steps = append(res.Steps, RemoveStepTaskBranchOnOrigin)
	}
	if out.err != nil {
		return res, out.err
	}
	return res, strayErr
}

// housekeepHook, nil in production, runs with the slug just before a Remove's store housekeeping, so a test observes whether the housekeeping ran.
var housekeepHook func(slug string)

// setAsideWeftTip puts a local set-aside tag on the tip of the pair's weft branch and returns its name.
// A pair with no local weft branch has nothing to keep and returns an empty name.
func setAsideWeftTip(rec *Mutations, l *lymcwd.Location, slug, weftBranch string, hasBranch bool) (string, error) {
	if !hasBranch {
		return "", nil
	}
	weftRoot, err := RecordsRepoRoot(l)
	if err != nil {
		return "", fmt.Errorf("resolve weft repo root: %w", err)
	}
	repo := gitrepo.New(weftRoot)
	tip, err := repo.RefSHA("refs/heads/" + weftBranch)
	if err != nil {
		return "", fmt.Errorf("read the sibling branch tip: %w", err)
	}
	return setAsideTag(rec, repo, slug, tip)
}

// regenerateHubWorkspaceNow regenerates the hub workspace file with one attempt at its lock and reports whether the lock was held.
// A held lock leaves the file stale for the next regenerating verb, with a warning naming the repair; any other failure is logged the same way and reports false.
func regenerateHubWorkspaceNow(rec *Mutations, l *lymcwd.Location) (skipped bool) {
	_, err := regenerateHubWorkspace(rec, l, 0)
	if err == nil {
		return false
	}
	logger.Warn(`fabricengine: hub workspace regeneration skipped; run "lym fabric reconcile" once the cause is fixed`, "verb", "remove", "error", err)
	return errors.Is(err, errHubWorkspaceLockHeld)
}

// removeSibling tears down whatever of the pair's sibling remains locally, its worktree and local branch, through removeWeftWorktree; a copy on origin is the teardown child's.
// It returns the steps that changed state, in the order they ran.
func (t *Topology) removeSibling(rec *Mutations, l *lymcwd.Location, slug, weftBranch string, pair pairState, force bool) ([]string, error) {
	var steps []string
	if !pair.siblingWorktree && !pair.siblingBranch {
		return nil, nil
	}

	if !pair.siblingWorktree {
		if weftRoot, err := RecordsRepoRoot(l); err == nil {
			// Same reason as the task side's prune: a registration left by a hand-removed worktree blocks the branch deletion.
			_, _ = gitexec.Run([]string{"worktree", "prune"}, weftRoot)
		}
	}

	// A weft-teardown failure is tolerated only when the weft worktree is actually gone (already
	// absent, or removed with just a branch/prune step failing) — a weft worktree still on disk
	// after a "successful" Remove is a half-torn pair the operator was never told about. This check
	// reads the error return alone.
	_, weftErr := removeWeftWorktree(rec, l, slug, weftBranch, force, true, false, t.cfg.BranchPrefix, "", t.leftover)
	if weftErr != nil {
		weftTarget := RecordsWorktreePath(l, slug)
		if _, statErr := os.Stat(weftTarget); statErr == nil {
			return nil, fmt.Errorf(
				"warp worktree removed, but weft teardown failed and the weft worktree remains at %s: %w",
				weftTarget, weftErr)
		}
	}
	if pair.siblingWorktree && !pathPresent(RecordsWorktreePath(l, slug)) {
		steps = append(steps, RemoveStepSiblingWorktree)
	}
	if pair.siblingBranch && !weftBranchExists(l, weftBranch) {
		steps = append(steps, RemoveStepSiblingBranch)
	}
	return steps, nil
}

// pairState is what of a pair is on disk or in the repositories' local refs when Remove looks, taken without touching the network.
type pairState struct {
	// taskWorktree reports a task worktree that is a registered linked worktree of the task repo.
	taskWorktree bool
	// strayPath reports something at the task worktree's location that is not a registered linked worktree; Remove reports it and never deletes it.
	strayPath bool
	// siblingWorktree reports a directory at the sibling worktree's location.
	siblingWorktree bool
	// taskBranch and siblingBranch report the pair's local branches.
	taskBranch, siblingBranch bool
	// launchers reports the hub-level launcher directory Remove tears down.
	launchers bool
}

// inspectPair reads pairState for slug.
func (t *Topology) inspectPair(l *lymcwd.Location, slug string) (pairState, error) {
	var s pairState
	target := WorktreePath(l, slug)
	if pathPresent(target) {
		if isRegisteredLinkedWorktree(l, target) {
			s.taskWorktree = true
		} else {
			s.strayPath = true
		}
	}
	s.siblingWorktree = pathPresent(RecordsWorktreePath(l, slug))
	pairLoc, err := lymcwd.NamedWorktree(l.HubPath, slug)
	if err != nil {
		return pairState{}, fmt.Errorf("read the anchor of %q: %w", slug, err)
	}
	s.launchers = pathPresent(LauncherDir(pairLoc, slug))

	taskBranch, err := localBranchExists(l.WorktreePath(), t.cfg.BranchPrefix+slug)
	if err != nil {
		return pairState{}, err
	}
	s.taskBranch = taskBranch
	s.siblingBranch = weftBranchExists(l, RecordsBranchName(t.cfg.BranchPrefix+slug))
	return s, nil
}

// refuseWhenNothingRemains returns an error wrapping ErrPairNotFound when no part of the pair is left locally and neither of its branches is on origin.
// Origin is asked only after every local part is found gone, and only for a repository that has an origin, so a pair with any local remnant never costs a network call.
// A stray path at the pair's location is named in the error and left alone.
func (t *Topology) refuseWhenNothingRemains(l *lymcwd.Location, slug string, s pairState) error {
	if s.taskWorktree || s.siblingWorktree || s.taskBranch || s.siblingBranch || s.launchers {
		return nil
	}

	weftRoot, err := RecordsRepoRoot(l)
	if err != nil {
		return fmt.Errorf("resolve weft repo root: %w", err)
	}
	warpBranch := t.cfg.BranchPrefix + slug
	for _, side := range []struct{ repoDir, branch string }{
		{l.WorktreePath(), warpBranch},
		{weftRoot, RecordsBranchName(warpBranch)},
	} {
		if _, urlErr := gitrepo.New(side.repoDir).RemoteURL(originRemoteName); urlErr != nil {
			continue
		}
		tip, err := remoteHeadTip(side.repoDir, side.branch)
		if err != nil {
			return err
		}
		if tip != "" {
			return nil
		}
	}

	if s.strayPath {
		return fmt.Errorf("%w: nothing of %q remains, and %s exists at the pair location but is not a linked worktree of this repo, so it was left alone",
			ErrPairNotFound, slug, WorktreePath(l, slug))
	}
	return fmt.Errorf("%w: nothing of %q remains", ErrPairNotFound, slug)
}

// pathPresent reports whether anything, a dangling link included, exists at path.
func pathPresent(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

// deleteWarpBranch deletes warpBranch from the warp repository through the destructive gate, which
// declares it the pair's own warp branch (never ownedManagedBranch, which refuses a bare-slug branch
// under the default empty branch_prefix) and refuses unless its work is on another ref or landed on
// parentBranch. It reports whether the branch was deleted, else the reason it was kept.
// An already-absent branch is neither: there is nothing to delete and nothing to explain.
// Every refusal and failure is a kept reason rather than an error, because both worktrees are already
// gone by the time it runs.
func deleteWarpBranch(rec *Mutations, l *lymcwd.Location, warpBranch, parentBranch string) (deleted bool, keptReason string) {
	repoDir := l.WorktreePath()
	exists, err := gitrepo.New(repoDir).BranchExists(warpBranch)
	if err != nil {
		return false, fmt.Sprintf("look up warp branch %s: %v", warpBranch, err)
	}
	if !exists {
		return false, ""
	}

	err = deleteBranch(rec, branchRequest{
		what:      "delete warp branch",
		repoDir:   repoDir,
		branch:    warpBranch,
		ownership: ownedPairWarpBranch(warpBranch, parentBranch),
		dirtiness: dirtyUnlandedWork(parentBranch),
		force:     false,
	})
	if err == nil {
		return true, ""
	}
	var refusal *destructiveRefusal
	if errors.As(err, &refusal) {
		return false, refusal.Reason
	}
	return false, fmt.Sprintf("delete warp branch %s: %v", warpBranch, err)
}

// deleteTaskBranchOnOrigin deletes the pair's task branch from the warp repository's origin when its remote tip's work is landed.
// It observes the tip (ls-remote), fetches it so its objects are local, then deletes through deleteTaskBranchAtTip, leased to the observed tip.
// An absent remote branch, or a warp repo with no origin, is neither deleted nor a reason.
// Every refusal and failure is a kept reason rather than an error, as in deleteWarpBranch.
// failed is set when the kept reason is a failed lookup, fetch or delete, which the caller reports; a refusal and a moved tip leave it false.
func deleteTaskBranchOnOrigin(rec *Mutations, l *lymcwd.Location, warpBranch, parentBranch string) (deleted bool, keptReason string, failed bool) {
	repoDir := l.WorktreePath()
	if _, err := gitrepo.New(repoDir).RemoteURL(originRemoteName); err != nil {
		return false, "", false
	}
	tip, err := remoteHeadTip(repoDir, warpBranch)
	if err != nil {
		return false, err.Error(), true
	}
	if tip == "" {
		return false, "", false
	}
	if _, err := gitrepo.New(repoDir).FetchWithArgs(originRemoteName, "refs/heads/"+warpBranch); err != nil {
		return false, fmt.Sprintf("fetch task branch %s from %s: %v", warpBranch, originRemoteName, err), true
	}
	return deleteTaskBranchAtTip(rec, l, warpBranch, parentBranch, tip)
}

// deleteTaskBranchAtTip is deleteTaskBranchOnOrigin's gated deletion, leased to tip: a remote tip that moved since tip was observed fails the lease and nothing is deleted.
// A refusal and a rejected lease are kept reasons; any other failure of the delete is a kept reason with failed set.
func deleteTaskBranchAtTip(rec *Mutations, l *lymcwd.Location, warpBranch, parentBranch, tip string) (deleted bool, keptReason string, failed bool) {
	deleted, err := deleteRemoteBranch(rec, remoteBranchRequest{
		what:      "delete task branch on remote",
		repoDir:   l.WorktreePath(),
		remote:    originRemoteName,
		branch:    warpBranch,
		ownership: ownedPairWarpBranch(warpBranch, parentBranch),
		dirtiness: dirtyUnlandedRemoteTip(parentBranch),
		leaseSHA:  tip,
		force:     false,
	})
	if err == nil {
		return deleted, "", false
	}
	if refusal, ok := RefusalOf(err); ok {
		return false, refusal.Reason, false
	}
	return false, fmt.Sprintf("delete task branch %s on %s: %v", warpBranch, originRemoteName, err), !errors.Is(err, gitrepo.ErrLeaseRejected)
}

// RemoveRefusal reports the refusal Remove would raise for slug before its first mutation, or nil when none applies.
// It covers slug validation, the prime refusal, a pair of which nothing remains (ErrPairNotFound), both merge-in-progress directions, task-side dirtiness without force, and sibling-side dirtiness outside the record pathspec without force.
// A gone task worktree skips only the checks that need it: task-side dirtiness and the task side of the merge probe.
// It mutates nothing, takes no lock and pushes nothing, and Remove calls it first, so the two can never drift.
// Its name carries no fabric side, because callers outside the owner set call it.
func (t *Topology) RemoveRefusal(l *lymcwd.Location, slug string, force bool) error {
	warpBranch := t.cfg.BranchPrefix + slug

	if err := validateWorktreeSlug(slug, t.cfg.Dirs()); err != nil {
		return err
	}

	if err := refusePrimeSlug(l, slug); err != nil {
		return err
	}

	target := WorktreePath(l, slug)
	pair, err := t.inspectPair(l, slug)
	if err != nil {
		return err
	}
	if err := t.refuseWhenNothingRemains(l, slug, pair); err != nil {
		return err
	}

	// Refuse before any teardown for the named pair: a mid-merge pair is not force's to override —
	// force answers dirtiness only, never a live merge record.
	// A gone task worktree makes the probe read false on its own, so the task side needs no skip.
	blocked, err := mergeBlocksMutation(target, RecordsWorktreePath(l, slug))
	if err != nil {
		return err
	}
	if blocked {
		return &ErrMergeInProgress{}
	}

	// Refuse for the other direction too: this pair may be idle itself while some OTHER pair in the
	// hub is mid-merge ON its branches. Removing it there deletes the weft branch that merge is
	// resolving against, so an abort would leave the source work reachable only from the remote.
	inFlight, err := mergeSourceInFlight(l, warpBranch)
	if err != nil {
		return err
	}
	if inFlight {
		return &ErrMergeInProgress{}
	}

	// force answers the two dirtiness checks only.
	if force {
		return nil
	}
	if pair.taskWorktree {
		dirty, _, err := worktreeDirty(scopeAll, target)
		if err != nil {
			return fmt.Errorf("check warp worktree status: %w", err)
		}
		if dirty {
			return fmt.Errorf("worktree has uncommitted changes; use --force")
		}
	}

	names, err := PathspecNames(ConfigDirs(l))
	if err != nil {
		return fmt.Errorf("load the record pathspec: %w", err)
	}
	pairLoc, err := lymcwd.NamedWorktree(l.HubPath, slug)
	if err != nil {
		return fmt.Errorf("read the anchor of %q: %w", slug, err)
	}
	return refuseDirtyWeftWorktree(RecordsWorktreePath(l, slug), pairLoc.AnchorRel, names)
}

// commitPendingRecords commits the pair's sibling worktree's uncommitted changes inside the record pathspec, so the archive tag that follows covers them.
// It stages exactly the scoped pathspec `lym fabric sync` uses, through CommitRecordsPaths, with the fixed DefaultCommitMessage plus a Warp-SHA trailer naming the task worktree's HEAD, and pushes nothing.
// An absent sibling or an empty pathspec commits nothing; KindCommitCreated is recorded only when a commit landed, and committed reports it.
// The trailer names the task worktree's HEAD while it is present, else the local task branch's tip, else the last Warp-SHA trailer recorded on the sibling branch; with none of those the commit carries no trailer.
// With try set the weft write lock is taken by one attempt and a held lock returns an error wrapping ErrPairBusy with nothing committed; otherwise the commit waits for the lock.
func commitPendingRecords(rec *Mutations, l *lymcwd.Location, slug, warpBranch string, try bool) (committed bool, err error) {
	weftTarget := RecordsWorktreePath(l, slug)
	if _, statErr := os.Stat(weftTarget); os.IsNotExist(statErr) {
		return false, nil
	}
	pairLoc, err := lymcwd.NamedWorktree(l.HubPath, slug)
	if err != nil {
		return false, fmt.Errorf("read the anchor of %q: %w", slug, err)
	}

	names, err := PathspecNames(ConfigDirs(l))
	if err != nil {
		return false, fmt.Errorf("load the record pathspec: %w", err)
	}
	// git add refuses a pathspec that matches nothing, and a configured optional directory may not exist in this pair.
	present := make([]string, 0, len(names))
	for _, name := range names {
		if _, err := os.Lstat(filepath.Join(weftTarget, pairLoc.AnchorRel, name)); err == nil {
			present = append(present, name)
		}
	}
	// Nothing to commit, so the trailer's git lookups would be wasted, and they fail outright where the sibling location is not a checkout.
	if len(present) == 0 {
		return false, nil
	}

	warpSHA, err := recordTrailerSHA(l, slug, warpBranch, weftTarget)
	if err != nil {
		return false, err
	}
	msg := DefaultCommitMessage
	if warpSHA != "" {
		msg = appendWarpSHATrailer(msg, warpSHA)
	}
	if try {
		committed, err = commitRecordsPathsTry(rec, weftTarget, pairLoc.AnchorRel, present, msg)
	} else {
		_, committed, err = CommitRecordsPaths(rec, weftTarget, pairLoc.AnchorRel, present, msg, SyncOptions{})
	}
	if err != nil {
		return false, fmt.Errorf("commit the pair's pending records: %w", err)
	}
	return committed, nil
}

// commitRecordsPathsTry commits relPaths in the weft worktree at weftPath under one try-acquire of the weft write lock.
// A held lock returns an error wrapping ErrPairBusy and commits nothing.
func commitRecordsPathsTry(rec *Mutations, weftPath, anchorRel string, relPaths []string, msg string) (committed bool, err error) {
	lockDir, err := ensureWeftLockDirAt(weftPath)
	if err != nil {
		return false, err
	}
	held, acquired, err := lock.TryAcquireWriteLock(filepath.Join(lockDir, weftWriteLockFile))
	if err != nil {
		return false, fmt.Errorf("fabricengine: acquire weft write lock: %w", err)
	}
	if !acquired {
		return false, fmt.Errorf("the weft write lock of %s is held by another writer: %w", weftPath, ErrPairBusy)
	}
	defer func() { _ = held.Release() }()

	_, committed, err = commitRecordsPathsHeld(rec, weftPath, anchorRel, relPaths, msg, SyncOptions{})
	return committed, err
}

// recordTrailerSHA picks the Warp-SHA a record commit names: the task worktree's HEAD while that worktree is a present directory, else the local task branch's tip, else the last Warp-SHA trailer on the sibling branch at weftTarget.
// It returns an empty SHA when none of the three exists.
func recordTrailerSHA(l *lymcwd.Location, slug, warpBranch, weftTarget string) (string, error) {
	if _, err := os.Stat(WorktreePath(l, slug)); err == nil && isRegisteredLinkedWorktree(l, WorktreePath(l, slug)) {
		sha, err := gitrepo.New(WorktreePath(l, slug)).CurrentSHA()
		if err != nil {
			return "", fmt.Errorf("read the task worktree's HEAD: %w", err)
		}
		return sha, nil
	}

	exists, err := localBranchExists(l.WorktreePath(), warpBranch)
	if err != nil {
		return "", err
	}
	if exists {
		sha, err := gitrepo.New(l.WorktreePath()).RefSHA("refs/heads/" + warpBranch)
		if err != nil {
			return "", fmt.Errorf("read the task branch tip: %w", err)
		}
		return sha, nil
	}

	log, err := gitexec.Run([]string{"log", "-n", "200", "--format=%B%x00", "HEAD"}, weftTarget)
	if err != nil {
		return "", fmt.Errorf("read the sibling branch's log: %w", err)
	}
	for _, message := range strings.Split(log, "\x00") {
		if sha, ok := parseWarpSHATrailer(message); ok {
			return sha, nil
		}
	}
	return "", nil
}

// localBranchExists reports whether refs/heads/<branch> exists in the repo at repoDir.
// A missing branch is false with a nil error; a failed read is returned.
func localBranchExists(repoDir, branch string) (bool, error) {
	exists, err := gitrepo.New(repoDir).BranchExists(branch)
	if err != nil {
		return false, fmt.Errorf("look up branch %s: %w", branch, err)
	}
	return exists, nil
}

// refuseDirtyWeftWorktree returns an error when the weft worktree at weftTarget carries
// uncommitted changes outside the record pathspec (recordNames scoped under anchorRel), or when its status could not be read at all.
// Changes inside the pathspec are not a refusal: Remove commits them (commitPendingRecords).
//
// An ABSENT weft worktree is not a refusal: there is no uncommitted work to lose, and tearing down
// a half-present pair is exactly what Remove is for.
// An unreadable one IS a refusal, and that is the whole point of this helper: the probe used to
// swallow its own spawn error in an empty if-branch, so a git that failed to run silently reported
// the weft side clean and the no-force gate simply disappeared.
func refuseDirtyWeftWorktree(weftTarget, anchorRel string, recordNames []string) error {
	if _, statErr := os.Stat(weftTarget); os.IsNotExist(statErr) {
		return nil
	}

	args := []string{"status", "--porcelain", "--untracked-files=all", "--", "."}
	for _, p := range ScopedPathspec(anchorRel, recordNames) {
		args = append(args, ":(exclude)"+filepath.ToSlash(p))
	}
	stdout, err := gitexec.Run(args, weftTarget)
	if err != nil {
		return fmt.Errorf("check weft worktree status in %s: %w", weftTarget, err)
	}

	var paths []string
	// stdout is not trimmed: the first porcelain line may begin with the status column's space.
	for _, line := range strings.Split(stdout, "\n") {
		line = strings.TrimRight(line, "\r")
		if len(line) > 3 {
			paths = append(paths, line[3:])
		}
	}
	if len(paths) == 0 {
		return nil
	}
	return siblingDirtyRefusal{fmt.Sprintf("weft worktree has uncommitted changes outside the record pathspec (%s); commit or remove them, or use --force", strings.Join(paths, ", "))}
}

// refusePrimeSlug returns an error when slug names the hub's prime (main) warp worktree.
// The prime is the warp repository itself rather than a pair Remove can tear down, and git refuses
// to remove a main working tree — so without this guard the removal reaches the directory-removal
// fallback and deletes the whole clone, gitdir included.
// A prime-name resolution failure is not fatal here: it means the hub geometry is already broken,
// and removeWarpWorktreeDir's own registered-worktree rule still refuses to delete anything git
// declined to remove.
func refusePrimeSlug(l *lymcwd.Location, slug string) error {
	primeName, err := PrimeName(l)
	if err != nil {
		return nil
	}
	if slug != primeName {
		return nil
	}
	return fmt.Errorf(
		"refusing to remove %q: it is this hub's prime worktree — the warp repository itself, not a pair; remove the whole hub directory instead if that is what you meant",
		slug)
}

// removeWarpWorktreeDir removes the warp worktree at target via the gate's removeGitWorktree
// executor, falling back to a second gated call ONLY when target is a registered LINKED worktree of
// this repo.
//
// The narrow fallback is the whole point of this helper.
// `git worktree remove` refuses a main working tree, a path that is not a worktree of this repo at all, and a worktree carrying state it will not discard;
// treating every one of those refusals as licence to delete the directory turned an ordinary typo (`lym fabric remove <prime>`, `lym fabric remove` naming the Bolt checkout) into the loss of a whole git clone.
// A registered linked worktree is fabric's own pair member and nothing else, so deleting it after a git refusal is recoverable bookkeeping rather than data loss.
//
// The fallback is itself gated because it fires on ANY nonzero exit from `git worktree remove`, and
// `git worktree remove` without `--force` refuses on untracked files — an ungated fallback would
// therefore delete exactly the untracked files git had just declined to discard.
//
// When git exits nonzero having deleted the registration and the directory is still on disk, the directory is finished through removeWorktreeLeftover, with leftover as its seam.
// A directory on disk after git's clean exit, refilled by a late writer once git deleted it, is finished the same way.
func removeWarpWorktreeDir(rec *Mutations, l *lymcwd.Location, target string, force bool, leftover leftoverSeam) error {
	req := pathRequest{
		what:      "remove warp worktree",
		container: l.HubPath,
		target:    target,
		slug:      nil,
		ownership: ownedRegisteredLinkedWorktree(l.WorktreePath()),
		dirtiness: dirtyScopeAll(),
		force:     force,
	}

	err := removeGitWorktree(rec, req, l.WorktreePath())
	if err == nil {
		return finishGitWorktreeRemoval(rec, l.WorktreePath(), l.HubPath, target, nil, leftover)
	}

	var refusal *destructiveRefusal
	if errors.As(err, &refusal) && !isRegisteredLinkedWorktree(l, target) {
		// The gate refused before git ever ran: target fails the exact same
		// isRegisteredLinkedWorktree predicate the post-git-failure branch below would have
		// applied, just evaluated earlier. Report the identical, pre-existing message rather
		// than a gate-internal one — git's own exit code and stderr are unavailable here
		// because git was never invoked.
		return fmt.Errorf(
			"refusing to remove worktree %s: %s; it is not a linked worktree of this repo, so fabric will not delete the directory itself",
			target, refusal.Reason)
	}

	var gitErr *gitexec.GitError
	if !errors.As(err, &gitErr) {
		// git never ran, or the gate refused before it could: destroy nothing.
		return fmt.Errorf("run git worktree remove for %s: %w", target, err)
	}

	if !isRegisteredLinkedWorktree(l, target) {
		if !pathPresent(target) {
			return fmt.Errorf(
				"git refused to remove worktree %s (git exit %d): %s; it is not a linked worktree of this repo, so fabric will not delete the directory itself",
				target, gitErr.ExitCode, strings.TrimSpace(gitErr.Stderr))
		}
		// git deleted the registration and failed on the directory: its run is the ownership proof.
		if leftoverErr := removeWorktreeLeftover(rec, l.HubPath, target, leftover); leftoverErr != nil {
			return fmt.Errorf(
				"git refused to remove worktree %s (git exit %d): %s; it is not a linked worktree of this repo and finishing its removal failed: %w",
				target, gitErr.ExitCode, strings.TrimSpace(gitErr.Stderr), leftoverErr)
		}
		return nil
	}

	// force must travel from the primary request into the fallback, and its absence here was a real
	// defect: an operator who passed --force against a worktree git declined for some OTHER reason
	// (a `git worktree lock`, say) got a refusal whose stated remedy was "use --force" — the one
	// thing they had already done — and a half-torn-down pair.
	// Propagating it preserves the protection the fallback exists for, rather than weakening it: in
	// the NO-force case git refused precisely because of untracked files, and the fallback must not
	// delete what git just declined to discard; in the force case git was already invoked WITH
	// --force, so untracked files cannot have been its reason.
	// pathRequest.force is a bool with no unset state, so it is the one field a call site can omit to
	// a silent zero value — the exact failure mode the type's own doc comment names for the others.
	fallbackReq := pathRequest{
		what:      "remove warp worktree",
		container: l.HubPath,
		target:    target,
		ownership: ownedRegisteredLinkedWorktree(l.WorktreePath()),
		dirtiness: dirtyScopeAll(),
		force:     force,
	}
	if removeErr := removePath(rec, fallbackReq); removeErr != nil {
		// A *destructiveRefusal propagates unwrapped so errors.As still works at the caller; only an
		// operational failure gets the "fallback removal failed" wrapper.
		var refusal *destructiveRefusal
		if errors.As(removeErr, &refusal) {
			return removeErr
		}
		return fmt.Errorf("fallback removal failed: %w", removeErr)
	}
	// Best-effort: a failed prune leaves a stale registration the next reconcile or prune
	// re-reports, and it must not turn a completed removal into an error.
	_, _ = gitexec.Run([]string{"worktree", "prune"}, l.WorktreePath())
	return nil
}

// isRegisteredLinkedWorktree reports whether target is registered in this repo's worktree list as a
// worktree OTHER than the main one.
// A failure to enumerate answers false, the conservative direction: an unenumerable repo is exactly
// where a blind directory removal is least defensible.
func isRegisteredLinkedWorktree(l *lymcwd.Location, target string) bool {
	return isRegisteredLinkedWorktreeIn(l.WorktreePath(), target)
}

// PairWorktreeStands reports whether the task worktree of slug stands on disk as a registered linked worktree of l's repository.
// A directory at its path that git does not register, such as one a removal left holding build output, reads false, as do an absent path and a registration whose directory is gone.
// A path that is present while the repository's worktrees cannot be listed reads true, since the answer guards refusals and an unlistable repository must keep refusing.
func PairWorktreeStands(l *lymcwd.Location, slug string) bool {
	target := WorktreePath(l, slug)
	if !pathPresent(target) {
		return false
	}
	entries, err := List(l.WorktreePath())
	if err != nil {
		return true
	}
	return listsLinkedWorktree(entries, target)
}

// isRegisteredLinkedWorktreeIn is the repo-agnostic form of isRegisteredLinkedWorktree: it asks the
// repo at repoDir whether target is one of its registered LINKED worktrees.
//
// It is shared rather than duplicated because both sides of the pair need the same rule.
// Remove asks it of the WARP repo before falling back to a directory removal;
// Prune asks it of the WEFT repo for the same reason, and for the same data loss — a hub directory
// whose name merely ends in the weft suffix is not fabric's to delete, however loudly git refuses to
// remove it as a worktree.
// A failure to enumerate answers false, the conservative direction.
func isRegisteredLinkedWorktreeIn(repoDir, target string) bool {
	entries, err := List(repoDir)
	if err != nil {
		return false
	}
	return listsLinkedWorktree(entries, target)
}

// listsLinkedWorktree reports whether entries, a repository's worktree list, holds target as a worktree other than the main one.
func listsLinkedWorktree(entries []WorktreeEntry, target string) bool {
	cleanTarget := filepath.Clean(target)
	for _, entry := range entries {
		if entry.Main {
			continue
		}
		if filepath.Clean(filepath.FromSlash(entry.Path)) == cleanTarget {
			return true
		}
	}
	return false
}
