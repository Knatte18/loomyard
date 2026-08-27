MILL_REVIEW_BEGIN
# Review: Add a local-only file category to weft

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-27
```

## Findings

### [BLOCKING:design] Push primitive for PushAnchored is unchosen
**Section:** `### commit-and-push-every-transition`
**Issue:** The decision says `PushAnchored` is "modelled on `PushWarpAt`'s `At`-shaped signature" but never picks the underlying primitive; `PushWarpAt` uses `gitrepo.PushCoalesced`, whose `pushWithRebaseRetry` path `spawn.go:110-123` documents as rewriting this side's SHAs on a rejected push and thereby "invalidat[ing] the correspondence index" — which contradicts `### correspondence-unchanged` and turns a push rejection into a silent history rewrite of the running weft rather than the warn-and-continue the discussion describes.
**Fix:** Decide explicitly between `PushCoalesced` and `PushRebaseFree` (`ErrPushRejected`, no rebase, no push-lock file) and state what a rejected push does to a live run.

### [BLOCKING:design] No probe exists for the mid-merge skip it specifies
**Section:** `### skip-while-mid-merge`
**Issue:** The rationale cites `mergeresolve.mergeInErrorResult`'s foreign-merge-state branch, but `Fabric.MergeInProgress` (`mergelifecycle.go:407-413`) "never consults `foreignMergeStatePresent`" and is false exactly in that case; it also requires an open `*Fabric`, and no `l`-in anchored merge-state probe exists, nor is one on the In-scope list beside `PushAnchored`.
**Fix:** Name the detection mechanism (which git state is consulted, foreign MERGE_HEAD included) and add the fabricengine-side vocabulary-neutral accessor to Scope-In.

### [BLOCKING:decision] mergeState's weft fields have no disposition
**Section:** `## Technical context` — "Where the weft merge happens"
**Issue:** "the plan decides whether those stay recorded as unmoved or are dropped" defers a named artifact to the plan, and the fields are load-bearing: `mergestate.go:45-51` is a persisted JSON schema, `mergelifecycle.go:237` refuses resume when `WeftOutcome == ""`, and `mergeguards.go:296,324` read `WeftCommitted`/`WeftOutcome`/`WeftSource`.
**Fix:** State the disposition here (keep recorded as unmoved vs drop) and say what a post-change binary does with a merge-state file written by a pre-change one.

### [BLOCKING:scope] Weft push residue and lock behaviour unstated
**Section:** `### commit-and-push-every-transition`
**Issue:** Per-transition pushes take `gitrepo.PushCoalesced`'s repo-root `PushLockFileName` write lock on every transition; the discussion never says whether that lock contends with `SpawnDetachedPush` children or landing-time pushes already running against the same weft repo, and a blocked or failed acquisition returns before `HasUnpushed` is consulted.
**Fix:** State the concurrency disposition of the transition push against fabric's existing weft push paths, or record it as an accepted no-contention claim with the reason.

### [NIT:design] Cross-machine resume names no pull step
**Section:** `## Problem`, `### commit-and-push-every-transition`
**Issue:** Nothing in `internal/loomcli` pulls; the second machine's resume depends on an unnamed operator-side `lyx fabric pull`/`PullWeft`, and the artifact files (`_lyx/discussion/`, `_lyx/plan/`) resume depends on are committed by separate closures (`wiring.go:179,205`) whose commits reach the remote only incidentally via the branch push.
**Fix:** State the pull side as an operator step and note that the transition push is branch-scoped, so it carries the artifact commits too.

### [NIT:consistency] Landing checkpoint cited at one call site of two
**Section:** `### landing-checkpoint-stays`
**Issue:** `Deps.CommitStatus` is called by both producers — `finalize.go:123` and `publish.go:114` — but the decision names only `Finalize.Call` step 1b.
**Fix:** Say "both landing producers", matching the Scope line that already claims both are untouched.

## Verdict

REQUEST_CHANGES
Four decisions are unmade or rest on a probe and a push primitive the source contradicts.
MILL_REVIEW_END
