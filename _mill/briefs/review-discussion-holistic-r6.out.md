MILL_REVIEW_BEGIN
# Review: Centralize glyph ref-shape enumeration

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-09-09
```

## Findings

### [BLOCKING:design] plan:-op exempt set is not package-qualified
**Section:** Decision: kind-policy-ledger
**Issue:** The `plan:`-op scan is declared to run "in either package" while its exempt set is bare basenames `{classify.go, shape.go, handle.go}`; `internal/planglyph/handle.go` shares that basename and holds four of the six handle-op migration sites (`handle.go:203`, `:256`, `:335`, `:402`), so a literal implementation exempts exactly the file the invariant most needs to bind.
**Fix:** State the exempt entries as package-qualified paths (`planparser/classify.go`, `planparser/shape.go`, `planparser/handle.go`) and say explicitly that no planglyph file is exempt from the `plan:`-op scan.

### [NIT:scope] Stale comments naming the deleted wrappers
**Section:** Decision: dead-wrappers / Testing (regression floor)
**Issue:** Only `doc.go:56` is listed as the production doc touched by wrapper deletion, but `normalize.go:7`, `normalize.go:70`, `normalize.go:107` and `parse.go:160` also name `isPathRef` in comments, and `parse_test.go:650-651`/`validate_test.go:1775` do too — after deletion these name a function that no longer exists.
**Fix:** Extend the doc-update list to those comment sites, or state that stale wrapper mentions in comments are accepted and untouched.

### [NIT:design] Enum↔slice sync test is count-only
**Section:** Decision: kind-policy-ledger ("The kind list's own source of truth")
**Issue:** The meta-test asserts `classify.go`'s const-block declaration count equals `len(allRefKinds)`; a hand-edited `allRefKinds` carrying a duplicate and omitting a kind satisfies both the count test and the domain-equality completeness test, leaving that kind unhandled everywhere.
**Fix:** Assert the const block's identifier set against the slice's rendered kinds (via `refKindName`), not just cardinality.

## Verdict

REQUEST_CHANGES
One under-specified exempt set would silently disarm the scan on planglyph's handle file.
MILL_REVIEW_END
