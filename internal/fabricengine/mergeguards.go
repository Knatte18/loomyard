// mergeguards.go holds the shared merge precondition helpers: guard evaluation and per-side merge-source resolution.
// It also covers the attached-HEAD and upstream-sync checks, and MergeAbort's conclude-already-landed check.
// Every helper returns reasons for aggregation via newMergeGuardError.
// Helpers change no worktree, index, branch tip or fabric record.
// The one exception is remote-tracking state: resolveMergeSources runs a best-effort Fetch() on both sides.

package fabricengine

import (
	"fmt"
	"path/filepath"
	"sort"

	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/logger"
)

// mergeSources holds, per side, the SHA a merge actually merges.
// The freshness rule has already picked it between a local branch and its remote-tracking counterpart.
type mergeSources struct {
	warpSHA string
	weftSHA string
}

// resolveMergeSources resolves the SHA each side of f merges, applying the freshness rule independently per side.
// The warp side resolves source itself.
// The weft side resolves RecordsBranchName(source), the sole "-weft" composition (branchname.go).
// Both sides run a best-effort Fetch first; a fetch failure is logged via logger.Warn and is never fatal.
// Each side then resolves its local branch and its origin/<branch> remote-tracking ref via gitrepo.ResolveSHA.
// A warp source resolvable on neither local nor remote appends mergeReasonSourceNotFound.
// That is the only reason this function returns.
//
// The weft SHA is still resolved and returned, best-effort and possibly empty.
// mergeState.WeftSource needs a value even then.
// This function never refuses on the weft's behalf.
// An unresolvable weft counterpart leaves weftSHA empty and appends no reason.
// It never appends mergeReasonNotFabricManaged.
// Its constant stays only because mergevocab_test.go pins the closed reason set.
//
// Every reason is collected rather than returned early, so each guard runs regardless of an earlier failure.
// newMergeGuardError deduplicates, so a source missing on both sides reports one mergeReasonSourceNotFound.
// It does not disclose that both sides were checked.
func resolveMergeSources(f *Fabric, source string) (mergeSources, []string) {
	var reasons []string
	weftBranch := RecordsBranchName(source)

	if err := f.code.Fetch(); err != nil {
		logger.Warn("fabricengine: best-effort fetch before merge source resolution failed", "side", "warp", "error", err)
	}
	warpLocalSHA, warpLocalErr := f.code.ResolveSHA(source)
	warpRemoteSHA, warpRemoteErr := f.code.ResolveSHA("origin/" + source)
	warpSHA, warpFound := pickMergeSourceSHA(f.code, warpLocalSHA, warpLocalErr == nil, warpRemoteSHA, warpRemoteErr == nil)
	if !warpFound {
		reasons = append(reasons, mergeReasonSourceNotFound)
	}

	if err := f.records.Fetch(); err != nil {
		logger.Warn("fabricengine: best-effort fetch before merge source resolution failed", "side", "weft", "error", err)
	}
	weftLocalSHA, weftLocalErr := f.records.ResolveSHA(weftBranch)
	weftRemoteSHA, weftRemoteErr := f.records.ResolveSHA("origin/" + weftBranch)
	weftSHA, _ := pickMergeSourceSHA(f.records, weftLocalSHA, weftLocalErr == nil, weftRemoteSHA, weftRemoteErr == nil)

	return mergeSources{warpSHA: warpSHA, weftSHA: weftSHA}, reasons
}

// pickMergeSourceSHA implements the freshness rule for one side.
// It picks remoteSHA when the local branch is absent, or when localSHA is a proper ancestor of remoteSHA;
// otherwise it picks localSHA.
// It reports found=false only when neither the local branch nor its remote-tracking ref resolved.
// The ancestry probe is best-effort: an IsAncestor failure falls back to the local SHA and is logged via logger.Warn.
// A degraded pick, merging a possibly-stale local tip, therefore always leaves a trace.
func pickMergeSourceSHA(repo *gitrepo.Repo, localSHA string, localFound bool, remoteSHA string, remoteFound bool) (sha string, found bool) {
	switch {
	case !localFound && !remoteFound:
		return "", false
	case !localFound:
		return remoteSHA, true
	case !remoteFound:
		return localSHA, true
	}

	if localSHA == remoteSHA {
		return localSHA, true
	}
	isAncestor, err := repo.IsAncestor(localSHA, remoteSHA)
	if err != nil {
		logger.Warn("fabricengine: freshness-rule ancestry probe failed; merging the local tip", "local", localSHA, "remote", remoteSHA, "error", err)
		return localSHA, true
	}
	if isAncestor {
		return remoteSHA, true
	}
	return localSHA, true
}

// pairDirtyReason reports mergeReasonWorktreeDirty when f's warp checkout carries uncommitted tracked changes.
// The weft side is not evaluated.
// It is not a merge participant, so its dirtiness cannot affect a warp-only merge's correctness.
// Checking it could only refuse a merge that would have been right.
// The returned paths are the checkout's worktree-relative, sorted tracked dirty paths, so a guard error can carry them.
func pairDirtyReason(f *Fabric) ([]string, []string, error) {
	paths, err := codeDirtyPaths(f)
	if err != nil {
		return nil, nil, fmt.Errorf("fabricengine: check checkout dirtiness: %w", err)
	}

	if len(paths) > 0 {
		return []string{mergeReasonWorktreeDirty}, paths, nil
	}
	return nil, nil, nil
}

// codeDirtyPaths returns the tracked uncommitted paths of f's code checkout, relative to it, slash-separated and sorted.
func codeDirtyPaths(f *Fabric) ([]string, error) {
	absolute, err := trackedDirtyPaths(f.warpPath)
	if err != nil {
		return nil, err
	}
	paths := make([]string, 0, len(absolute))
	for _, p := range absolute {
		rel, err := filepath.Rel(f.warpPath, p)
		if err != nil {
			return nil, fmt.Errorf("relativize %s to %s: %w", p, f.warpPath, err)
		}
		paths = append(paths, filepath.ToSlash(rel))
	}
	sort.Strings(paths)
	return paths, nil
}

// upstreamSHAAt resolves dir's checked-out branch's upstream tracking ref to a plain SHA.
// No configured upstream, a missing upstream ref and a detached HEAD all report hasUpstream=false, not an error.
func upstreamSHAAt(dir string) (sha string, hasUpstream bool, err error) {
	repo := gitrepo.New(dir)
	branch, detached, err := repo.HeadRef()
	if err != nil {
		return "", false, fmt.Errorf("fabricengine: resolve upstream in %s: %w", dir, err)
	}
	if detached {
		return "", false, nil
	}
	sha, hasUpstream, err = repo.Upstream(branch)
	if err != nil {
		return "", false, fmt.Errorf("fabricengine: resolve upstream in %s: %w", dir, err)
	}
	return sha, hasUpstream, nil
}

// detachedHeadReason reports mergeReasonDetachedHead when f's warp checkout has a detached HEAD.
// A merge concluded on a detached HEAD lands a commit no ref reaches, so the next checkout discards it silently.
// Refusing before the attempt starts is the only point where that loss is avoided.
// The weft side is not evaluated.
// It is not a merge participant, so its head attachment cannot affect a warp-only merge's correctness.
// Checking it could only refuse a merge that would have been right.
func detachedHeadReason(f *Fabric) ([]string, error) {
	warpDetached, err := f.code.HeadDetached()
	if err != nil {
		return nil, fmt.Errorf("fabricengine: check checkout head attachment: %w", err)
	}

	if warpDetached {
		return []string{mergeReasonDetachedHead}, nil
	}
	return nil, nil
}

// mergeInProgressReason reports mergeReasonAlreadyInProgress when a fabric-written merge-state
// record already exists on f's pair.
func mergeInProgressReason(f *Fabric) ([]string, error) {
	exists, err := f.mergeRecordExists()
	if err != nil {
		return nil, err
	}
	if exists {
		return []string{mergeReasonAlreadyInProgress}, nil
	}
	return nil, nil
}

// syncedToUpstreamReason reports mergeReasonNotSynced when f's warp side is diverged from its own upstream.
// A checkout with no upstream passes.
// An upstream passes when the tip is not diverged from it.
// An upstream that is an ancestor of HEAD (in sync or ahead) passes.
// So does HEAD being an ancestor of the upstream (behind), since the pre-merge sync step advances it.
// Only a divergence, where neither is an ancestor of the other, fails.
// The weft side is not evaluated.
// Per-transition status pushes warn and continue on a rejected push, so a locally diverged weft is routine.
// Refusing on it would block every later landing.
//
// This is a pre-fetch fast path, not the whole not-synced precondition.
// Every helper in this file reads @{u} from remote-tracking state the checkout already carries.
// Nothing before this runs fetches.
// A divergence someone else pushed, which this checkout has not fetched, therefore reads as ahead and passes.
// syncSideBeforeMerge fetches and re-decides the same predicate after that fetch.
// It refuses with the same reason, so the promise holds even when this fast path cannot see the divergence.
// Anything that must hold post-fetch belongs there too.
func syncedToUpstreamReason(f *Fabric) ([]string, error) {
	warpNotSynced, err := sideNotSyncedToUpstream(f.code, f.warpPath)
	if err != nil {
		return nil, err
	}
	if warpNotSynced {
		return []string{mergeReasonNotSynced}, nil
	}
	return nil, nil
}

// sideNotSyncedToUpstream reports whether dir's HEAD has diverged from its upstream.
// It reports false when dir has no upstream or when HEAD equals the upstream.
// It also reports false when the upstream is an ancestor of HEAD, which covers in sync and ahead.
// It reports false when HEAD is an ancestor of the upstream, which means behind.
// It reports true only when neither commit is an ancestor of the other.
func sideNotSyncedToUpstream(repo *gitrepo.Repo, dir string) (bool, error) {
	upstreamSHA, hasUpstream, err := upstreamSHAAt(dir)
	if err != nil {
		return false, err
	}
	if !hasUpstream {
		return false, nil
	}

	head, err := repo.CurrentSHA()
	if err != nil {
		return false, fmt.Errorf("fabricengine: resolve HEAD in %s: %w", dir, err)
	}
	if head == upstreamSHA {
		return false, nil
	}

	upstreamAncestorOfHead, err := repo.IsAncestor(upstreamSHA, head)
	if err != nil {
		return false, fmt.Errorf("fabricengine: classify sync state in %s: %w", dir, err)
	}
	if upstreamAncestorOfHead {
		return false, nil
	}

	headAncestorOfUpstream, err := repo.IsAncestor(head, upstreamSHA)
	if err != nil {
		return false, fmt.Errorf("fabricengine: classify sync state in %s: %w", dir, err)
	}
	if headAncestorOfUpstream {
		return false, nil
	}

	return true, nil
}

// concludeLandedReason reports mergeReasonConcludeLanded when either side may already carry a conclude commit.
// Aborting the merge would reset both sides to their recorded pre-merge SHAs, which discards a commit that landed.
// In a merge-in with conflicts, that commit holds the operator's own conflict resolutions.
// The reset runs with force, so nothing else stops it.
// Refusing leaves MergeContinue as the one correct recovery.
// It skips a side whose conclude SHA is already recorded, so it is idempotent across a resumed run.
// Both sides are evaluated before the result is combined, so the single reason never reveals which side had landed.
func concludeLandedReason(f *Fabric, st *mergeState) ([]string, error) {
	warpLanded, err := sideConcludeMayHaveLanded(f.code, st.WarpCommitted, st.WarpOutcome, st.WarpStart)
	if err != nil {
		return nil, err
	}
	weftLanded, err := sideConcludeMayHaveLanded(f.records, st.WeftCommitted, st.WeftOutcome, st.WeftStart)
	if err != nil {
		return nil, err
	}

	if warpLanded || weftLanded {
		return []string{mergeReasonConcludeLanded}, nil
	}
	return nil, nil
}

// recordedMergeGoneReason reports mergeReasonRecordedMergeGone when either side has lost its recorded merge.
// Concluding a side runs git commit, which commits whatever MERGE_HEAD names.
// An operator may legitimately discard the staged merge and start a different one before the continue.
// Plain git in the warp checkout is allowed to do that.
// Without this check, that different merge would be committed and recorded as this merge's conclude SHA.
// It would be reported as a merge_committed mutation, paired in the correspondence index, and erased with the record.
// The result is a silent false success that leaves the merge source un-merged with nothing left to inspect.
// Both sides are evaluated first, so the single reason never reveals which side lost its merge.
func recordedMergeGoneReason(f *Fabric, st *mergeState) ([]string, error) {
	warpGone, err := sideRecordedMergeGone(f.code, f.warpPath, st.WarpCommitted, st.WarpOutcome, st.WarpSource, st.Squash)
	if err != nil {
		return nil, err
	}
	weftGone, err := sideRecordedMergeGone(f.records, f.weftPath, st.WeftCommitted, st.WeftOutcome, st.WeftSource, st.Squash)
	if err != nil {
		return nil, err
	}

	if warpGone || weftGone {
		return []string{mergeReasonRecordedMergeGone}, nil
	}
	return nil, nil
}

// sideRecordedMergeGone reports whether a side that is about to be concluded has lost the merge its record names.
// Only a staged or conflicted side with no conclude SHA recorded is checked.
// Only those get a commit from git commit.
// A side with a fast-forwarded or up-to-date outcome is never refused, since it gets no commit.
//
// A squash merge and a record with no source SHA are never refused.
// A squash writes no MERGE_HEAD, and an old record stores no source, so there is no evidence to compare.
// That residual case is left open rather than refused.
//
// The source must be the only entry in MERGE_HEAD, not merely one of its entries.
// Containment would accept an uncommitted octopus whose first head is the source.
// Committing it would land the other head's content under this merge's name.
//
// An ancestor source counts as landed however the checkout has moved since.
// That covers a crash that left HEAD on the conclude commit itself.
// It also covers a second unrelated merge started on top of a landed conclude.
// It also covers a hand-landed merge of the source onto a wrong base.
// Refusing them would leave the pair with no route out, since MergeAbort refuses them too.
//
// With no MERGE_HEAD and a clean checkout, git commit already fails, so this check does not refuse it.
// Refusing there would pre-empt the adoption path's own checks.
// A checkout with tracked changes is refused.
// There git commit succeeds and commits the leftover staged content as this merge's conclude.
func sideRecordedMergeGone(repo *gitrepo.Repo, checkoutPath, committed, outcome, sourceSHA string, squash bool) (bool, error) {
	if committed != "" {
		return false, nil
	}
	if outcome != mergeOutcomeStaged && outcome != mergeOutcomeConflicted {
		return false, nil
	}
	if squash || sourceSHA == "" {
		return false, nil
	}

	heads, err := repo.MergeHeads()
	if err != nil {
		return false, err
	}
	if len(heads) == 1 && heads[0] == sourceSHA {
		return false, nil
	}

	head, err := repo.CurrentSHA()
	if err != nil {
		return false, fmt.Errorf("fabricengine: resolve HEAD to classify recorded merge state: %w", err)
	}
	sourceAlreadyLanded, err := repo.IsAncestor(sourceSHA, head)
	if err != nil {
		return false, fmt.Errorf("fabricengine: classify recorded merge state: %w", err)
	}
	if sourceAlreadyLanded {
		return false, nil
	}

	if len(heads) > 0 {
		return true, nil
	}

	dirty, _, err := worktreeDirty(scopeTracked, checkoutPath)
	if err != nil {
		return false, fmt.Errorf("fabricengine: check checkout dirtiness to classify recorded merge state: %w", err)
	}
	return dirty, nil
}

// sideConcludeMayHaveLanded reports whether a side may already carry a conclude commit.
// It is true when the record holds the side's conclude SHA.
// It is also true when a staged or conflicted side's HEAD has moved off its recorded pre-merge SHA.
// The HEAD check matters because the conclude SHA is written last.
// It is written only after git commit, the HEAD read, and the record save have all succeeded.
// A failure in the HEAD read or the save leaves a landed commit that the record does not mention.
// Keying only on the recorded SHA would let an abort discard exactly those commits.
// Only a staged or conflicted side can receive a commit, so no other outcome is checked.
// An up_to_date side never moves, and an empty outcome never started.
// A fast_forwarded side moved legitimately, and an abort resets it.
// An unreadable HEAD returns an error, and an unexpected move refuses rather than letting a destructive reset proceed.
func sideConcludeMayHaveLanded(repo *gitrepo.Repo, committed, outcome, start string) (bool, error) {
	if committed != "" {
		return true, nil
	}
	if outcome != mergeOutcomeStaged && outcome != mergeOutcomeConflicted {
		return false, nil
	}

	head, err := repo.CurrentSHA()
	if err != nil {
		return false, fmt.Errorf("fabricengine: resolve HEAD to classify conclude state: %w", err)
	}
	return head != start, nil
}
