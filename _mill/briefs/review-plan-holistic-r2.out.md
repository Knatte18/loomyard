MILL_REVIEW_BEGIN
# Review: Shed recipe: engine registry — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: claude-sonnet-5 (claude-sonnet-4-5 class, per Anthropic's own naming)
reviewed_file: plan/
date: 2026-08-21
```

## Findings

### [NIT:scope] Card 13's shape-discovery pointer omits the file that actually defines it
**Location:** batch 04-singlellm-entry.md, Card 13.
**Issue:** Requirements tells the implementer to "read `internal/stencilstore/reconcile.go` to determine that shape rather than guessing" for the on-disk layout `stencilstore.Read` resolves a name to, but `reconcile.go` only calls `Path`/`RelPath` — both are declared in `internal/stencilstore/stencilstore.go` (the `<family>/<name>.md` convention), which is not in Card 13's `Context:`.
**Fix:** Add `internal/stencilstore/stencilstore.go` to Card 13's `Context:` (and, for the same reason, Card 17's), or reword the pointer to name the file that actually defines `Path`/`RelPath`.

## Verdict

REQUEST_CHANGES
One minor Context-completeness gap (Card 13); every other batch, decision, and cross-file claim verified accurate against source.
MILL_REVIEW_END
