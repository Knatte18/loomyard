MILL_REVIEW_BEGIN
# Review: Fix Bouncer anchor-path and run-dir clearing — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: Claude Sonnet 4.5 (claude-sonnet-4-5), self-assessed
reviewed_file: plan/
date: 2026-08-26
```

## Findings

### [BLOCKING:scope] Batch 2's Call-entry clear breaks shedrecipe's CommitSeam tests, uncaught by any verify
**Location:** batch rundir-clear, card 10 (also implicates card 3/batch anchor-root's `entries_bouncer_test.go`)
**Issue:** `internal/shedrecipe/entries_bouncer_test.go`'s `TestBouncerEntry_CommitSeam` (`PlanResolvesToCommitPlan`, `DiscussionResolvesToCommitDiscussion`) use `layoutSettledBouncerRound1` + one `Call()` — the identical "replay vehicle" that `internal/shedadapters/bouncer_commit_test.go` uses and that card 10 explicitly repairs. After card 10 lands, `Call()` over an already-APPROVED round 1 archives and re-seeds instead of settling, so `Commit`/`CommitDiscussion` is never invoked; those two subtests then assert `planCalls`/`discussionCalls == 1` against an actual `0`. Batch 2's own `verify:` (`internal/shedadapters`, `internal/loomrecipe`) never runs `internal/shedrecipe`, so this is caught only by the final `pipeline.done_gate`, directly contradicting the batch's own claim that "`internal/shedadapters` has no production importers whose behaviour changes beyond what `internal/loomrecipe` already exercises."
**Fix:** Add a card repairing `TestBouncerEntry_CommitSeam`'s two affected subtests (harvest vehicle, mirroring card 10's `bouncer_commit_test.go` treatment) and its `layoutSettledBouncerRound1` comment, which also cites `shedadapters.TestBouncer_Replay_Approved` as "precedent" for behaviour card 10 removes.

### [BLOCKING:scope] Card 10's Context omits the file declaring `fixedClock`
**Location:** batch rundir-clear, card 10
**Issue:** Card 10's Requirements direct the new `bouncer_clear_test.go` to reuse "the existing ... `fixedClock` ... helpers," but `fixedClock` is declared in `internal/shedadapters/archive_test.go`, which is absent from card 10's `Context:` (and not an `Edits:`/`Creates:` target either). Every other named helper's file (`bouncer_seed_test.go`, `bouncer_judge_test.go`) is correctly listed; only this one is missing.
**Fix:** Add `internal/shedadapters/archive_test.go` to card 10's `Context:` list.

### [NIT:consistency] A stale defect-2 narrative survives in a file card 12 already reads
**Location:** batch rundir-clear, card 12 (`internal/loomrecipe/revalidate_test.go`, listed in card 12's Context)
**Issue:** That file's header comment (lines 22-27) narrates defect 2 as a currently-live, unfixed "pre-existing shedadapters defect ... filed on the follow-up roadmap item" — the very item card 14 moves to Done. Neither stale-assertion inventory's enumeration method reaches it (test file, wrong package), so no card updates it, leaving a self-contradictory claim once this task lands.
**Fix:** Reword the comment's last two sentences to state the defect is now fixed rather than filed/pending, in the same card that runs this file's tests (card 12).

## Verdict

REQUEST_CHANGES
Card 10's fix silently breaks shedrecipe's CommitSeam tests outside any batch's verify; add a repair card.
MILL_REVIEW_END
