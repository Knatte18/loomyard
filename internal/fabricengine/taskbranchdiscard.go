// taskbranchdiscard.go implements DiscardTaskBranch, which archives and deletes a task branch locally and on origin.
// It also holds deleteArchivedTaskBranch, which deletes the branch only under an archive tag covering each tip.
// Destruction itself stays in destroy.go's executors.

package fabricengine

import (
	"fmt"
	"time"

	"github.com/Knatte18/loomyard/internal/lymcwd"
)

// DiscardResult reports what a discard of a task branch did.
// It embeds MutationRecord, which holds the mutations accumulated over the call.
// ArchiveTags lists the archive tags that now hold the discarded tips, none when no branch existed.
type DiscardResult struct {
	MutationRecord
	ArchiveTags   []string `json:"archive_tags,omitempty"`
	LocalDeleted  bool     `json:"local_deleted,omitempty"`
	OriginDeleted bool     `json:"origin_deleted,omitempty"`
}

// DiscardTaskBranch archives and deletes slug's task branch locally and on the task repository's origin.
// It does so whether or not the pair's work is landed.
// The task branch is the one the pair stands on, read from the pair itself.
// With no local branch of the pair's names, the first name origin holds is taken,
// so a pair made before the branch prefix changed is discarded by its own branch.
// It tags the local tip archive/<slug>/<first 12 hex>, and deletes the local branch under that tag.
// It then hands the remote half to the holder of the slug's teardown lock, and returns without waiting on it.
// The remote half reads origin's tip and pushes the tags covering the tips.
// When one tip holds the other, it archives only the containing tip.
// It then deletes the branch on origin with a lease on the tip it read.
// A failure there is logged and mailed to the hub's orch with its way forward.
// A failed local tag deletes nothing and wraps ErrArchiveFailed,
// and a refused gate is returned as an error naming the branch.
// ArchiveTags names the local tip's tag, or, when the remote half ran in process, the tags it pushed.
// Before its first tip read, it takes the slug's teardown lock exclusive, waiting at most the topology's teardown wait.
// The lock is held until the remote half ends.
// When the wait runs out, it returns an error wrapping ErrPairPushRunning.
// The error names re-running the command that discarded the task branch, with nothing archived or deleted.
func (t *Topology) DiscardTaskBranch(l *lymcwd.Location, slug string) (res DiscardResult, err error) {
	rec := NewMutations(l.HubPath)
	defer func() { res.Mutations = rec.Snapshot() }()

	// The lock file is hub scratch and outlives the call, not a change to the pair,
	// so it goes to a record of its own, and a slug with no branch keeps a zero result.
	hold, err := t.holdTeardown(NewMutations(l.HubPath), rec, l, slug, false, "re-run the command that discarded the task branch", true)
	if err != nil {
		return DiscardResult{}, err
	}
	var job *remoteTeardownJob
	defer func() {
		if job == nil {
			hold.finish(nil)
		}
	}()

	repoDir := l.WorktreePath()
	warpBranch, err := t.PairTaskBranch(l, slug)
	if err != nil {
		return DiscardResult{}, err
	}
	localTip, err := localBranchTip(repoDir, warpBranch)
	if err != nil {
		return DiscardResult{}, err
	}
	if localTip == "" {
		// No local branch names the task branch, so origin names it.
		// That is required for a pair made before the branch prefix changed.
		onOrigin, err := t.originTaskBranch(l, slug)
		if err != nil {
			return DiscardResult{}, err
		}
		if onOrigin != "" {
			warpBranch = onOrigin
		}
	}
	if localTip != "" {
		localTag, err := createArchiveTag(repoDir, slug, localTip, "task")
		if err != nil {
			return DiscardResult{}, fmt.Errorf("%w: %w", ErrArchiveFailed, err)
		}
		if _, _, err := deleteArchivedTaskBranch(rec, repoDir, warpBranch, localTag, "", ""); err != nil {
			return DiscardResult{}, err
		}
		res.LocalDeleted = true
		res.ArchiveTags = []string{localTag}
	}

	job = &remoteTeardownJob{Kind: remoteTeardownDiscard, Slug: slug, Worktree: repoDir, WarpBranch: warpBranch, TaskTip: localTip}
	out := hold.finish(job)
	if out.ran {
		res.ArchiveTags, res.OriginDeleted = out.discard.ArchiveTags, out.discard.OriginDeleted
		return res, out.err
	}
	return res, nil
}

// archiveDiscardedTips archives the tips a discard must keep.
// It returns the tag covering the local tip and the tag covering the origin tip.
// Each is empty when that side has no branch.
// A tip contained in the other is covered by the other's tag; sleep spaces an archive-tag push's attempts.
func archiveDiscardedTips(rec *Mutations, repoDir, slug, localTip, originTip string, sleep func(time.Duration)) (localTag, originTag string, err error) {
	switch {
	case originTip == "":
		localTag, err = archiveTaskTip(rec, repoDir, slug, localTip, sleep)
		return localTag, "", err
	case localTip == "":
		originTag, err = archiveTaskTip(rec, repoDir, slug, originTip, sleep)
		return "", originTag, err
	}

	localInOrigin, err := isAncestorOrEqual(repoDir, localTip, originTip)
	if err != nil {
		return "", "", fmt.Errorf("compare the local and origin tips of the task branch: %w", err)
	}
	if localInOrigin {
		tag, err := archiveTaskTip(rec, repoDir, slug, originTip, sleep)
		return tag, tag, err
	}
	originInLocal, err := isAncestorOrEqual(repoDir, originTip, localTip)
	if err != nil {
		return "", "", fmt.Errorf("compare the local and origin tips of the task branch: %w", err)
	}
	if originInLocal {
		tag, err := archiveTaskTip(rec, repoDir, slug, localTip, sleep)
		return tag, tag, err
	}
	if localTag, err = archiveTaskTip(rec, repoDir, slug, localTip, sleep); err != nil {
		return "", "", err
	}
	originTag, err = archiveTaskTip(rec, repoDir, slug, originTip, sleep)
	return localTag, originTag, err
}

// deleteArchivedTaskBranch deletes warpBranch from origin under originTag, leased to leaseTip.
// It then deletes it from the repo at repoDir under localTag.
// An empty tag skips that side.
// A refused gate or a lost lease is returned as an error naming the branch,
// and a side not yet deleted is left in place.
func deleteArchivedTaskBranch(rec *Mutations, repoDir, warpBranch, localTag, originTag, leaseTip string) (localDeleted, originDeleted bool, err error) {
	if originTag != "" {
		originDeleted, err = deleteRemoteBranch(rec, remoteBranchRequest{
			what:      "delete archived task branch on origin",
			repoDir:   repoDir,
			remote:    originRemoteName,
			branch:    warpBranch,
			ownership: ownedPairWarpBranch(warpBranch, ""),
			dirtiness: dirtyArchivedOnRemote(originTag),
			leaseSHA:  leaseTip,
		})
		if err != nil {
			return false, false, fmt.Errorf("delete task branch %q on %q: %w", warpBranch, originRemoteName, err)
		}
	}
	if localTag != "" {
		err = deleteBranch(rec, branchRequest{
			what:      "delete archived task branch",
			repoDir:   repoDir,
			branch:    warpBranch,
			ownership: ownedPairWarpBranch(warpBranch, ""),
			dirtiness: dirtyArchivedTip(localTag),
		})
		if err != nil {
			return false, originDeleted, fmt.Errorf("delete local task branch %q: %w", warpBranch, err)
		}
		localDeleted = true
	}
	return localDeleted, originDeleted, nil
}
