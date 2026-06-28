MILL_REVIEW_BEGIN
# Review: Board fixes from sandbox run — payload keys, help, rerender — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-06-28
```

## Findings

### [NIT] Missing explicit test: set-status rejects stray `phase` key
**Location:** `C:\Code\loomyard\wts\board-sandbox-fixes\internal\board\cli_test.go`
**Issue:** The plan's Testing section (discussion.md) explicitly requires `set-status '{"slug":"x","phase":"done"}'` to error; `TestCLILookupContract` covers `get` with the old `id_or_slug` key but has no case for `set-status` or `remove` with a stray `phase`/`id_or_slug` key. The `resolveLookup` mechanism is correct and exercised via `get`, so the behaviour is sound — only the pinned test case is missing.
**Fix:** Add a `TestCLILookupContract` subcase `set_status_stray_phase_errors` with payload `{"slug":"x","phase":"done","status":"active"}` (or absent status) asserting exit 1 + "unknown field".

### [NIT] Plan–implementation mismatch: `merge` `upsert` field optionality
**Location:** `C:\Code\loomyard\wts\board-sandbox-fixes\_mill\plan\02-cli-help.md` (Card 6) vs `C:\Code\loomyard\wts\board-sandbox-fixes\internal\board\cli.go:382-388`
**Issue:** Batch 2 Card 6 documents `upsert` as "optional" in `merge`; the implementation and the `Long` text both correctly enforce it as required (consistent with Batch 1 Card 5 describing "upsert one task"). The plan card's wording is imprecise rather than the code being wrong.
**Fix:** No code change needed; the next plan revision should change the card 6 description to "required" to match the implementation.

## Verdict

APPROVE
Implementation is complete and correct; two NITs, no blocking issues.
MILL_REVIEW_END
