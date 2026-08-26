# Plan: Custom-typed plan cards skip path-missing checks

```yaml
task: "Custom-typed plan cards skip path-missing checks"
slug: "plan-custom-card-skips-path-check"
approved: true
started: "20260826-175419"
parent: "main"
root: ""
verify: null
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to schedule batches.
Every batch lives at `NN-<batch-slug>.md` in this directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: planparser-model-and-parse
    file: 01-planparser-model-and-parse.md
    depends-on: []
    verify: go build ./... && go test ./internal/planparser/...
  - number: 2
    name: group-scoped-validation
    file: 02-group-scoped-validation.md
    depends-on: [1]
    verify: go build ./... && go test ./internal/planparser/...
  - number: 3
    name: format-contract-sweep
    file: 03-format-contract-sweep.md
    depends-on: [1, 2]
    verify: go build ./... && go test ./internal/planparser/... ./internal/loomengine/... ./internal/loomshed/... ./internal/loomrecipe/... ./internal/websterengine/... ./internal/webstercli/... ./contracts/stencils/... && go test -tags integration ./internal/websterengine/...
```

## Shared Decisions

### Decision: multi-label-cards-are-the-fix

- **Decision:** a format-4 plan card carries **one or more** bold type labels from the existing seven, each label owning its own indented backtick-wrapped target sub-bullets, and each group is validated under its own type's rules.
  No eighth card type is added and no per-target inline annotation syntax is introduced.
- **Rationale:** the reported defect is that the plan stencil's bundle-your-own-test rule plus its exactly-one-type-label rule force an edit-plus-create card into `**Custom:**`, which is exactly the label whose targets `checkPathMissing` skips.
  Multi-label states the truth in vocabulary the format already has, and shrinks `Custom` back to a genuine last resort.
- **Applies to:** all batches

### Decision: additive-model-flat-union-retained

- **Decision:** `planparser.Card` gains `TargetGroups []TargetGroup`; the existing flat `Card.Targets`, `Card.Pairs`, and `Card.RenameRaw` are **retained** as the union across all groups, in body order. `Card.Type` (first label seen) and `Card.TypeLabelCount` are retained but are no longer validation state, and their godoc says so outright.
- **Rationale:** `internal/websterengine/sequence.go`'s `SequenceBatches` derives its whole dependency graph from flat `Targets`/`Uses` ref intersection and never reads `CardType`;
  `checkCardFieldOverlap`, `checkCardPathMalformed`, and `normalizeCard`'s card-level pass likewise operate on the flat list.
  Retaining the flat union confines the diff to the parser plus the six type-conditional checks, leaving `internal/websterengine` and `internal/batcher` untouched.
- **Applies to:** all batches

### Decision: pairs-and-renameraw-are-group-owned

- **Decision:** `TargetGroup` owns its own `Pairs` and `RenameRaw` alongside its `Refs`, and a `Rename` group's `Refs` carries both endpoints of every one of its own pairs, `Old` then `New`, in pair order.
- **Rationale:** `Card.Pairs` is a flat card-level accumulator today, so a group-scoped `checkPathMissing` iterating `c.Pairs` once per `Rename` group would report each missing `Pairs.Old` once per `Rename` group on the card.
  Group ownership is what makes "a repeated label needs no check of its own" true for all seven labels rather than six.
- **Applies to:** planparser-model-and-parse, group-scoped-validation

### Decision: normalization-covers-both-sides-independently

- **Decision:** `normalizeCard` normalizes every `TargetGroup`'s own `Refs` and `Pairs` in place **in addition to** the card-level `Targets`, `Uses`, and `Pairs` it normalizes today.
  Neither side is rebuilt from the other. `RenameRaw` is never normalized on either side.
- **Rationale:** `parseTypeLabelCase` appends pairs to the card by value, so a group's `Pairs` is a struct copy that today's in-place card-level loop cannot reach.
  Without this, a group-scoped `path-missing` reading `TargetGroups[*].Pairs.Old` would stat the raw unprefixed path under a plan-level `root:` and emit a false positive.
  Normalizing both sides independently follows the precedent `normalizeCard`'s own doc comment already records for the existing `Targets`/`Pairs` overlap, and preserves `normalizeRefSlice`'s nil-vs-empty-slice distinction a rebuild-by-concatenation would flatten.
- **Applies to:** planparser-model-and-parse

### Decision: custom-exemption-retained

- **Decision:** a `Custom` group stays exempt from `path-missing` on its own targets and from `prosa-symbol-target`, and stays bound by every card-generic check.
  The new `card-custom-not-alone` check flags a card carrying a `Custom` group alongside a group whose `Type` **differs** — never merely alongside any other group, so two `Custom` groups on one card stay legal.
- **Rationale:** the defect is that ordinary cards were forced into `Custom`, not that the exemption exists.
  Removing the exemption would false-positive every legitimate `Custom` card that creates something.
- **Applies to:** group-scoped-validation, format-contract-sweep

### Decision: check-count-goes-16-to-17

- **Decision:** one new check ID, `card-custom-not-alone`, inserted at row 5 immediately after `card-type-missing`;
  rows 5-16 shift down to 6-17. `card-type-missing` drops its `TypeLabelCount > 1` branch and keeps its zero-label branch.
  The format-only set `ValidateFormat` runs goes 15 -> 16, and `plan-unapproved` remains the one ID belonging only to the wider `Validate` entry point.
- **Rationale:** the `>1` branch is the literal enforcement of the rule this task removes.
  `Custom`-must-be-alone is a genuinely distinct defect from "no label at all";
  bundling it into `card-type-missing`'s detail text would walk back the spec's own cleanup of a former 14-row list that bundled two IDs into one row.
- **Applies to:** group-scoped-validation, format-contract-sweep

### Decision: per-type-table-composition

- **Decision:** the card-types table stays one row per type and gains a stated composition rule per column: **Target list holds** — per group, no composition; **Mechanical check** — union across groups, each run against that group's own targets, with `Create`'s "none — check nothing equivalent exists first" cell joining the union as a real obligation; **`ImpactSummary`** — required when any group is `Edit` or `Delete`, one per card, never per group; **Batchable?** — least permissive wins, with `Prosa`/`Custom`'s "—" never a vote.
- **Rationale:** every column silently assumed a card has exactly one row.
  Union-for-obligations and least-permissive-for-permissions is the only pairing under which adding a second label can never *reduce* what a card owes, which is what keeps multi-label from becoming a new escape hatch in place of `Custom`.
- **Applies to:** format-contract-sweep

### Decision: markdown-semantic-line-breaks

- **Decision:** every `.md` file this plan edits — the spec, both loom rubrics, the webster implementer stencil, the design docs, the sandbox suite doc, and the golden fixture card — is written with one sentence per line plus breaks at internal independent-clause boundaries, using plain newlines only.
- **Rationale:** `CLAUDE.md`'s Markdown rule applies to every `.md` file in the repo, not only newly-written ones.
- **Applies to:** format-contract-sweep

### Decision: roadmap-untouched

- **Decision:** `manifest/roadmap.md` and `CONSTRAINTS.md` are not touched by this task.
- **Rationale:** this is a format correction, not a planned-item completion, and it introduces no new cross-cutting invariant.
  The Planparser Sole-Parser Invariant, the Told-Geometry Invariant, and the Test Tier Purity Invariant all hold unchanged: no package outside `internal/planparser` learns to parse type labels, `checkPathMissing` keeps taking `worktreeRoot` as a told value, and no lookup needing a subprocess is introduced.
- **Applies to:** all batches

## All Files Touched

- `contracts/recipes/loom-recipe.yaml`
- `contracts/specs/loom-plan-spec.md`
- `contracts/stencils/loom/loom-rubric-plan-review.md`
- `contracts/stencils/loom/loom-rubric-webster-review.md`
- `contracts/stencils/loom/loom-template-plan.md`
- `contracts/stencils/webster/webster-body-implementer.md`
- `internal/loomengine/plan_test.go`
- `internal/loomrecipe/fixture_test.go`
- `internal/loomshed/planvalidate_test.go`
- `internal/planparser/doc.go`
- `internal/planparser/normalize.go`
- `internal/planparser/normalize_test.go`
- `internal/planparser/parse.go`
- `internal/planparser/parse_test.go`
- `internal/planparser/plan.go`
- `internal/planparser/testdata/goodplan/02-json-flag.md`
- `internal/planparser/validate.go`
- `internal/planparser/validate_test.go`
- `internal/webstercli/validate.go`
- `internal/websterengine/runlevel_test.go`
- `internal/websterengine/sequence_test.go`
- `manifest/designs/loom.md`
- `manifest/designs/plan-card-format.md`
- `manifest/designs/scout-plan-symbol-fields.md`
- `tools/sandbox/SANDBOX-WEBSTER-SUITE.md`
