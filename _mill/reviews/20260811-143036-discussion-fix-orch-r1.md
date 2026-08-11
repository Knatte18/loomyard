# Discussion fix — orchestrator review r1

Source review: `_mill/reviews/20260811-orch-review-r1.md` (verdict APPROVE, one NIT).

## Fixed

- `[NIT:clarity]` "The exported `Check` set" doesn't name which package exports it — verified accurate against the tree: `internal/fabricengine`'s `destructiveCheck` (`destroy.go:33-40`) is entirely unexported, and the exported `Check` set lives in `internal/fabricengine/fabrictest/refusal.go:19-30` behind the `integration` build tag.
  A grep of `internal/fabricengine` for `CheckContainment` returns nothing, confirming the reviewer's stated failure mode for an implementer.
  Rewrote the "The refusal type" paragraph in Technical context to attribute each enum to its own package explicitly, and mirrored the attribution into the Constraints section's `checkForce` bullet.

## Pushed Back

None.

## Beyond the finding

The fix surfaced a consequence the finding did not state, added while correcting the attribution rather than deferred:

- The structured-refusal decision emits `refusal.check` as a machine-readable JSON field from **production** code, which can only read the unexported `fabricengine` enum — fabrictest's exported mirror is test-support behind `//go:build integration` and is not importable from production.
  Naming the two enums without naming which one production serialises from would have left an implementer free to reach for the wrong (and uncompilable) one.
- Emitting `refusal.check` also promotes `destructiveCheck.String()`'s output to part of fabric's public contract, making fabrictest's string-backed copy a second encoding of it.
  Flagged in Technical context as an explicit decision for mill-plan (reconcile the two, or deliberately keep parallel copies) rather than something to settle by default.
