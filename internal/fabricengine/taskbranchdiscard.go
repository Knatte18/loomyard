// taskbranchdiscard.go implements DiscardTaskBranch, the archive-and-delete of a pair's task branch locally and on origin, and deleteArchivedTaskBranch, the deletion half that only runs under an archive tag covering each tip.
// Destruction itself stays in destroy.go's executors.

package fabricengine

import (
	"fmt"
	"slices"
	"strings"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

// DiscardResult is what DiscardTaskBranch did.
// It embeds MutationRecord, which carries the mutation record accumulated over the call.
// ArchiveTags lists the archive tags that now hold the discarded tips, none when no branch existed.
type DiscardResult struct {
	MutationRecord
	ArchiveTags   []string `json:"archive_tags,omitempty"`
	LocalDeleted  bool     `json:"local_deleted,omitempty"`
	OriginDeleted bool     `json:"origin_deleted,omitempty"`
}

// DiscardTaskBranch archives and deletes slug's task branch locally and on the task repository's origin, whether or not its work is landed.
// Every tip is tagged archive/<slug>/<first 12 hex> and the tag pushed to origin before any branch is deleted, so a failed archive deletes nothing and a re-run retries it.
// When one tip contains the other only the containing tip is archived; otherwise both are.
// A branch present neither locally nor on origin returns a zero result and no error.
// A lost origin lease or a refused gate is returned as an error naming the branch.
func (t *Topology) DiscardTaskBranch(l *lyxcwd.Location, slug string) (res DiscardResult, err error) {
	rec := NewMutations(l.HubPath)
	defer func() { res.Mutations = rec.Snapshot() }()

	repoDir := l.WorktreePath()
	warpBranch := t.cfg.BranchPrefix + slug

	localOut, err := gitexec.Run([]string{"for-each-ref", "--format=%(objectname)", "refs/heads/" + warpBranch}, repoDir)
	if err != nil {
		return DiscardResult{}, fmt.Errorf("resolve task branch %q: %w", warpBranch, err)
	}
	localTip := strings.TrimSpace(localOut)

	originTip := ""
	if _, urlErr := gitrepo.New(repoDir).RemoteURL(originRemoteName); urlErr == nil {
		originTip, err = remoteHeadTip(repoDir, warpBranch)
		if err != nil {
			return DiscardResult{}, err
		}
	}
	if originTip != "" {
		if _, err := gitexec.Run([]string{"fetch", originRemoteName, "refs/heads/" + warpBranch}, repoDir); err != nil {
			return DiscardResult{}, fmt.Errorf("fetch task branch %q from %q: %w", warpBranch, originRemoteName, err)
		}
	}
	if localTip == "" && originTip == "" {
		return DiscardResult{}, nil
	}

	localTag, originTag, err := t.archiveDiscardedTips(rec, repoDir, slug, localTip, originTip)
	if err != nil {
		return DiscardResult{}, err
	}
	for _, tag := range []string{localTag, originTag} {
		if tag != "" && !slices.Contains(res.ArchiveTags, tag) {
			res.ArchiveTags = append(res.ArchiveTags, tag)
		}
	}

	res.LocalDeleted, res.OriginDeleted, err = deleteArchivedTaskBranch(rec, repoDir, warpBranch, localTag, originTag, originTip)
	return res, err
}

// archiveDiscardedTips archives the tips DiscardTaskBranch must keep, returning the tag covering the local tip and the one covering the origin tip, each empty when that side has no branch.
// A tip contained in the other is covered by the other's tag.
func (t *Topology) archiveDiscardedTips(rec *Mutations, repoDir, slug, localTip, originTip string) (localTag, originTag string, err error) {
	switch {
	case originTip == "":
		localTag, err = archiveTaskTip(rec, repoDir, slug, localTip)
		return localTag, "", err
	case localTip == "":
		originTag, err = archiveTaskTip(rec, repoDir, slug, originTip)
		return "", originTag, err
	}

	localInOrigin, err := isAncestorOrEqual(repoDir, localTip, originTip)
	if err != nil {
		return "", "", fmt.Errorf("compare the local and origin tips of the task branch: %w", err)
	}
	if localInOrigin {
		tag, err := archiveTaskTip(rec, repoDir, slug, originTip)
		return tag, tag, err
	}
	originInLocal, err := isAncestorOrEqual(repoDir, originTip, localTip)
	if err != nil {
		return "", "", fmt.Errorf("compare the local and origin tips of the task branch: %w", err)
	}
	if originInLocal {
		tag, err := archiveTaskTip(rec, repoDir, slug, localTip)
		return tag, tag, err
	}
	if localTag, err = archiveTaskTip(rec, repoDir, slug, localTip); err != nil {
		return "", "", err
	}
	originTag, err = archiveTaskTip(rec, repoDir, slug, originTip)
	return localTag, originTag, err
}

// deleteArchivedTaskBranch deletes warpBranch from origin, leased to leaseTip, under originTag and then from the repo at repoDir under localTag.
// An empty tag skips that side.
// A refused gate or a lost lease is returned as an error naming the branch, and a side not yet deleted is left in place.
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
