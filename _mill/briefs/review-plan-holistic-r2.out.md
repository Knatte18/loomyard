MILL_REVIEW_BEGIN
# Review: self-report Tier 2: per-agent friction notes for unsupervised runs — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: Claude Sonnet 5 (model id claude-sonnet-5)
reviewed_file: plan/
date: 2026-09-12
```

## Findings

### [BLOCKING:design] Composer-level friction.Directive errors hard-fail the run
**Location:** batch 4 card 14 (`loomengine.DiscussionSpec`/`PlanSpec`), batch 5 card 17 (`burlerengine.Engine.Run`), batch 6 card 21 (all four `websterengine` composers).
**Issue:** the Shared Decision "every Tier 2 failure is a Warn, never a run failure" (applies to all batches) explicitly lists "a failed stencil read behind an already-composed prompt" as one of the Warn-only cases — this is exactly the `friction.Directive` stencil read inside a composer, which happens after the producer's own main template has already been read successfully. But cards 14/17/21 each instruct wrapping a `friction.Directive` error via `fmt.Errorf(...)` "matching the function's existing wrapping" / "exactly as the pattern.Directive error is wrapped" — i.e. mirroring `pattern.Directive`'s own hard-fail propagation (verified in `internal/loomengine/plan.go:71-74` and `internal/burlerengine/engine.go:104-107`, both of which return the wrapped error out of the Spec/Result constructor, which shedengine.Run's `callErr != nil` branch then treats as a full task failure). Since Tier 2 defaults to on (`friction: opus[effort=high]` per card 7), any transient stencil-read failure at Discussion-Write, Plan-Write, a Burler round, or any Webster prompt render would fail the entire task over optional bookkeeping — the exact outcome the Shared Decision was written to prevent.
**Fix:** cards 14/17/21 should instead have the composer catch a non-nil `friction.Directive` error, `logger.Warn` it (naming the stencil/role), and proceed with an empty directive (same as Tier-2-off), never propagating it out of `DiscussionSpec`/`PlanSpec`/`Engine.Run`/the four `Render*Prompt` functions.

### [NIT:scope] `shuttleengine.OutcomeAsking` runtime-failure branch is untested
**Location:** batch 3, card 12 (`internal/frictionengine/reflect_test.go`).
**Issue:** card 11 names four distinct runtime-failure outcomes `Reflect` must treat identically (`Warn`+`StatusFailed`+nil error, directory left untouched): a `Shuttle` error, `OutcomeDied`, `OutcomeTimeout`, and `OutcomeAsking`. Card 12's test enumeration only lists the first three ("A Shuttle error, shuttleengine.OutcomeDied, and shuttleengine.OutcomeTimeout each yield…") and never mentions `OutcomeAsking`.
**Fix:** add `OutcomeAsking` to card 12's bullet so all four documented failure branches are actually asserted.

### [NIT:scope] `internal/logger/logger.go` not in card 1's Context despite a direct `logger.Warn` call
**Location:** batch 1, card 1 (`internal/friction/friction.go`).
**Issue:** card 1's Requirements twice name the identifier `logger.Warn` verbatim (inside `WarnIfMarkerAbsent` and `EnsureDir`), but Context lists only `pattern.go`/`doc.go`/`stencil.go`/`reconcile.go`/`burlerengine/engine.go` — never `internal/logger/logger.go`. Cards 11, 23, and 25 all cite `internal/logger/logger.go` explicitly for the same kind of new call site, so this looks like an inconsistent omission rather than a deliberate choice (the included `burlerengine/engine.go` does demonstrate the calling convention, which is likely why this wasn't a bigger problem, but it isn't the file that declares the function).
**Fix:** add `internal/logger/logger.go` to card 1's Context list.

## Verdict

REQUEST_CHANGES
Composer-level friction-directive stencil-read failures are wired to hard-fail the run, contradicting the plan's own "Tier 2 never fails a run" decision.
MILL_REVIEW_END
