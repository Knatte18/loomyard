# Discussion: Migrate planparser.Card to Edits/Uses fields

```yaml
task: Migrate planparser.Card to Edits/Uses fields
slug: planparser-card-format-migration
status: discussing
parent: main
```

## Problem

`internal/planparser` is the sole parser of the on-disk plan format under `_lyx/plan/`.
Its `Card` type today models a plan card as five typed *file-op* fields — `Context:`, `Edits:`, `Creates:`, `Deletes:`, `Moves:` — plus `What:`, `Depends-on:`, `Commit:`, and `verify:`.
Every one of those fields names **files**.

A 2026-08-23 design discussion replaced that model with a symbol-level card format, written up in [`manifest/designs/plan-card-format.md`](../manifest/designs/plan-card-format.md).
In the new shape a card's *type* is the key (`Create`/`Edit`/`Delete`/`Rename`/`Move`/`Prosa`/`Custom`), the type's own list holds the card's targets (symbols and file paths mixed), and a separate `Uses:` list holds what the card reads but does not change.
Dependency edges are no longer authored at all — they are derived by intersecting one card's `Uses:` against every other card's target list.

**Why now:** this migration is a hard prerequisite for both Wave 3 roadmap items (`loom: Plan-Write producer` and `webster: DAG-derived card sequencing`) — see `manifest/roadmap.md`'s "loom: rewrite for the new Plan Card format" section.
Neither can be built while `planparser` still parses a file-only card.
The design doc is written and settled;
this task is the bounded, format-only migration that makes it real.

An important correction to the roadmap entry's framing, discovered during exploration:
the entry says to "update every direct consumer in `internal/websterengine` (rendering, report parsing, deviation-checking)".
**No non-test code in `internal/websterengine` reads the card file-op fields at all.**
`render.go` renders only `SourcePath`/`Number`/`Slug`/`Summary`;
`report.go`'s `Deviations` is a free string list the fork supplies;
deviation-checking is prose in a stencil, not Go.
The real consumer surface is much smaller — see Technical context.

## Scope

**In:**

- `internal/planparser` — the whole package: `plan.go` (the `Card`/`Plan` model), `parse.go` (grammar), `normalize.go` (path resolution), `validate.go` (the check set), `doc.go` (package doc), `sections.go` (unchanged in behavior, see Decision `rename-mechanic-section`), plus every test and the `testdata/goodplan/` golden fixture.
- `contracts/specs/loom-plan-spec.md` — rewritten in full to pin the new format as planparser's as-built contract.
- `contracts/stencils/webster/webster-body-implementer.md` — field names, grammar, and the deviation-union definition only.
- `contracts/stencils/loom/loom-template-plan.md` — field names, grammar, and validator rules only.
- `internal/loomengine/plan_test.go` — the stencil-pinning tests that assert old-format prose.
- Test fixtures carrying an old-format card body: `internal/loomrecipe/fixture_test.go`, `internal/loomshed/planvalidate_test.go`, `internal/websterengine/runlevel_test.go`, `internal/websterengine/template_test.go`.
- Stale comment corrections: `internal/websterengine/doc.go`'s "dead DAG seam" passage and its deviation-union sentence;
  the "14 checks" counts in `internal/webstercli/validate.go`, `internal/planparser/validate.go`, and `internal/planparser/validate_test.go`.
- `manifest/roadmap.md` — move the Wave 2 planparser item to Done on completion.
- `manifest/designs/plan-card-format.md` — flip its "Status: designed, not implemented" banner, and record the three open items this task closes.

**Out:**

- **Any behavior change to webster.** Cards still execute in strict declared plan order.
  The DAG is Wave 3.
- **The Wave 3 prompt redesign** in `contracts/stencils/loom/loom-template-plan.md`.
  This task touches only that stencil's mechanical field-name/grammar/validator content;
  the `Plan-Write` producer's prompt rewrite belongs to the Wave 3 roadmap item.
- **The three-tier Verify model's implementation.** Tier1's automatic package-scoped test run is specified only, never wired.
- **Quarry.** No card type is defined in terms of it;
  every mechanical step works in degraded mode, which is the default, not a fallback.
- **`internal/batcher`.** The identity batcher passes `[]planparser.Card` through untouched and needs no change.
- **The `HasSymbolFields()` seam's activation** in `internal/websterengine`. Its doc comment is corrected;
  the seam stays dead.
- **`manifest/designs/scout-plan-symbol-fields.md` and `manifest/designs/webster-parallel-execution.md`.** Both are stale, both are already assigned elsewhere by the roadmap — `webster-parallel-execution.md` to the Wave 3 DAG task specifically (`manifest/roadmap.md:62`).
- **`Delete`'s assert-no-callers and `Create`'s "nothing equivalent exists"** mechanical checks.
  Both are execution-time concerns needing symbol lookup;
  planparser is a parser, not an impact analyser.
- **Back-compatibility with `format: 3` plans.** No dual-reader.

## Decisions

### card-grammar

- **Decision:** the card's type name becomes a bold markdown label, exactly like today's file-op fields: `**Edit:**` on its own line, followed by backtick-wrapped `- ` sub-bullets.
  Exactly one recognized type label per card, from the set `Create`, `Edit`, `Delete`, `Rename`, `Move`, `Prosa`, `Custom`.
  The other labels a card may carry are `**Uses:**`, `**Intent:**`, `**ImpactSummary:**`, `**Verify:**`, and `**Commit:**`.
- **Rationale:** the design doc's `<TypeName>:` notation is YAML-ish pseudo-code;
  the real on-disk format is markdown.
  Making the type a label reuses `isCardLabelLine`, `parseFileOpField`, and the whole existing bullet-collection machinery unchanged, and it honours the design doc's "the type name is the key — no separate `Type:` field".
- **Rejected:** a separate `**Type:** Edit` label plus a `**Targets:**` list (explicitly contradicts the design doc);
  the type as a card-heading suffix (`# Card 3 — Edit — <name>`), which moves a semantic field out of the body parser into the heading regex.

### field-mapping

- **Decision:** the old-to-new field mapping is:
  - `Context:` → **`Uses:`** (read/depended-on, not the target).
  - `Edits:`/`Creates:`/`Deletes:`/`Moves:` → **absorbed into the type label's own target list**;
    the operation is now the card's type, not a per-path field.
  - `What:` → **`Intent:`**. The design doc's `Intent` ("what, and why — prose, can be multi-sentence") is exactly `What`'s role.
  - `Depends-on:` → **dropped entirely.** The design doc is explicit: "No `DependsOn`/`Produces` field. Dependency edges are derived, never authored."
  - `Commit:` → **survives unchanged.** It is webster's per-card commit-subject pin, orthogonal to the card format.
  - `verify:` → **`Verify:`**, surviving as the optional, verbatim, rare escape hatch the design doc's Verify model keeps it for.
  - New: **`ImpactSummary:`**, required for `Edit` and `Delete` only.
  - The Card Index entry's one-line summary keeps its place in the index grammar, but its struct field is renamed `Card.Intent` → **`Card.Summary`**, freeing the name `Intent` for the new body-level field.
- **Rationale:** every rename or removal is either stated outright in the design doc or forced by it;
  `Commit:` and `Verify:` are the two fields the design doc does not touch, and neither has anything to do with the file-op shape being replaced.
- **Rejected:** keeping `What:` as the on-disk label with `Intent:` as an alias (two names for one thing);
  dropping `Commit:` too (loses `commit-subject-mismatch` and webster's per-card commit pinning for no gain);
  keeping `Depends-on:` as an optional authored override (directly contradicts the design doc, and Wave 3 derives the same edges mechanically).

### target-model

- **Decision:** `Card` carries a `Type` enum plus a flat `Targets []string` and a flat `Uses []string`.
  Symbol-vs-path classification happens at **validation** time via a shared pure function, never at parse time, and is never baked into a typed field.
- **Rationale:** mirrors the design doc's "distinguished by shape" verbatim and keeps the parser dumb — nothing is inferred or guessed while reading the document, which is the same discipline `doc.go`'s lenient-card-parse decision already encodes.
- **Rejected:** splitting into `TargetSymbols`/`TargetPaths` at parse time (the parser then owns a heuristic, and a misclassification is baked permanently into the model);
  a `Ref struct{ Raw, Kind }` slice (lossless and explicit, but heavier, and every consumer has to reach through `.Raw`).

### shape-classifier

- **Decision:** one pure function in `planparser` classifies a raw entry.
  The rule, in order:
  1. contains `/` → **path** (this covers the `//` worktree-root escape too);
  2. else, the segment after the final `.` is entirely lowercase ASCII alphanumerics → **path**;
  3. else → **symbol**.
  So `internal/boardcli/list.go` and bare `list.go` are paths;
  `shedrecipe.Lookup` is a symbol.
  The same function is the sole classifier used by `card-path-malformed`, `path-missing`, `prosa-symbol-target`, and by `normalizeCard`.
- **Rationale:** no `go doc`, no process spawn, no disk stat during classification — `planparser` stays a leaf, consistent with the Planparser Sole-Parser Invariant and the Test Tier Purity Invariant.
- **Known limitation, to be documented in the spec:** an unexported symbol reference (`shedrecipe.lookup`) misclassifies as a path and surfaces as a `path-missing` finding.
  The author writes the exported name or a `//`-escaped path.
  This is acceptable: cards reference exported API in practice, and the failure mode is a loud validation finding, never a silent misparse.
- **Rejected:** an explicit `file:` prefix disambiguator (zero heuristic, but departs from the design doc's "distinguished by shape" and adds noise to every path bullet);
  treating `/` as the only signal (unambiguous, but makes `root:`'s bare-filename convenience useless, contradicting the `root-resolution` decision below).

### root-resolution

- **Decision:** `root:` and the `//` escape survive unchanged in the format.
  `normalizeCard` applies `normalizeCardPath` **only to entries the shape classifier calls paths**;
  symbol entries pass through verbatim.
- **Rationale:** today `normalizeCard` root-joins every entry unconditionally, which under a mixed list would turn `shedrecipe.Lookup` into `internal/boardcli/shedrecipe.Lookup`.
  Gating on the shared classifier is the minimal correct fix and keeps `Plan.Root` and the whole of `normalize.go` intact.
- **Rejected:** dropping `root:`/`//` from the format entirely (simpler model, but a much larger spec delta and it deletes a real terse-plan convenience);
  restricting `root:` to `Prosa` cards only (arbitrary, and `Prosa` is the rarest type).

### field-presence

- **Decision:** the `none` sentinel is **no longer admitted on any field**.
  A field with no content is omitted, per the design doc.
  Required per card: exactly one type label, `Intent:`, and — for `Edit` and `Delete` — `ImpactSummary:`.
  Optional: `Uses:`, `Verify:`, `Commit:`.
  The parser still records a `HasX` presence bit per recognized label and still preserves the nil-vs-empty distinction, so a label written with no bullets under it becomes a new `card-field-empty` finding rather than a silent nil.
- **Rationale:** the design doc is explicit ("Omit any field with no content — no `Uses: []`, no `Uses: None`"), and keeping the presence bits preserves the parse-lenient/validate-enumerates split that `doc.go` documents as a deliberate decision.
  Losing the distinction would make "author forgot the label" and "author wrote the label and left it blank" indistinguishable.
- **Rejected:** keeping `none` mandatory on every field (preserves today's three-state model exactly, contradicts the design doc);
  dropping the presence bits so absent ≡ empty (smallest model, loses a real defect class).

### prose-fields

- **Decision:** `Intent:` is multi-line, reusing `What:`'s existing collect-until-next-label prose collection verbatim.
  `ImpactSummary:` takes its value **inline on the label line only**;
  a following non-label line is a defect (`impact-summary-multiline`).
  No character cap is enforced.
- **Rationale:** the design doc keeps `ImpactSummary` as its own field specifically so it stays terse — "folding it into `Intent` lets it balloon into unbounded reasoning".
  One-line is the enforceable half of "hard-capped";
  a character threshold would be an arbitrary magic number that gets argued with rather than obeyed.
- **Rejected:** both fields multi-line (symmetric, but reintroduces exactly the ballooning the design doc separates them to prevent);
  adding a character-count check.

### format-version

- **Decision:** bump `recognizedFormat` from `3` to `4`.
  A `format: 3` plan is hard-rejected by the existing `format-unrecognized` check.
  No dual-reader.
- **Rationale:** nothing is in flight;
  a dual-reader would permanently double the parser's surface and the validator's check set to serve zero real plans.
- **Rejected:** dual-reading 3 and 4;
  keeping `format: 3` (no version signal that the card shape changed, so a stale plan misparses instead of failing loud).

### validator-checks

- **Decision:** the check set moves from 14 to 13.
  Disposition of every existing check:

  | Existing check | Disposition |
  |---|---|
  | `format-unrecognized` | Keep, `recognizedFormat` 3 → 4 |
  | `plan-unapproved` | Keep unchanged |
  | `index-file-mismatch` | Keep unchanged |
  | `card-numbering` | Keep unchanged |
  | `commit-subject-mismatch` | Keep unchanged |
  | `card-path-malformed` | Rework — applies only to path-shaped entries |
  | `path-missing` | Rework — applies only to path-shaped entries |
  | `card-field-overlap` | Rework — now means "an entry appears in both this card's target list and its own `Uses:`" |
  | `card-missing-field` | Replaced by `card-type-missing` + `impact-summary-missing` |
  | `move-format` | Replaced by `rename-format` |
  | `move-mechanic-missing` | Renamed `rename-mechanic-missing` (see `rename-mechanic-section`) |
  | `move-redundant` | Drop |
  | `move-source-missing` | Drop |
  | `move-target-collision` | Drop |
  | `depends-on-order` | Drop |

  New checks: `card-type-missing` (a card carries zero, or more than one, recognized type label), `impact-summary-missing` (an `Edit` or `Delete` card has no `ImpactSummary:`), `impact-summary-multiline`, `card-field-empty`, `rename-format` (a `Rename` bullet failing the pair grammar), `prosa-symbol-target` (a `Prosa` card's entries must all be path-shaped).

- **Rationale:** the three dropped `move-*` checks all rest on a mechanical `old -> new` pair plus a destination path, which only `Rename` still has (see `rename-grammar`);
  `Move`'s destination lives in `Intent` prose by design, so there is nothing to cross-check.
  `depends-on-order` has no field left to check.
  This closes the design doc's open item "Reconciliation with `contracts/specs/loom-plan-spec.md`'s existing 14 validator checks".
- **Rejected:** preserving the `move-*` checks by giving `Move` a mechanical destination field (departs from the design doc);
  stripping to format/approval/index-consistency only and deferring content checks to a later hardening pass (leaves the new format almost unchecked at exactly the moment it is least understood).

### rename-grammar

- **Decision:** bullet grammar is type-conditional.
  A `Rename` card's bullets parse against the existing `` `old` -> `new` `` regex into `[]MovePair` — the type is reused, not deleted.
  Every other type's bullets are plain refs.
  A `Rename` bullet failing the pair grammar lands in a `RenameRaw` slot, exactly as `MovesRaw` works today, and the new `rename-format` check reports it.
- **Rationale:** the design doc's card-type table gives `Rename` "existing symbol(s), `old -> new` pairs" and `Move` "destination stated in `Intent`, not the list".
  That asymmetry is deliberate and has to be reflected in the parser.
  Retargeting `MovesRaw`'s lenient-capture mechanism keeps the parse-lenient discipline intact for the one type that still has a grammar to fail.
- **Rejected:** uniform plain-string bullets everywhere with `MovePair` deleted (simplest parser, but no mechanical rename checking ever);
  giving `Move` a mechanical `To:` field to restore the collision checks (departs from the design doc).

### rename-mechanic-section

- **Decision:** the plan-level `## Rename mechanic` section survives.
  `Plan.RenameMechanic` and `sections.go` are unchanged.
  `move-mechanic-missing` is renamed `rename-mechanic-missing` and fires when any card is type `Rename` and the section is absent.
- **Rationale:** this is a deliberate partial amendment to `validator-checks` — five `move-*` checks drop, one returns renamed.
  The design doc requires a `Rename` to be executed by an AST-aware script, never text/regex, which makes the plan-level mechanic section *more* load-bearing than it was for the old `Moves:` field, not less.
  Letting it fall with the other four would invite exactly the text/regex rename the design doc bans.
- **Rejected:** keeping the section but dropping the check (nothing then enforces its presence);
  dropping both (a `Rename` card could ship with no stated mechanic).

### verify-model-scope

- **Decision:** the three-tier Verify model is **specified only**.
  `Verify:` stays the optional, verbatim string it is today.
  Tier1's automatic package-scoped `go test` run is not implemented.
- **Rationale:** tier1 needs symbol→package resolution and caller impact lookup that do not exist, and wiring it would be precisely the behavior change the roadmap entry rules out ("No behavior change — webster still executes strictly in declared plan order after this lands").
  The rewritten spec records the tier model as designed-not-implemented, exactly as the design doc does.
- **Rejected:** implementing tier1 now;
  dropping the per-card `Verify:` field (loses the explicit tier3 escape hatch the design doc keeps it for).

### doc-reach

- **Decision:** rewrite `contracts/specs/loom-plan-spec.md` in full.
  In `contracts/stencils/webster/webster-body-implementer.md` and `contracts/stencils/loom/loom-template-plan.md`, rewrite **only** field names, grammar, and validator rules.
- **Rationale:** `loom-plan-spec.md` is the pinned as-built contract `planparser` implements and is cited in roughly fifteen Go doc comments;
  leaving it describing format 3 rots every one of them.
  `webster-body-implementer.md` is live in real webster runs today and must name fields that exist.
  `loom-template-plan.md` is only driven by the `Plan-Write` **stub**, so its prompt-level redesign is genuinely Wave 3's job — but its mechanical field content must not describe a format the parser rejects.
- **Rejected:** spec plus webster stencil only (leaves the loom stencil emitting an unparseable format);
  spec only;
  retiring `loom-plan-spec.md` and promoting `manifest/designs/plan-card-format.md` to pinned spec (would mean repointing every Go comment and conflating a design doc with an as-built contract — the spec's own banner explains why they are separate).

### deviation-union

- **Decision:** in `webster-body-implementer.md`, the deviation union becomes: every path-shaped target entry, plus the files holding every symbol-shaped target, the implementer resolving those itself.
  `Uses:` stays out of the union — it is read, not written.
- **Rationale:** keeps the deviation list as informative as it is today.
  The fork is an LLM already in the worktree;
  resolving `shedrecipe.Lookup` to its file is one read.
  This is stencil prose only — `report.go`'s `Deviations` is a free string list either way, so there is no Go change and no contract change.
- **Rejected:** path-shaped targets only (strictly mechanical, but a card whose targets are all symbols predicts no files and reports its whole diff as deviation — harmless, since deviations are always informational, but noisy);
  dropping `deviations` from the report contract (a real change to webster's fork-return contract, out of scope).

### stale-comments

- **Decision:** correct three classes of now-false comment in this task.
  1. `internal/websterengine/doc.go`'s "dead DAG seam" passage currently says "`HasSymbolFields()` is unreachable in v0 — plan-format cards carry no symbol fields yet".
     Rewrite so the reason the seam is inactive is "Wave 3 activates it", not "cards carry no symbol fields".
     The seam stays dead.
  2. The same file's deviation-union sentence, per `deviation-union`.
  3. The "14 checks" counts in `internal/webstercli/validate.go`, `internal/planparser/validate.go`, and `internal/planparser/validate_test.go`.
- **Rationale:** all three become factually false the moment this lands, and a knowingly-false doc comment is worse than no comment.
  All are comment-only, no code, no behavior change.
- **Rejected:** leaving them for Wave 3;
  deleting the seam passage entirely (loses the design rationale in the meantime).

### no-new-invariant

- **Decision:** no new entry in `CONSTRAINTS.md`, and no edit to the existing Planparser Sole-Parser Invariant.
- **Rationale:** that invariant's text is about *who parses* the plan format and *who declares its path* — it says nothing about the field set, so a field-set change does not touch it.
  This task introduces no new cross-cutting structural rule.

## Technical context

**`internal/planparser` — the package being migrated.**

- `plan.go` (88 lines) — `Plan`, `Card`, `MovePair`. `Card`'s fields to change: `ContextFiles`/`EditsFiles`/`CreatesFiles`/`DeletesFiles` (`[]string` each), `Moves []MovePair`, `MovesRaw []string`, `HasContext`/`HasEdits`/`HasCreates`/`HasDeletes`/`HasMoves`/`HasDependsOn` bools, `DependsOn []int`, `What string`, `HasWhat bool`, and `Intent string` (the Card Index one-liner, being renamed `Summary`).
  Unchanged: `Number`, `Slug`, `Title`, `SourcePath`, `Commit`, `Verify`.
  `Plan` is unchanged except that `Format` now means 4.
- `parse.go` (482 lines) — the label constants block (`whatLabel` … `cardVerifyLabel`), `cardLabels`, `isCardLabelLine`, the `parseCardBody` switch, `parseFileOpField`, `parseMovesField`, `parseDependsOnField`, `noneSentinel`, `moveLineRe`.
  `parseFileOpField` and `parseMovesField` are the two reusable bullet collectors;
  `parseDependsOnField` and `noneSentinel` go away.
  Note `parseFileOpField` currently returns `[]string{}` for the `none` sentinel and errors on an inline value — under `field-presence` the `none` branch is deleted and the inline-value error stays.
- `normalize.go` (60 lines) — `normalizeCardPath`, `hasWorktreeRootEscape`, `cleanPosixPath`, `normalizeCard`, `normalizePathSlice`.
  Only `normalizeCard` changes shape (it must consult the classifier);
  the rest is reusable as-is.
- `validate.go` (616 lines) — `Validate`'s fixed call sequence plus one `checkXxx` function per check, `ValidationError`, `cardID`, and the helpers `createsUnion`, `movesTargetsUnion`, `pathExistsOnDisk`, `cardPathMalformedReason`, `cardHeadingNumber`.
  `createsUnion`/`movesTargetsUnion` exist only to serve dropped checks and go away;
  `pathExistsOnDisk`, `cardPathMalformedReason`, `cardHeadingNumber`, and `cardID` all survive.
- `sections.go` (69 lines) — unchanged.
- `doc.go` (62 lines) — the package doc describes the five typed file-op fields, the `none` sentinel's three-state model, and "the plan format's 14 validation checks".
  All three passages need rewriting.
- `testdata/goodplan/` — a four-card golden fixture (`00-overview.md` + `01-json-flag.md` … `04-helptree-rename.md`) that is a materialization of the spec's worked example.
  It uses `root: internal/boardcli`, a `//`-escaped path, a `Depends-on:` chain, a pinned `Commit:`, and a `Moves:` card with the `## Rename mechanic` section.

**Consumers outside `planparser` — much smaller than the roadmap entry implies.**

- `internal/loomshed/planvalidate.go` — calls `planparser.ParsePlan` then `planparser.Validate` and maps a non-empty `[]ValidationError` to `shedengine.Stuck`. **No field reads.** Needs no change;
  its test fixture does.
- `internal/batcher` — `Batch{Cards []planparser.Card}` and the identity batcher pass cards through. **No field reads.** No change.
- `internal/websterengine` — `render.go` reads only `Card.SourcePath` (in `renderCardPointers`), and `Number`/`Slug`/`Intent` in `RenderBatchIndex`/`RenderProgress`.
  The `Intent` → `Summary` rename touches those two functions.
  `runlevel.go` calls `ParsePlan`/`Validate`. `beginbatch.go`/`recoverbatch.go`/`integration.go` hold a `*planparser.Plan` and read `Plan.Verify`.
  **No non-test file reads any file-op field.**
- `internal/loomengine/plan.go` — `PlanSpec` composes the `loom-template-plan` stencil prompt.
  No field reads;
  but `plan_test.go` has stencil-pinning tests (`TestPlanSpec_PromptStatesContextSemantics`, `..._PromptStatesMoveRedundantRule`, `..._PromptStatesMovedFileNotInEdits`, `..._PromptStatesDependsOnCriterion`, `..._PromptStatesCardCriteria`, `..._PromptStatesRootResolution`, `..._PromptStatesVerifyIsRunnable`) asserting the old rules are present in the stencil text.
  These must be retargeted alongside the stencil rewrite.

**Test fixtures carrying a literal old-format card body** (each needs its string rewritten):

- `internal/loomrecipe/fixture_test.go:163-164`
- `internal/loomshed/planvalidate_test.go:27`
- `internal/websterengine/runlevel_test.go:198`
- `internal/websterengine/template_test.go:759` (sets `card.Moves = []planparser.MovePair{...}`)

**Gotchas discovered during exploration:**

- `normalizeCard` root-joining a symbol is the single sharpest edge — see `root-resolution`.
- `Card.Intent` already exists and means the Card Index one-liner.
  The new body-level `Intent:` collides with it;
  the `Summary` rename must land before or with the new field or the two silently conflate.
- `parseFileOpField`'s `none` handling returns an **empty non-nil slice**, which is how `HasX` + nil-vs-empty encodes three states today.
  Removing `none` changes what "empty non-nil" means — under `field-presence` it now means "label present, no bullets", i.e. the `card-field-empty` defect.
- `checkIndexFileConsistency` reads `plan.Dir` from disk to find unreferenced `.md` files.
  Untouched by this migration but sensitive to fixture layout changes.
- The design doc's card-type table is the authority on which types require `ImpactSummary` (`Create`, `Edit`, `Delete` — but see below) versus not (`Rename`, `Move`, `Prosa`).
  Note the table marks `ImpactSummary` "required" for `Create` as well, while the design doc's prose in the Card fields block says "required for Edit/Delete only".
  **This discussion resolves that conflict in favour of the prose: `ImpactSummary` is required for `Edit` and `Delete` only.** A `Create` card has no existing callers to have a blast radius over, which is why the prose reads the way it does;
  the table row is a drafting slip.
  The rewritten spec should state Edit/Delete and say so unambiguously.

## Constraints

From `CONSTRAINTS.md`:

- **Planparser Sole-Parser Invariant** — `internal/planparser` is the sole parser of `_lyx/plan/`;
  no other package may parse `00-overview.md`/`NN-<card-slug>.md`, and consumers read plan-level sections only from the `planparser.Plan` model.
  `planparser` is also the sole declarer of the plan directory's path (`PlanDirName`/`PlanDirRel`/`PlanDir`/`PlanOverview`), never resolves cwd, and never imports `internal/lyxcwd`.
  This migration must not push any parsing into a consumer to avoid a parser change.
- **Cwd Resolution Invariant** — `internal/lyxcwd` alone resolves cwd;
  `planparser` takes the anchor path from its caller (`AnchorPath()`, never `WorktreePath()`).
- **Test Tier Purity Invariant** — tier1 tests do no cwd resolution and spawn no processes.
  The shape classifier must therefore be a pure function, never a `go doc` call.
- **Documentation Lifecycle** — a task changing cross-cutting infrastructure updates its docs in the **same commit**.
  Here that means `contracts/specs/loom-plan-spec.md`, the two stencils, and `manifest/roadmap.md`.
- **Markdown Link Integrity** — the rewritten spec and design-doc banner must not leave dangling relative links.
- **Stencil Ownership Invariant** — stencils are read at runtime from a told stencils dir via `stencilstore.Read`, with embedded defaults in `contracts/stencils/stencils.go`. Editing a stencil means editing the embedded default;
  `loom-template-plan` and the `webster-body-implementer` assets are both registered there.

From `CLAUDE.md`:

- **Markdown: semantic line breaks** — one sentence per line, plus breaks at internal independent-clause boundaries.
  Applies to the rewritten spec, both stencils, and the design-doc banner.
- **Task completion — docs land in the same commit.** `manifest/roadmap.md` moves only on completing a planned item, which this is.

Discovered during discussion:

- **No behavior change.** After this lands, webster must execute cards in the same strict declared order it does today.
  A reviewer should be able to confirm that by inspection: no scheduler, no graph, no topological sort appears anywhere in the diff.
- **`planparser` must remain free of process spawns and symbol-resolution machinery.** The shape classifier is string analysis only.

## Testing

**`internal/planparser` — the bulk of the work, and the TDD candidate.**

The parser and validator are pure functions over strings and a small on-disk fixture tree, with no cwd resolution and no process spawn.
Write the tests first here.

- **Golden fixture rewrite** (`testdata/goodplan/`): rebuild it in format 4, exercising **every one of the seven card types** — including a `Rename` card with well-formed `old -> new` pairs, a `Prosa` card with file-only targets, and at least one card whose target list mixes a symbol and a path.
  Keep it in parity with the rewritten spec's worked example, exactly as it is today.
  Keep `root:` and a `//`-escaped entry so `root-resolution` is covered by the golden path.
- **Round-trip test** (`parse_test.go`'s existing `goodPlanDir` pattern): parse the fixture and assert every field of every card, including that a symbol target survives `normalizeCard` **unmodified** under a non-empty `root:` — that is the single most important regression this migration can introduce.
- **Shape classifier**: a table test over the classification rule.
  Cover `internal/x/y.go`, bare `list.go`, `//cmd/lyx/main.go`, `shedrecipe.Lookup`, a bare `Lookup`, and the documented `shedrecipe.lookup` misclassification so the limitation is pinned rather than discovered later.
  **Deliberately no fuzz test** — the classifier is a small pure function fully covered by a table, and fuzzing it would be scope beyond a bounded migration with no observed problem to solve.
- **One test per validator check.** Every surviving, reworked, and new check from the `validator-checks` table gets its own focused test.
  The five `move-*` tests and the `depends-on-order` test are **deleted, not adapted** — retrofitting a dropped check's test onto a new check produces a test that documents the wrong thing.
- **Golden all-checks-pass test**: `validate_test.go`'s existing "all checks pass simultaneously on the happy path" test, retargeted to the new count.
- **Parse-lenient scenarios**: a label present with no bullets (`card-field-empty`), a card with two type labels and a card with none (`card-type-missing`), a malformed `Rename` bullet landing in `RenameRaw` (`rename-format`), a multi-line `ImpactSummary` (`impact-summary-multiline`), and a `Prosa` card with a symbol target (`prosa-symbol-target`).
- **Fail-loud scenarios stay fail-loud**: an inline value on a field admitting only bullets must still be a `ParsePlan` error, not a finding.

**`internal/loomengine` — stencil-pinning tests.**

Retarget the seven `TestPlanSpec_PromptStates*` tests to assert the new rules.
`..._PromptStatesMoveRedundantRule`, `..._PromptStatesMovedFileNotInEdits`, and `..._PromptStatesDependsOnCriterion` pin rules that no longer exist and should be deleted;
add pins for the new type-label grammar, the `Uses:` semantics, and the `ImpactSummary` requirement.

**Fixture-only updates** (`loomrecipe`, `loomshed`, `websterengine`): rewrite each literal card body to format 4.
These are not new coverage — they must simply stop describing a format the parser rejects.
`websterengine/template_test.go:759`'s `card.Moves` assignment becomes a `Rename`-type card.

**Whole-tree gate:** `go build ./... && go test ./...`. The migration is only complete when the tree is green — the compiler is the real backstop for the field renames, since `Card.Intent` → `Card.Summary` and the removal of six field names will surface every missed reader.

## Q&A log

- **Q:** How far does the doc/stencil rewrite reach? **A:** Spec rewritten in full, plus both stencils but only their mechanical field-name/grammar/validator content — the Wave-3 prompt redesign in `loom-template-plan.md` is not this task's job, while the live `webster-body-implementer.md` must name fields that exist.
- **Q:** What happens to `What:`, `Depends-on:`, `Commit:`, `verify:`, and the Card Index one-liner? **A:** `Intent:` replaces `What:`;
  `Depends-on:` dropped;
  `Commit:` and `Verify:` survive;
  `Card.Intent` renamed `Card.Summary`.
- **Q:** Which of the 14 validator checks survive? **A:** Keep 5, rework 3, drop 6, add 6 — net 13. See the `validator-checks` table.
- **Q:** How is a card's mixed symbol/path target list modelled in Go? **A:** One flat `Targets []string` plus a `Type` enum, classified by shape at validation time — "distinguished by shape" is verbatim from the design doc, and shape-based classification at validation keeps the parser dumb, in line with nothing being inferred or guessed at parse time.
- **Q:** How is `Rename`'s pair grammar reconciled with `Move` stating its destination in prose? **A:** Type-conditional bullet grammar — `Rename` reuses the arrow regex and `MovePair`, everything else is plain refs, and a failed `Rename` pair lands in `RenameRaw`.
- **Q:** Bold-label type key, or a separate `Type:` field? **A:** Bold label (`**Edit:**`), reusing the existing bullet machinery;
  a separate `Type:` field is explicitly banned by the design doc.
- **Q:** How is `root:` reconciled with symbol references? **A:** One shared shape classifier gates normalization — path entries get root-resolved, symbols pass through verbatim.
- **Q:** Dual-read `format: 3`? **A:** No. Bump to 4 and hard-reject 3.
- **Q:** Is the three-tier Verify model implemented here? **A:** Specified only.
  Implementing tier1 would be exactly the behavior change the roadmap entry rules out, so spec-only is the correct scope, not merely the recommended one.
- **Q:** Testing approach? **A:** Golden fixture rewritten to cover all seven types, per-check tests retargeted 1:1, dropped checks' tests deleted rather than adapted, table test for the classifier.
  Fuzzing the classifier was considered and **deliberately rejected** — it is a trivial pure function already covered by a table, and fuzzing is scope beyond a bounded migration with no observed problem it solves (YAGNI).
- **Q:** `none` sentinel or omission, and what happens to the `HasX` bits? **A:** Omission, per the design doc;
  presence bits and nil-vs-empty kept, with a new `card-field-empty` finding for a label written with no bullets.
- **Q:** What is the exact shape-classifier rule? **A:** `/` → path;
  else an all-lowercase final dot-segment → path;
  else symbol.
  The unexported-symbol misclassification is a documented, loud limitation.
- **Q:** How are `Intent:` and `ImpactSummary:` parsed? **A:** `Intent:` multi-line;
  `ImpactSummary:` inline-only with no character cap — one-line is the enforceable half of "hard-capped", and a magic number would be arbitrary.
- **Q:** What is the deviation union once targets can be symbols? **A:** The implementer resolves symbol targets to their holding files and unions those with path-shaped targets;
  `Uses:` stays out.
  Stencil prose only, no Go change.
- **Q:** Type-specific checks for `Prosa`, `Custom`, `Create`, `Delete`? **A:** `impact-summary-missing` (Edit/Delete), `rename-format`, `prosa-symbol-target`.
  `Custom` gets none — a principled closure of that design-doc open item in the affirmative, not an oversight.
- **Q:** What about the two stale design docs? **A:** Left alone.
  The roadmap already assigns `webster-parallel-execution.md` to the Wave 3 DAG task.
- **Q:** The now-false "no symbol fields" claim in `websterengine/doc.go`? **A:** Corrected in this task, comment-only;
  the seam stays dead.
  Also fixes the stale "14 checks" counts.
  No new `CONSTRAINTS.md` invariant.
- **Q:** Does the plan-level `## Rename mechanic` section survive? **A:** Yes, with `move-mechanic-missing` renamed `rename-mechanic-missing` and retargeted to `Rename` cards.
  This is a genuine revision of the drop-all-`move-*` answer, not another instance of it: the AST-aware requirement makes the check *more* valuable than it was, and letting it fall would open exactly the text/regex rename the design doc bans.
