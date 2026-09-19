MILL_REVIEW_BEGIN
# Review: Worktree spawn/teardown as Shed producers

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude Opus 4.x-class model (Anthropic); exact build not self-verifiable
reviewed_file: _mill/discussion.md
date: 2026-09-18
```

## Findings

### [BLOCKING:design] Prime-lock reason cannot name the holding slug
**Section:** `cross-slug-concurrency-takes-a-prime-wide-lock`; Testing (`lifecyclecli`/producer layer)
**Issue:** The decision requires the `Stuck` reason to name "the holding slug" and Testing asserts it, but `internal/lock/lock.go:31` `TryAcquireWriteLock` reports only `(nil, false, nil)` on contention, the file carries no holder record, and the stated seam `Acquire func() (release func() error, ok bool, err error)` has no identity return — the requirement is unsatisfiable as written.
**Fix:** Either drop the holder-naming requirement (reason names the lock path only, as the per-slug refusal does) or decide a holder-identity mechanism (e.g. a sidecar holder file written under the lock) and put it in the seam's signature.

### [BLOCKING:design] `abandonedSession` has no carrier off a `Done` row
**Section:** Technical context "Reed APIs"; Testing (`lifecyclecli`, final bullet)
**Issue:** `ShedProducer.Call` returns only `(Outcome, OutputPointer{Path string}, error)` and `shedengine.Result` carries only `Outcome/HaltedProducer/Reason/History` (`shed.go:60`), so a producer has no channel to put a string on `lyx lifecycle run`'s envelope; the discussion explicitly rules out the stuck reason ("it returns `Done`") and explicitly rules out the CLI closure owning it.
**Fix:** Name the actual carrier — an `OutputPointer.Path` artifact, a producer-held field the CLI reads after `Run`, or `Shutdown`'s closure recording it CLI-side — and reconcile the "the producer, not the CLI closure, owns putting it on the result" sentence with it.

### [BLOCKING:consistency] `Env.PrimeLock` missing from both fixture enumerations
**Section:** Scope bullet 6; Technical context "The second registry-wide test"
**Issue:** Both lists enumerate exactly four new `newTestEnv` fields (`Slug`, `CreateWorktree`, `LoomRun`, `Teardown`) and state the consequence as complete, but round 6 added a fifth read seam, `Env.PrimeLock`, validated by the two bookend entries — `TestBuild_EveryRegisteredEngineBuilds` (driven off `shedrecipe.Names()`) fails until the fixture fills it too.
**Fix:** Add `Env.PrimeLock` to both enumerations, with a fake `Acquire` returning a no-op release.

### [BLOCKING:decision] Fabric mutation records have no stated disposition
**Section:** Constraints (Mutation Record Invariant); `three-registry-entries-closures-not-engine-imports`
**Issue:** The Constraints bullet states a conditional ("anything surfacing them through the CLI envelope exposes `mutations`/`partial`") that no section resolves, while the chosen seams (`CreateWorktree func(ctx) error`, `Remove func(ctx) error`) silently discard `AddResult`/`RemoveResult` and their embedded `MutationRecord` (`remove.go:24-26`) — so a partially-rolled-back `Add` leaves the operator only fabric's error text.
**Fix:** Decide explicitly whether the lifecycle envelope carries `mutations`/`partial` from the bookends or deliberately drops them, and record the reason in a decision.

## Verdict

REQUEST_CHANGES
Two unbuildable seams, one stale fixture list, one undecided mutation-record disposition.
MILL_REVIEW_END
