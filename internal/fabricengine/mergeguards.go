// mergeguards.go implements the shared merge precondition machinery: guard evaluation, per-side
// merge-source resolution with the freshness rule, the attached-HEAD precondition, and the
// upstream-sync helper batch 4's Merge guard set builds on.
// Every helper here returns reasons for aggregation via newMergeGuardError and mutates nothing — the
// upstream sync itself (a mutation) is deliberately not here; it is a batch-4 pre-merge step, per
// the guards decision.

package fabricengine

import (
	"errors"
	"fmt"
	"strings"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

// mergeSources holds, per side, the resolved SHA a merge call actually merges — the freshness rule's
// output, already picked between a local branch and its remote-tracking counterpart.
type mergeSources struct {
	warpSHA string
	weftSHA string
}

// resolveMergeSources resolves the SHA each side of f actually merges, applying the freshness rule
// independently per side: warp resolves source itself, weft resolves WeftBranchName(source) — the
// sole "-weft" composition (branchname.go).
// Both sides run a best-effort Fetch() first — a fetch failure is tolerated and logged via
// logger.Warn, never fatal (millhouse's fetch-then-prefer-origin rule) — then resolve the local
// branch and its origin/<branch> remote-tracking ref via gitrepo.ResolveSHA.
// A warp source resolvable on neither local nor remote appends mergeReasonSourceNotFound; a weft
// counterpart existing neither locally (weftBranchExists(l, ...)) nor as origin/<source>-weft
// post-fetch appends mergeReasonNotFabricManaged, per the Shared Decision on the post-fetch
// remote-only weft counterpart.
// Both reasons are collected, never returned early — every guard is evaluated regardless of an
// earlier failure.
func resolveMergeSources(f *Fabric, l *lyxcwd.Location, source string) (mergeSources, []string) {
	var reasons []string
	weftBranch := WeftBranchName(source)

	if err := f.warp.Fetch(); err != nil {
		logger.Warn("fabricengine: best-effort fetch before merge source resolution failed", "side", "warp", "error", err)
	}
	warpLocalSHA, warpLocalErr := f.warp.ResolveSHA(source)
	warpRemoteSHA, warpRemoteErr := f.warp.ResolveSHA("origin/" + source)
	warpSHA, warpFound := pickMergeSourceSHA(f.warp, warpLocalSHA, warpLocalErr == nil, warpRemoteSHA, warpRemoteErr == nil)
	if !warpFound {
		reasons = append(reasons, mergeReasonSourceNotFound)
	}

	if err := f.weft.Fetch(); err != nil {
		logger.Warn("fabricengine: best-effort fetch before merge source resolution failed", "side", "weft", "error", err)
	}
	weftLocalSHA, weftLocalErr := f.weft.ResolveSHA(weftBranch)
	weftRemoteSHA, weftRemoteErr := f.weft.ResolveSHA("origin/" + weftBranch)
	weftManaged := weftBranchExists(l, weftBranch) || weftRemoteErr == nil
	if !weftManaged {
		reasons = append(reasons, mergeReasonNotFabricManaged)
	}
	weftSHA, _ := pickMergeSourceSHA(f.weft, weftLocalSHA, weftLocalErr == nil, weftRemoteSHA, weftRemoteErr == nil)

	return mergeSources{warpSHA: warpSHA, weftSHA: weftSHA}, reasons
}

// pickMergeSourceSHA implements the freshness rule for one side: merge the remote-tracking SHA when
// the local branch is absent, or when it is an ancestor of the remote-tracking SHA and not equal to
// it (IsAncestor); the local SHA otherwise.
// It reports found=false only when neither the local branch nor its remote-tracking ref resolved.
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
	if err == nil && isAncestor {
		return remoteSHA, true
	}
	return localSHA, true
}

// pairDirtyReason reports mergeReasonWorktreeDirty when either checkout of f carries uncommitted
// tracked changes.
// Both sides are evaluated unconditionally before combining, so the aggregated reason never reveals
// which side was dirty, nor that two subjects were checked.
func pairDirtyReason(f *Fabric) ([]string, error) {
	warpDirty, _, err := worktreeDirty(scopeTracked, f.warpPath)
	if err != nil {
		return nil, fmt.Errorf("fabricengine: check warp dirtiness: %w", err)
	}
	weftDirty, _, err := worktreeDirty(scopeTracked, f.weftPath)
	if err != nil {
		return nil, fmt.Errorf("fabricengine: check weft dirtiness: %w", err)
	}

	if warpDirty || weftDirty {
		return []string{mergeReasonWorktreeDirty}, nil
	}
	return nil, nil
}

// upstreamSHAAt resolves dir's checked-out branch's upstream tracking ref (`@{u}`) to a plain SHA
// via `git rev-parse @{u}`, classifying a *gitexec.GitError (no upstream configured) as
// hasUpstream=false rather than an error — following weftHasUpstream's classification in pull.go.
// It is consumed by batch 4's sync guard; declared here so batch 4 does not reshape this file.
func upstreamSHAAt(dir string) (sha string, hasUpstream bool, err error) {
	stdout, runErr := gitexec.Run([]string{"rev-parse", "@{u}"}, dir)
	if runErr == nil {
		return strings.TrimSpace(stdout), true, nil
	}
	var gitErr *gitexec.GitError
	if errors.As(runErr, &gitErr) {
		return "", false, nil
	}
	return "", false, fmt.Errorf("fabricengine: resolve upstream in %s: %w", dir, runErr)
}

// detachedHeadReason reports mergeReasonDetachedHead when either checkout of f has HEAD pointing
// straight at a commit instead of at a branch.
// A merge concluded on a detached HEAD lands a commit no ref reaches, so the next checkout discards
// it silently — while the paired repo's half of the same merge, whose own HEAD was on a branch, is
// already landed for good and no longer abortable, since the merge verb deleted its record on the
// way out. Refusing before the attempt starts is the only point at which that is recoverable.
// Both sides are evaluated unconditionally before combining, so the aggregated reason never reveals
// which side (if either) was detached.
func detachedHeadReason(f *Fabric) ([]string, error) {
	warpDetached, err := f.warp.HeadDetached()
	if err != nil {
		return nil, fmt.Errorf("fabricengine: check warp head attachment: %w", err)
	}
	weftDetached, err := f.weft.HeadDetached()
	if err != nil {
		return nil, fmt.Errorf("fabricengine: check weft head attachment: %w", err)
	}

	if warpDetached || weftDetached {
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

// syncedToUpstreamReason reports mergeReasonNotSynced when either side of f is genuinely diverged
// from its own upstream: a side with no upstream passes vacuously (Fabric.Pull's no-upstream rule),
// and a side with an upstream passes when its tip is not diverged from it — upstream ancestor of HEAD
// (in sync or ahead) passes, HEAD ancestor of upstream (behind, since Merge's own pre-merge sync step
// will advance it) passes, and only neither direction (a genuine divergence) fails.
// Both sides are evaluated unconditionally before combining, so the aggregated reason never reveals
// which side (if either) was out of sync.
func syncedToUpstreamReason(f *Fabric) ([]string, error) {
	warpNotSynced, err := sideNotSyncedToUpstream(f.warp, f.warpPath)
	if err != nil {
		return nil, err
	}
	weftNotSynced, err := sideNotSyncedToUpstream(f.weft, f.weftPath)
	if err != nil {
		return nil, err
	}
	if warpNotSynced || weftNotSynced {
		return []string{mergeReasonNotSynced}, nil
	}
	return nil, nil
}

// sideNotSyncedToUpstream implements syncedToUpstreamReason's per-side predicate: false (synced) when
// dir has no upstream, when its HEAD already equals its upstream, when its upstream is an ancestor of
// its HEAD (in sync or ahead), or when its HEAD is an ancestor of its upstream (behind); true (a
// genuine divergence) only when neither direction holds.
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
