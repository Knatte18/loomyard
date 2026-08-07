MILL_REVIEW_BEGIN
# Review: fabric: close the weft-visibility leak (slice 8) — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-08-07
```

## Findings

### [BLOCKING] configsync carve-out documented narrower than the Shared Decision and the actual enforcement rule
**Location:** `CONSTRAINTS.md:134`, `internal/fabricengine/doc.go:399`
**Issue:** Both say the `internal/configsync` owner-set row is "(string literals only)". `_mill/plan/00-overview.md`'s `vocabulary owner set and carve-outs` Shared Decision explicitly widens this to "string literals and comments, never identifiers", and the batch-07 implementation (`internal/lyxcwd/enforcement_test.go`'s `failsBareVocabularyCheck`) matches the widened rule: for `configsyncOwnerDir` it fails only on `bareIdent`, never on a literal-or-comment hit — comments are carved out too. `internal/configsync/configsync.go` itself relies on this (its comments at lines 20-24, 36-43 name `warp.yaml`/`weft.yaml` freely, per card 22's explicit instruction to leave them verbatim). CONSTRAINTS.md is the repo's authoritative, partly-machine-enforced rule statement, so its text should match what the test actually enforces and what the Shared Decision actually says.
**Fix:** Update both lines to "(string literals and comments, never identifiers)" to match the overview's Shared Decision and the enforcement test's actual behavior.

## Verdict

REQUEST_CHANGES
One documentation/Shared-Decision-alignment defect (configsync carve-out wording); implementation, tests, and consumer migration otherwise verified correct.
MILL_REVIEW_END
