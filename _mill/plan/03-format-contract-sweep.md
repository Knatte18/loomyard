# Batch: format-contract-sweep

```yaml
task: "Custom-typed plan cards skip path-missing checks"
batch: "format-contract-sweep"
number: 3
cards: 6
verify: go build ./... && go test ./internal/planparser/... ./internal/loomengine/... ./internal/loomshed/... ./internal/loomrecipe/... ./internal/websterengine/... ./internal/webstercli/... ./contracts/stencils/... && go test -tags integration ./internal/websterengine/...
depends-on: [1, 2]
```

## Batch Scope

This batch makes the shipped contract match the code batches 1 and 2 changed: the golden fixture gains a multi-label card, and every site in the repo stating the validation-check count, the `ValidateFormat`/`Validate` entry-point split, or the singularity of a card's type label is rewritten.
It is one batch because it is a single sweep against one predicate — any site asserting a card has one file-operation type, in any phrasing — and splitting it would leave the repo half-migrated between two commits, with the spec, the stencil that generates plans, the rubric that reviews them, and the design doc temporarily disagreeing.
The card split inside the batch is by document family, not by phrase, so each card's diff is reviewable on its own.

Card 8 must land before card 9: the spec's worked example is byte-consistent with the golden fixture, and the golden round-trip test is what proves it, so the fixture is the source and the spec mirrors it.

Batch-local decisions beyond `## Shared Decisions`: the `Custom` exemption text in the plan-review rubric stays in place and gains a sentence rather than being replaced, because the exemption itself is retained;
and every literal substring `contracts/stencils/rubric_test.go` and `internal/loomengine/plan_test.go` assert against must either survive the edit or be updated in the same card as the edit that breaks it.

## Cards

### Card 8: golden fixture exercises a multi-label card

- **Context:**
  - `internal/planparser/testdata/goodplan/00-overview.md`
  - `internal/planparser/testdata/goodplan/03-json-emission.md`
  - `internal/planparser/plan.go`
- **Edits:**
  - `internal/planparser/testdata/goodplan/02-json-flag.md`
  - `internal/planparser/parse_test.go`
  - `internal/planparser/validate_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add a `**Create:**` group to `internal/planparser/testdata/goodplan/02-json-flag.md`, placed immediately after its existing `**Edit:**` group and before its `**Uses:**` group, carrying the single sub-bullet `` - `list_json_test.go` ``.
  Leave that card's `**Edit:**`, `**Uses:**`, `**Intent:**`, and `**ImpactSummary:**` fields byte-identical.
  Do not renumber any card and do not add an eighth card file — `03-json-emission.md` stays `Custom` so that label is still exercised.
  In `internal/planparser/parse_test.go`, update `TestParsePlan_GoldenFixture`'s `wants` entry for card 2: `typeLabelCount` becomes 2, `typ` stays `planparser.CardTypeEdit` because `Type` is still the first label the card body carried, and `targets` becomes the three-entry concatenation of both groups' refs in body order, with the `Create` group's ref resolving under the plan's `root: internal/boardcli` to `internal/boardcli/list_json_test.go`.
  Extend the `wantCard` struct and its assertion loop with a per-card expected `TargetGroups` shape, and assert card 2's two groups explicitly — `CardTypeEdit` then `CardTypeCreate`, each with its own `Refs` — while every other golden card asserts exactly one group.
  Update `TestParsePlan_GoldenFixture`'s doc comment to say the fixture round-trips a multi-label card.
  In `internal/planparser/validate_test.go`, leave `internal/boardcli/list_json_test.go` deliberately absent from `TestValidate_GoldenFixture_ZeroFindings`'s `materializeFiles` call and add a comment beside the existing deliberate-absence comment recording that it is card 2's own `Create`-group target and is therefore exempt.
  Update that test's doc comment and the file-header comment's description of the golden fixture accordingly.
- **Commit:** `test(planparser): exercise a multi-label card in the golden fixture`

### Card 9: plan-format spec states the multi-label grammar

- **Context:**
  - `internal/planparser/testdata/goodplan/02-json-flag.md`
  - `internal/planparser/validate.go`
  - `manifest/designs/plan-card-format.md`
- **Edits:**
  - `contracts/specs/loom-plan-spec.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In `contracts/specs/loom-plan-spec.md`, change the status line's "the sixteen checks below are already implemented" to seventeen.
  Rewrite the `## Card fields and order` item 2 from "**Exactly one bold type label**" to one or more bold type labels from the same seven-name set, each label's own indented backtick-wrapped sub-bullets being that label's own targets, and state that a card carrying a `**Custom:**` group may carry no group of a different type.
  Add a worked two-line example showing an `**Edit:**` group followed by a `**Create:**` group on one card.
  Update the "a required field a card omits (its type label ...)" sentence so it reads in the one-or-more world.
  Beneath the `## Card types` table, add the four composition rules named in the plan overview's `per-type-table-composition` Shared Decision, one bullet per column, stating explicitly that a `Create` group's "none — check nothing equivalent exists first" cell is a real obligation that joins the mechanical-check union, that `ImpactSummary` stays one per card and states the blast radius across every `Edit`/`Delete` group's targets together, and that `Prosa`/`Custom`'s "—" in the Batchable column is never a vote.
  In `## Validation checks`, change "sixteen rows, sixteen IDs" to seventeen, change the entry-point split sentence from fifteen-of-sixteen to sixteen-of-seventeen, rewrite row 4 `card-type-missing` to "every card carries at least one recognized type label; zero is flagged", insert a new row 5 `card-custom-not-alone` describing the differing-`Type` predicate and the one-finding-per-card cardinality, renumber the former rows 5 through 16 as 6 through 17, and rewrite each renumbered row whose wording assumes one type per card — `card-path-malformed`, `rename-mechanic-missing`, `card-missing-field`, `card-field-empty`, `card-field-overlap`, `prosa-symbol-target`, and `path-missing` — so each states its rule per target group where the rule is group-scoped and per card where it is card-generic.
  Keep `path-missing`'s sub-bullet recording that `Custom` stays exempt on its own targets and from `prosa-symbol-target`, restated in group terms.
  In `## Deferred / forward-compat`, add one sentence confirming the `changes-files`/deviation union is defined over the flat target set and is therefore unchanged by multi-label.
  In `## Worked example`, update the intro sentence that says all seven type labels are exercised so it also records that card 2 is the multi-label example, and add the same `**Create:**` group to the `_lyx/plan/02-json-flag.md` listing so it stays byte-consistent with the golden fixture.
  Add `list_json_test.go` to the trailing paragraph that enumerates which example entries resolve under `root:`.
- **Commit:** `docs(spec): multi-label card grammar and card-custom-not-alone`

### Card 10: plan-write stencil teaches Edit-plus-Create

- **Context:**
  - `contracts/specs/loom-plan-spec.md`
  - `internal/loomengine/plan.go`
- **Edits:**
  - `contracts/stencils/loom/loom-template-plan.md`
  - `internal/loomengine/plan_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In `contracts/stencils/loom/loom-template-plan.md`, rewrite the `### Each NN-<card-slug>.md` grammar line so it reads one or more bold type labels from the seven-name set, each label's own indented backtick-wrapped sub-bullets being the card's targets for that label.
  Add an explicit adjacent line stating that an implementation card bundling its own new test file writes `**Edit:**` for the implementation and `**Create:**` for the new test file, in that order, and that this is the normal shape rather than an exception.
  Add a line stating `**Custom:**` is a last resort used only where none of the other six genuinely fits, that a card whose targets can be expressed as a multi-label combination is not `Custom`, and that a `**Custom:**` group may not be combined with a group of a different type.
  In `internal/loomengine/plan_test.go`, update both literal substrings `TestPlanSpec_PromptStatesTypeLabelGrammar` asserts so they match the rewritten stencil wording, keeping the test's purpose intact — it proves the stencil's grammar text reaches the rendered `Plan-Write` prompt.
- **Commit:** `docs(stencil): plan template teaches multi-label cards`

### Card 11: rubrics, recipe, implementer stencil and loom design doc

- **Context:**
  - `contracts/stencils/rubric_test.go`
  - `contracts/specs/loom-plan-spec.md`
  - `manifest/designs/plan-card-format.md`
- **Edits:**
  - `contracts/stencils/loom/loom-rubric-plan-review.md`
  - `contracts/stencils/loom/loom-rubric-webster-review.md`
  - `contracts/stencils/webster/webster-body-implementer.md`
  - `contracts/recipes/loom-recipe.yaml`
  - `manifest/designs/loom.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In `contracts/stencils/loom/loom-rubric-plan-review.md`, change "fifteen of them upstream by `Plan-Validate`" to sixteen, change "a sixteen-check mechanical validator" to seventeen-check, and in the `Do not flag` list change "The sixteen check IDs" and "fifteen of the sixteen upstream" to seventeen and sixteen-of-seventeen.
  In the same file's `Custom` is a last resort bullet, keep the existing exemption sentence and append that a `Custom` card whose targets could be expressed as a multi-label combination is a finding.
  Apply the identical count changes and the identical appended sentence to `manifest/designs/loom.md`'s mirrored `Do not flag` and `Custom` is a last resort bullets, so the two copies do not drift.
  In `contracts/stencils/loom/loom-rubric-webster-review.md`, rewrite the `Per-card mechanical check` bullet so it is plural — every one of the card's groups' type-specific mechanical checks must have run and passed, each against that group's own targets, not just the first label's — and apply the same rewrite to `manifest/designs/loom.md`'s mirrored `Per-card mechanical check` bullet.
  In `contracts/stencils/webster/webster-body-implementer.md`, rewrite step 2 so "in exactly the targets its type label names" becomes plural over the card's type labels.
  In `contracts/recipes/loom-recipe.yaml`, change the Plan-Review row's `instructions` phrase "re-deriving the other fifteen" to sixteen.
  Preserve every literal substring `contracts/stencils/rubric_test.go` asserts against these rubrics — "is a last resort", "assert-no-callers", "no graded blast radius to summarise", "commit-subject-mismatch", "Dependency edges are derived, never authored", "blast-radius conclusion", "independently reviewable/testable unit", "_lyx/discussion/decision-record.md", and "support-log.md".
- **Commit:** `docs(loom): rubrics and stencils state the multi-label rule`

### Card 12: design docs, sandbox suite and CLI help

- **Context:**
  - `contracts/specs/loom-plan-spec.md`
  - `internal/planparser/validate.go`
- **Edits:**
  - `manifest/designs/plan-card-format.md`
  - `manifest/designs/scout-plan-symbol-fields.md`
  - `tools/sandbox/SANDBOX-WEBSTER-SUITE.md`
  - `internal/webstercli/validate.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In `manifest/designs/plan-card-format.md`, add beneath its own `## Card types` table the same four per-column composition rules card 9 adds to the spec's copy, worded identically in substance so the two copies stay in step.
  Rewrite the `Open, not decided here` bullet closing the `Custom` question so the exemption is described in multi-label terms: `Custom` stays a principled escape hatch exempt from `path-missing` on its own targets and from the `Prosa` target-shape rule, and a card that can name a typed group has by definition found a fit and is therefore not `Custom`.
  Change the third `Open, not decided here` bullet's "sixteen distinct check IDs" to seventeen and note the added `card-custom-not-alone` row.
  In `manifest/designs/scout-plan-symbol-fields.md`, change "existing 16 checks" to 17.
  In `tools/sandbox/SANDBOX-WEBSTER-SUITE.md`, rewrite the plan-shape sentence's "carrying exactly one bold type label from the seven-name set" so it reads one or more bold type labels, each with its own backtick-wrapped target bullets.
  In `internal/webstercli/validate.go`, change the `validate` subcommand's `Long` help text from "the 16 checks" to "the 17 checks" and add `card-custom-not-alone`'s concern to the clause listing "card type presence and retired-label detection" so the help enumerates what actually runs.
  Change nothing else in that file: the command's behaviour and its consumption of the flat `Targets` are unchanged, and this is a help-wording correction the CLI/Cobra Invariant makes a review obligation.
- **Commit:** `docs: restate check count and multi-label rule across design docs and CLI help`

### Card 13: package doc and fixture-comment sweep

- **Context:**
  - `internal/planparser/plan.go`
  - `internal/planparser/validate.go`
  - `internal/planparser/normalize.go`
- **Edits:**
  - `internal/planparser/doc.go`
  - `internal/loomshed/planvalidate_test.go`
  - `internal/loomrecipe/fixture_test.go`
  - `internal/websterengine/runlevel_test.go`
  - `internal/websterengine/sequence_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In `internal/planparser/doc.go`, rewrite the `# Type model` paragraph so a `Card` carries one or more type labels, each contributing a `TargetGroup` holding that label's own refs and, for a `Rename` group, its own pairs and malformed raw bullets, with the flat `Targets`/`Pairs`/`RenameRaw` fields retained as the union across groups for downstream consumers.
  State in that paragraph that `Type` and `TypeLabelCount` are retained for compatibility and are not validation state.
  Change the `# Validation lives in validate.go` paragraph's "16 validation checks" to 17 and add `card-custom-not-alone` to the parenthetical list of example concerns.
  In `internal/loomshed/planvalidate_test.go`, `internal/loomrecipe/fixture_test.go`, and `internal/websterengine/runlevel_test.go`, reword each comment that says a `Create` card's targets stay exempt from on-disk existence checking so it says a `Create` group's targets, since the exemption is now group-scoped;
  change no fixture content and no assertion in those three files.
  In `internal/websterengine/sequence_test.go`, reword the sub-test name that says "a Rename card's Pairs endpoints" so it says a `Rename` group's, changing nothing else.
- **Commit:** `docs(planparser): package doc and fixture comments state group scoping`

## Batch Tests

`verify:` runs a repo-wide build, then a package-scoped test run over every package this batch touches or whose fixtures it re-words, then a second invocation carrying `-tags integration` for `internal/websterengine` because `internal/websterengine/runlevel_test.go` is behind that build tag and would otherwise never compile under this batch's gate.
The second invocation is appended as its own `&&`-chained command rather than comma-joined into a single `-tags` value, so no assumption is made about whether this repo gives its tagged suites mutually exclusive semantics.

Named coverage: `internal/planparser/parse_test.go` (`TestParsePlan_GoldenFixture`, card 8), `internal/planparser/validate_test.go` (`TestValidate_GoldenFixture_ZeroFindings`, card 8), `internal/loomengine/plan_test.go` (`TestPlanSpec_PromptStatesTypeLabelGrammar`, card 10 — the one hard test failure the stencil edit causes, and updating it in the same card is the point), `contracts/stencils/rubric_test.go` (the literal-phrase pins card 11 must preserve), `internal/loomshed/planvalidate_test.go`, `internal/loomrecipe/fixture_test.go`, `internal/websterengine/sequence_test.go` and `internal/websterengine/runlevel_test.go` (card 13's comment-only edits, guarded against accidental fixture damage), and `internal/webstercli` (card 12's help-text edit, guarded by that package's own help-tree tests per the CLI/Cobra Invariant).
The repo-wide `go build ./...` is what proves card 12's `Long` string edit and card 13's comment sweep broke no compile unit outside the tested set.
