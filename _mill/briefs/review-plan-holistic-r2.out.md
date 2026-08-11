MILL_REVIEW_BEGIN
# Review: batcher: split out of webster into a standalone configreg module with its own batcher.yaml — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnetxhigh
reviewer_self_id: Claude Sonnet 5 (claude-sonnet-5)
reviewed_file: plan/
date: 2026-08-11
```

## Findings

### [NIT:design] Card 12's doc-count rationale is arithmetically off
**Location:** batch 3 / card 12 (`internal/configcli/configcli_test.go`)
**Issue:** Card 12 says the comment "The other nine modules are absent" is falsified because "there are ten now," implying the count must be bumped. But pre-task total is 9 modules (confirmed in `configreg.go`/`configreg_test.go`), so "other" (excluding seeded `board`) = 8 today — the comment is already a pre-existing off-by-one, and it is `strings.Count(...) < 2` in the test, never numerically asserted. Post-task total is 10, so "other" = 9 — the literal word "nine" becomes correct by coincidence once `batcher` is added, and needs no edit at all; bumping it to "ten" (as the rationale implies) would make it wrong.
**Fix:** Either drop this sub-instruction, or state explicitly that "other" = total − 1 (seeded `board`) = 9, so the existing word stays "nine" unchanged.

### [NIT:consistency] Batch 2's "atomic compile unit" framing overstates the compile coupling
**Location:** batch 2 / Batch Scope
**Issue:** The scope text claims cards 5–8 "must move together" because removing `Config.Batcher` is "an atomic compile unit" spanning production code, the untagged config test, and the two `//go:build integration` files. In fact neither `runlevel_test.go` nor `verbs_test.go` has a compile-breaking reference to `Config.Batcher` — `newRunFixture`'s `Config{}` literal never sets a `Batcher:` key, and `verbs_test.go`'s only reference is inside a comment plus an unaffected `batcher.Select("")` call. Card 5 alone still compiles under both build tags; what actually breaks at that intermediate point is test *behavior* (nil `RunDeps.Batcher` → `ErrNilBatcher` on every `TestRun_*`; a stale `strings.Replace` target that silently no-ops).
**Fix:** Reword to "atomic correctness unit" (or similar) — the card grouping and sequencing are unaffected either way.

## Verdict

APPROVE
No BLOCKING findings; the plan is well cross-referenced against source and internally consistent.
MILL_REVIEW_END
