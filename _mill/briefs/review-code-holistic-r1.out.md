MILL_REVIEW_BEGIN
# Review: webster: stop re-rendering already-inherited context into fork prompts — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-08-02
```

## Findings

### [NIT] Stale RenderForkPrompt comment in planparser's What-prose parsing
**Location:** `internal/planparser/parse.go:418`
**Issue:** The comment above the `**What:**` prose-collection loop still says "the implementer's concrete instruction — RenderForkPrompt injects it verbatim, so it must survive parsing" — but `RenderForkPrompt` no longer takes `What` or any card field at all; the card file (including `What`) is now read directly by the fork via `SourcePath`, per this task's own `fork-context-hygiene` decision. `plan.go`'s `Card.What` godoc was correctly updated to state this; this inline comment in `parse.go` was missed.
**Fix:** Reword to state the prose must survive parsing because the fork/recovery strand reads it directly from the card file via `SourcePath`, not because `RenderForkPrompt` injects it.

## Verdict

APPROVE
Both batches match the plan and Shared Decisions precisely; only one stale, non-functional doc comment found.
MILL_REVIEW_END
