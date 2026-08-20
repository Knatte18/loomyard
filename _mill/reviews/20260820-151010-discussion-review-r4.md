MILL_REVIEW_BEGIN
# Review: shedadapters: Burler-round producer

```yaml
duration_s: 171.0
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude Opus-class model (self-reported Opus 5); exact build not independently verifiable
reviewed_file: _mill/discussion.md
date: 2026-08-20
```

## Findings

### [NIT:consistency] Success `Stuck` under cancellation vs. seam contract
**Demoted-from:** BLOCKING
**Section:** ### Cancellation / ### Always `Stuck`, never `Done`
**Issue:** `internal/shedengine/producer.go:28-29` states as a binding implementation obligation "surface context cancellation as a non-nil error, **never as Stuck**"; the discussion only reconciles the *shedadapters package doc*'s wording, not this one, and Shed's own loop (`run.go:180-215`) will route that `Stuck` to `OnStuck`, append a history entry and consume one bounce of the segment's round cap before step 3 pauses at the `Bouncer` on the next iteration.
**Fix:** State a disposition for the seam obligation itself — either accept the deviation and record its two observable consequences (a bounce consumed, the run pausing at the `Bouncer` rather than at the round producer), or amend `shedengine`'s contract wording, which the Scope section currently rules out.

### [BLOCKING:design] Orphan review from a killed process wedges hydration
**Section:** ### Round resolution … / ### Prior-round hydration
**Issue:** The "archive on every non-success exit" rule only runs on returns, so a producer process killed mid-round (the phase-A-written/phase-B-pending window the discussion itself names as most likely) leaves `round-<N>-review.md` with no fixer report; the next `Call` then advances to `N+1` and hydrates `round-<N>-fixer-report.md`, which `Profile.validate`'s `requireExistingPaths` (`profile.go:83-88`) rejects fail-loud — every subsequent `Call` repeats, permanently wedging the segment, and the `Bouncer` meanwhile reads round `N` as complete.
**Fix:** Decide the disposition for a review-without-fixer-report pair — e.g. make the round/hydration predicate the *pair*'s existence rather than the review alone, or state skip-and-warn behaviour for a prior round missing its fixer report — and say so where the discriminator is defined.

### [NIT:scope] Fifth stale "three adapters" doc site not enumerated
**Demoted-from:** BLOCKING
**Section:** ## Scope → In (doc updates)
**Issue:** The doc-site list names `shed.md`'s Engine-adapters section, `shed.md:324`, and `docs/overview.md:235,316,318`, but `manifest/designs/shed.md:3`'s status blockquote also says "the three engine adapters (`SingleLLMProducer`, the `perch` adapter, the `Webster` adapter) ship as `internal/shedadapters`" — present tense, stale for exactly the reason given for `:324`.
**Fix:** Add `manifest/designs/shed.md:3` to the same-commit doc list (`manifest/roadmap.md:294` is a past-tense completion entry and correctly stays as-is).

## Verdict

REQUEST_CHANGES
Seam-contract conflict, a crash-window wedge, and one missed stale doc site.
_Note: 2 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 1._
MILL_REVIEW_END
