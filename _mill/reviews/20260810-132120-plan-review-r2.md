MILL_REVIEW_BEGIN
# Review: fabric: one ownership-and-dirtiness gate for all destruction (slice 12) — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: claude-sonnet-5 (per harness metadata; Sonnet-4.5-class reasoning)
reviewed_file: plan/
date: 2026-08-10
```

## Findings

### [BLOCKING:design] Card 3's `dirtyReason` treatment rests on an unachievable premise
**Location:** batch 01-dirtiness-probe, Card 3 (`internal/fabricengine/warpclean.go`)
**Issue:** Card 3 instructs "keep the function, its `label` parameter and its **two error formats**, and replace its body with a `worktreeDirty(scopeAll, dir)` call." `dirtyReason`'s current source has two distinct formats — `"%s: %w"` (spawn failure) and `"%s failed with exit code %d: %s"` (nonzero exit) — but Card 1 defines `worktreeDirty` to return a single consolidated `err` for *both* failure modes, with the exit code embedded inside that one error's text rather than returned separately. A body that is just `_, detail, err := worktreeDirty(...)` can therefore only ever wrap `err` with one format string (`"%s: %w"`), never reproduce the second, distinct nonzero-exit format. This is the identical collapse Card 2 explicitly concedes for `add.go`/`checkout.go` ("Add now has one error path where it had two … keep the wording of the spawn-failure form"), but Card 3 asserts the opposite outcome for `dirtyReason` instead of making the same concession.
**Fix:** Reword Card 3's `dirtyReason` instruction to match Card 2's pattern: keep the `label` parameter and the *spawn-failure* format only, and state explicitly that the nonzero-exit-specific format is subsumed into the single wrapped error, exactly as done for `add.go`/`checkout.go`.

### [NIT:consistency] Batch 1's blanket "preserved verbatim" claim contradicts Card 2's own text
**Location:** batch 01-dirtiness-probe, Batch Scope section
**Issue:** The batch scope states "Every error message and every returned error shape is preserved verbatim" — but Card 2 explicitly says "Because `worktreeDirty` collapses the spawn error and the nonzero-exit error into one returned error, `Add` now has one error path where it had two." These two statements directly contradict each other within the same batch document, and (combined with the Card 3 issue above) risk an implementer treating the blanket claim as authoritative over the card-level detail.
**Fix:** Qualify the Batch Scope sentence, e.g. "every refusal message and every surviving error path's wording is preserved verbatim; where two error paths collapse into one (add.go, checkout.go, and — per the fix above — warpclean.go), the surviving wording is the spawn-failure form."

## Verdict

REQUEST_CHANGES
Fix Card 3's self-contradictory `dirtyReason` error-format instruction and the batch-scope claim it conflicts with.
MILL_REVIEW_END
