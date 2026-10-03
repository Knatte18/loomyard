// remove.go implements Remove: every refusal — slug, prime, target-exists, merge-in-progress and the no-force dirtiness checks with their status probes — runs first, through the read-only probe RemoveRefusal, and leaves the hub and the weft origin untouched.
// Then the sibling's pending records are committed (commitPendingRecords) and the weft tip is archived,
// and only then are the portal and launchers torn down, so a refused call never loses a launcher or pushes a tag that misses uncommitted records.
// The weft branch it removes is WeftBranchName(warpBranch).
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
	"strings"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

// ErrPairSiblingDirty is the sentinel Remove's no-force refusal wraps when the pair's other worktree carries uncommitted changes outside the record pathspec.
// Changes inside the pathspec never raise it: Remove commits them before the archive.
// It is worded without naming either side of the pair so callers outside the fabric vocabulary owner set can match it with errors.Is and offer their own remedy.
var ErrPairSiblingDirty = errors.New("the pair's sibling worktree has uncommitted changes")

// siblingDirtyRefusal carries the refusal text unchanged while unwrapping to ErrPairSiblingDirty.
type siblingDirtyRefusal struct{ msg string }

func (e siblingDirtyRefusal) Error() string { return e.msg }

func (e siblingDirtyRefusal) Unwrap() error { return ErrPairSiblingDirty }

// ErrPairNotFound is the sentinel Remove's error wraps when nothing of the pair remains: no worktree on either side, no branch locally or on origin, no portal and no launcher entry.
// It is worded without naming either side of the pair so callers outside the fabric vocabulary owner set can match it with errors.Is.
var ErrPairNotFound = errors.New("pair not found")

// The steps a Remove call reports in RemoveResult.Steps, a closed set; each is appended only after it changed state.
const (
	// RemoveStepRecordCommit is the commit of the sibling's pending records.
	RemoveStepRecordCommit = "record_commit"
	// RemoveStepArchive is the archive tag pushed to origin.
	RemoveStepArchive = "archive"
	// RemoveStepPortal is the removal of the pair's portal link.
	RemoveStepPortal = "portal"
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
	Steps []string `json:"steps"`
	// Finished is true when the task worktree was already gone at entry, so the call completed a half-removed pair.
	Finished bool `json:"finished"`
	// StrayPath names a path at the task worktree's location that is not a registered linked worktree; Remove reports it and never deletes it.
	StrayPath string `json:"stray_path,omitempty"`
	// RemoteBranchDeleted reports whether the pair's weft branch was observably removed from the
	// remote. It is true only when the remote deletion was attempted and actually removed a ref.
	RemoteBranchDeleted bool `json:"remote_branch_deleted,omitempty"`
	// RemoteBranchError is non-empty when the remote deletion was attempted and did not succeed. Its
	// text always names the layer that said no — the gate's own refusal, or the remote deletion
	// itself.
	RemoteBranchError string `json:"remote_branch_error,omitempty"`
	// RemoteSkippedReason carries a once-per-verb reason the sibling branch's remote copy was not deleted and no deletion was attempted:
	// with remote, a weft repo with no origin remote configured;
	// without remote, a sibling branch left only on origin, which is kept.
	RemoteSkippedReason string `json:"remote_skipped_reason,omitempty"`
	// WarpBranchDeleted reports whether the pair's local warp branch was deleted after the teardown.
	WarpBranchDeleted bool `json:"warp_branch_deleted"`
	// WarpBranchKeptReason is non-empty when the warp branch was left in place, naming why: the
	// destructive gate's refusal, or a failure to delete it. A kept branch is not a failure of Remove.
	WarpBranchKeptReason string `json:"warp_branch_kept_reason,omitempty"`
	// RemoteWarpBranchDeleted reports whether the pair's task branch was observably removed from the warp repo's origin.
	// It is true only with remote, when the branch's work was landed.
	RemoteWarpBranchDeleted bool `json:"remote_warp_branch_deleted"`
	// RemoteWarpBranchKeptReason is non-empty when remote was set and the task branch on origin was left in place, naming why: the gate's refusal or a failure to look it up or delete it.
	// A kept branch is not a failure of Remove.
	RemoteWarpBranchKeptReason string `json:"remote_warp_branch_kept_reason,omitempty"`
	// ArchiveTag names the archive/<slug>/<tip> tag pushed to the weft origin before the teardown;
	// empty when none was pushed.
	ArchiveTag string `json:"archive_tag,omitempty"`
	// ArchiveSkippedReason is non-empty when no archive was attempted — today only a weft repo with no origin remote configured.
	ArchiveSkippedReason string `json:"archive_skipped_reason,omitempty"`
}

// Remove removes a paired warp and weft git worktree with all associated artifacts.
// If force is false, both worktrees must be clean;
// if force is true, uncommitted changes are forcefully removed.
// It validates slug through the same validator Add uses (slug.go), so hub geometry — `_board`,
// `_portals`, `_launchers`, `_lyx`, `.lyx`, and every weft sibling — can never be handed to a
// teardown verb as if it were a pair.
// It refuses the hub's prime worktree outright too: the prime is the warp repository itself, not a
// pair this verb can tear down, and git's own refusal to remove a main working tree is not a
// licence to delete the clone.
// Its no-force dirtiness checks, status probes included, are refusals like the rest: a refusal leaves the hub and the weft origin untouched, with no tag pushed and nothing torn down.
// The sibling's dirtiness inside the record pathspec is no refusal: Remove commits it first, with or without force, so the archive covers it.
// After every refusal and before its first teardown mutation, Remove archives the pair's weft tip — an archive/<slug>/<tip> tag pushed to the weft origin (archiveWeftTip) — so the run records on the branch outlive its deletion.
// A failed archive returns its error with everything still in place, so a plain re-run retries it.
// A failure after the archive leaves the tag in place for the re-run to reuse.
// The archive runs whatever remote says, since it protects the local branch's commits as much as the remote copy, and force does not skip it: force answers dirtiness only.
// A weft repo with no origin proceeds, with ArchiveSkippedReason set on the result.
// Portal and launcher cleanup run after the archive but before the git removal.
// remote gates whether the pair's weft branch, once deleted locally, is also deleted on the weft
// repo's origin remote; a remote deletion failure never makes Remove return a non-nil error.
// Once both worktrees are removed, Remove deletes the local warp branch (BranchPrefix + slug) through
// the destructive gate, which refuses unless the branch's work is on another ref or landed on the
// parent recorded in the pair's origin record; a refusal fills WarpBranchKeptReason and Remove still
// succeeds. force never answers that check.
// With remote, Remove then deletes the task branch on the warp repository's origin, through the same gate and a lease on the observed remote tip, when the tip's work is landed;
// a refusal, a lost lease or a failed push fills RemoteWarpBranchKeptReason and Remove still succeeds.
// The recorded parent is read before any teardown, since the record lives in the weft worktree the
// teardown deletes.
// A pair whose task worktree is already gone is finished rather than refused: Remove performs whatever teardown remains, in the same order and through the same gates, and reports it in Steps with Finished set.
// With nothing of the pair left it returns an error wrapping ErrPairNotFound.
// A sibling branch present only on origin is archived from there and, with remote, deleted there; without remote it stays and the result says so.
// A path at the task worktree's location that is not a registered linked worktree is reported in StrayPath and never deleted.
func (t *Topology) Remove(l *lyxcwd.Location, slug string, force, remote bool) (res RemoveResult, err error) {
	rec := NewMutations(l.HubPath)
	defer func() { res.Mutations = rec.Snapshot() }()

	warpBranch := t.cfg.BranchPrefix + slug
	weftBranch := WeftBranchName(warpBranch)

	// Every refusal runs here, through the same probe a caller can ask without removing anything, so the two can never drift.
	if err := t.RemoveRefusal(l, slug, force); err != nil {
		return RemoveResult{}, err
	}

	target := WorktreePath(l, slug)
	pair, err := t.inspectPair(l, slug)
	if err != nil {
		return RemoveResult{}, err
	}
	steps := []string{}

	// Read the recorded parent now: the record lives in the weft worktree the teardown deletes. A
	// missing or unreadable record leaves parentBranch empty, which asks the gate for the stricter
	// reachability check alone.
	parentBranch := ""
	if origin, found, originErr := ReadOriginFor(l, slug); originErr == nil && found {
		parentBranch = origin.ParentBranch
	}

	// Commit the sibling's pending records so the archive tag covers them; force answers dirtiness only, so it never skips this step either.
	committed, err := commitPendingRecords(rec, l, slug, warpBranch)
	if err != nil {
		return RemoveResult{}, err
	}
	if committed {
		steps = append(steps, RemoveStepRecordCommit)
	}

	// Archive the weft tip after every refusal and before the first teardown mutation: a failed archive leaves the worktrees, portal, launchers and both branches untouched, so a plain re-run retries it.
	// force answers dirtiness only, so it never skips this step.
	archiveTag, archiveSkippedReason, err := archiveWeftTip(rec, l, slug, weftBranch)
	if err != nil {
		return RemoveResult{}, err
	}
	if archiveTag != "" {
		steps = append(steps, RemoveStepArchive)
	}

	// removePortal and removeLaunchers are best-effort: an operational failure is discarded exactly as
	// before, but a gate refusal must surface rather than vanish at the verb the slice's worst defect
	// came from.
	if err := surfaceRefusal(removePortal(rec, l, slug)); err != nil {
		return RemoveResult{}, err
	}
	if pair.portal && !pathPresent(PortalLink(l, slug)) {
		steps = append(steps, RemoveStepPortal)
	}
	if err := surfaceRefusal(removeLaunchers(rec, l, slug)); err != nil {
		return RemoveResult{}, err
	}
	if pair.launchers && !pathPresent(LauncherDir(l, slug)) {
		steps = append(steps, RemoveStepLaunchers)
	}

	// Sweep the ANCHORED directory, and only the links fabric itself created there.
	// The previous sweep read the worktree ROOT and removed every symlink it found: on a
	// subpath-anchored hub that saw none of the pair's junctions (they live at
	// <worktree>/<anchorRel>) and reported LinksRemoved: 0, and at a root anchor it deleted the
	// user's own checked-in symlinks alongside fabric's.
	// A task worktree that is gone, or a path there that is not a registered linked worktree, has no junctions of fabric's to sweep and nothing for the directory removal to take.
	linksRemoved := 0
	if pair.taskWorktree {
		if ownedNames, scanErr := scanOnDiskJunctionNames(l, slug); scanErr == nil {
			removeErr := removeWarpJunction(rec, l, slug, ownedNames)
			if err := surfaceRefusal(removeErr); err != nil {
				return RemoveResult{}, err
			}
			if removeErr == nil {
				linksRemoved = len(ownedNames)
			}
		}
		if err := removeWarpWorktreeDir(rec, l, target, force); err != nil {
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

	teardown, siblingSteps, err := t.removeSibling(rec, l, slug, weftBranch, pair, force, remote)
	if err != nil {
		return RemoveResult{}, err
	}
	steps = append(steps, siblingSteps...)

	warpDeleted, warpKeptReason := deleteWarpBranch(rec, l, warpBranch, parentBranch)
	if warpDeleted {
		steps = append(steps, RemoveStepTaskBranch)
	}

	remoteWarpDeleted, remoteWarpKeptReason := false, ""
	if remote {
		remoteWarpDeleted, remoteWarpKeptReason = deleteTaskBranchOnOrigin(rec, l, warpBranch, parentBranch)
		if remoteWarpDeleted {
			steps = append(steps, RemoveStepTaskBranchOnOrigin)
		}
	}

	strayPath := ""
	if pair.strayPath {
		strayPath = target
	}
	return RemoveResult{
		Slug:                 slug,
		Path:                 target,
		LinksRemoved:         linksRemoved,
		RemoteBranchDeleted:  teardown.remoteBranchDeleted,
		RemoteBranchError:    teardown.remoteBranchError,
		RemoteSkippedReason:  teardown.remoteSkippedReason,
		WarpBranchDeleted:    warpDeleted,
		WarpBranchKeptReason: warpKeptReason,

		RemoteWarpBranchDeleted:    remoteWarpDeleted,
		RemoteWarpBranchKeptReason: remoteWarpKeptReason,
		ArchiveTag:                 archiveTag,
		ArchiveSkippedReason:       archiveSkippedReason,
		Steps:                      steps,
		Finished:                   !pair.taskWorktree,
		StrayPath:                  strayPath,
	}, nil
}

// removeSibling tears down whatever of the pair's sibling remains: its worktree and local branch through removeWeftWorktree, or, when only origin holds the branch, its copy there.
// Without remote an origin-only copy stays, and the returned teardown says so.
// It returns the steps that changed state, in the order they ran.
func (t *Topology) removeSibling(rec *Mutations, l *lyxcwd.Location, slug, weftBranch string, pair pairState, force, remote bool) (weftTeardownResult, []string, error) {
	var steps []string

	if !pair.siblingWorktree && !pair.siblingBranch {
		weftRoot, err := WeftRepoRoot(l)
		if err != nil {
			return weftTeardownResult{}, nil, fmt.Errorf("resolve weft repo root: %w", err)
		}
		if _, urlErr := gitrepo.New(weftRoot).RemoteURL(originRemoteName); urlErr != nil {
			return weftTeardownResult{}, nil, nil
		}
		tip, err := remoteHeadTip(weftRoot, weftBranch)
		if err != nil {
			return weftTeardownResult{}, nil, err
		}
		if tip == "" {
			return weftTeardownResult{}, nil, nil
		}
		if !remote {
			return weftTeardownResult{remoteSkippedReason: fmt.Sprintf("the sibling branch %q exists only on %q and was kept; pass --remote to delete it", weftBranch, originRemoteName)}, nil, nil
		}
		entry := CleanupBranchEntry{Branch: weftBranch}
		deleteWeftBranchOnRemote(rec, l, weftBranch, t.cfg.BranchPrefix, weftRoot, &entry)
		if entry.RemoteDeleted {
			steps = append(steps, RemoveStepSiblingBranchOnOrigin)
		}
		return weftTeardownResult{remoteBranchDeleted: entry.RemoteDeleted, remoteBranchError: entry.RemoteError}, steps, nil
	}

	if !pair.siblingWorktree {
		if weftRoot, err := WeftRepoRoot(l); err == nil {
			// Same reason as the task side's prune: a registration left by a hand-removed worktree blocks the branch deletion.
			_, _ = gitexec.Run([]string{"worktree", "prune"}, weftRoot)
		}
	}

	// A weft-teardown failure is tolerated only when the weft worktree is actually gone (already
	// absent, or removed with just a branch/prune step failing) — a weft worktree still on disk
	// after a "successful" Remove is a half-torn pair the operator was never told about. This check
	// reads the error return alone, never the teardown struct: the remote outcome never affects it.
	teardown, weftErr := removeWeftWorktree(rec, l, slug, weftBranch, force, true, remote, t.cfg.BranchPrefix)
	if weftErr != nil {
		weftTarget := WeftWorktreePath(l, slug)
		if _, statErr := os.Stat(weftTarget); statErr == nil {
			return weftTeardownResult{}, nil, fmt.Errorf(
				"warp worktree removed, but weft teardown failed and the weft worktree remains at %s: %w",
				weftTarget, weftErr)
		}
	}
	if pair.siblingWorktree && !pathPresent(WeftWorktreePath(l, slug)) {
		steps = append(steps, RemoveStepSiblingWorktree)
	}
	if pair.siblingBranch && !weftBranchExists(l, weftBranch) {
		steps = append(steps, RemoveStepSiblingBranch)
	}
	if teardown.remoteBranchDeleted {
		steps = append(steps, RemoveStepSiblingBranchOnOrigin)
	}
	return teardown, steps, nil
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
	// portal and launchers report the hub-level entries Remove tears down.
	portal, launchers bool
}

// inspectPair reads pairState for slug.
func (t *Topology) inspectPair(l *lyxcwd.Location, slug string) (pairState, error) {
	var s pairState
	target := WorktreePath(l, slug)
	if pathPresent(target) {
		if isRegisteredLinkedWorktree(l, target) {
			s.taskWorktree = true
		} else {
			s.strayPath = true
		}
	}
	s.siblingWorktree = pathPresent(WeftWorktreePath(l, slug))
	s.portal = pathPresent(PortalLink(l, slug))
	s.launchers = pathPresent(LauncherDir(l, slug))

	taskBranch, err := localBranchExists(l.WorktreePath(), t.cfg.BranchPrefix+slug)
	if err != nil {
		return pairState{}, err
	}
	s.taskBranch = taskBranch
	s.siblingBranch = weftBranchExists(l, WeftBranchName(t.cfg.BranchPrefix+slug))
	return s, nil
}

// refuseWhenNothingRemains returns an error wrapping ErrPairNotFound when no part of the pair is left locally and neither of its branches is on origin.
// Origin is asked only after every local part is found gone, and only for a repository that has an origin, so a pair with any local remnant never costs a network call.
// A stray path at the pair's location is named in the error and left alone.
func (t *Topology) refuseWhenNothingRemains(l *lyxcwd.Location, slug string, s pairState) error {
	if s.taskWorktree || s.siblingWorktree || s.taskBranch || s.siblingBranch || s.portal || s.launchers {
		return nil
	}

	weftRoot, err := WeftRepoRoot(l)
	if err != nil {
		return fmt.Errorf("resolve weft repo root: %w", err)
	}
	warpBranch := t.cfg.BranchPrefix + slug
	for _, side := range []struct{ repoDir, branch string }{
		{l.WorktreePath(), warpBranch},
		{weftRoot, WeftBranchName(warpBranch)},
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
func deleteWarpBranch(rec *Mutations, l *lyxcwd.Location, warpBranch, parentBranch string) (deleted bool, keptReason string) {
	repoDir := l.WorktreePath()
	if _, err := gitexec.Run([]string{"rev-parse", "--verify", "--quiet", "refs/heads/" + warpBranch}, repoDir); err != nil {
		// rev-parse --verify --quiet exits 1, and only 1, for a ref that does not exist.
		var gitErr *gitexec.GitError
		if errors.As(err, &gitErr) && gitErr.ExitCode == 1 {
			return false, ""
		}
		return false, fmt.Sprintf("look up warp branch %s: %v", warpBranch, err)
	}

	err := deleteBranch(rec, branchRequest{
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
func deleteTaskBranchOnOrigin(rec *Mutations, l *lyxcwd.Location, warpBranch, parentBranch string) (deleted bool, keptReason string) {
	repoDir := l.WorktreePath()
	if _, err := gitrepo.New(repoDir).RemoteURL(originRemoteName); err != nil {
		return false, ""
	}
	tip, err := remoteHeadTip(repoDir, warpBranch)
	if err != nil {
		return false, err.Error()
	}
	if tip == "" {
		return false, ""
	}
	if _, err := gitexec.Run([]string{"fetch", originRemoteName, "refs/heads/" + warpBranch}, repoDir); err != nil {
		return false, fmt.Sprintf("fetch task branch %s from %s: %v", warpBranch, originRemoteName, err)
	}
	return deleteTaskBranchAtTip(rec, l, warpBranch, parentBranch, tip)
}

// deleteTaskBranchAtTip is deleteTaskBranchOnOrigin's gated deletion, leased to tip: a remote tip that moved since tip was observed fails the lease and nothing is deleted.
func deleteTaskBranchAtTip(rec *Mutations, l *lyxcwd.Location, warpBranch, parentBranch, tip string) (deleted bool, keptReason string) {
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
		return deleted, ""
	}
	if refusal, ok := RefusalOf(err); ok {
		return false, refusal.Reason
	}
	return false, fmt.Sprintf("delete task branch %s on %s: %v", warpBranch, originRemoteName, err)
}

// RemoveRefusal reports the refusal Remove would raise for slug before its first mutation, or nil when none applies.
// It covers slug validation, the prime refusal, a pair of which nothing remains (ErrPairNotFound), both merge-in-progress directions, task-side dirtiness without force, and sibling-side dirtiness outside the record pathspec without force.
// A gone task worktree skips only the checks that need it: task-side dirtiness and the task side of the merge probe.
// It mutates nothing, takes no lock and pushes nothing, and Remove calls it first, so the two can never drift.
// Its name carries no fabric side, because callers outside the owner set call it.
func (t *Topology) RemoveRefusal(l *lyxcwd.Location, slug string, force bool) error {
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
	blocked, err := mergeBlocksMutation(target, WeftWorktreePath(l, slug))
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

	names, err := PathspecNames(BoardDir(l.HubPath))
	if err != nil {
		return fmt.Errorf("load the record pathspec: %w", err)
	}
	return refuseDirtyWeftWorktree(WeftWorktreePath(l, slug), l.AnchorRel, names)
}

// commitPendingRecords commits the pair's sibling worktree's uncommitted changes inside the record pathspec, so the archive tag that follows covers them.
// It stages exactly the scoped pathspec `lyx fabric sync` uses, through CommitWeftPaths, with the fixed DefaultCommitMessage plus a Warp-SHA trailer naming the task worktree's HEAD, and pushes nothing.
// An absent sibling or an empty pathspec commits nothing; KindCommitCreated is recorded only when a commit landed, and committed reports it.
// The trailer names the task worktree's HEAD while it is present, else the local task branch's tip, else the last Warp-SHA trailer recorded on the sibling branch; with none of those the commit carries no trailer.
func commitPendingRecords(rec *Mutations, l *lyxcwd.Location, slug, warpBranch string) (committed bool, err error) {
	weftTarget := WeftWorktreePath(l, slug)
	if _, statErr := os.Stat(weftTarget); os.IsNotExist(statErr) {
		return false, nil
	}

	names, err := PathspecNames(BoardDir(l.HubPath))
	if err != nil {
		return false, fmt.Errorf("load the record pathspec: %w", err)
	}
	// git add refuses a pathspec that matches nothing, and a configured optional directory may not exist in this pair.
	present := make([]string, 0, len(names))
	for _, name := range names {
		if _, err := os.Lstat(filepath.Join(weftTarget, l.AnchorRel, name)); err == nil {
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
	_, committed, err = CommitWeftPaths(rec, weftTarget, l.AnchorRel, present, msg, SyncOptions{})
	if err != nil {
		return false, fmt.Errorf("commit the pair's pending records: %w", err)
	}
	return committed, nil
}

// recordTrailerSHA picks the Warp-SHA a record commit names: the task worktree's HEAD while that worktree is a present directory, else the local task branch's tip, else the last Warp-SHA trailer on the sibling branch at weftTarget.
// It returns an empty SHA when none of the three exists.
func recordTrailerSHA(l *lyxcwd.Location, slug, warpBranch, weftTarget string) (string, error) {
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
		sha, err := gitexec.Run([]string{"rev-parse", "--verify", "refs/heads/" + warpBranch + "^{commit}"}, l.WorktreePath())
		if err != nil {
			return "", fmt.Errorf("read the task branch tip: %w", err)
		}
		return strings.TrimSpace(sha), nil
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
// rev-parse --verify --quiet exits 1, and only 1, for a ref that does not exist; any other failure is returned.
func localBranchExists(repoDir, branch string) (bool, error) {
	if _, err := gitexec.Run([]string{"rev-parse", "--verify", "--quiet", "refs/heads/" + branch}, repoDir); err != nil {
		var gitErr *gitexec.GitError
		if errors.As(err, &gitErr) && gitErr.ExitCode == 1 {
			return false, nil
		}
		return false, fmt.Errorf("look up branch %s: %w", branch, err)
	}
	return true, nil
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
func refusePrimeSlug(l *lyxcwd.Location, slug string) error {
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
// `git worktree remove` refuses a main working tree, a path that is not a worktree of this repo at
// all, and a worktree carrying state it will not discard;
// treating every one of those refusals as licence to delete the directory turned an ordinary typo
// (`lyx fabric remove <prime>`, `lyx fabric remove _board`) into the loss of a whole git clone.
// A registered linked worktree is fabric's own pair member and nothing else, so deleting it after a
// git refusal is recoverable bookkeeping rather than data loss.
//
// The fallback is itself gated because it fires on ANY nonzero exit from `git worktree remove`, and
// `git worktree remove` without `--force` refuses on untracked files — an ungated fallback would
// therefore delete exactly the untracked files git had just declined to discard.
func removeWarpWorktreeDir(rec *Mutations, l *lyxcwd.Location, target string, force bool) error {
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
		return nil
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
		return fmt.Errorf(
			"git refused to remove worktree %s (git exit %d): %s; it is not a linked worktree of this repo, so fabric will not delete the directory itself",
			target, gitErr.ExitCode, strings.TrimSpace(gitErr.Stderr))
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
func isRegisteredLinkedWorktree(l *lyxcwd.Location, target string) bool {
	return isRegisteredLinkedWorktreeIn(l.WorktreePath(), target)
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
