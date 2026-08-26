# Batch: planparser-model-and-parse

```yaml
task: "Custom-typed plan cards skip path-missing checks"
batch: "planparser-model-and-parse"
number: 1
cards: 3
verify: go build ./... && go test ./internal/planparser/...
depends-on: []
```

## Batch Scope

This batch adds the per-label `TargetGroup` model to `internal/planparser`, populates it at parse time, and extends `normalizeCard` to cover every group's own refs and pairs.
It is one batch because the three changes are a single model migration: the struct, the one function that fills it, and the one function that rewrites its paths — none of them is separately meaningful, and all three live in the same package under 800 lines of production code total.
No validation behaviour changes here;
every existing `validate.go` check keeps reading `Card.Type` and the flat `Card.Targets`/`Card.Pairs`, which this batch preserves as the union across groups.

The external interface batch 2 consumes is `Card.TargetGroups []TargetGroup`, where `TargetGroup` is `{Type CardType; Refs []string; Pairs []MovePair; RenameRaw []string}` — one entry per type label the card body carried, in body order, with every path-shaped entry already normalized against the plan's `root:`.

Batch-local decision beyond `## Shared Decisions`: the parse-time nil-versus-empty distinction `parseRefField` already produces is preserved on a group's own `Refs`, so a type label written with no bullets under it yields a group with a non-nil empty `Refs` rather than a nil one — batch 2's `card-field-empty` group scoping depends on that distinction being visible per group.

## Cards

### Card 1: TargetGroup in the Card model

- **Context:**
  - `internal/planparser/parse.go`
  - `internal/planparser/validate.go`
  - `internal/planparser/normalize.go`
- **Edits:**
  - `internal/planparser/plan.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add an exported type `TargetGroup` to `internal/planparser/plan.go` with exactly four fields — `Type CardType`, `Refs []string`, `Pairs []MovePair`, `RenameRaw []string` — carrying a godoc comment on the type and on each field per `golang:golang-comments`.
  `TargetGroup` is one type label's own occurrence on a card: its `Type` is the `CardType` that label declares, its `Refs` is that label's own backtick-wrapped sub-bullets in body order, and its `Pairs`/`RenameRaw` are populated only when `Type` is `CardTypeRename`.
  Document on `Refs` that for a `Rename` group it carries both endpoints of every one of that group's own `Pairs` entries, `Old` then `New`, in pair order.
  Add the field `TargetGroups []TargetGroup` to `Card`, documented as one entry per recognized type label the card body carried, in body order — a card carrying two labels has two entries, and a card carrying the same label twice also has two entries.
  Rewrite `Card.Type`'s godoc: it is still the first recognized type label the card body carried, or `CardTypeUnknown` when none, and it is explicitly **not** validation state — no check in `internal/planparser/validate.go` reads it, and a new check must key on `TargetGroups`, never on `Type`, or it silently reintroduces first-label-wins.
  State it is retained for exported-field compatibility only.
  Rewrite `Card.TypeLabelCount`'s godoc the same way: it counts recognized type labels the card body carried, a count above one is legal rather than a defect, and it is not validation state.
  Rewrite `Card.Targets`'s godoc: the flat union across every `TargetGroups` entry's own `Refs`, in body order, retained because downstream consumers and the card-generic checks read it.
  Rewrite `Card.Pairs`'s and `Card.RenameRaw`'s godoc the same way: the flat union across every `Rename` group's own `Pairs`/`RenameRaw`, in body order.
  Update the file-header comment at the top of `plan.go` so its inventory of the public struct model names `TargetGroup`.
  Do not add a `Types []CardType` field — it is derivable from `TargetGroups` and would be redundant state that can drift.
  Do not remove `Card.Type`, `Card.TypeLabelCount`, or `Card.HasType`.
- **Commit:** `feat(planparser): add TargetGroup to the Card model`

### Card 2: parse-time population of TargetGroups

- **Context:**
  - `internal/planparser/plan.go`
  - `internal/planparser/normalize.go`
  - `internal/planparser/classify.go`
- **Edits:**
  - `internal/planparser/parse.go`
  - `internal/planparser/parse_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Change `parseTypeLabelCase` in `internal/planparser/parse.go` so each call appends exactly one `TargetGroup` to `card.TargetGroups`, with `Type` set to `typeLabels[label]`.
  Keep the existing `card.TypeLabelCount++`, `card.HasType = true`, and first-label-only `card.Type` assignment unchanged.
  In the `renameLabel` branch, set the new group's `Pairs` to that call's own `pairs` slice, its `RenameRaw` to that call's own `raw` slice, and its `Refs` to that call's own pair endpoints in `Old`-then-`New` pair order;
  keep appending the same values to `card.Pairs`, `card.RenameRaw`, and `card.Targets` so those three stay the flat union.
  In the non-`Rename` branch, set the new group's `Refs` to that call's own `refs` slice and keep appending `refs` to `card.Targets`.
  Preserve the nil-versus-empty distinction `parseRefField` returns: one of the six non-`Rename` type labels present with zero bullets under it yields a group whose `Refs` is the non-nil empty slice `parseRefField` returned.
  In the `renameLabel` branch, initialize the group's `Refs` to a non-nil empty slice before appending pair endpoints, so a `**Rename:**` label carrying zero well-formed pairs also yields a non-nil empty `Refs` rather than a nil one — without that initializer the branch's endpoint-append leaves `Refs` nil, and the seven labels would not share one stated invariant.
  Do not let a group's `Refs` alias the card-level `card.Targets` backing array — card 3's `normalizeCard` change rewrites both sides independently and must not double-apply `root:` through a shared array.
  Do not reorder `parseCardBody`'s switch cases.
  Update the wording of `parse.go`'s file-header comment, of the `typeLabels` table's comment, of the seven-type-label const block's comment, and of `parseTypeLabelCase`'s own doc comment so none of them asserts a card carries one type label;
  each states the one-or-more grammar and that each label contributes its own `TargetGroup`.
  In `internal/planparser/parse_test.go`, rewrite `TestParsePlan_Card_TypeLabelCount` from a defect-shape test into the multi-label happy-path test: its doc comment states that two labels on one card is the supported shape, its `two type labels` sub-test additionally asserts `len(card.TargetGroups) == 2` with `Type` `CardTypeEdit` then `CardTypeDelete`, each group's own `Refs`, and `card.Targets` equal to the concatenation of both groups' `Refs` in body order.
  Keep the `no type label` sub-test unchanged in intent — zero labels stays a defect shape.
  Add a sub-test proving a repeated label produces two groups whose union equals one merged group's refs, a sub-test proving a single-label card produces exactly one group, and a sub-test proving a card carrying two `**Rename:**` labels gives each group its own `Pairs` while `card.Pairs` equals the concatenation of both groups' `Pairs` in body order.
  Update `parse_test.go`'s file-header comment to name the type-label model as one-or-more.
- **Commit:** `feat(planparser): populate per-label TargetGroups at parse time`

### Card 3: normalization covers every group

- **Context:**
  - `internal/planparser/plan.go`
  - `internal/planparser/parse.go`
  - `internal/planparser/classify.go`
- **Edits:**
  - `internal/planparser/normalize.go`
  - `internal/planparser/normalize_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Change `normalizeCard` in `internal/planparser/normalize.go` so that, in addition to the card-level `card.Targets`, `card.Uses`, and `card.Pairs` it rewrites in place today, it also rewrites in place every `card.TargetGroups[i].Refs` via `normalizeRefSlice` and both endpoints of every `card.TargetGroups[i].Pairs` entry via `normalizeRefIfPath`.
  Never normalize `RenameRaw` on either side — it holds unparsed sub-bullet text captured verbatim so `rename-format` has something to report.
  Do not rebuild `card.Targets` or `card.Pairs` by concatenating the groups, and do not rebuild a group's fields from the card-level ones;
  both sides are normalized independently, which is the precedent `normalizeCard`'s own doc comment already records for the existing `Targets`/`Pairs` overlap and is what preserves `normalizeRefSlice`'s nil-versus-empty-slice distinction.
  Rewrite `normalizeCard`'s doc comment to state the post-condition explicitly: after the call, `Card.Targets` equals the concatenation of `TargetGroups[*].Refs` in body order and `Card.Pairs` equals the concatenation of `TargetGroups[*].Pairs` in body order, with symbol-shaped entries passing through verbatim on both sides.
  Update `normalize.go`'s file-header comment so its description of what `normalizeCard` touches names the groups.
  In `internal/planparser/normalize_test.go`, add a test proving that post-condition under a non-empty `root:` with at least one `//`-escaped entry and at least one symbol-shaped entry in a group, asserting both concatenation equalities and the symbol pass-through.
  Add a test proving a `Rename` group's own `Pairs.Old` is root-joined after `normalizeCard`, since that is the value batch 2's `checkPathMissing` stats — an un-normalized group pair would make it stat the unprefixed path.
  Extend `TestNormalizeCard_NilSliceStaysNil` so a group with a nil `Refs` and a group with a non-nil empty `Refs` both survive normalization with that distinction intact.
- **Commit:** `fix(planparser): normalize per-group refs and pairs`

## Batch Tests

`verify: go build ./... && go test ./internal/planparser/...` — the build gate catches any consumer of `planparser.Card` that a struct change breaks, and the package test run covers `internal/planparser/parse_test.go`, `internal/planparser/normalize_test.go`, `internal/planparser/validate_test.go`, `internal/planparser/classify_test.go`, `internal/planparser/approve_test.go`, `internal/planparser/sections_test.go`, and `internal/planparser/planpath_test.go`.
`go build ./...` rather than a package-scoped build is deliberate and cheap: `TargetGroup` is an exported model addition, and the whole point of retaining the flat `Targets`/`Pairs` union is that `internal/websterengine`, `internal/batcher`, and `internal/webstercli` keep compiling untouched — a repo-wide build is the only thing that proves it.
The pre-existing `validate_test.go` suite is the regression guard for this batch: every check still reads `Card.Type` and the flat unions here, so a green run proves the model addition changed no validation behaviour.
