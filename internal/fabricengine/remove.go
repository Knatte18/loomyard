// remove.go implements Remove, which tears down a warp and weft worktree pair and its artifacts.
// Every refusal runs first, through the read-only probe RemoveRefusal, and leaves the hub and the weft origin untouched.
// Then the sibling's pending records are committed (commitPendingRecords);
// the weft tip is then tagged locally for its archive.
// Only then are the launchers and worktrees removed.
// A refused call therefore never loses a launcher or tags a tip that misses uncommitted records.
// The pushes of the weft tip and the tag, and every delete on origin, run last in the detached teardown child.
// The weft branch it removes is RecordsBranchName(warpBranch).
// After both worktrees are gone, the local warp branch is deleted only when the destructive gate proves no work is lost.
// Every commit must be on another ref or already landed on the pair's recorded parent, as deleteWarpBranch checks.
// A branch that fails the gate is kept with a reason.
// The link sweep is anchored and ownership-filtered.
// removeWarpWorktreeDir never deletes a directory git declined to remove.
// It deletes one only when git lists it as a registered linked worktree of this repo.
// Any other directory risks data loss, so it is left in place.

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

// ErrPairSiblingDirty is the sentinel a no-force refusal wraps when the pair's other worktree is dirty.
// Changes inside the record pathspec never raise it: they are committed before the archive.
// The message names neither side of the pair.
// Callers outside this package can match it with errors.Is and offer their own remedy.
var ErrPairSiblingDirty = errors.New("the pair's sibling worktree has uncommitted changes")

// siblingDirtyRefusal carries the refusal text unchanged while unwrapping to ErrPairSiblingDirty.
type siblingDirtyRefusal struct{ msg string }

func (e siblingDirtyRefusal) Error() string { return e.msg }

func (e siblingDirtyRefusal) Unwrap() error { return ErrPairSiblingDirty }

// ErrPairNotFound is the sentinel an error wraps when no trace of the pair remains.
// No worktree stands on either side, no branch exists locally or on origin, and no launcher entry is left.
// The message names neither side of the pair, so callers outside this package can match it with errors.Is.
var ErrPairNotFound = errors.New("pair not found")

// ErrStrayPathKept is the sentinel an error wraps when a non-worktree directory stays at a pair's task worktree path.
// Deleting it failed after the pair's removal.
// The pair itself is gone; the directory blocks the slug's next pair until it is deleted.
var ErrStrayPathKept = errors.New("a directory that is no worktree stays at the pair's task worktree path")

// The steps a Remove call reports in RemoveResult.Steps, a closed set; each is appended only after it changed state.
// The two origin steps are the teardown child's, so they are listed only when its steps ran in process.
const (
	// RemoveStepRecordCommit is the commit of the sibling's pending records.
	RemoveStepRecordCommit = "record_commit"
	// RemoveStepArchive is the archive tag made on the local weft tip before the teardown;
	// its push to origin is the teardown child's.
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

// RemoveResult reports the outcome of removing a worktree pair.
// It embeds MutationRecord, which holds the mutations the call made.
type RemoveResult struct {
	MutationRecord
	Slug         string `json:"slug"`
	Path         string `json:"path"`
	LinksRemoved int    `json:"links_removed"`
	// Steps lists the steps this call performed, in order, from the RemoveStep constants; always an array.
	// A teardown child's steps are listed only when they ran in process;
	// then RemoveStepArchive stands for the local tag alone.
	Steps []string `json:"steps"`
	// Finished is true when the task worktree was already gone at entry, so the call completed a half-removed pair.
	Finished bool `json:"finished"`
	// StrayPath names a path at the task worktree's location that is not a registered linked worktree.
	// Remove never deletes a path found there at entry.
	// A directory that a late writer refilled after the task worktree's removal is reported here too;
	// when a last removal could not delete it, Remove returns an error wrapping ErrStrayPathKept.
	StrayPath string `json:"stray_path,omitempty"`
	// StrayPathDeleted is true when a caller's separate delete removed a stray directory at the task worktree's path;
	// Remove itself never sets it.
	StrayPathDeleted bool `json:"stray_path_deleted,omitempty"`
	// RemoteBranchDeleted reports whether the pair's weft branch was observably removed from the remote.
	// It is true only when the remote deletion was attempted and actually removed a ref.
	RemoteBranchDeleted bool `json:"remote_branch_deleted,omitempty"`
	// RemoteBranchError is non-empty when the remote deletion was attempted and did not succeed.
	// Its text always names the layer that said no: the gate's own refusal, or the remote deletion itself.
	RemoteBranchError string `json:"remote_branch_error,omitempty"`
	// RemoteBranchKeptReason is non-empty when the pair's weft branch on the remote was left in place;
	// the branch moved after the archive read its tip.
	// A kept branch is another process's live branch, not a failure of Remove.
	RemoteBranchKeptReason string `json:"remote_branch_kept_reason,omitempty"`
	// RemoteSkippedReason is the once-per-verb reason no deletion of the sibling branch's remote copy was attempted:
	// with remote, a weft repo with no origin remote configured;
	// without remote, a sibling branch left only on origin, which is kept.
	RemoteSkippedReason string `json:"remote_skipped_reason,omitempty"`
	// CodeBranchDeleted reports whether the pair's local warp branch was deleted after the teardown.
	CodeBranchDeleted bool `json:"code_branch_deleted"`
	// CodeBranchKeptReason is non-empty when the warp branch was left in place, naming why:
	// the destructive gate's refusal, or a failure to delete it.
	// A kept branch is not a failure of Remove.
	CodeBranchKeptReason string `json:"code_branch_kept_reason,omitempty"`
	// RemoteCodeBranchDeleted reports whether the pair's task branch was observably removed from the warp repo's origin.
	// It is true only with remote, when the branch's work was landed.
	RemoteCodeBranchDeleted bool `json:"remote_code_branch_deleted"`
	// RemoteCodeBranchKeptReason is non-empty when remote was set and the task branch on origin was left in place;
	// it names why: the gate's refusal, or a failure to look it up or delete it.
	// A kept branch is not a failure of Remove.
	RemoteCodeBranchKeptReason string `json:"remote_code_branch_kept_reason,omitempty"`
	// ArchiveTag names the archive/<slug>/<tip> tag made on the local weft tip before the teardown;
	// its push to the weft origin is the teardown child's.
	// It is empty when none was made.
	ArchiveTag string `json:"archive_tag,omitempty"`
	// OriginArchiveTag names the second archive tag, made at origin's weft tip.
	// It is pushed when that tip is neither the local tip nor an ancestor of it, and is empty when none was needed.
	OriginArchiveTag string `json:"origin_archive_tag,omitempty"`
	// ArchiveSkippedReason is non-empty when no archive was attempted;
	// that is the case only for a weft repo with no origin remote configured.
	ArchiveSkippedReason string `json:"archive_skipped_reason,omitempty"`
	// RemoteHandedOff is true when the remote steps went to the detached teardown child;
	// it runs them after Remove returns, and the remote outcome fields then stay empty.
	RemoteHandedOff bool `json:"remote_handed_off,omitempty"`
	// SetAsideTag names the local set-aside/<slug>/<tip> tag put on the weft tip by a local removal;
	// it takes the place of an archive tag and is empty when none was put.
	SetAsideTag string `json:"set_aside_tag,omitempty"`
	// WorkspaceSkipped is true when a local removal found the hub workspace lock held and left the workspace file stale.
	WorkspaceSkipped bool `json:"workspace_skipped,omitempty"`
	// SnapshotTag names the snapshot tag that kept the pair's uncommitted paths before a forced local removal;
	// it is empty when none was taken.
	SnapshotTag string `json:"snapshot_tag,omitempty"`
	// SnapshotPaths lists the paths the snapshot tag holds.
	SnapshotPaths []string `json:"snapshot_paths,omitempty"`
	// SiblingBranch names the pair's sibling branch the removal tore down, read from the pair before its teardown.
	SiblingBranch string `json:"sibling_branch,omitempty"`
}

// Remove removes a paired warp and weft worktree with all of its artifacts.
// Without force both worktrees must be clean; with force, uncommitted changes in them are discarded.
// The sibling's dirtiness inside the record pathspec is no refusal: it is committed first, with or without force,
// so the archive covers it.
// Remove refuses the hub's prime worktree outright, since the prime is the warp repository itself.
// It validates slug first, so hub geometry (the Bolt checkout, `_portals`, `_launchers`, `_lym`, `.lym`,
// and every weft sibling) is never torn down as a pair.
// The no-force dirtiness checks, status probes included, are refusals too;
// a refusal leaves the hub and the weft origin untouched, with no tag pushed and nothing torn down.
// After every refusal and before its first teardown mutation, Remove tags the pair's local weft tip.
// The tag, archive/<slug>/<tip>, is reported in ArchiveTag;
// it keeps the run's records alive after the branch is deleted.
// A failed tag returns an error wrapping ErrArchiveFailed, with everything still in place, so a plain re-run retries it.
// The tag is taken whatever remote says, and force does not skip it: force answers dirtiness only.
// A weft repo with no origin proceeds untagged, with ArchiveSkippedReason set on the result.
// Launcher cleanup runs after the tag and before the git removal.
// The pair's branches are read from the pair before any teardown, through PairTaskBranch.
// A pair made under an earlier branch prefix is torn down by the branches it stands on.
// With no worktree or local branch of the pair left, origin names them.
// After both worktrees are removed, Remove deletes the local warp branch through the destructive gate.
// The gate refuses unless the work is on another ref or has landed on the parent in the pair's origin record.
// A refusal fills CodeBranchKeptReason and Remove still succeeds; force never answers that check.
// The remote steps run last, in a detached child that holds the pair's teardown lock.
// Remove returns without waiting on them and reports RemoteHandedOff.
// In order, the child pushes the local weft tip to origin when origin lacks it, and a refusal is only logged.
// It then pushes the archive tag;
// a second tag goes to origin's tip when that tip is neither the local tip nor an ancestor of it.
// With remote, it deletes the weft branch on origin.
// The lease is on the origin tip it read, or on the local tip once its push landed.
// A branch that origin moved since is kept.
// With remote, it deletes the task branch on origin.
// The deletion goes through the landed-work gate and holds a lease on the observed remote tip.
// A failed remote step is logged and mailed to the hub's orch with its way forward; a kept branch is no failure.
// In a test binary the remote steps run in process before Remove returns.
// They fill ArchiveTag, OriginArchiveTag, the RemoteBranch and RemoteCodeBranch fields and their steps;
// an archive failure is returned as the error.
// The recorded parent is read before any teardown, since the record lives in the weft worktree the teardown deletes.
// A pair whose task worktree is already gone is finished rather than refused.
// Remove performs whatever teardown remains, in the same order and through the same gates;
// it reports those steps in Steps and sets Finished.
// With nothing of the pair left it returns an error wrapping ErrPairNotFound.
// A sibling branch present only on origin is archived from there by the child and, with remote, deleted there;
// without remote it stays.
// A path at the task worktree's location that is not a registered linked worktree is reported in StrayPath;
// Remove never deletes it.
// A late writer, such as a build server or a session's exit hooks, may refill that path after its removal.
// Remove removes such a directory once more after the sibling's teardown.
// If it cannot, the directory is reported in StrayPath;
// Remove finishes the teardown and returns an error wrapping ErrStrayPathKept that names the directory.
// Before its first mutation, Remove has the teardown child take the pair's teardown lock exclusive.
// This happens after the refusals and the pair inspection.
// The wait for a push of the pair to let go of the lock lasts at most the topology's teardown wait.
// When the wait runs out, Remove returns an error wrapping ErrPairPushRunning before any mutation.
// The child holds the lock past the remote steps.
// A failure after taking it that leaves a worktree of the pair in place releases the lock and respawns the pair's push.
// After a successful teardown, and before it returns, Remove disables auto gc in both hub stores.
// It then runs a best-effort foreground `git gc --auto` in each.
// A failure there is logged and never fails Remove, and a refused or failed Remove runs none of it.
func (t *Topology) Remove(l *lymcwd.Location, slug string, force, remote bool) (res RemoveResult, err error) {
	return t.removePair(l, slug, force, remote, false)
}

// RemoveLocal removes a paired warp and weft worktree without any origin step and without waiting on another writer.
// It makes every refusal of removal, with force answering dirtiness only.
// A held teardown lock or weft write lock returns an error wrapping ErrPairBusy with nothing changed.
// It commits the sibling's pending records.
// It then puts a local set-aside/<slug>/<sha> tag on the weft tip, reported in SetAsideTag;
// it takes the place of the pushed archive tag.
// It runs no ls-remote and respawns no push after a failure.
// It skips the store housekeeping.
// It regenerates the hub workspace file with one attempt at its lock and reports WorkspaceSkipped when the lock is held.
// The commits the pushed archive tag would have carried stay under the local tag.
// The branch deletions keep their gates.
func (t *Topology) RemoveLocal(l *lymcwd.Location, slug string, force bool) (res RemoveResult, err error) {
	return t.removePair(l, slug, force, false, true)
}

// removePair removes a worktree pair. In localMode it skips every origin step and every wait on another writer.
func (t *Topology) removePair(l *lymcwd.Location, slug string, force, remote, localMode bool) (res RemoveResult, err error) {
	rec := NewMutations(l.HubPath)
	defer func() { res.Mutations = rec.Snapshot() }()

	// Every refusal runs here, through the read-only probe RemoveRefusal, so the probe and this call cannot drift.
	onOrigin, err := t.refuseRemoval(l, slug, force, localMode)
	if err != nil {
		return RemoveResult{}, err
	}

	target := WorktreePath(l, slug)
	pair, err := t.inspectPair(l, slug)
	if err != nil {
		return RemoveResult{}, err
	}
	// With no worktree or local branch of the pair left, origin names the branch the refusal found;
	// it must, for a pair made under an earlier branch prefix.
	if !localMode && onOrigin != "" {
		pair.warpBranch = onOrigin
	}
	warpBranch := pair.warpBranch
	weftBranch := RecordsBranchName(warpBranch)
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
	// A failure after this point that leaves a worktree behind respawns the pair's push once the lock is released.
	// A child may have exited on the held lock, so the respawn is needed.
	// A local removal holds the lock itself and respawns nothing:
	// the surviving worktree's commits stay local for the next reconcile.
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

	// Read the recorded parent now, since the record lives in the weft worktree the teardown deletes.
	// A missing or unreadable record leaves parentBranch empty.
	// That asks the gate for the stricter reachability check alone.
	parentBranch := ""
	if origin, found, originErr := ReadOriginFor(l, slug); originErr == nil && found {
		parentBranch = origin.ParentBranch
	}

	weftTip, archiveTag, archiveSkippedReason, setAsideTagName := "", "", "", ""
	// Commit the sibling's pending records so the archive tag covers them;
	// force answers dirtiness only, so it never skips this step either.
	committed, err := commitPendingRecords(rec, l, slug, warpBranch, localMode)
	if err != nil {
		return RemoveResult{}, err
	}
	if committed {
		steps = append(steps, RemoveStepRecordCommit)
	}

	if localMode {
		// The weft tip is kept under a local tag, since no origin step may run;
		// the tag covers what the pushed archive tag would have.
		setAsideTagName, err = setAsideWeftTip(rec, l, slug, weftBranch, pair.siblingBranch)
		if err != nil {
			return RemoveResult{}, err
		}
	} else {
		// Tag the weft tip after every refusal and before the first teardown mutation:
		// a failed tag leaves the worktrees, launchers and both branches untouched, so a plain re-run retries it.
		// force answers dirtiness only, so it never skips this step; the tag's push is the teardown child's.
		weftTip, archiveTag, archiveSkippedReason, err = tagWeftTipForArchive(l, slug, weftBranch)
		if err != nil {
			return RemoveResult{}, err
		}
		if archiveTag != "" {
			steps = append(steps, RemoveStepArchive)
		}
	}

	// removeLaunchers is best-effort: an operational failure is discarded,
	// but a gate refusal must surface rather than vanish.
	if err := surfaceRefusal(removeLaunchers(rec, pairLoc, slug)); err != nil {
		return RemoveResult{}, err
	}
	if pair.launchers && !pathPresent(LauncherDir(pairLoc, slug)) {
		steps = append(steps, RemoveStepLaunchers)
	}

	// Sweep the anchored directory, and only the links fabric itself created there.
	// The worktree root is the wrong place to sweep.
	// On a subpath-anchored hub the junctions live under <worktree>/<anchorRel>.
	// At a root anchor the sweep would delete the user's own symlinks too.
	// The scratch link is no junction, so the sweep skips it;
	// unlinkScratchLink removes it before the worktree directory removal.
	// The sweep and the directory removal run only for a registered task worktree.
	linksRemoved := 0
	if pair.taskWorktree {
		if scannedNames, scanErr := scanOnDiskJunctionNames(pairLoc, slug); scanErr == nil {
			ownedNames := withoutScratchLink(scannedNames)
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
		// A worktree removed by hand stays registered.
		// Git refuses to delete a branch checked out at a registered worktree.
		// The prune is best-effort: a failed prune surfaces as the branch gate's own refusal.
		_, _ = gitexec.Run([]string{"worktree", "prune"}, l.WorktreePath())
	}

	// A sibling with neither worktree nor local branch is known only from origin, and only the teardown child asks origin.
	siblingSteps, err := t.removeSibling(rec, l, slug, weftBranch, pair, force)
	if err != nil {
		return RemoveResult{}, err
	}
	steps = append(steps, siblingSteps...)

	if removeAfterTeardownHook != nil {
		removeAfterTeardownHook()
	}

	// A late writer can refill the task worktree's directory after its removal finished,
	// and a directory left there blocks the slug's next pair.
	// This call's own removal of the registered worktree is the ownership proof.
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
	// A refused or failed Remove returns above and runs no housekeeping,
	// and a local removal runs none either.
	if !localMode {
		if housekeepHook != nil {
			housekeepHook(slug)
		}
		housekeepStores(l, slug, t.inFlight, t.gcAutoThreshold)
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
		SiblingBranch:        weftBranch,
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

// housekeepHook, nil in production, runs with the slug just before a Remove's store housekeeping,
// so a test observes whether the housekeeping ran.
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

// regenerateHubWorkspaceNow makes one attempt at the hub workspace lock and regenerates the file under it.
// It reports whether the lock was held: a held lock leaves the file stale for the next regenerating verb,
// and a warning names the repair.
// Any other failure is logged the same way and reports false.
func regenerateHubWorkspaceNow(rec *Mutations, l *lymcwd.Location) (skipped bool) {
	_, err := regenerateHubWorkspace(rec, l, 0)
	if err == nil {
		return false
	}
	logger.Warn(`fabricengine: hub workspace regeneration skipped; run "lym fabric reconcile" once the cause is fixed`, "verb", "remove", "error", err)
	return errors.Is(err, errHubWorkspaceLockHeld)
}

// removeSibling removes whatever of the pair's sibling remains locally: its worktree and its local branch.
// A copy on origin is left to the teardown child.
// It returns the steps that changed state, in the order they ran.
func (t *Topology) removeSibling(rec *Mutations, l *lymcwd.Location, slug, weftBranch string, pair pairState, force bool) ([]string, error) {
	var steps []string
	if !pair.siblingWorktree && !pair.siblingBranch {
		return nil, nil
	}

	if !pair.siblingWorktree {
		if weftRoot, err := RecordsRepoRoot(l); err == nil {
			// A registration left by a hand-removed worktree blocks the branch deletion, so it is pruned first.
			_, _ = gitexec.Run([]string{"worktree", "prune"}, weftRoot)
		}
	}

	// A weft-teardown failure is tolerated only when the weft worktree is gone:
	// already absent, or removed with only a branch or prune step failing.
	// A weft worktree that remains on disk after a failed teardown is returned as an error.
	_, weftErr := removeWeftWorktree(rec, l, slug, weftBranch, force, true, false, "", t.leftover)
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

// pairState records what of a pair is on disk or in the local refs, read without touching the network.
type pairState struct {
	// taskWorktree reports a task worktree that is a registered linked worktree of the task repo.
	taskWorktree bool
	// strayPath reports something at the task worktree's location that is not a registered linked worktree;
	// the removal reports it and never deletes it.
	strayPath bool
	// siblingWorktree reports a directory at the sibling worktree's location.
	siblingWorktree bool
	// taskBranch and siblingBranch report the pair's local branches.
	taskBranch, siblingBranch bool
	// launchers reports the hub-level launcher directory, which the removal tears down.
	launchers bool
	// warpBranch is the pair's warp branch, read locally from the pair itself.
	warpBranch string
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

	warpBranch, err := t.PairTaskBranch(l, slug)
	if err != nil {
		return pairState{}, err
	}
	s.warpBranch = warpBranch
	taskBranch, err := localBranchExists(l.WorktreePath(), warpBranch)
	if err != nil {
		return pairState{}, err
	}
	s.taskBranch = taskBranch
	s.siblingBranch = weftBranchExists(l, RecordsBranchName(warpBranch))
	return s, nil
}

// branchPartRemains reports whether a worktree or local branch of the pair is left, the parts that name its warp branch.
func (s pairState) branchPartRemains() bool {
	return s.taskWorktree || s.siblingWorktree || s.taskBranch || s.siblingBranch
}

// refuseWhenNothingRemains returns an error wrapping ErrPairNotFound when no part of the pair is left locally.
// It returns that error only when none of the pair's branches, under any of its names, is on origin.
// Origin is asked only once no worktree and no local branch of the pair remains.
// It is asked only for a repository with an origin.
// A pair with a worktree or branch left never costs a network call.
// A pair whose only local remnant is its launcher directory is asked too, because the directory names no branch.
// In localMode such a pair is not asked; otherwise it is no error when origin holds nothing.
// When origin holds the pair, it returns the name origin holds it under, and otherwise "".
// A stray path at the pair's location is named in the error and left alone.
func (t *Topology) refuseWhenNothingRemains(l *lymcwd.Location, slug string, s pairState, localMode bool) (onOrigin string, err error) {
	if s.branchPartRemains() || (s.launchers && localMode) {
		return "", nil
	}

	onOrigin, err = t.originPairBranch(l, slug)
	if err != nil {
		return "", err
	}
	if onOrigin != "" {
		return onOrigin, nil
	}
	if s.launchers {
		return "", nil
	}

	if s.strayPath {
		return "", fmt.Errorf("%w: nothing of %q remains, and %s exists at the pair location but is not a linked worktree of this repo, so it was left alone",
			ErrPairNotFound, slug, WorktreePath(l, slug))
	}
	return "", fmt.Errorf("%w: nothing of %q remains", ErrPairNotFound, slug)
}

// pathPresent reports whether anything, a dangling link included, exists at path.
func pathPresent(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

// deleteWarpBranch deletes warpBranch from the warp repository through the destructive gate.
// It declares the branch as the pair's own warp branch.
// Managed-branch ownership would refuse a bare slug under the default empty branch_prefix.
// The gate refuses unless the branch's work is on another ref or has landed on parentBranch.
// It reports whether the branch was deleted, else the reason it was kept.
// An already-absent branch is neither: there is nothing to delete and nothing to explain.
// Every refusal and failure is a kept reason, not an error, since both worktrees are already gone when it runs.
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

// deleteTaskBranchOnOrigin deletes the pair's task branch from origin once the work at its remote tip is landed.
// It observes the tip with ls-remote, fetches it so its objects are local, and deletes it leased to the observed tip.
// A tip that moved in between fails the lease and nothing is deleted.
// An absent remote branch, or a warp repo with no origin, is neither deleted nor a reason.
// Every refusal and failure is a kept reason rather than an error.
// failed is set when the kept reason is a failed lookup, fetch or delete, which the caller reports;
// a refusal and a moved tip leave it false.
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

// deleteTaskBranchAtTip deletes the pair's task branch on origin through the destructive gate, leased to tip.
// A remote tip that moved since tip was observed fails the lease, and nothing is deleted.
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

// RemoveRefusal reports the refusal a removal of slug raises before its first mutation, or nil when none applies.
// It covers slug validation, the prime refusal, a pair of which nothing remains (ErrPairNotFound), both merge-in-progress directions, task-side dirtiness without force, and sibling-side dirtiness outside the record pathspec without force.
// A gone task worktree skips only the checks that need it: task-side dirtiness and the task side of the merge probe.
// It mutates nothing, takes no lock and pushes nothing, and the removal calls it first, so the two cannot drift.
// Its name carries no fabric side, because callers outside the owner set call it.
func (t *Topology) RemoveRefusal(l *lymcwd.Location, slug string, force bool) error {
	_, err := t.refuseRemoval(l, slug, force, false)
	return err
}

// refuseRemoval reports the refusal for a removal of slug; localMode is true for a removal that runs no origin step.
// With no refusal it also returns the name origin holds the pair under;
// it is empty when origin was not asked or does not hold the pair, so a removal asks origin once.
func (t *Topology) refuseRemoval(l *lymcwd.Location, slug string, force, localMode bool) (onOrigin string, err error) {
	if err := validateWorktreeSlug(slug, t.cfg.Dirs()); err != nil {
		return "", err
	}

	if err := refusePrimeSlug(l, slug); err != nil {
		return "", err
	}

	target := WorktreePath(l, slug)
	pair, err := t.inspectPair(l, slug)
	if err != nil {
		return "", err
	}
	onOrigin, err = t.refuseWhenNothingRemains(l, slug, pair, localMode)
	if err != nil {
		return "", err
	}

	// Refuse before any teardown for the named pair: a mid-merge pair is refused whatever force says.
	// force answers dirtiness only, never a live merge record.
	// A gone task worktree makes the probe read false on its own, so the task side needs no skip.
	blocked, err := mergeBlocksMutation(target, RecordsWorktreePath(l, slug))
	if err != nil {
		return "", err
	}
	if blocked {
		return "", &ErrMergeInProgress{}
	}

	// Refuse the other direction too: this pair may be idle while another pair in the hub is mid-merge on its branches.
	// Removing this pair would delete the weft branch that merge is resolving against,
	// so an abort would leave the source work reachable only from the remote.
	inFlight, err := mergeSourceInFlight(l, pair.warpBranch)
	if err != nil {
		return "", err
	}
	if inFlight {
		return "", &ErrMergeInProgress{}
	}

	// force answers the two dirtiness checks only.
	if force {
		return onOrigin, nil
	}
	if pair.taskWorktree {
		dirty, _, err := worktreeDirty(scopeAll, target)
		if err != nil {
			return "", fmt.Errorf("check warp worktree status: %w", err)
		}
		if dirty {
			return "", fmt.Errorf("worktree has uncommitted changes; use --force")
		}
	}

	names, err := PathspecNames(ConfigDirs(l))
	if err != nil {
		return "", fmt.Errorf("load the record pathspec: %w", err)
	}
	pairLoc, err := lymcwd.NamedWorktree(l.HubPath, slug)
	if err != nil {
		return "", fmt.Errorf("read the anchor of %q: %w", slug, err)
	}
	if err := refuseDirtyWeftWorktree(RecordsWorktreePath(l, slug), pairLoc.AnchorRel, names); err != nil {
		return "", err
	}
	return onOrigin, nil
}

// commitPendingRecords commits the sibling worktree's uncommitted changes inside the record pathspec.
// It stages the record pathspec entries present in the pair and pushes nothing.
// The message is DefaultCommitMessage, plus the Warp-SHA trailer described below.
// An absent sibling or an empty pathspec commits nothing; committed reports whether a commit landed.
// The trailer names the task worktree's HEAD while it is present.
// Otherwise it names the local task branch's tip, or else the last Warp-SHA trailer recorded on the sibling branch.
// With none of those the commit carries no trailer.
// With try set, a single attempt takes the weft write lock,
// and a held lock returns an error wrapping ErrPairBusy with nothing committed.
// Otherwise the commit waits for the lock.
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
	// Nothing to commit, so the trailer's git lookups would be wasted,
	// and they fail outright where the sibling location is not a checkout.
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

// recordTrailerSHA picks the Warp-SHA a record commit names.
// It uses the task worktree's HEAD while that worktree is a present directory.
// Otherwise it uses the local task branch's tip.
// When that branch is absent, it uses the last Warp-SHA trailer on the sibling branch at weftTarget.
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

// refuseDirtyWeftWorktree returns an error when the weft worktree has uncommitted changes outside the record pathspec.
// It also returns an error when the status of that worktree cannot be read.
// Changes inside the pathspec are not a refusal: removal commits them.
//
// An absent weft worktree is not a refusal: there is no uncommitted work to lose,
// and a half-present pair is what removal tears down.
// An unreadable one is a refusal, so a git that fails to run cannot make the weft side look clean.
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
// The prime is the warp repository itself, not a pair that can be torn down.
// Git refuses to remove a main working tree.
// Without this guard the removal reaches the directory-removal fallback and deletes the whole clone, gitdir included.
// A failure to resolve the prime name is not fatal here: the hub geometry is already broken,
// and the directory removal still refuses to delete anything git declined to remove.
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

// removeWarpWorktreeDir removes the warp worktree at target through a gated `git worktree remove`.
// It falls back to a second gated removal only when target is a registered linked worktree of this repo.
//
// The narrow fallback is the whole point of this helper.
// `git worktree remove` refuses a main working tree, a path that is not a worktree of this repo at all,
// and a worktree carrying state it will not discard.
// Treating each of those refusals as licence to delete would turn an ordinary typo into the loss of a whole git clone.
// A registered linked worktree is fabric's own pair member and nothing else.
// Deleting it after a git refusal is recoverable bookkeeping rather than data loss.
//
// The fallback is itself gated because it fires on any nonzero exit from `git worktree remove`,
// and `git worktree remove` without `--force` refuses on untracked files.
// An ungated fallback would therefore delete the untracked files git had just declined to discard.
//
// After git exits nonzero having deleted the registration, the leftover seam finishes a directory still on disk.
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
		// The gate refused before git ran, so there is no git exit code or stderr to report.
		// The target is not a linked worktree of this repo, so report that rather than a gate-internal message.
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

	// The fallback must carry force from the primary request.
	// Without force, git refused because of untracked files,
	// and the fallback must not delete what git just declined to discard.
	// With force, git ran with --force, so untracked files cannot have been the reason it refused.
	// pathRequest.force is a bool with no unset state, so a request that omits it silently means no force.
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
	// Best-effort: a failed prune leaves a stale registration that the next reconcile or prune re-reports,
	// and it must not turn a completed removal into an error.
	_, _ = gitexec.Run([]string{"worktree", "prune"}, l.WorktreePath())
	return nil
}

// isRegisteredLinkedWorktree reports whether target is registered in this repo's worktree list as a linked worktree.
// A failure to enumerate answers false, the conservative direction:
// an unenumerable repo is where a blind directory removal is least defensible.
func isRegisteredLinkedWorktree(l *lymcwd.Location, target string) bool {
	return isRegisteredLinkedWorktreeIn(l.WorktreePath(), target)
}

// PairWorktreeStands reports whether the task worktree of slug stands on disk as a linked worktree of l's repository.
// A directory at its path that git does not register, such as one a removal left holding build output, reports false.
// An absent path, and a registration whose directory is gone, also report false.
// A path that is present while the repository's worktrees cannot be listed reports true;
// the answer guards refusals, and an unlistable repository must keep refusing.
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

// isRegisteredLinkedWorktreeIn reports whether target is one of the registered linked worktrees of the repo at repoDir.
//
// The warp and weft repos of a pair both need the same answer.
// A hub directory whose name merely ends in the weft suffix is not fabric's to delete;
// git refusing to remove it as a worktree does not change that.
// A failure to enumerate answers false, the conservative direction.
func isRegisteredLinkedWorktreeIn(repoDir, target string) bool {
	entries, err := List(repoDir)
	if err != nil {
		return false
	}
	return listsLinkedWorktree(entries, target)
}

// listsLinkedWorktree reports whether entries, the worktree list of a repository, holds target as a linked worktree.
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
