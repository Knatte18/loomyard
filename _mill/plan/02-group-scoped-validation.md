# Batch: group-scoped-validation

```yaml
task: "Custom-typed plan cards skip path-missing checks"
batch: "group-scoped-validation"
number: 2
cards: 4
verify: go build ./... && go test ./internal/planparser/...
depends-on: [1]
```

## Batch Scope

This batch converts all six type-conditional checks in `internal/planparser/validate.go` from card scope to group scope, relaxes `card-type-missing` from "exactly one label" to "at least one label", and adds the one new check ID `card-custom-not-alone`.
It is one batch because the six checks are exactly the sites that read `Card.Type` today, and converting only some of them would leave checks that disagree about what a card is — a card whose first label is `Create` would still escape `prosa-symbol-target` on its `Prosa` group.
The whole diff is confined to `validate.go` and `validate_test.go`, under 1500 lines together.

The card-generic checks must NOT change: `checkCardPathMalformed`, `checkCardFieldOverlap`, and the `Uses` loop inside `checkPathMissing` all read the flat card-level union batch 1 preserved, and that is deliberate.

Batch-local decision beyond `## Shared Decisions`: `card-custom-not-alone` emits exactly one finding per offending card, never one per offending group — the rule is a property of the card's label set, and per-group reporting would make a card look worse the more `Custom` groups it piled up.

## Cards

### Card 4: path-missing iterates target groups

- **Context:**
  - `internal/planparser/plan.go`
  - `internal/planparser/parse.go`
  - `internal/planparser/normalize.go`
  - `internal/planparser/classify.go`
- **Edits:**
  - `internal/planparser/validate.go`
  - `internal/planparser/validate_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Rewrite `checkPathMissing` in `internal/planparser/validate.go` so its per-card `switch c.Type` becomes a loop over `c.TargetGroups`, switching on each group's own `Type`.
  For a group whose `Type` is `CardTypeEdit`, `CardTypeDelete`, `CardTypeMove`, or `CardTypeProsa`, check that group's own path-shaped `Refs`.
  For a group whose `Type` is `CardTypeRename`, check that group's own `Pairs` entries' `Old` side and skip that group's `Refs` entirely — read the group's `Pairs`, never the card-level `c.Pairs`, which would report each missing `Old` once per `Rename` group on the card.
  For a group whose `Type` is `CardTypeCreate` or `CardTypeCustom`, skip its refs.
  Leave the card-level `c.Uses` loop above the switch exactly as it is — `Uses` is always checked, including on a `Custom` card.
  Leave the `satisfied` closure and the `report` helper unchanged.
  Rewrite `checkPathMissing`'s doc comment so every per-type rule it states is phrased per group rather than per card.
  In `internal/planparser/validate_test.go`, extend the `validCard` helper so the baseline card it returns also carries a `TargetGroups` slice holding one `planparser.TargetGroup` with `Type` `planparser.CardTypeEdit` and `Refs` equal to that card's own `Targets` value — without this every group-scoped check in this batch sees an empty group list and the whole suite stops exercising them.
  Update `validCard`'s doc comment to say so.
  Add to `TestValidate_PathMissing` the defect's own regression case: one card carrying an `Edit` group naming a path absent from the hermetic `worktreeRoot` plus a `Create` group naming a second absent path yields exactly one `path-missing` finding, on the `Edit` group's path only.
  Add a case proving a card carrying two `Rename` groups, one of whose `Old` sides is absent, yields exactly one `path-missing` finding rather than one per group.
  Add a case proving a card whose first group is `Create` and whose second group is `Edit` gets the `Edit` group's absent path reported — the observable consequence of first-label-wins being gone.
- **Commit:** `fix(planparser): scope path-missing to each card's own target groups`

### Card 5: create-union and prosa-symbol-target per group

- **Context:**
  - `internal/planparser/plan.go`
  - `internal/planparser/classify.go`
- **Edits:**
  - `internal/planparser/validate.go`
  - `internal/planparser/validate_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Change `createTargetsUnion` in `internal/planparser/validate.go` to collect path-shaped refs from every `TargetGroup` whose `Type` is `CardTypeCreate`, across every card, instead of from every card whose `Type` is `CardTypeCreate`.
  Change `checkProsaSymbolTarget` to iterate each card's `TargetGroups` and flag symbol-shaped entries in a group whose `Type` is `CardTypeProsa` only, so a symbol in the same card's `Edit` group is not flagged.
  Rewrite both function doc comments in group terms, and rewrite the `prosa-symbol-target` finding's `Detail` string so it names the offending group's label rather than calling the whole card a `Prosa` card.
  In `internal/planparser/validate_test.go`, add to `TestValidate_ProsaSymbolTarget` a case proving a card carrying an `Edit` group holding a symbol plus a `Prosa` group holding a symbol yields exactly one `prosa-symbol-target` finding, and a case proving a card whose only symbol lives in its `Edit` group yields none.
  Add a test proving a `Create` group on an otherwise-`Edit` card satisfies a later card's `Edit` target naming the same path, so the legitimate cross-card create-then-edit sequencing is not flagged as `path-missing`.
- **Commit:** `fix(planparser): scope create-union and prosa-symbol-target to target groups`

### Card 6: field-empty, ImpactSummary and rename-mechanic per group

- **Context:**
  - `internal/planparser/plan.go`
  - `internal/planparser/parse.go`
- **Edits:**
  - `internal/planparser/validate.go`
  - `internal/planparser/validate_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In `internal/planparser/validate.go`, replace `checkCardFieldEmpty`'s card-level `c.HasType && len(c.Targets) == 0` branch with a loop over `c.TargetGroups` emitting one `card-field-empty` finding per group whose own `Refs` is empty, whose `Detail` names that group's own bold label derived from the group's `Type`.
  Leave the `Uses:`, `Intent:`, and `ImpactSummary:` branches of that function unchanged.
  Change `checkCardMissingField` so the `ImpactSummary:` requirement fires when **any** of the card's `TargetGroups` has `Type` `CardTypeEdit` or `CardTypeDelete`, replacing the `c.Type == CardTypeEdit || c.Type == CardTypeDelete` test.
  Change `checkRenameMechanicMissing` so `hasRename` becomes true when any card carries any `TargetGroup` whose `Type` is `CardTypeRename`, and update both its doc comment and its finding `Detail` string so they say `Rename` group rather than `Rename` card.
  Rewrite `checkCardMissingField`'s doc comment in group terms.
  In `internal/planparser/validate_test.go`, add to `TestValidate_CardFieldEmpty` a case proving a card with a populated `Edit` group and an empty `Create` group yields exactly one `card-field-empty` finding whose `Detail` names the `Create` label.
  Add to `TestValidate_CardMissingField` a case proving a `Create`-plus-`Edit` card with no `ImpactSummary` yields a `card-missing-field` finding and a case proving a `Create`-plus-`Prosa` card with none yields no finding.
  Add to `TestValidate_RenameMechanicMissing` a case proving a plan whose only `Rename` group sits on a multi-label card, with no `## Rename mechanic` section, yields the finding.
- **Commit:** `fix(planparser): scope field-empty, ImpactSummary and rename-mechanic to target groups`

### Card 7: relax card-type-missing and add card-custom-not-alone

- **Context:**
  - `internal/planparser/plan.go`
  - `internal/planparser/parse.go`
- **Edits:**
  - `internal/planparser/validate.go`
  - `internal/planparser/validate_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In `internal/planparser/validate.go`, delete `checkCardTypeMissing`'s `case c.TypeLabelCount > 1` branch and its `"exactly one is required"` detail string, keeping the zero-label branch and its detail string unchanged, and rewrite the function's doc comment to say every card must carry at least one recognized type label.
  Add a new function `checkCustomNotAlone` implementing the check ID `card-custom-not-alone`: for each card, if some `TargetGroup` has `Type` `CardTypeCustom` and some other `TargetGroup` on the same card has a `Type` that **differs** from `CardTypeCustom`, emit exactly one finding for that card — never one per offending group, and never a finding for a card carrying two `Custom` groups and nothing else.
  Give it a doc comment stating that repetition of a label is legal for all seven labels including `Custom`, so the predicate is "a `Custom` group coexists with a group whose `Type` differs", not "a `Custom` group coexists with any other group".
  Insert `findings = append(findings, checkCustomNotAlone(plan)...)` into `validate`'s fixed dispatch list immediately after the `checkCardTypeMissing` call, so `card-custom-not-alone` is row 5 and every later check shifts down one, ending at row 17.
  Rewrite `validate.go`'s file-header comment: `ValidateFormat` emits sixteen of the seventeen distinct `ValidationError.Check` IDs, everything but `plan-unapproved`, and `Validate` emits all seventeen — with `card-custom-not-alone` named in the ID list in its row-5 position.
  Update `Validate`'s and `ValidateFormat`'s own doc comments to the new counts.
  In `internal/planparser/validate_test.go`, update the file-header comment to seventeen checks and to the format-only subset of sixteen, rewrite `TestValidate_CardTypeMissing` so its `two type labels` case now asserts zero `card-type-missing` findings while the zero-label case still asserts exactly one, and add `TestValidate_CustomNotAlone` covering: a card with a `Custom` group plus an `Edit` group yields exactly one finding;
  a `Custom`-only card yields none;
  a card carrying two `Custom` groups and nothing else yields none;
  a card carrying two `Custom` groups plus one `Edit` group yields exactly one finding, not two;
  a multi-label card with no `Custom` group yields none.
- **Commit:** `feat(planparser): relax card-type-missing and add card-custom-not-alone`

## Batch Tests

`verify: go build ./... && go test ./internal/planparser/...` — the same command as batch 1, for the same reason: the check-set change is exported behaviour (`Validate`/`ValidateFormat` are consumed by `internal/loomshed`, `internal/webstercli`, and `internal/websterengine`), so a repo-wide build is what proves no consumer signature broke, and the package test run covers `internal/planparser/validate_test.go`, which is where every one of this batch's four cards adds its assertions.
`TestValidate_GoldenFixture_ZeroFindings` is the batch's own regression guard: the golden fixture is still all single-label cards at this point, so a green run proves group scoping is behaviour-identical to card scoping on every existing single-label plan.
Batch 3 is what changes the fixture to a multi-label one and re-proves it.
