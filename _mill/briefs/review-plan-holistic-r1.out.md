MILL_REVIEW_BEGIN
# Review: Custom-typed plan cards skip path-missing checks — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: claude-sonnet-4.5 (self-assessed; exact point version not introspectable)
reviewed_file: plan/
date: 2026-08-26
```

## Findings

### [BLOCKING:scope] validate_test.go's hand-built Card fixtures desync from TargetGroups
**Location:** batch 2, cards 4-7 (`internal/planparser/validate_test.go`)
**Issue:** After batch 2, six checks (`checkPathMissing`, `createTargetsUnion`, `checkProsaSymbolTarget`, `checkCardFieldEmpty`, `checkCardMissingField`, `checkRenameMechanicMissing`) read `Card.TargetGroups` instead of `Card.Type`/`Card.Targets`/`Card.Pairs`. Card 4 only teaches `validCard()`'s *baseline* to carry one `TargetGroups` entry (`Type: CardTypeEdit`, `Refs = Targets` at construction time). Dozens of pre-existing subtests construct `card := validCard(...)` then mutate `card.Type =`/`card.Targets =`/`card.Pairs =`/`card.Uses =` afterward — a Go slice/field reassignment that never touches the separately-held `TargetGroups[0]`. Concretely this breaks (verified by tracing the actual code): `TestValidate_ProsaSymbolTarget/"symbol target on a Prosa card"` (want 1, get 0 — stale group stays `Edit`, never `Prosa`); `TestValidate_PathMissing/"Create card absent target produces none"` (want 0, get 1 — stale `Edit` group's original `pkg/card1.go` ref is unmaterialized); `TestValidate_PathMissing/"Custom card..."` (want 1, get 2); `TestValidate_RenameMechanicMissing/"Rename card, mechanic absent"` (want 1, get 0 — `hasRename` never true since no group is ever retyped to `Rename`); the `TestValidate_CardMissingField` `otherTypes` loop (5 sub-cases, want 0, get 1). Worse, several others (`TestValidate_PathMissing`'s Rename-pair and Edit/Delete/Move/Prosa-loop subtests, `TestValidate_CardMissingField`'s "Delete missing ImpactSummary") coincidentally still report the expected count for the *wrong* reason (the stale `Edit` group happens to fall in the same checked bucket), silently testing nothing about the mutated `Type`.
**Fix:** Add an explicit batch-2 requirement (card 4, or a batch-scope instruction) to sweep every existing `validate_test.go` subtest that mutates a `validCard()`-derived `Type`/`Targets`/`Pairs`/`Uses` and keep `TargetGroups` in sync — ideally by replacing the pattern with a small helper (e.g. `cardOfType(number, slug string, typ CardType, refs []string) Card`) that sets `Type`, `Targets`, and the single `TargetGroups` entry together, so hand-built fixtures cannot desync from the checks under test.

## Verdict
REQUEST_CHANGES
Batch 2 leaves ~10 pre-existing validate_test.go subtests silently desynced from TargetGroups, several of which will fail or false-pass.
MILL_REVIEW_END
