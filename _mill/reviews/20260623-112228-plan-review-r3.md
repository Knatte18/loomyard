I have enough to produce the review.

MILL_REVIEW_BEGIN
# Review: weft producers: _lyx/config, lyx config, codeguide — holistic

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: plan/
date: 2026-06-23
```

## Findings

### [NIT] Card 14 unit tests need a Layout but paths.go not in Context
**Location:** Batch 4 / Card 14 (a, d)
**Issue:** `dispatch(l *paths.Layout, ...)` derives `baseDir` from `l.WorktreeRoot`+`l.RelPath`, so unit-testing `dispatch` (and the menu) requires constructing a `paths.Layout{}`, but `internal/paths/paths.go` is absent from Card 14's `Context:` (the Layout fields are only visible transitively via configcli.go).
**Fix:** Either add `internal/paths/paths.go` to Card 14 `Context:`, or have a/d call `editOne` (which takes `baseDir` directly) and route the menu test through a trivially-constructed Layout documented in the card.

### [NIT] Card 10 lists output.go in Context but registry never uses it
**Location:** Batch 4 / Card 10
**Issue:** The registry/`templateFor`/`moduleNames` body only touches the module template funcs; `internal/output/output.go` in `Context:` is unused at this card (output is first consumed in Card 11's messages, which prints plain text via the writer, not `output.Ok/Err`).
**Fix:** Drop `internal/output/output.go` from Card 10 Context (and note Card 11 prints human-readable text, not JSON envelopes, so output.go is informational only).

## Verdict

APPROVE
Plan is constraint-clean, DAG-sound, decision-faithful; two minor context nits only.
MILL_REVIEW_END