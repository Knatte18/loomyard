MILL_REVIEW_BEGIN
# Review: preflight: split into two Shed rows -- a generic one, and loom's own — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnetxhigh
reviewer_self_id: claude-sonnet-4.5 (self-assessed; exact point version not directly knowable)
reviewed_file: plan/
date: 2026-08-20
```

## Findings

### [NIT:consistency] overview.md placement instruction rests on a false adjacency premise
**Location:** batch 5 / card 30
**Issue:** Card 30 tells the implementer to place the new `internal/preflightshed` line "beside its two neighbours — the `internal/landingshed` line and the `internal/preflight` line," but in `docs/overview.md` today those two lines are not adjacent to each other — `mergeresolve`, `hubgeom`, and `standalonegeom` sit between them (lines 238–242) — so "beside both" has no single literal placement.
**Fix:** Reword to something unambiguous, e.g. "placed immediately above the `internal/preflight` line, which it wraps," since that is the more semantically relevant neighbour.

## Verification notes (no findings, spot-checked against source)

Cross-checked a large sample of the plan's specific factual claims against current source and found them accurate: `runCheck4`'s check-4 half starting at `preflight.go:87`; the single `checkCoherence(shed, product)` call site; `loomshed.go`'s current 12-row list/`Deps.Landing`'s pre-existing "rows 12/13" comment (confirmed already wrong today, confirmed correct once row 2 is inserted, exactly as card 13 states); `stub.go`'s "12-row"/five-stub wording; `smoke_test.go` line 641's `loomengine.Preflight` call and the "eight of its thirteen rows" doubly-wrong claim; the exact four `fabricengine` comment sites naming `loomengine.Preflight` (closed set, verified via repo grep); `wiring.go`/`wiring_test.go`/`cli.go`'s exact current text targeted by cards 15–16; `manifest/designs/loom.md`'s closed set of four row-number references (verified via grep, no fifth reference missed); `docs/overview.md`'s pre-existing "13-row" line (confirmed already-correct, needs no edit, per card 30); `loom-status-spec.md`'s three targeted passages (card 32); and the 8-direct-counterpart + 2-orphan + 3-seed-test = 13 disposition table for the retiring `internal/loomengine/preflight_integration_test.go` (card 23), including the exact 2-vs-5 and 1-vs-6 sub-case deltas cards 21 migrates. All of these matched the plan's claims exactly. The Batch Index DAG, global step numbering (1–33, no gaps), Moves formatting, and Rename-mechanic-section presence in batches 1 and 3 are all well-formed.

## Verdict
APPROVE
No blocking findings; one NIT on an imprecise placement instruction in card 30.
MILL_REVIEW_END
