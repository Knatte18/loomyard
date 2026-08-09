MILL_REVIEW_BEGIN
# Review: builder: delete internal/builderengine and internal/buildercli, retire builder-contract.md as a reference — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: claude-sonnet-5
reviewed_file: plan/
date: 2026-08-09
```

## Findings

### [BLOCKING:scope] Bare-word exclusion list misses a ninth ordinary-English site
**Location:** batch 5, card 18 (acceptance sweep), and the `sweep-completion-is-a-grep-not-a-judgment` Shared Decision.
**Issue:** `internal/lyxtest/lyxtest.go:60` reads `// Template builders: cached, built once per test binary via sync.Once.` — a live, case-insensitive, word-boundary hit on the bare-word `builder` pattern (same class as the enumerated "fixture builder(s)" tokens), ordinary English unrelated to the deleted module.
It is not on card 18's enumerated exclusion list (which the discussion/plan claim is "exactly eight" derived sites) and `internal/lyxtest/lyxtest.go` is not in any batch's `Edits:`, so no card touches it.
Per card 18's own rule ("If the scan turns up any other ordinary-English or unrelated-fixture site, stop and report it as a finding rather than adding a token to this list"), running the sweep as written will legitimately halt on this site at the very last card of the task rather than complete clean.
**Fix:** Add `internal/lyxtest/lyxtest.go:60`'s "Template builders" to card 18's enumerated bare-word exclusions now (or add the file to a batch's `Edits:` and reword the comment), so the acceptance gate is genuinely zero-hit rather than discovering this live during execution.

## Verdict

REQUEST_CHANGES
One enumerated-exclusion gap in card 18's bare-word sweep; every other batch, card, and cross-reference checked against source verified accurate.
MILL_REVIEW_END
