// mergerecords.go keeps the receiving side's records through a merge in a one-repository hub.
// There a worktree's records live on its code branch beside its code, so a merge would carry the source's records in too.
// The helpers commit the receiving records ahead of the merge and refuse a source that edited the anchor marker by hand.
// Once the merge is staged they set the records paths of both sides back to the receiving HEAD's.

package fabricengine

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/gitrepo"
)

// receivingRecordsCommitMessage is the subject of the commit that takes the receiving worktree's uncommitted records.
const receivingRecordsCommitMessage = "fabric merge: commit the records before the merge"

// mergeLiteralPathspecEnv makes git read the records paths it is given as paths, never as patterns.
var mergeLiteralPathspecEnv = []string{"GIT_LITERAL_PATHSPECS=1"}

// startMerge stages the merge of source into f's checkout without committing it.
// In a one-repository worktree the merge never fast-forwards.
// A source ahead of the receiver then still makes a merge commit, whose records the settling step keeps.
func (f *Fabric) startMerge(source string, squash, oneRepository bool) (gitrepo.MergeOutcome, error) {
	if oneRepository {
		return f.code.MergeStartNoFastForward(source, squash)
	}
	return f.code.MergeStart(source, squash)
}

// prepareOneRepositoryMerge runs what a merge in a one-repository worktree does before it starts, under the write lock.
// It refuses a source that edited the anchor marker by hand, then commits the receiving worktree's uncommitted records.
// A refusal leaves the checkout as it was.
func (f *Fabric) prepareOneRepositoryMerge(rec *Mutations, sourceSHA string) error {
	head, err := f.code.CurrentSHA()
	if err != nil {
		return fmt.Errorf("fabricengine: resolve checkout HEAD: %w", err)
	}
	if err := refuseSourceMarkerChange(f.warpPath, head, sourceSHA); err != nil {
		return err
	}
	return f.commitReceivingRecords(rec)
}

// commitReceivingRecords commits the records paths of HEAD that carry uncommitted changes.
// Its caller holds the write lock.
// It commits nothing when no records path has content to commit.
func (f *Fabric) commitReceivingRecords(rec *Mutations) error {
	recordsPaths, err := RecordsPathsAt(f.warpPath, "HEAD")
	if err != nil {
		return err
	}
	present, positive, err := weftPathspecFilter(f.warpPath, recordsPaths)
	if err != nil {
		return err
	}
	if !positive {
		return nil
	}
	_, _, err = commitRecordsPathsHeld(rec, f.warpPath, ".", present, receivingRecordsCommitMessage, SyncOptions{})
	return err
}

// refuseSourceMarkerChange refuses a merge of source into receiving whose source branch edited `.lym-anchor` by hand.
// Both are commits of the repository at repoDir.
// It passes when the marker at source equals the one at the merge base, or when no source commit edited it.
// Fabric merge commits are not edits, since their marker was settled by the merge that made them.
// Otherwise it passes when the newest source commit that edits the marker carries an AnchorChangeTrailerKey trailer.
// It returns an *ErrMarkerChange for it when that commit carries none.
func refuseSourceMarkerChange(repoDir, receiving, source string) error {
	base, err := gitrepo.New(repoDir).MergeBase(receiving, source)
	if err != nil {
		return fmt.Errorf("fabricengine: find the merge base of the merge source: %w", err)
	}
	sourceMarker, sourceFound, err := committedMarker(repoDir, source)
	if err != nil {
		return err
	}
	baseMarker, baseFound, err := committedMarker(repoDir, base)
	if err != nil {
		return err
	}
	if sourceFound == baseFound && sourceMarker == baseMarker {
		return nil
	}

	listed, err := gitexec.Run([]string{"rev-list", "--topo-order", source, "^" + receiving}, repoDir)
	if err != nil {
		return fmt.Errorf("fabricengine: list the commits the merge brings in: %w", err)
	}
	for _, sha := range strings.Fields(listed) {
		edits, err := editsAnchorMarker(repoDir, sha)
		if err != nil {
			return err
		}
		if !edits {
			continue
		}
		fabricMerge, err := IsFabricMergeCommit(repoDir, sha)
		if err != nil {
			return err
		}
		if fabricMerge {
			continue
		}
		message, err := gitexec.Run([]string{"show", "-s", "--format=%B", sha}, repoDir)
		if err != nil {
			return fmt.Errorf("fabricengine: read the message of %s in %s: %w", sha, repoDir, err)
		}
		if trailerValue(message, AnchorChangeTrailerKey) != "" {
			return nil
		}
		return &ErrMarkerChange{Commit: sha}
	}
	return nil
}

// editsAnchorMarker reports whether the commit sha in the repository at repoDir changes the anchor marker.
// The comparison is against its first parent.
// A root commit changes it when it holds one.
func editsAnchorMarker(repoDir, sha string) (bool, error) {
	marker, found, err := committedMarker(repoDir, sha)
	if err != nil {
		return false, err
	}
	parents, err := parentsOf(repoDir, sha)
	if err != nil {
		return false, err
	}
	if len(parents) == 0 {
		return found, nil
	}
	parentMarker, parentFound, err := committedMarker(repoDir, parents[0])
	if err != nil {
		return false, err
	}
	return found != parentFound || marker != parentMarker, nil
}

// settleRecordsAfterMergeStart runs the steps that follow a merge start in a one-repository worktree.
// It runs afterMergeStart when set, settles the records and returns the outcome the merge now has.
// Records conflicts end with the settling, so a merge that conflicted on records alone comes back staged.
// A squash that has nothing staged once its records are settled comes back as already up to date.
// The outcome is saved to the merge record.
// An error leaves the merge parked, with its record standing.
func (f *Fabric) settleRecordsAfterMergeStart(rec *Mutations, st *mergeState, outcome gitrepo.MergeOutcome) (gitrepo.MergeOutcome, error) {
	if outcome == gitrepo.MergeAlreadyUpToDate {
		st.RecordsUnsettled = false
		return outcome, f.saveMergeState(st)
	}
	if f.afterMergeStart != nil {
		if err := f.afterMergeStart(); err != nil {
			return outcome, err
		}
	}
	if _, err := f.settleSourceRecords(rec, st); err != nil {
		return outcome, err
	}

	conflicted, err := f.code.ConflictedFiles()
	if err != nil {
		return outcome, err
	}
	if len(conflicted) == 0 && outcome == gitrepo.MergeConflicted {
		outcome = gitrepo.MergeStaged
	}
	if st.Squash && outcome == gitrepo.MergeStaged {
		_, diffErr := gitexec.Run([]string{"diff", "--cached", "--quiet"}, f.warpPath)
		switch {
		case diffErr == nil:
			outcome = gitrepo.MergeAlreadyUpToDate
		case !exitedWithStatusOne(diffErr):
			return outcome, fmt.Errorf("fabricengine: check what the squash staged: %w", diffErr)
		}
	}
	st.WarpOutcome = mergeOutcomeString(outcome)
	return outcome, f.saveMergeState(st)
}

// settleSourceRecords sets the records paths of both sides of the staged merge st records back to the receiving HEAD's.
// It does so in index and worktree, and a path only the source holds is removed.
// It returns the paths it set back, and clears RecordsUnsettled in the saved record.
// A record already settled changes nothing.
// A records path whose index or worktree content changed since the merge was staged refuses with a *MergeGuardError.
// The refusal names the path as dirty and leaves the merge parked with its records unsettled.
func (f *Fabric) settleSourceRecords(rec *Mutations, st *mergeState) ([]string, error) {
	if !st.RecordsUnsettled {
		return nil, nil
	}
	receivingPaths, err := RecordsPathsAt(f.warpPath, "HEAD")
	if err != nil {
		return nil, err
	}
	sourcePaths, err := RecordsPathsAt(f.warpPath, st.WarpSource)
	if err != nil {
		return nil, err
	}
	paths := slices.Compact(slices.Sorted(slices.Values(slices.Concat(receivingPaths, sourcePaths))))

	changed, err := f.recordsChangedSinceMergeStart(paths)
	if err != nil {
		return nil, err
	}
	if len(changed) > 0 {
		refusal := newMergeGuardError([]string{mergeReasonWorktreeDirty}).withDirt(f.warpPath, changed)
		refusal.Parked = true
		return nil, refusal
	}

	known, err := f.recordsPathsGitKnows(paths)
	if err != nil {
		return nil, err
	}
	if len(known) > 0 {
		if err := restoreRecordsToHead(rec, f.warpPath, known); err != nil {
			return nil, err
		}
	}
	st.RecordsUnsettled = false
	if err := f.saveMergeState(st); err != nil {
		return nil, err
	}
	return known, nil
}

// recordsChangedSinceMergeStart returns the sorted paths under the given records paths.
// It returns those whose worktree content differs from the index, and the untracked ones.
// A staged merge leaves none of them, so each was written after the merge was staged.
// A path with unmerged index entries is the merge's own conflict and is left out.
func (f *Fabric) recordsChangedSinceMergeStart(paths []string) ([]string, error) {
	conflicted, err := f.code.ConflictedFiles()
	if err != nil {
		return nil, err
	}
	modified, err := gitexec.RunWithEnv(append([]string{"diff", "--name-only", "-z", "--"}, paths...), f.warpPath, mergeLiteralPathspecEnv)
	if err != nil {
		return nil, fmt.Errorf("fabricengine: list the records paths changed since the merge was staged: %w", err)
	}
	untracked, err := gitexec.RunWithEnv(append([]string{"ls-files", "--others", "--exclude-standard", "-z", "--"}, paths...), f.warpPath, mergeLiteralPathspecEnv)
	if err != nil {
		return nil, fmt.Errorf("fabricengine: list the untracked records paths: %w", err)
	}
	changed := make(map[string]bool)
	for _, entry := range strings.Split(modified+"\x00"+untracked, "\x00") {
		if entry != "" && !slices.Contains(conflicted, entry) {
			changed[entry] = true
		}
	}
	return slices.Sorted(maps.Keys(changed)), nil
}

// recordsPathsGitKnows returns the paths of the given records paths that the index or HEAD holds.
// Git refuses a restore of a path it knows nowhere.
func (f *Fabric) recordsPathsGitKnows(paths []string) ([]string, error) {
	var known []string
	for _, path := range paths {
		indexed, err := gitexec.RunWithEnv([]string{"ls-files", "--cached", "-z", "--", path}, f.warpPath, mergeLiteralPathspecEnv)
		if err != nil {
			return nil, fmt.Errorf("fabricengine: look for %s in the index: %w", path, err)
		}
		committed, err := gitexec.RunWithEnv([]string{"ls-tree", "--name-only", "HEAD", "--", path}, f.warpPath, mergeLiteralPathspecEnv)
		if err != nil {
			return nil, fmt.Errorf("fabricengine: look for %s at HEAD: %w", path, err)
		}
		if indexed != "" || committed != "" {
			known = append(known, path)
		}
	}
	return known, nil
}
