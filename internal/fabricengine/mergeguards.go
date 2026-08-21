// mergeguards.go implements the shared merge precondition machinery: guard evaluation, per-side
// merge-source resolution with the freshness rule, the attached-HEAD precondition, the
// upstream-sync helper batch 4's Merge guard set builds on, and MergeAbort's own
// conclude-already-landed precondition.
// Every helper here returns reasons for aggregation via newMergeGuardError and mutates no worktree,
// index, branch tip or fabric record — the upstream sync itself, which does, is deliberately not
// here; it is a batch-4 pre-merge step, per the guards decision.
// The one thing this file does mutate is remote-tracking state: resolveMergeSources runs a
// best-effort Fetch() on both sides before resolving, so "mutates nothing" would be false as
// written. Nothing a caller can observe as a change to the pair happens here.

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
// Those two reasons stay disjoint on purpose: an unmanaged source reports ONLY
// mergeReasonNotFabricManaged, never that reason plus source-not-found, since "this branch is not a
// fabric pair" is the precise thing the operator has to act on and adding a second, vaguer reason
// beside it tells them nothing more.
//
// A managed weft counterpart that nevertheless fails to RESOLVE is a third case, and it appends
// mergeReasonSourceNotFound because the alternative is worse. weftManaged and weft resolvability are
// not the same test: weftBranchExists is a raw `git rev-parse --verify refs/heads/<branch>` at the
// weft REPO root, while pickMergeSourceSHA's local arm is a go-git ResolveRevision in the weft
// WORKTREE. Whenever the first succeeds and the second does not, weftManaged is true, and with the
// pick's found-ness discarded no reason was appended at all — leaving weftSHA the empty string, handed
// straight to MergeStart as `git merge --ff --no-commit ""`. The blast radius was contained (the git
// error routes into selfAbortMergeAttempt, which resets both sides and deletes the record), but the
// error a caller saw described a malformed git argument rather than the precondition that failed.
// Gating the reason on weftManaged makes an empty ref unreachable by construction without disturbing
// the unmanaged case's single-reason contract.
//
// Every reason is collected, never returned early — each guard is evaluated regardless of an earlier
// failure — and newMergeGuardError deduplicates, so a source missing on both sides still reports one
// mergeReasonSourceNotFound rather than disclosing that two subjects were checked.
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
	weftSHA, weftFound := pickMergeSourceSHA(f.weft, weftLocalSHA, weftLocalErr == nil, weftRemoteSHA, weftRemoteErr == nil)
	if weftManaged && !weftFound {
		reasons = append(reasons, mergeReasonSourceNotFound)
	}

	return mergeSources{warpSHA: warpSHA, weftSHA: weftSHA}, reasons
}

// pickMergeSourceSHA implements the freshness rule for one side: merge the remote-tracking SHA when
// the local branch is absent, or when it is an ancestor of the remote-tracking SHA and not equal to
// it (IsAncestor); the local SHA otherwise.
// It reports found=false only when neither the local branch nor its remote-tracking ref resolved.
// The ancestry probe is best-effort, matching this file's best-effort Fetch rule: an IsAncestor
// failure falls back to the local SHA — the pre-freshness-rule answer — and is logged via
// logger.Warn rather than swallowed silently, so a degraded pick (merging a possibly-stale local
// tip) always leaves a trace.
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

// pairDirtyReason reports mergeReasonWorktreeDirty when either checkout of f carries uncommitted
// tracked changes.
// Both sides are evaluated unconditionally before combining, so the aggregated reason never reveals
// which side was dirty, nor that two subjects were checked.
func pairDirtyReason(f *Fabric) ([]string, error) {
	warpDirty, _, err := worktreeDirty(scopeTracked, f.warpPath)
	if err != nil {
		return nil, fmt.Errorf("fabricengine: check checkout dirtiness: %w", err)
	}
	weftDirty, _, err := worktreeDirty(scopeTracked, f.weftPath)
	if err != nil {
		return nil, fmt.Errorf("fabricengine: check checkout dirtiness: %w", err)
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
		return nil, fmt.Errorf("fabricengine: check checkout head attachment: %w", err)
	}
	weftDetached, err := f.weft.HeadDetached()
	if err != nil {
		return nil, fmt.Errorf("fabricengine: check checkout head attachment: %w", err)
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
//
// This is a pre-fetch FAST PATH, not the whole of the not-synced precondition, and reading it as the
// whole of it is a mistake with a live failure behind it. Every helper in this file resolves @{u}
// from whatever remote-tracking state the checkout already carries — nothing in Merge's guard stage
// fetches before this runs — so a divergence created by someone else's push that this checkout has
// not fetched yet is invisible here: @{u} still points at a commit that IS an ancestor of HEAD, the
// side classifies as "ahead", and the guard passes. Merge merged straight over a genuinely diverged
// target that way.
// syncSideBeforeMerge re-decides the same predicate after its own fetch and refuses with this same
// reason, so the promise holds even when this fast path could not see the divergence. Anything added
// here that must hold post-fetch belongs there too.
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

// concludeLandedReason reports mergeReasonConcludeLanded when st's attempt may already have put a
// conclude-commit on either side — the one precondition MergeAbort has, and the mirror of
// MergeContinue's mergeAttemptIncompleteReason.
// MergeAbort restores both sides from the recorded pre-merge SHAs, so running it against a
// half-concluded attempt discards a commit that really landed. In the MergeIn-with-conflicts flow
// that commit carries the operator's own hand-written conflict resolutions, and resetMergeSides runs
// with force: true, so nothing else stands in the way. Refusing leaves MergeContinue — which skips a
// side whose committed SHA is already recorded, and is therefore idempotent across a resumed run —
// as the one correct recovery, exactly as the incomplete-attempt refusal leaves MergeAbort as the
// one correct recovery for the opposite shape.
// Both sides are evaluated unconditionally before combining, so the single aggregated reason never
// reveals which side (if either) had landed.
func concludeLandedReason(f *Fabric, st *mergeState) ([]string, error) {
	warpLanded, err := sideConcludeMayHaveLanded(f.warp, st.WarpCommitted, st.WarpOutcome, st.WarpStart)
	if err != nil {
		return nil, err
	}
	weftLanded, err := sideConcludeMayHaveLanded(f.weft, st.WeftCommitted, st.WeftOutcome, st.WeftStart)
	if err != nil {
		return nil, err
	}

	if warpLanded || weftLanded {
		return []string{mergeReasonConcludeLanded}, nil
	}
	return nil, nil
}

// sideConcludeMayHaveLanded implements concludeLandedReason's per-side predicate: true when the
// record already carries this side's conclude SHA, or when the side's recorded outcome is
// staged/conflicted and its HEAD has moved off its recorded pre-merge SHA.
// The second clause is not redundant with the first. concludeMergeSides writes a side's conclude SHA
// only after `git commit` returned and CurrentSHA resolved and the record was re-saved, so an I/O
// failure at either of those two steps leaves a landed commit the record does not mention — and
// keying only on the recorded SHA would let MergeAbort discard exactly the commits that are hardest
// to notice missing.
// Reading it off HEAD instead is exact: an up_to_date side is never concluded and cannot move, a
// fast_forwarded side moved legitimately and MergeAbort is documented to reset it, and a side whose
// outcome is empty never started. Only a staged or conflicted side can have a commit put on it, and
// the conclude is the only thing that puts one there.
// The failure direction is safe: an unreadable HEAD errors out and an unexpected move over-refuses,
// rather than proceeding into a destructive reset.
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
