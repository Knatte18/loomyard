MILL_REVIEW_BEGIN
# Review: fabric: close the corrindex two-phase read-modify-write race (slice 15) — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: claude-sonnet-5
reviewed_file: plan/
date: 2026-08-12
```

## Findings

### [BLOCKING:decision] state/doc.go card omits the one-direction-race statement the Shared Decision names it for
**Location:** batch 1, card 3 (`01-state-updatejson.md`)
**Issue:** The "this task closes one direction of the race, not both" Shared Decision (applies-to: all batches) explicitly lists `internal/state/doc.go` as one of four places the guarantee ("record() is serialised against every other write... not serialised against RebuildIndex's scan-to-write span") must be stated precisely — alongside `corrindex.go`'s doc comment, `internal/fabricengine/doc.go`, and the roadmap Done entry.
Cards 4, 5, and 7 each carry this statement (verified against their Requirements text), but card 3's Requirements for `internal/state/doc.go` only cover the generic RMW rule, why `UpdateJSON` can't compose `ReadJSON`+`WriteJSON`, and single-consumer adoption — the residual-window/one-direction caveat named by the Shared Decision is absent.
**Fix:** Either add an instruction to card 3 to state (in package-appropriate, consumer-agnostic terms) that `UpdateJSON` only closes the race for callers that use it, not for any writer that bypasses it — or narrow the Shared Decision's location list to the three docs that actually carry fabricengine-specific vocabulary (`corrindex.go`, `fabricengine/doc.go`, roadmap), dropping `internal/state/doc.go` from it.

## Verdict

REQUEST_CHANGES
Card 3's Requirements silently drop one of the four locations the Shared Decision commits the one-direction-race caveat to.
MILL_REVIEW_END
