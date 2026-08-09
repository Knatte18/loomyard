MILL_REVIEW_BEGIN
# Review: Scope the Shed producer-model rewrite into buildable tasks — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-08-09
```

## Findings

### [NIT] F's doc-amendment list nested under Migration instead of its own step
**Location:** `_mill/followup/F-batcher-standalone-split.md:62-66`
**Issue:** Card 3 (`01-code-task-bodies.md:111`) says "Add the doc-amendment list as its own numbered step," but the body nests `**Doc amendments**` as a sub-bullet-list inside numbered item 4 (Migration) rather than as item 5.
**Fix:** Promote `**Doc amendments**` to a standalone numbered step (5), sibling to Module shape / Config wiring / The inventory / Migration.

### [NIT] Sentence-per-line rule not fully applied in places
**Location:** `_mill/followup/A-builder-retire.md:13`
**Issue:** "Builder is not dormant: `cmd/lyx/main.go:107` registers `buildercli.Command()`, and it appears in `cmd/lyx/helptree_test.go`'s module list and `cmd/lyx/notransients_test.go`." packs two independent clauses (comma + "and", second clause has its own subject/verb) on one line, against the Shared Decision's semantic-line-break rule.
**Fix:** Break after the comma before "and it appears" (a few similar instances exist across the six bodies; worth a pass, not blocking).

## Verdict

APPROVE
Bodies transcribe the discussion faithfully with high fidelity; both prior non-blocking items now read as resolved; only minor NITs remain.
MILL_REVIEW_END
