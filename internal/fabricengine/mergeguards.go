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
// The two reasons are gated asymmetrically, and which of them can accompany the other is worth stating
// exactly, because the obvious summary — "they stay disjoint" — is false against the shipped behaviour
// and against the two places that pin it.
// A source that DOES resolve on warp but has no weft counterpart reports mergeReasonNotFabricManaged
// alone: the weft arm's own source-not-found is gated on weftManaged, so "this branch is not a fabric
// pair" arrives without a second, vaguer reason beside it that tells the operator nothing more.
// A source that resolves NOWHERE reports both, and that is deliberate rather than leakage — the warp
// arm's mergeReasonSourceNotFound is ungated, because a mistyped branch name is unmanaged AND missing,
// and an operator told only "not fabric-managed" would go looking for a weft counterpart to create
// instead of at the name they got wrong. TestRunCLI_MergeNonexistentBranchReportsAggregatedGuardError
// and SANDBOX-FABRIC-SUITE's F19 both pin that dual answer, so gating the warp arm to make the pair
// literally disjoint would break the behaviour, not tidy it.
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

// pairDirtyReason reports mergeReasonWorktreeDirty when f's warp checkout carries uncommitted
// tracked changes.
// The weft side is not evaluated: it is not a merge participant, so its dirtiness cannot affect a
// warp-only merge's correctness, and checking it could only refuse a merge that would have been
// right.
func pairDirtyReason(f *Fabric) ([]string, error) {
	warpDirty, _, err := worktreeDirty(scopeTracked, f.warpPath)
	if err != nil {
		return nil, fmt.Errorf("fabricengine: check checkout dirtiness: %w", err)
	}

	if warpDirty {
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

// detachedHeadReason reports mergeReasonDetachedHead when f's warp checkout has HEAD pointing
// straight at a commit instead of at a branch.
// A merge concluded on a detached HEAD lands a commit no ref reaches, so the next checkout discards
// it silently — the only point at which that is recoverable is before the attempt starts.
// The weft side is not evaluated: it is not a merge participant, so its head attachment cannot
// affect a warp-only merge's correctness, and checking it could only refuse a merge that would have
// been right.
func detachedHeadReason(f *Fabric) ([]string, error) {
	warpDetached, err := f.warp.HeadDetached()
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

// recordedMergeGoneReason reports mergeReasonRecordedMergeGone when either side that MergeContinue is
// about to conclude no longer carries the git-level merge st recorded — MergeContinue's own
// precondition, and the commit-side twin of sideConcludeAlreadyLanded's evidence rule.
// concludeMergeSides finishes a side by running `git commit`, which commits whatever MERGE_HEAD names.
// Between the attempt and the resumed MergeContinue an operator may — legitimately, per the Fabric Git
// Invariant's own carve-out for plain git in the warp checkout — have discarded fabric's staged merge
// and started a different one. Without this precondition that different merge is committed, written
// into the record as this merge's conclude SHA, reported as a merge_committed mutation, paired into the
// correspondence index and then erased along with the record: a silent false success leaving the merge
// source un-merged with nothing left to inspect.
// Both sides are evaluated unconditionally before combining, so the single aggregated reason never
// reveals which side (if either) had lost its merge.
func recordedMergeGoneReason(f *Fabric, st *mergeState) ([]string, error) {
	warpGone, err := sideRecordedMergeGone(f.warp, f.warpPath, st.WarpCommitted, st.WarpOutcome, st.WarpSource, st.Squash)
	if err != nil {
		return nil, err
	}
	weftGone, err := sideRecordedMergeGone(f.weft, f.weftPath, st.WeftCommitted, st.WeftOutcome, st.WeftSource, st.Squash)
	if err != nil {
		return nil, err
	}

	if warpGone || weftGone {
		return []string{mergeReasonRecordedMergeGone}, nil
	}
	return nil, nil
}

// sideRecordedMergeGone implements recordedMergeGoneReason's per-side predicate: true when this side is
// genuinely about to be concluded by `git commit` and the recorded source is neither the merge that
// commit would conclude nor already in this side's history.
//
// It is scoped to exactly the states concludeMergeSides runs `git commit` in, so it can never refuse a
// side that verb would skip: a side whose conclude SHA is already recorded is skipped, and so is one
// whose outcome is fast_forwarded or up_to_date, since neither fabricates a commit.
//
// The first exit is the ordinary one — MERGE_HEAD names the recorded per-side source and nothing else,
// so `git commit` concludes precisely the merge the record describes.
// Demanding the whole head SET be that one SHA, rather than testing membership in it, is what rejects
// the uncommitted octopus: `git merge --no-commit <recorded source> <decoy>` leaves a MERGE_HEAD whose
// FIRST entry is the recorded source, and every rev-parsing spelling of the query reports only that
// first entry (see gitrepo.MergeHeads). Committing it lands the decoy's content under this merge's
// name, brought in by no side of the merge and named by no merge_staged entry — the same commit the
// adoption arm refuses to adopt, reached by simply not committing it first.
//
// The second exit is what keeps this precondition from wedging states that are already fine, and it is
// deliberately reachability rather than an exact-shape test. Once the recorded source is an ancestor of
// this side's HEAD the merge has genuinely landed, whatever else the operator has since done in the
// checkout, so there is no false claim left for this precondition to prevent: it covers the crash the
// adoption arm exists to finish (HEAD IS the conclude, whose second parent is the source), a conclude
// landed with a second unrelated merge started on top of it, and a hand-landed merge of the source onto
// a wrong base. Refusing those would leave the pair with no route out at all, since MergeAbort refuses
// every one of them too on concludeLandedReason.
//
// The third exit is the one that keeps this precondition from BLINDING the adoption arm, and it is a
// coverage property rather than a behavioural nicety. With no merge live and a clean checkout, `git
// commit` fails on its own and *ErrMergeIncomplete already comes back — so refusing here would only
// swap one refusal for another, while making every adoption-evidence test (first parent, second parent,
// exact arity, unrelated commit) refuse before it ever reached the clause it exists to pin. Those tests
// abort both sides by design, so this precondition would have decided every one of them.
// What is NOT exempt is the same no-live-merge state with tracked dirt in the checkout, because there
// `git commit` succeeds: it lands an ordinary commit of whatever the operator left staged, which fabric
// would then write into the record as this merge's conclude SHA.
//
// What survives all three exits is the shape with a real destructive consequence: the recorded source
// is nowhere in this side's history, and concluding would commit some other merge — or some other staged
// content — and record, report and pair it as this merge's own conclude before deleting the only
// evidence that it was not.
//
// A squash record is exempt and an empty recorded source SHA is exempt, for the identical reason the
// adoption arm gives: `git merge --squash` writes no MERGE_HEAD at all, so a squash conclude carries no
// evidence to compare against, and a record written by a binary predating the recorded source SHAs
// carries no source. No evidence is not failed evidence here any more than it is satisfied evidence
// there — refusing every squash --continue would break the ordinary squash flow, so that residual stays
// open and is stated rather than papered over.
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
