MILL_REVIEW_BEGIN
# Review: Reed and Fabric as standalone modules: public API design

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Opus-class), Anthropic
reviewed_file: _mill/discussion.md
date: 2026-09-06
```

## Findings

### [BLOCKING:consistency] Q&A log repeats the banned render-imports claim
**Section:** Q&A log, "What layout would a standalone Reed repo take?" (`:471`)
**Issue:** It states `reedengine/render` is "773 lines, zero internal deps, `fmt` and `strings` only" — the exact wording the `reed-standalone-layout-mirrors-quarry` decision (`:177`) says "is wrong and must not be written"; `internal/reedengine/render/policy.go:9` is `import "sort"`.
**Fix:** Correct the Q&A entry to `fmt`, `sort`, `strings` so the plan writer cannot copy the superseded phrasing into the doc.

### [NIT:consistency] Cited `pattern` line numbers are mostly comment prose
**Section:** `fabric-verdict-do-not-extract-and-say-why-precisely` (`:123`)
**Issue:** Of the cited `pull.go:423,441,464`, only `:464` is a code reference; `:423` and `:441` are doc-comment prose — the same contamination the discussion elsewhere warns about (`:308-312`). The underlying claim (import at `pull.go:23`, `pull.go` alone) is correct.
**Fix:** Cite `pull.go:23` (import) and `pull.go:464` (sole use), or mark the other two as comment references.

## Verdict

REQUEST_CHANGES
One superseded claim survives verbatim in the Q&A log; everything else verified against source.
MILL_REVIEW_END
