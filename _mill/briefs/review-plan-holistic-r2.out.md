MILL_REVIEW_BEGIN
# Review: webster: DAG-derived card sequencing — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnetxhigh
reviewer_self_id: claude-sonnet-5
reviewed_file: plan/
date: 2026-08-24
```

## Findings

### [NIT:consistency] Stale declared-order claim in planparser/validate.go is left uncorrected
**Location:** Batch 3 (docs), cards 13–15
**Issue:** `internal/planparser/validate.go`'s file banner ("per the no-behavior-change-in-webster decision, cards still execute in strict declared plan order") becomes false once this task ships, but no card touches it and it's absent from `discussion.md`'s own "docs that make a now-false claim" list — the file is even in card 11's Context.
**Fix:** Add a one-line comment fix to validate.go's banner in batch 3 (a comment-only edit, so it doesn't violate the no-planparser-change Shared Decision), or explicitly record the omission as accepted.

### [NIT:consistency] accumulatedCardSHAs' doc comment goes stale after card 4's reorder
**Location:** Batch 2, Card 4 (`internal/websterengine/runlevel.go`)
**Issue:** `accumulatedCardSHAs`' doc comment says it "walks batches in plan order"; after card 4 re-binds `batches` to `SequenceBatches`' output, it actually walks execution order (correct, arguably more correct for bisect) — the wording goes stale in a file this same card already edits.
**Fix:** Add one sentence to card 4's Requirements updating that comment's "plan order" phrase to "execution order."

### [NIT:consistency] Redundant ordering clauses survive card 8's template reword
**Location:** Batch 2, Card 8 (`contracts/stencils/webster/webster-template-master.md`)
**Issue:** Card 8 replaces only the "there is no DAG here to reorder around" clause, leaving the adjacent, equally number-vs-position-ambiguous clause "batch N assumes every batch before it is already committed" untouched right beside the new "every entry ABOVE it in the list" clause — mildly redundant, though not incorrect once the disambiguating "not ascending batch number" sentence lands.
**Fix:** Have card 8 also fold or rephrase that first clause so the paragraph states the ordering rule once, cleanly.

## Verdict

APPROVE
Plan is internally consistent, fully cross-checked against source; only minor doc-wording NITs found.
MILL_REVIEW_END
