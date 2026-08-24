# Batch: planparser tests — golden fixture and per-check suite

```yaml
task: Migrate planparser.Card to Edits/Uses fields
batch: planparser tests — golden fixture and per-check suite
number: 2
cards: 4
verify: go test ./internal/planparser/...
depends-on: [1]
```

## Batch Scope

This batch restores `go test ./internal/planparser/...` to green against batch 1's format-4 model.
It rebuilds the golden fixture under `internal/planparser/testdata/goodplan/` as a seven-card format-4 plan exercising every card type, then rewrites the package's four test files so each of the sixteen validator checks, the shape classifier, the retired-label routing, and the classifier-gated normalization are pinned by a focused test.

It is one batch because all four test files read the same fixture and the same `Card` model, and because splitting the fixture rewrite from the tests that assert against it would leave a batch whose `verify:` cannot pass.

Batch-local decisions beyond `## Shared Decisions`:

- **The fixture is regenerated, not renamed.** The four format-3 card files are deleted and seven format-4 card files are created.
  This is deliberately expressed as `Deletes:` plus `Creates:` rather than `Moves:` pairs: no line of any old card file survives, the card count and the slug set both change, and there is no relocation whose git history is worth preserving — a `Moves:` pair would claim a continuity that does not exist.
- **Dropped checks' tests are deleted, never adapted.** The five `move-*` tests and the `depends-on-order` test go away wholesale.
  Retrofitting a dropped check's test onto a renamed or reworked check produces a test that documents the wrong thing.
- **The fixture proves the exemptions positively.** Two paths the fixture names are deliberately left unmaterialized by the zero-findings test — the `Custom` card's own path-shaped target and the `Rename` pair's post-rename side — so a regression that drops either exemption fails the golden test rather than passing silently.
- **No fuzz test for the classifier.** It is a small pure function fully covered by a table;
  fuzzing it is scope beyond a bounded migration with no observed problem to solve.

## Cards

### Card 8: format-4 golden fixture

- **Context:**
  - `internal/planparser/plan.go`
  - `internal/planparser/parse.go`
  - `internal/planparser/validate.go`
  - `internal/planparser/classify.go`
- **Edits:**
  - `internal/planparser/testdata/goodplan/00-overview.md`
- **Creates:**
  - `internal/planparser/testdata/goodplan/01-json-row-type.md`
  - `internal/planparser/testdata/goodplan/02-json-flag.md`
  - `internal/planparser/testdata/goodplan/03-json-emission.md`
  - `internal/planparser/testdata/goodplan/04-legacy-rows-delete.md`
  - `internal/planparser/testdata/goodplan/05-rowmapper-rename.md`
  - `internal/planparser/testdata/goodplan/06-helppins-move.md`
  - `internal/planparser/testdata/goodplan/07-json-docs.md`
- **Deletes:**
  - `internal/planparser/testdata/goodplan/01-json-flag.md`
  - `internal/planparser/testdata/goodplan/02-json-emission.md`
  - `internal/planparser/testdata/goodplan/03-json-tests.md`
  - `internal/planparser/testdata/goodplan/04-helptree-rename.md`
- **Moves:** none
- **Requirements:** Rebuild the golden fixture as a seven-card format-4 plan that exercises every one of the seven card types exactly once, keeps its `root: internal/boardcli` frontmatter and a `//`-escaped entry so the root-resolution rule stays on the golden path, and stays in parity with the worked example the rewritten spec carries in batch 4.
  In the overview: set `format: 4`, keep `approved: true` and `root: internal/boardcli`, keep the existing task-framing paragraph verbatim, keep the `## Shared Decisions` section's `json-envelope-reuse` entry verbatim, and keep the `## verify:` section's single command line verbatim.
  Rewrite the `## Rename mechanic` section so its four numbered steps describe format 4 rather than format 3: step 3 currently tells the author to use a `Creates:` field, which no longer exists, and must instead say that a genuinely new file with no predecessor belongs in a separate `Create` card rather than in the `Rename` pair.
  Keep steps 1, 2, and 4 intact in meaning.
  Replace the Card Index with seven entries in this order, each still reading `N — <card-slug> — <one-line summary>`: `json-row-type`, `json-flag`, `json-emission`, `legacy-rows-delete`, `rowmapper-rename`, `helppins-move`, `json-docs`.
  Write the seven card files so that each carries exactly one type label, an `**Intent:**` field, an `**ImpactSummary:**` field on the `Edit` and `Delete` cards only, and no format-3 label anywhere.
  Card 1 is `Create`, its single target the symbol `boardcli.RowJSON`, and it carries the plan's only `**Commit:**` (pinned as `1: json-row-type`) and its only `**Verify:**` line, so both optional fields stay covered.
  Card 2 is `Edit` with a mixed target list — the symbol `boardcli.newListCmd` and the root-relative path `list.go` — plus a `**Uses:**` list holding the `//`-escaped `//internal/output/envelope.go`.
  Card 3 is `Custom` with a mixed target list — the symbol `boardcli.emitJSON` and the `//`-escaped `//internal/output/emit.go` — plus a `**Uses:**` list holding the root-relative `list.go`.
  Card 4 is `Delete`, its single target the `//`-escaped `//internal/boardengine/legacyrows.go`.
  Card 5 is `Rename` with two well-formed pairs on the ASCII arrow grammar — a symbol pair `boardengine.MapRow` to `boardengine.MapRowJSON`, and a path pair `//internal/boardengine/rows.go` to `//internal/boardengine/rowsjson.go` — and deliberately carries no `**Uses:**` label at all, so the optional-field-omitted path is covered.
  Card 6 is `Move`, its single target the `//`-escaped `//cmd/lyx/helppins.go`, with its destination stated in `**Intent:**` prose rather than in the target list, per the `Move` type's own rule.
  Card 7 is `Prosa` with two path-shaped targets and no symbol among them — the root-relative `doc.go` and the `//`-escaped `//docs/boardcli-json.md`.
  Every card file's heading stays `# Card N — <slug>` so `card-numbering` keeps passing, and the file prefix `NN` matches.
  The fixture must produce zero findings from `Validate` once card 11's zero-findings test materializes the six paths it names, and the two paths it deliberately leaves absent must be exactly the `Custom` card's own path-shaped target and the `Rename` pair's post-rename side.
- **Commit:** `test(planparser): rebuild the golden fixture as a seven-type format-4 plan`

### Card 9: parse tests

- **Context:**
  - `internal/planparser/plan.go`
  - `internal/planparser/parse.go`
  - `internal/planparser/classify.go`
  - `internal/planparser/testdata/goodplan/00-overview.md`
  - `internal/planparser/testdata/goodplan/01-json-row-type.md`
  - `internal/planparser/testdata/goodplan/02-json-flag.md`
  - `internal/planparser/testdata/goodplan/03-json-emission.md`
  - `internal/planparser/testdata/goodplan/04-legacy-rows-delete.md`
  - `internal/planparser/testdata/goodplan/05-rowmapper-rename.md`
  - `internal/planparser/testdata/goodplan/06-helppins-move.md`
  - `internal/planparser/testdata/goodplan/07-json-docs.md`
- **Edits:**
  - `internal/planparser/parse_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Rewrite `parse_test.go` against the format-4 grammar, keeping `writePlanFiles` and the file's existing table-driven style.
  Update `minimalOverview` to `format: 4` and rewrite `minimalCardFile` to emit a format-4 card body — one type label with a bullet list plus an `**Intent:**` line, and no `none` sentinel anywhere.
  Keep `TestParsePlan_Overview`, `TestParsePlan_Overview_ASCIIDashSeparators`, `TestParsePlan_Overview_Errors`, `TestParsePlan_Overview_MissingFormatOrApprovedIsNotFailLoud`, `TestParsePlan_CardFile_NotFound`, `TestParsePlan_CardHeading`, and `TestParsePlan_Card_SourcePath` as behavioral tests, adjusting only what the new grammar and the `Intent`-to-`Summary` field rename force.
  Delete `TestParsePlan_Card_FiveFieldsNoneSentinel` and `TestParsePlan_Card_DependsOnMalformed` outright — both pin behavior format 4 removes.
  Rename `TestParsePlan_Card_MovesGrammar` to a `Rename`-pair test asserting that a well-formed pair reaches `Pairs`, that a malformed sub-bullet reaches `RenameRaw`, and that both endpoints of every pair are projected into `Targets` in pair order, `Old` before `New`.
  Keep `TestParsePlan_InlineFieldValueFailsLoud` and retarget it: an inline value on a bullet-only field is still a `ParsePlan` error rather than a finding, for both a type label and `**Uses:**`.
  Retarget `TestParsePlan_CardCommitAndVerify` to the recapitalized `**Verify:**` label and assert `HasVerify`.
  Add new tests covering:
  a card carrying two recognized type labels and a card carrying none, asserting `TypeLabelCount` and `Type` in both cases;
  a label present with no bullets under it, asserting the field is a non-nil zero-length slice rather than nil, so the absent-versus-empty distinction survives;
  a multi-line `**ImpactSummary:**`, asserting the inline remainder lands in `ImpactSummary` and every following non-label line lands in `ImpactSummaryTrailing`;
  a half-migrated card carrying a retired `**Context:**` label, asserting the label text reaches `RetiredLabels` and that its presence terminated the preceding `**Intent:**` prose collection rather than being swallowed into it;
  a second half-migrated card carrying the format-3 lowercase `**verify:**` label, asserting the same two things and specifically pinning that the case-sensitive match routes it to `RetiredLabels` rather than letting it fall through as unrecognized text or be mistaken for the format-4 `**Verify:**` field.
  Rewrite `TestParsePlan_GoldenFixture` as a full round-trip over the new seven-card fixture, asserting every field of every card: `Number`, `Slug`, `Title`, `Summary`, `Type`, `TypeLabelCount`, `Targets`, `Pairs`, `Uses`, `Intent`, `ImpactSummary`, `Commit`, and `Verify`.
  It must assert that `plan.Format` is 4 and that a symbol target survives normalization unmodified under the fixture's non-empty `root:` — that single assertion is the sharpest regression this migration can introduce and must be present.
  Update the file's leading file-purpose comment to describe format-4 coverage.
- **Commit:** `test(planparser): rewrite parse tests for the format-4 card grammar`

### Card 10: normalization and plan-section tests

- **Context:**
  - `internal/planparser/normalize.go`
  - `internal/planparser/classify.go`
  - `internal/planparser/plan.go`
  - `internal/planparser/sections.go`
  - `internal/planparser/testdata/goodplan/00-overview.md`
- **Edits:**
  - `internal/planparser/normalize_test.go`
  - `internal/planparser/sections_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `normalize_test.go`, keep `TestNormalizeCardPath` exactly as it is — the three-case resolution rule is unchanged.
  Replace `TestNormalizeCard_MovesBothEndpoints` with a test over `Pairs` asserting both endpoints of every pair are normalized, and replace `TestNormalizeCard_NilSliceStaysNil` with the same nil-preservation assertion over `Targets` and `Uses`.
  Add a test asserting the classifier gate: under a non-empty root, a symbol entry in `Targets` and a symbol entry in `Uses` each pass through `normalizeCard` byte-identical, while a path entry in the same list is root-joined.
  Add a test asserting that a `Rename` card's `Pairs` and its projected `Targets` normalize to the same strings on both endpoints, so the two representations cannot drift.
  In `sections_test.go`, keep both tests and update `TestParsePlan_GoldenFixture_PlanLevelSections` to expect the fixture's rewritten `## Rename mechanic` body text and its unchanged `## Shared Decisions` and `## verify:` bodies.
  Update both files' leading file-purpose comments where the format-3 wording no longer holds.
- **Commit:** `test(planparser): cover classifier-gated normalization and the format-4 fixture sections`

### Card 11: validator tests

- **Context:**
  - `internal/planparser/validate.go`
  - `internal/planparser/plan.go`
  - `internal/planparser/classify.go`
  - `internal/planparser/parse.go`
  - `internal/planparser/testdata/goodplan/00-overview.md`
  - `internal/planparser/testdata/goodplan/01-json-row-type.md`
  - `internal/planparser/testdata/goodplan/02-json-flag.md`
  - `internal/planparser/testdata/goodplan/03-json-emission.md`
  - `internal/planparser/testdata/goodplan/04-legacy-rows-delete.md`
  - `internal/planparser/testdata/goodplan/05-rowmapper-rename.md`
  - `internal/planparser/testdata/goodplan/06-helppins-move.md`
  - `internal/planparser/testdata/goodplan/07-json-docs.md`
- **Edits:**
  - `internal/planparser/validate_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Rewrite `validate_test.go` so every one of the sixteen checks has its own focused test with at least one triggering case and one clean case, keeping the file's existing helpers `countFor` and `materializeFiles` and its "assert on `Check` names and cardinality, never on `Detail` text" discipline.
  Rewrite `validCard` to return a fully well-formed format-4 card — `Type` set to `CardTypeEdit`, `TypeLabelCount` 1, `HasType` true, a single path-shaped `Targets` entry, `HasUses` true with an empty non-nil `Uses`, `HasIntent` true with non-empty `Intent`, `HasImpactSummary` true with a non-empty one-line `ImpactSummary`, no `RetiredLabels`, no `RenameRaw`, and a correctly prefixed `Commit`.
  Each subtest starts from that baseline and mutates exactly the one field its own check cares about, so an observed finding can only come from the check under test.
  Delete `TestValidate_MoveFormat`, `TestValidate_MoveRedundant`, `TestValidate_MoveMechanicMissing`, `TestValidate_MoveSourceMissing`, `TestValidate_MoveTargetCollision`, and `TestValidate_DependsOnOrder` — the first is replaced by a new `rename-format` test and the third by a new `rename-mechanic-missing` test, and the other four pin checks that no longer exist.
  Keep and retarget `TestValidate_FormatAndApproval` (asserting `recognizedFormat` is now 4 and that a `format: 3` plan produces a `format-unrecognized` finding), `TestValidate_IndexFileMismatch`, `TestValidate_CardPathMalformed`, `TestValidate_CardMissingField`, `TestValidate_CardFieldOverlap`, `TestValidate_CardNumbering`, `TestValidate_PathMissing`, and `TestValidate_CommitSubjectMismatch`.
  `TestValidate_CardPathMalformed` must assert that a malformed symbol-shaped entry produces no finding while a malformed path-shaped entry in the same list does — the check now applies to path-shaped entries only.
  `TestValidate_CardMissingField` must assert that a card with no `Intent` produces one finding, that an `Edit` card and a `Delete` card each need `ImpactSummary`, and that a `Create`, `Rename`, `Move`, `Prosa`, or `Custom` card without `ImpactSummary` produces none.
  `TestValidate_CardFieldOverlap` must assert the reworked meaning: an entry present in both a card's `Targets` and its own `Uses`.
  Add new tests for `card-type-missing` (zero type labels and two type labels each producing one finding, exactly one producing none), `card-retired-label` (one finding per `RetiredLabels` entry), `rename-format` (one finding per `RenameRaw` entry), `rename-mechanic-missing` (a `Rename` card with an empty `Plan.RenameMechanic` produces one plan-level finding, and a plan whose only cards are other types produces none even with an empty section), `card-field-empty` (a present label with zero-length content on each of the four applicable fields), `impact-summary-multiline` (a non-empty `ImpactSummaryTrailing`), and `prosa-symbol-target` (a symbol-shaped target on a `Prosa` card, with a path-only `Prosa` card producing none).
  Rewrite `TestValidate_PathMissing` to pin the type-conditional rework exhaustively, using a hermetic `t.TempDir()` worktree root: an `Edit`, `Delete`, `Move`, or `Prosa` card's path-shaped target that is absent produces a finding;
  a `Create` card's path-shaped target that is absent produces none;
  a `Rename` pair's absent `Old` side produces a finding while its absent `New` side produces none;
  a `Custom` card's absent path-shaped target produces none while that same card's absent `Uses` path does produce one;
  and an otherwise-missing path is satisfied by membership in either union — a `Create` card's path-shaped target, or a `Rename` pair's `New` side.
  Add a test asserting that a `Custom` card remains bound by the card-generic checks, so a blanket-skip regression fails: give a `Custom` card a malformed path-shaped target, no `Intent`, an entry in both `Targets` and `Uses`, and a badly prefixed `Commit`, then assert one finding each from `card-path-malformed`, `card-missing-field`, `card-field-overlap`, and `commit-subject-mismatch`.
  Rewrite `TestValidate_GoldenFixture_ZeroFindings` to parse the seven-card fixture and materialize under a `t.TempDir()` worktree root exactly the six paths the fixture's checked entries name, deliberately leaving absent the `Custom` card's own path-shaped target and the `Rename` pair's post-rename side, then assert zero findings across the whole sixteen-check run.
  Update the file's leading file-purpose comment to say sixteen checks and to name the two deliberately-absent paths and why.
- **Commit:** `test(planparser): cover the format-4 sixteen-check validator set`

## Batch Tests

`verify: go test ./internal/planparser/...` is scoped to exactly the package this batch touches.
It covers `parse_test.go`, `validate_test.go`, `normalize_test.go`, `sections_test.go`, `planpath_test.go` (untouched, and a useful regression signal that the path constructors survived), and card 1's `classify_test.go` from batch 1, which runs green here for the first time alongside the rest of the package.

The golden fixture under `internal/planparser/testdata/goodplan/` is exercised by three of those files at once — `parse_test.go`'s round-trip, `sections_test.go`'s plan-level sections, and `validate_test.go`'s zero-findings run — so a fixture defect surfaces from three directions rather than one.

Packages outside `internal/planparser` still fail to compile their test binaries after this batch;
batch 3 is what restores them, and `pipeline.done_gate` is the whole-tree backstop at the end.
