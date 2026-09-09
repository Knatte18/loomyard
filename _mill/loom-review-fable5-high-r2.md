# loom review — round 2 safety pass (fable5-high-r2)

Clean-room review of the two refactors (ref-shape registry centralization in `internal/planparser/shape.go`; quarry v0.2.0 `Status.Known()`/`ResolveResult.Rejected()` adoption in `internal/planglyph`), driven live through the real built `cmd/lyx` binary.
Round context: round 1 (`opus5-high-r1`) closed F1–F5 and drove eleven live scenarios; this round is a genuinely independent pass to find anything both it and the orchestrator's verification missed.

## Executive summary

(written last)

## Scope assessment

(written last)

## Code findings

(provisional entries appended as formed)

## Docs & operability findings

(provisional entries appended as formed)

## What was tested

Observations appended immediately after each command/scenario returns.

### Spec/contract reading notes

- quarry v0.2.0 `engine.Status.Known()` (module cache `internal/engine/answer.go:65`): true exactly for the four documented values `found`/`not_found`/`ambiguous`/`multipart`; false for `""` and any other string.
- `ResolveResult.Rejected()` (`answer.go:262`): `r.Status == ""` — the documented pre-resolution-rejection marker (Status and Error never both set).
- These match the call sites' assumptions: `donecheck.go:163` (`!r.Status.Known()` fail-closed before reading the two booleans), `resolve.go:139` (`r.Rejected()` selecting the rejection detail).
