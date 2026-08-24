# Batch: planparser core — format-4 model, classifier, parser, validator

```yaml
task: Migrate planparser.Card to Edits/Uses fields
batch: planparser core — format-4 model, classifier, parser, validator
number: 1
cards: 7
verify: go build ./...
depends-on: []
```

## Batch Scope

This batch replaces `internal/planparser`'s file-op card model with the symbol-level format-4 model from `manifest/designs/plan-card-format.md`, and fixes the single non-test consumer read that the model change breaks.
It is one batch because the change is compile-atomic: `Card.ContextFiles`/`EditsFiles`/`CreatesFiles`/`DeletesFiles`/`Moves`/`MovesRaw`/`DependsOn`/`HasWhat` disappear and `Card.Intent` is renamed `Card.Summary` in the same commit range that `parse.go`, `normalize.go`, `validate.go`, and `internal/websterengine/render.go` must already be reading the new shape.
No ordering of these edits keeps `go build ./...` green partway through, so the batch's `verify:` is `go build ./...` and the test suites are restored by batches 2 and 3.

The external interface batches 2–4 consume is the finished `Card` model: a `CardType` enum, flat `Targets`/`Uses` ref lists, `Pairs`/`RenameRaw` for `Rename`, `Intent`/`ImpactSummary`/`ImpactSummaryTrailing`, `RetiredLabels`, the per-label `HasX` presence bits, and `Validate`'s 16-ID check set at `recognizedFormat = 4`.

Batch-local decisions beyond `## Shared Decisions`:

- **Unexported classifier.** `classifyRef`/`isPathRef` stay unexported;
  nothing outside `internal/planparser` classifies a ref, and exporting them would invite a consumer to re-derive plan grammar in violation of the Planparser Sole-Parser Invariant.
- **`Rename` projects both endpoints into `Targets`.** `Pairs` is the structured form for the rename mechanic and `rename-format`;
  `Targets` is what every target-list-based check reads, so `Rename` is never a hole in the model.
- **Ground-truth classification is declined on purpose.** The design doc's "resolvable against ground truth (`go doc` for a symbol, file existence for a path)" clause is taken only in its shape half.
  A `go doc` call is a process spawn, barred from tier1 by the Test Tier Purity Invariant;
  the file-existence half survives as the `path-missing` check, which already stats the disk at validation time.

## Cards

### Card 1: shape classifier

- **Context:**
  - `internal/planparser/normalize.go`
  - `internal/planparser/doc.go`
  - `internal/planparser/normalize_test.go`
  - `CONSTRAINTS.md`
- **Edits:** none
- **Creates:**
  - `internal/planparser/classify.go`
  - `internal/planparser/classify_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add a new file declaring `package planparser` with a `refKind` type (values `refKindPath` and `refKindSymbol`), a `classifyRef(raw string) refKind` function, and an `isPathRef(raw string) bool` convenience wrapper returning `classifyRef(raw) == refKindPath`.
  `classifyRef` applies exactly three rules in this order:
  (1) `strings.Contains(raw, "/")` is true → `refKindPath` (this covers the `//` worktree-root escape, since it contains a slash);
  (2) otherwise, take the segment after the final `.` — if the entry contains a `.` and that final segment is non-empty and consists entirely of lowercase ASCII letters and ASCII digits → `refKindPath`;
  (3) otherwise → `refKindSymbol`.
  Rule 3 is the explicit default for two distinct cases and both must be reachable: an entry with no `.` at all never reaches rule 2's test and falls to rule 3 (`Lookup`, `Makefile`), and an entry whose final dot-segment is not all-lowercase falls to rule 3 (`shedrecipe.Lookup`).
  Implement the lowercase-alphanumeric test by hand over the segment's bytes rather than by calling `strings.ToLower` and comparing, so a non-ASCII rune can never be treated as lowercase.
  The function performs string analysis only: do not call `os.Stat`, do not spawn a process, and do not import `internal/lyxcwd` in this file.
  Write the accompanying table test in `package planparser` (matching `normalize_test.go`'s internal-test convention, not the external `planparser_test` one) covering at minimum `internal/boardcli/list.go`, bare `list.go`, `//cmd/lyx/main.go`, `shedrecipe.Lookup`, bare `Lookup`, bare `Makefile`, and the documented `shedrecipe.lookup` misclassification-as-path, each with the expected `refKind` pinned.
  Give the file a leading file-purpose comment in the style the package's other files use.
- **Commit:** `feat(planparser): add the shape classifier for symbol-vs-path card refs`

### Card 2: format-4 Card and Plan model

- **Context:**
  - `internal/planparser/parse.go`
  - `internal/planparser/validate.go`
  - `manifest/designs/plan-card-format.md`
- **Edits:**
  - `internal/planparser/plan.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Rewrite the `Card` struct and add a `CardType` type.
  Declare `type CardType string` with the constants `CardTypeUnknown` (`""`), `CardTypeCreate` (`"Create"`), `CardTypeEdit` (`"Edit"`), `CardTypeDelete` (`"Delete"`), `CardTypeRename` (`"Rename"`), `CardTypeMove` (`"Move"`), `CardTypeProsa` (`"Prosa"`), and `CardTypeCustom` (`"Custom"`).
  Keep `MovePair` exactly as it is — it is reused as the `Rename` pair type, not deleted.
  Keep `Plan` unchanged in shape, including `RenameMechanic` and `Verify`;
  update only the doc comment on `Format` to say the recognized version is now 4.
  Keep these `Card` fields unchanged: `Number`, `Slug`, `Title`, `SourcePath`, `Commit`, `Verify`.
  Rename `Card.Intent` (the Card Index one-line summary) to `Card.Summary`, freeing the name `Intent` for the new body-level field.
  Remove these fields entirely: `What`, `HasWhat`, `ContextFiles`, `EditsFiles`, `CreatesFiles`, `DeletesFiles`, `Moves`, `MovesRaw`, `HasContext`, `HasEdits`, `HasCreates`, `HasDeletes`, `HasMoves`, `HasDependsOn`, `DependsOn`.
  Add these fields:
  `Type CardType` (the first recognized type label seen, `CardTypeUnknown` when none);
  `TypeLabelCount int` (how many recognized type labels the card body carried, so a two-label card is expressible);
  `HasType bool` (`TypeLabelCount > 0`);
  `Targets []string` (the flat target ref list, symbols and paths mixed, and for a `Rename` card both endpoints of every pair projected in pair order — `Old` then `New`);
  `Pairs []MovePair` (populated for a `Rename` card only);
  `RenameRaw []string` (a `Rename` sub-bullet that failed the pair grammar);
  `Uses []string` and `HasUses bool`;
  `Intent string` and `HasIntent bool` (the multi-line body prose);
  `ImpactSummary string` and `HasImpactSummary bool` (the inline one-line value);
  `ImpactSummaryTrailing []string` (non-label lines following the `ImpactSummary:` label line, captured rather than discarded so `impact-summary-multiline` has something to report);
  `HasVerify bool`;
  `RetiredLabels []string` (one entry per format-3 label occurrence, holding the label's own literal text such as `**Context:**`).
  Give every field a doc comment in the package's existing style, and rewrite the file's leading file-purpose comment so it describes the type-label model rather than the five typed file-op fields.
- **Commit:** `feat(planparser): replace the file-op Card model with the format-4 type/target model`

### Card 3: format-4 card body parser

- **Context:**
  - `internal/planparser/plan.go`
  - `internal/planparser/classify.go`
  - `manifest/designs/plan-card-format.md`
- **Edits:**
  - `internal/planparser/parse.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Rewrite the card-body half of the parser;
  leave `ParsePlan`, `parseOverviewFrontmatter`, `splitFrontmatter`, `splitFraming`, `parseCardIndex`, `normalizeWhitespace`, `cardFileName`, `cardHeadingRe`, `PlanDirName`, `PlanDirRel`, `PlanDir`, and `PlanOverview` unchanged except where a renamed field forces a one-line edit.
  In `cardIndexEntry`, rename the `Intent` field to `Summary`, and in `parseCardFile` seed `card.Summary` from it.
  Replace the label-constant block with three groups.
  Type labels: `createLabel = "**Create:**"`, `editLabel = "**Edit:**"`, `deleteLabel = "**Delete:**"`, `renameLabel = "**Rename:**"`, `moveLabel = "**Move:**"`, `prosaLabel = "**Prosa:**"`, `customLabel = "**Custom:**"`.
  Field labels: `usesLabel = "**Uses:**"`, `intentLabel = "**Intent:**"`, `impactSummaryLabel = "**ImpactSummary:**"`, `commitLabel = "**Commit:**"`, `cardVerifyLabel = "**Verify:**"` (note the new capital V).
  Retired labels, all eight kept as constants: `whatLabel`, `contextLabel`, `editsLabel`, `createsLabel`, `deletesLabel`, `movesLabel`, `dependsOnLabel`, and `legacyVerifyLabel = "**verify:**"` (the format-3 lowercase spelling, matched case-sensitively and therefore distinct from `cardVerifyLabel`).
  Add a package-level `map[string]CardType` keyed by the seven type labels so the switch and the type resolution share one table.
  `cardLabels` must list all twenty labels — the seven type labels, the five field labels, and the eight retired labels — so `isCardLabelLine` still terminates `**Intent:**`'s prose collection on any of them.
  Do not remove a retired label from `cardLabels`.
  Delete `noneSentinel`, `parseDependsOnField`, and `dependsOnSplitRe`.
  Keep `moveLineRe`, `stripBackticks`, and `isCardLabelLine` as they are.
  Rename `parseFileOpField` to `parseRefField` and delete its `none` branch;
  keep its inline-value fail-loud error and reword the message to drop the `none` alternative, so an inline value on a bullet-only field is still a `ParsePlan` error rather than a finding.
  Make `parseRefField` return a non-nil empty slice when the label is present with zero bullets under it, so a label written with no bullets is distinguishable from an absent label and reaches the `card-field-empty` check.
  Rename `parseMovesField` to `parseRenameField`, taking the `Rename` label line, deleting its `none` branch, keeping its inline-value error, and returning `pairs`, `raw`, and the next index exactly as before.
  Rewrite `parseCardBody`'s switch:
  a type-label case increments `card.TypeLabelCount`, sets `card.HasType`, sets `card.Type` from the shared label-to-type table when `card.Type` is still `CardTypeUnknown` (first label wins), and then collects its bullets — for `**Rename:**` via `parseRenameField`, appending the returned pairs to `card.Pairs`, the raw entries to `card.RenameRaw`, and both endpoints of each pair to `card.Targets` in pair order, and for every other type label via `parseRefField`, appending the returned refs to `card.Targets`;
  a `**Uses:**` case sets `card.HasUses` and fills `card.Uses` via `parseRefField`;
  an `**Intent:**` case sets `card.HasIntent` and collects prose exactly as the old `**What:**` case did — the label line's own remainder plus every following line up to the next card label line, joined with newlines and trimmed;
  an `**ImpactSummary:**` case sets `card.HasImpactSummary`, takes the label line's own trimmed remainder as `card.ImpactSummary`, and then appends every following non-blank, non-label line to `card.ImpactSummaryTrailing` until the next card label line;
  a `**Commit:**` case and a `**Verify:**` case behave as their format-3 counterparts, with the `**Verify:**` case also setting `card.HasVerify`;
  each of the eight retired labels appends its own literal label constant to `card.RetiredLabels` and then consumes the label line plus every following line up to the next card label line, storing none of that content, so a retired label never fails the parse and never leaks into another field.
  Because no label in the set is a prefix of another (`**Edit:**` and `**Edits:**` differ at their seventh byte, and the same holds for `**Create:**`/`**Creates:**`, `**Delete:**`/`**Deletes:**`, and `**Move:**`/`**Moves:**`), the switch's case order carries no semantics — state that in a comment so a later edit does not reorder it into a trap.
  Rewrite the file's leading file-purpose comment to describe the format-4 grammar.
- **Commit:** `feat(planparser): parse format-4 type labels, Uses, Intent, ImpactSummary, and retired labels`

### Card 4: classifier-gated path normalization

- **Context:**
  - `internal/planparser/classify.go`
  - `internal/planparser/plan.go`
- **Edits:**
  - `internal/planparser/normalize.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Keep `normalizeCardPath`, `hasWorktreeRootEscape`, and `cleanPosixPath` byte-for-byte as they are.
  Rewrite `normalizeCard` to normalize `card.Targets`, `card.Uses`, and both endpoints of every entry in `card.Pairs`, and to apply `normalizeCardPath` only to entries `isPathRef` classifies as paths — a symbol entry passes through verbatim.
  Rename `normalizePathSlice` to `normalizeRefSlice`, keeping its in-place, nil-vs-empty-preserving shape, and add the `isPathRef` gate inside it.
  This gate is the single sharpest regression this migration can introduce: without it a non-empty `root:` turns `shedrecipe.Lookup` into `internal/boardcli/shedrecipe.Lookup`.
  Because `Targets` already carries both endpoints of every `Rename` pair, normalizing `Pairs` and `Targets` independently yields the same result on both sides;
  keep both normalizations rather than deriving one from the other.
  Update the file's leading file-purpose comment to say that normalization is classifier-gated.
- **Commit:** `feat(planparser): gate card-path normalization on the shape classifier`

### Card 5: the sixteen-check validator

- **Context:**
  - `internal/planparser/plan.go`
  - `internal/planparser/classify.go`
  - `internal/planparser/parse.go`
  - `internal/planparser/normalize.go`
- **Edits:**
  - `internal/planparser/validate.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Set `recognizedFormat` to `4`.
  Keep `ValidationError`, its `Error` method, `cardID`, `pathExistsOnDisk`, `cardPathMalformedReason`, and `cardHeadingNumber` unchanged.
  Delete `checkMoveRedundant`, `checkMoveSourceMissing`, `checkMoveTargetCollision`, and `checkDependsOnOrder` outright.
  Rename `createsUnion` to `createTargetsUnion`, redefined as the union across the plan of every `CardTypeCreate` card's path-shaped `Targets` entries;
  rename `movesTargetsUnion` to `renameTargetsUnion`, redefined as the union across the plan of every `Pairs` entry's `New` side that is path-shaped.
  Both helpers survive because `checkPathMissing` still consumes both.
  Rewrite `Validate`'s call sequence to exactly this fixed order, emitting exactly these sixteen distinct `Check:` IDs:
  `checkFormatAndApproval` (`format-unrecognized`, `plan-unapproved`),
  `checkIndexFileConsistency` (`index-file-mismatch`),
  `checkCardTypeMissing` (`card-type-missing`),
  `checkCardRetiredLabel` (`card-retired-label`),
  `checkCardPathMalformed` (`card-path-malformed`),
  `checkRenameFormat` (`rename-format`),
  `checkRenameMechanicMissing` (`rename-mechanic-missing`),
  `checkCardMissingField` (`card-missing-field`),
  `checkCardFieldEmpty` (`card-field-empty`),
  `checkCardFieldOverlap` (`card-field-overlap`),
  `checkImpactSummaryMultiline` (`impact-summary-multiline`),
  `checkProsaSymbolTarget` (`prosa-symbol-target`),
  `checkCardNumbering` (`card-numbering`),
  `checkPathMissing` (`path-missing`),
  `checkCommitSubjectMismatch` (`commit-subject-mismatch`).
  `checkFormatAndApproval`, `checkIndexFileConsistency`, `checkCardNumbering`, and `checkCommitSubjectMismatch` keep their current bodies unchanged apart from `recognizedFormat`.
  `checkCardTypeMissing` emits one finding per card whose `TypeLabelCount` is not exactly 1, with a detail distinguishing zero recognized type labels from more than one.
  `checkCardRetiredLabel` emits one finding per entry in a card's `RetiredLabels`, naming the label and its format-3-to-format-4 mapping: `**What:**` became `**Intent:**`, `**Context:**` became `**Uses:**`, each of `**Edits:**`/`**Creates:**`/`**Deletes:**`/`**Moves:**` was absorbed into the card's own type-label target list, `**Depends-on:**` was dropped because dependency edges are derived rather than authored, and `**verify:**` became `**Verify:**`.
  `checkCardPathMalformed` iterates a card's `Targets` and `Uses`, applies `cardPathMalformedReason` to path-shaped entries only, and skips symbol-shaped entries;
  it does not iterate `Pairs` separately, because both endpoints of every pair are already projected into `Targets`.
  `checkRenameFormat` emits one finding per entry in a card's `RenameRaw`, naming the required `` `old` -> `new` `` grammar.
  `checkRenameMechanicMissing` emits one plan-level finding when at least one card has `Type == CardTypeRename` and `plan.RenameMechanic` is empty.
  `checkCardMissingField` keeps its ID and its existing `[]cardFieldLabel` iteration shape;
  the required set it iterates becomes `Intent:` for every card, plus `ImpactSummary:` for a card whose `Type` is `CardTypeEdit` or `CardTypeDelete`.
  `checkCardFieldEmpty` emits one finding per label that is present but carries no content: `HasType` with a zero-length `Targets`, `HasUses` with a zero-length `Uses`, `HasIntent` with an empty `Intent`, and `HasImpactSummary` with an empty `ImpactSummary`.
  `checkCardFieldOverlap` keeps its ID but now means that a single entry appears in both that card's `Targets` and its own `Uses`, reported once per duplicated entry in sorted order.
  `checkImpactSummaryMultiline` emits one finding per card whose `ImpactSummaryTrailing` is non-empty.
  `checkProsaSymbolTarget` emits one finding per symbol-shaped `Targets` entry on a card whose `Type` is `CardTypeProsa`.
  `checkPathMissing` stays existence-dependent and becomes type-conditional:
  it checks the path-shaped entries of every card's `Uses`, including a `CardTypeCustom` card's;
  it checks a card's path-shaped `Targets` when the card's `Type` is `CardTypeEdit`, `CardTypeDelete`, `CardTypeMove`, or `CardTypeProsa`;
  for a `CardTypeRename` card it checks the path-shaped `Old` side of every entry in `Pairs` and skips that card's `Targets` entirely, so the `New` side is never checked;
  it skips a `CardTypeCreate` card's `Targets` and a `CardTypeCustom` card's `Targets`.
  A path otherwise reported missing is satisfied by existing on disk, by membership in `createTargetsUnion`, or by membership in `renameTargetsUnion`.
  `CardTypeCustom` is exempt from `path-missing` on its own targets and from `prosa-symbol-target`'s target-shape rule, and from nothing else: a `Custom` card is still bound by `card-type-missing`, `card-missing-field`, `card-field-empty`, `card-field-overlap`, `card-path-malformed`, `card-retired-label`, `card-numbering`, and `commit-subject-mismatch`.
  Do not write a blanket type check that skips a `Custom` card wholesale.
  Rewrite the file's leading comment block to describe the sixteen-ID order above, and correct its "final 14 checks" phrase to 16.
  No scheduler, dependency graph, or topological sort belongs in this file.
- **Commit:** `feat(planparser): rework Validate to the format-4 sixteen-check set`

### Card 6: package documentation

- **Context:**
  - `internal/planparser/plan.go`
  - `internal/planparser/parse.go`
  - `internal/planparser/validate.go`
  - `internal/planparser/classify.go`
  - `internal/planparser/normalize.go`
- **Edits:**
  - `internal/planparser/doc.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Rewrite three passages of the package doc and leave the rest intact.
  The `# Type model` section must describe the format-4 `Card`: its Card Index fields (`Number`, `Slug`, `Summary`), its own file's `Title`, its `Type` and flat `Targets` ref list with `Pairs`/`RenameRaw` for `Rename`, its `Uses` list, its `Intent` prose and inline `ImpactSummary` (with `ImpactSummaryTrailing` capturing the multi-line defect), its per-label `HasX` presence bits, `RetiredLabels`, and the optional `Commit`/`Verify` fields.
  It must state that only path-shaped entries are normalized and that symbol entries are stored verbatim, and that classification is by shape at validation time via the package's own pure classifier — never `go doc`, never a process spawn, so the package stays a tier1-pure leaf.
  Replace the `# The none sentinel` section with a `# Field presence and omission` section: format 4 omits a field with no content rather than writing a `none` sentinel;
  the parser records a `HasX` presence bit per recognized label and preserves the nil-versus-empty distinction, so a label written with no bullets becomes a `card-field-empty` finding rather than a silent nil, and an omitted required label becomes a `card-missing-field` finding.
  Add a short paragraph to that section stating that the eight format-3 labels stay recognized and route to `RetiredLabels`, each producing a `card-retired-label` finding, because a label removed from the recognized set would be silently swallowed into `Intent:`'s prose.
  In the `# Validation lives in validate.go` section, change "The plan format's 14 validation checks" to 16 and replace the parenthesized examples with format-4 ones (card type presence, path malformation, the `Rename` pair grammar, on-disk existence, and so on).
  Leave the `# Path ownership` and `# The root:/// resolution rule` sections unchanged.
- **Commit:** `docs(planparser): rewrite the package doc for the format-4 card model`

### Card 7: websterengine Card Index rendering

- **Context:**
  - `internal/planparser/plan.go`
- **Edits:**
  - `internal/websterengine/render.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `RenderBatchIndex`, change the single read of `c.Intent` to `c.Summary`.
  This is the only non-test read of a renamed or removed `Card` field anywhere outside `internal/planparser`: `renderCardPointers` reads `Card.SourcePath` only, `RenderProgress` reads `Number`/`Slug` only, and `beginbatch.go`, `recoverbatch.go`, `integration.go`, and `runlevel.go` touch `Plan.Verify` or call `ParsePlan`/`Validate` without reading a card field.
  Do not change the rendered output's text or column layout — the field rename is the whole card.
- **Commit:** `refactor(websterengine): read Card.Summary after the planparser field rename`

## Batch Tests

The batch's `verify:` is `go build ./...` rather than a test run, and that is the honest gate for this batch: every test file in `internal/planparser`, `internal/websterengine`, `internal/loomshed`, `internal/loomrecipe`, `internal/loomcli`, and `internal/webstercli` still constructs the format-3 card shape and therefore fails to compile until batches 2 and 3 land.
`go build ./...` excludes `_test.go` files, so it is a real signal here: it proves the new model, parser, normalizer, and validator compile together and that every non-test consumer has been migrated.

Card 1's classifier ships with its own table test (`internal/planparser/classify_test.go`), which is new code with no format-3 coupling — it compiles and passes from the moment it lands, and batch 2's `go test ./internal/planparser/...` is where it first runs green alongside the rest of the package.

The whole-tree gate for this migration is `pipeline.done_gate` (`go test ./... && go test -tags integration ./...`), which mill-go runs from the repo root before marking the task done;
that is what proves the field renames left no missed reader anywhere in the tree.
