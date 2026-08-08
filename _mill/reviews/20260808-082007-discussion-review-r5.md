MILL_REVIEW_BEGIN
# Review: Scout owns its own lyxcwd-based geometry accessors (drop Options.AnchorRoot threading)

```yaml
verdict: GAPS_FOUND
reviewer_model: opus
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-08
```

## Findings

### [GAP] baseOptions premise contradicted by cli.go source
**Section:** `fix-assert-no-callers-literal` / Technical context "The CLI side" **Issue:** The discussion states "A direct `buildOptions` swap is not possible: the literal carries no `Query`" — but `cli.go:587` parses `query` *before* the literal at `:593`, and both derived values set the identical `defOpts.Query = query` (`:613`) and `refOpts.Query = query` (`:621`), so one `buildOptions(registry, dir, layout, lang, query, timeout)` value is byte-equivalent for both calls and the second extraction is not forced. **Fix:** Restate the rationale for `baseOptions` on its real merits (shared construction site / future divergence), or drop the extraction and keep only `lookupContext`, which is the seam that actually owns the bug.

### [GAP] buildOptions' own doc comment left unassigned
**Section:** `docs-in-same-commit` (mandated doc comments list) **Issue:** `cli.go:489-490` — "buildOptions constructs a scoutengine.Options value, ensuring all construction sites thread **WorktreeRoot** consistently" — survives the change as the wrapper's doc and becomes false, yet it appears in neither the six-site `scoutengine` list nor the separately-listed mandated comments (which name only `lookupContext`, `baseOptions`, `resolveLocation`, and the three `:147-150`/`:295-296`/`:416-417` blocks). **Fix:** Add `cli.go:489-490` to the mandated-comment list, rewording to `Layout`.

### [NOTE] supervised_test.go call-site count is three, not four
**Section:** Technical context, "Affected test files", entry 2 **Issue:** The text says "**four** `ensureSupervised(…)` calls at 96, 153, 318" — grep of the file confirms exactly three `ensureSupervised` call sites, at those three lines; the parenthetical about the third fixture having two consumers does not make a fourth call. **Fix:** Correct the count to three, so the enumeration the discussion bills as its own largest correctness risk is internally consistent.

## Verdict

GAPS_FOUND
Two rationale/enumeration defects verified against source; core design is sound.
MILL_REVIEW_END
