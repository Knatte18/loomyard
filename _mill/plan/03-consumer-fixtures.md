# Batch: consumer fixtures — websterengine, loomshed, loomrecipe, loomcli, webstercli

```yaml
task: Migrate planparser.Card to Edits/Uses fields
batch: consumer fixtures — websterengine, loomshed, loomrecipe, loomcli, webstercli
number: 3
cards: 5
verify: go test -tags integration ./internal/websterengine/... ./internal/loomshed/... ./internal/loomrecipe/... ./internal/loomcli/... ./internal/webstercli/... ./internal/batcher/...
depends-on: [2]
```

## Batch Scope

This batch restores test compilation and green tests in the five packages outside `internal/planparser` that carry a format-3 card fixture or construct a `planparser.Card` in Go.
None of these are new coverage — each fixture must simply stop describing a format the parser now rejects, and each Go-level `Card` construction must stop naming a field that no longer exists.

Two of the five carriers are Go-model carriers the markdown sweep does not find: `internal/websterengine/template_test.go` builds a `planparser.Card` directly and assigns `card.Moves`, and `internal/loomengine/plan_test.go` (handled in batch 4, since its coupling is to stencil prose rather than to the card model) contains no card markdown either.
The compiler is the authority on this set;
the greps in `_mill/discussion.md` exist only to see the carriers ahead of time.

`internal/batcher` needs no change at all — `Batch{Cards []planparser.Card}` and the identity batcher pass cards through without reading a field — and it is in this batch's `verify:` scope purely as proof of that claim.
`internal/loomshed/planvalidate.go` likewise reads no field;
only its test fixture moves.

Batch-local decisions beyond `## Shared Decisions`:

- **Fixtures convert to the smallest legal format-4 card.** Every placeholder card becomes a single type label with one bullet plus an `**Intent:**` line, choosing the type that keeps each fixture's existing intent — a fixture whose card declared a `Creates:` target becomes a `Create` card, so its target stays exempt from `path-missing` exactly as `Creates:` was.
- **The Gate Self-Check Parity Invariant is proven, not assumed.** `internal/loomcli/parity_test.go` is deliberately left untouched and is in this batch's `verify:` scope: if `TestGateParity_PlanValidate` breaks, something re-implemented a check instead of calling `planparser.Validate` through, and that is a finding rather than a test to fix.

## Cards

### Card 12: websterengine template tests

- **Context:**
  - `internal/planparser/plan.go`
  - `internal/websterengine/render.go`
- **Edits:**
  - `internal/websterengine/template_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Update the file's two couplings to the old `Card` model.
  In `cardWithSourcePath`, rename the `intent` parameter and the struct field it sets to `Summary`, matching the `Card.Intent`-to-`Card.Summary` rename, and update every call site in the file.
  In the test that asserts the rendered fork prompt carries the `## Rename mechanic` section regardless of whether the batch has a rename-bearing card, replace the `card.Moves = []planparser.MovePair{...}` assignment with a format-4 `Rename` card: set `card.Type` to `planparser.CardTypeRename`, set `card.Pairs` to the same single pair, and set `card.Targets` to that pair's two endpoints in pair order.
  Keep the test's own assertion unchanged — the rendered output must still carry the section either way.
  Reword that same test's own doc comment while its body is being touched: it says the section renders regardless of whether the batch has a "Moves-bearing" card, and `Moves` is a field this migration removes, so it must say "rename-bearing" instead.
  Leave every other test in the file untouched.
- **Commit:** `test(websterengine): build format-4 cards in the template tests`

### Card 13: websterengine runlevel fixtures

- **Context:**
  - `internal/planparser/plan.go`
  - `internal/planparser/parse.go`
  - `internal/planparser/validate.go`
- **Edits:**
  - `internal/websterengine/runlevel_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Update the plan-fixture builder that writes `numCards` placeholder card files plus an overview.
  Change the overview frontmatter to `format: 4`.
  Rewrite each card body from the seven format-3 labels to a format-4 `Create` card: a `**Create:**` label with the existing per-card new-file path as its single bullet, followed by an `**Intent:**` line carrying the existing placeholder prose.
  `Create` is the right type because the fixture's cards each declared a `Creates:` target, and a `Create` card's targets stay exempt from `path-missing` exactly as `Creates:` entries were — so the fixture keeps validating cleanly without materializing anything on disk.
  Leave every assertion in the file unchanged: this fixture feeds run-level orchestration tests that assert on card counts and ordering, not on card fields.
- **Commit:** `test(websterengine): move the runlevel plan fixtures to format 4`

### Card 14: loomshed and loomrecipe fixtures

- **Context:**
  - `internal/planparser/plan.go`
  - `internal/planparser/validate.go`
  - `internal/loomshed/planvalidate.go`
- **Edits:**
  - `internal/loomshed/planvalidate_test.go`
  - `internal/loomrecipe/fixture_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Both files carry a byte-identical plan-fixture helper writing one overview plus one placeholder card;
  convert both the same way.
  Change each overview's frontmatter to `format: 4`.
  Rewrite each `cardBody` from the seven format-3 labels to a format-4 `Create` card whose single `**Create:**` bullet is the same new-file path the old `Creates:` field named, followed by an `**Intent:**` line carrying the existing placeholder prose.
  Update each helper's own doc comment where it says the card carries a `Creates:` entry, so it names the `Create` type label instead while keeping the stated reason — the target is exempt from on-disk existence checking, which is what lets the fixture validate cleanly without materializing files.
  Leave every assertion in both files unchanged: both exercise the approved-versus-unapproved gate path, which is `plan-unapproved`'s concern rather than any card field's.
- **Commit:** `test(loomshed,loomrecipe): move the plan fixtures to format 4`

### Card 15: loomcli validate-plan fixture

- **Context:**
  - `internal/planparser/plan.go`
  - `internal/planparser/validate.go`
  - `internal/loomcli/parity_test.go`
- **Edits:**
  - `internal/loomcli/validate_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Convert `planFixture`'s plan to format 4.
  Change the overview frontmatter to `format: 4`, keeping its `approved:` parameterization and its empty `root:` key exactly as they are — `approved` is what the file's own unapproved-path test trips.
  Rewrite the card from the seven format-3 labels plus `**Commit:**` and the lowercase `**verify:**` into a format-4 `Create` card: a `**Create:**` label whose single bullet is the same fixture output path the old `Creates:` field named, an `**Intent:**` line carrying the existing prose, and the same pinned `**Commit:**` value.
  Drop the trailing `**verify:** none` line rather than recapitalizing it — format 4 admits no `none` sentinel on any field, and a `**Verify:**` label carrying the literal word `none` would be stored verbatim as a card verify command, which is worse than omitting an optional field.
  Update the helper's doc comment where it describes the card's fields.
  Leave `internal/loomcli/parity_test.go` untouched: `TestGateParity_PlanValidate` must keep passing unchanged, because the parity it asserts is structural — both the `Plan-Validate` `ShedProducer` row and the `validate-plan` verb still call `planparser.Validate` — and a break there would mean a check was re-implemented rather than called through, per the Gate Self-Check Parity Invariant.
- **Commit:** `test(loomcli): move the validate-plan fixture to format 4`

### Card 16: webstercli validate fixtures

- **Context:**
  - `internal/planparser/plan.go`
  - `internal/planparser/validate.go`
  - `internal/webstercli/validate.go`
- **Edits:**
  - `internal/webstercli/cli_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Convert both plan-seeding helpers to format 4.
  In the valid-plan seeder, change the overview to `format: 4` and rewrite the card as a format-4 `Create` card whose single `**Create:**` bullet is the same new-file path the old `Creates:` field named, followed by an `**Intent:**` line.
  In the missing-field seeder, which today produces a `card-missing-field` finding by omitting the `**Deletes:**` label, retarget it to format 4's own required set: keep the `**Create:**` label and its bullet and omit the `**Intent:**` label, so the card still produces exactly one `card-missing-field` finding — `Intent:` is now the field every card must carry.
  Rename that helper and update its doc comment so both name the omitted `**Intent:**` label rather than `**Deletes:**`.
  Leave the assertions in the tests these helpers feed unchanged: they assert the exit code, the `valid`/`cards` envelope keys, and that findings are keyed by the flat card key, none of which the format change alters.
- **Commit:** `test(webstercli): move the validate fixtures to format 4`

## Batch Tests

`verify:` names the six packages this batch's cards touch or prove: `internal/websterengine`, `internal/loomshed`, `internal/loomrecipe`, `internal/loomcli`, `internal/webstercli`, and `internal/batcher`.
It is a per-package list rather than the unbounded `go test ./...` because the batch's edits are confined to those trees.

`internal/batcher` carries no card in this batch and is listed on purpose: the discussion's claim that the identity batcher passes `[]planparser.Card` through without reading a field is only worth stating if a test run proves it.

`internal/loomcli` is in scope for two reasons — card 15's own fixture, and `parity_test.go`'s `TestGateParity_PlanValidate`, which this batch deliberately leaves untouched so that a break there reads as a design regression rather than a fixture to update.

After this batch, `go test ./...` is green everywhere except for whatever batch 4's stencil rewrite touches in `internal/loomengine`;
`pipeline.done_gate` runs the whole-tree gate, unit and integration, before the task is marked done.
