MILL_REVIEW_BEGIN
# Review: Shed-setup validity checker

```yaml
verdict: APPROVE
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-21
```

## Findings

### [NIT:consistency] "Three rows bounce backwards" names five rows
**Section:** Technical context (`internal/loomshed/loomshed.go:137-151` bullet) **Issue:** The sentence says three rows bounce backwards, then lists five (`DiscussionValidate`, `DiscussionReview`, `PlanValidate`, `PlanReview`, `WebsterReview`); `loomshed.go` confirms five rows with a non-empty `OnStuck` and three distinct bounce targets. **Fix:** Say "five rows bounce backwards to three targets", so the plan writer's mental model of the graph the invariant test walks is right.

### [NIT:consistency] Perch items still specify `Segment`, which recipe rows drop
**Section:** Problem ("why now") / `Segment` is untouched **Issue:** The premise that the one cross-wiring guard is departing is correct (`manifest/roadmap.md`'s recipe row shape `{Name, Engine, Config, OnDone, OnStuck, MaxBounces}` carries no `Segment`), but the three perch items still instruct each row to set `Segment: "Discussion-Review"` etc., and they are sequenced *after* the conversion — so those instructions are already stale. **Fix:** Note in passing that the perch items' `Segment` instruction is expected to become moot, so a plan writer does not read it as a live requirement `shedcheck` must accommodate.

## Verdict

APPROVE
Endpoints, kinds, field mapping, ordering and malformed-input behaviour are all decided and verifiable.
MILL_REVIEW_END
