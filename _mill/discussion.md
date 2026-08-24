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
- Every file carrying an old-format card body or a `format: 3` plan — see `## Technical context` for the enumeration method and the full list, which a plan writer must re-run rather than trust as a hand list.
- Stale comment corrections: `internal/websterengine/doc.go`'s "dead DAG seam" passage and its deviation-union sentence;
  the "14 checks" counts in `internal/webstercli/validate.go`, `internal/planparser/validate.go`, and `internal/planparser/validate_test.go` — which are **already wrong today**, see the `validator-checks` decision.
- `manifest/roadmap.md` — move the Wave 2 planparser item to Done on completion.
- `manifest/designs/plan-card-format.md` — its status banner (`:3`) is rewritten **clause by clause**, not just its opening phrase:
  - *"Status: designed, not implemented"* → implemented, naming this task.
  - *"Supersedes `loom-plan-spec.md`'s Card fields and `loom-template-plan.md` — neither is rewritten yet"* → both **are** rewritten here (`doc-reach`), so the clause becomes a plain pointer at the spec rather than a pending-work note.
  - *"`scout-plan-symbol-fields.md` and `webster-parallel-execution.md` … reconcile or delete them when this lands"* → **repointed at the owning roadmap items** — `roadmap.md:14`'s group-level reconcile instruction plus the Someday item at `roadmap.md:61-62` for `webster-parallel-execution.md`, and `roadmap.md:136-138` for `scout-plan-symbol-fields.md` — rather than left as an unactioned instruction this task visibly does not follow.
    Do **not** encode "assigned to the Wave 3 DAG task" here: `roadmap.md:31`'s Wave 3 item does not name that doc.
    Leaving it as-is would make the doc read as instructing work the task deliberately declines.
  - The doc's *"Open, not decided here"* section records that this task closes **all three** items: `Custom` needs no *type-specific* mechanical check (`validator-checks`), `ImpactSummary` on `Delete` stays one line of prose (`prose-fields`), and the validator-check reconciliation is the `validator-checks` table.
    Its own check-count figure at `:84` is corrected by Sweep 3 (`stale-comments`).

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
- **Reconciling `manifest/designs/scout-plan-symbol-fields.md` and `manifest/designs/webster-parallel-execution.md`.** Both are stale as documents;
  `webster-parallel-execution.md` is owned by the **Someday** item "webster: worktree-per-card parallel execution" (`manifest/roadmap.md:61-62`) and by the group-level reconcile instruction at `roadmap.md:14` — **not** by the Wave 3 DAG item, which does not mention the doc at all.
  `scout-plan-symbol-fields.md` has its own roadmap item (`manifest/roadmap.md:136-138`).
  **Narrow exception:** `scout-plan-symbol-fields.md:64`'s stale check-count figure *is* corrected here, as one of six sites — see `stale-comments`.
  Correcting a number is not reconciling a document.
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
- **Rationale:** follows the design doc's "distinguished by shape" clause (see `shape-classifier` for the one clause of that rule this task deliberately declines) and keeps the parser dumb — nothing is inferred or guessed while reading the document, which is the same discipline `doc.go`'s lenient-card-parse decision already encodes.
- **Rejected:** splitting into `TargetSymbols`/`TargetPaths` at parse time (the parser then owns a heuristic, and a misclassification is baked permanently into the model);
  a `Ref struct{ Raw, Kind }` slice (lossless and explicit, but heavier, and every consumer has to reach through `.Raw`).

### shape-classifier

- **Decision:** one pure function in `planparser` classifies a raw entry.
  The rule, in order:
  1. contains `/` → **path** (this covers the `//` worktree-root escape too);
  2. else, the segment after the final `.` is entirely lowercase ASCII alphanumerics → **path**;
  3. else → **symbol**.
  Rule 3 is the explicit default for two cases, not an implementation accident:
  an entry containing **no `.` at all** (`Lookup`, `Makefile`, a bare directory name) never reaches rule 2's test and falls to rule 3 → symbol;
  and an entry whose final dot-segment is not all-lowercase (`shedrecipe.Lookup`) → symbol.
  Both cases are pinned by the classifier table test.
  So `internal/boardcli/list.go` and bare `list.go` are paths;
  `shedrecipe.Lookup` is a symbol.
  The same function is the sole classifier used by `card-path-malformed`, `path-missing`, `prosa-symbol-target`, and by `normalizeCard`.
- **Rationale:** no `go doc`, no process spawn, no disk stat during classification — `planparser` stays a leaf, consistent with the Planparser Sole-Parser Invariant and the Test Tier Purity Invariant.
- **Deliberate deviation from the design doc.** The doc's full rule is "distinguished by shape … and, where ambiguous, resolvable against ground truth (`go doc` for a symbol, file existence for a path)".
  This decision takes the shape half and **declines the ground-truth half**, on purpose:
  a `go doc` call is a process spawn, which the Test Tier Purity Invariant bars from tier1 and which would stop `planparser` being a leaf.
  The file-existence half of that clause is not lost — it survives as the `path-missing` check, which already stats the disk, just at validation time rather than as a classification input.
  The rewritten spec must state this deviation and its reason explicitly, so the spec does not read as silently contradicting the design doc.
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
  Those requirements are enforced by `card-missing-field` (for `Intent:` and the conditional `ImpactSummary:`) and `card-type-missing` (for the type label) — see `validator-checks`;
  no required field is left without an enforcing check.
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
  This also **closes the design doc's open item "Whether `ImpactSummary` on Delete needs a structured shape beyond one line of prose": it does not.** A `Delete` card's `ImpactSummary` is one line of free prose, identical in shape to an `Edit` card's.
  A structured shape would need a caller enumeration planparser cannot produce without the symbol lookup this task explicitly excludes, and inventing a schema no producer can fill is worse than prose.
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

- **Decision:** the check set moves from **15 to 16**.

  First, a metric mismatch the migration must resolve — **not** a miscount, as an earlier draft of this discussion wrongly claimed.
  The repo's "14" and this discussion's "15" count two different things:
  - **14** is the **row count** of `contracts/specs/loom-plan-spec.md:200-217`'s numbered list, whose row 1 bundles two IDs (`format-unrecognized` / `plan-unapproved`).
    The three Go comment sites cite that row count, so they are internally consistent with the spec today.
  - **15** is the count of **distinct `Check:` IDs** `internal/planparser/validate.go` emits: `format-unrecognized`, `plan-unapproved`, `index-file-mismatch`, `card-path-malformed`, `move-format`, `move-redundant`, `move-source-missing`, `move-target-collision`, `move-mechanic-missing`, `card-missing-field`, `card-field-overlap`, `card-numbering`, `path-missing`, `commit-subject-mismatch`, `depends-on-order`.
    Reproduce with `grep -o 'Check: *"[a-z-]*"' internal/planparser/validate.go | grep -o '"[a-z-]*"' | sort -u | wc -l`.

  **Decision: the figure is a count of distinct `Check:` IDs**, and the rewritten spec's numbered list must carry **one row per ID** — `format-unrecognized` and `plan-unapproved` unbundled into their own rows.
  Otherwise a 15-row list under a "16 checks" banner recreates exactly the row-count-versus-ID-count discrepancy this task is resolving.
  A finding's ID is what a consumer greps for and what `ValidationError.Check` carries;
  a row is a presentation choice.

  Disposition of every existing check:

  | Existing check | Disposition |
  |---|---|
  | `format-unrecognized` | Keep, `recognizedFormat` 3 → 4 |
  | `plan-unapproved` | Keep unchanged |
  | `index-file-mismatch` | Keep unchanged |
  | `card-numbering` | Keep unchanged |
  | `commit-subject-mismatch` | Keep unchanged |
  | `card-path-malformed` | Rework — applies only to path-shaped entries |
  | `path-missing` | Rework — path-shaped entries only, **and type-conditional**; see `path-missing-rework` |
  | `card-field-overlap` | Rework — now means "an entry appears in both this card's target list and its own `Uses:`" |
  | `card-missing-field` | **Keep the ID, retarget the required set** — now enforces `Intent:` on every card, and `ImpactSummary:` on `Edit`/`Delete` cards |
  | `move-format` | Renamed `rename-format` |
  | `move-mechanic-missing` | Renamed `rename-mechanic-missing` (see `rename-mechanic-section`) |
  | `move-redundant` | Drop |
  | `move-source-missing` | Drop |
  | `move-target-collision` | Drop |
  | `depends-on-order` | Drop |

  New checks (5): `card-type-missing` (a card carries zero, or more than one, recognized type label), `impact-summary-multiline`, `card-field-empty`, `prosa-symbol-target` (a `Prosa` card's entries must all be path-shaped), and `card-retired-label` (a format-4 card carrying a format-3 field label — see `retired-label-disposition`).

  **`Custom` is exempt from type-specific checks only**, never from the card-generic ones — the full split is enumerated in `path-missing-rework`.
  "No mechanical check" for `Custom` means no *type-specific* rule, not a blanket skip.

  **Required-field presence stays one check, not three.** `card-missing-field` keeps its ID and its existing `[]cardFieldLabel` iteration shape in `checkCardMissingField`;
  only the required set it iterates changes, from the seven old labels to `Intent:` plus the conditional `ImpactSummary:`.
  A card with no `Intent:` therefore produces a `card-missing-field` finding, exactly as a card with no `What:` does today.
  `card-type-missing` is separate because it is not a missing-field condition — it also fires when a card carries **two** type labels, which no presence check can express.

  **Arithmetic:** 15 old − 4 dropped = 11 carried (5 keep + 3 rework + 1 retarget + 2 rename) + 5 new = **16**.
  That is the number to write into the rewritten spec's banner and every stale-figure site listed under `stale-comments`.

- **Rationale:** the three dropped `move-*` checks all rest on a mechanical `old -> new` pair plus a destination path resolvable against other cards' `Creates:`/`Deletes:` — and `Creates:`/`Deletes:` no longer exist as fields (see `rename-grammar`);
  `Move`'s destination lives in `Intent` prose by design, so there is nothing to cross-check.
  `depends-on-order` has no field left to check.
  This closes the design doc's open item "Reconciliation with `contracts/specs/loom-plan-spec.md`'s existing 14 validator checks" — the doc's "14" cites the spec's row count and is corrected to 16 by Sweep 3 along with every other site.
- **Rejected:** preserving the `move-*` checks by giving `Move` a mechanical destination field (departs from the design doc);
  stripping to format/approval/index-consistency only and deferring content checks to a later hardening pass (leaves the new format almost unchecked at exactly the moment it is least understood).

### retired-label-disposition

- **Decision:** the **eight** retired labels — `**What:**`, `**Context:**`, `**Edits:**`, `**Creates:**`, `**Deletes:**`, `**Moves:**`, `**Depends-on:**`, and the format-3 lowercase spelling `**verify:**` — stay **recognized by the parser** even though no field consumes them.
  The eighth is easy to miss: `field-mapping` recapitalizes the card field to `**Verify:**`, and `parse.go:307`'s `cardVerifyLabel = "**verify:**"` is matched case-sensitively via `strings.HasPrefix`, so a stale lowercase line would otherwise fall through `default: i++` and vanish silently — the exact misparse class this decision exists to prevent.
  `card-retired-label` reports it with the mapping `**verify:**` → `**Verify:**`.
  **Deliberate asymmetry:** only the *card* field is recapitalized.
  The *plan-level* `## verify:` section heading stays lowercase, because `sections.go` is unchanged (`planVerifyHeading = "## verify:"`) and nothing in the design doc asks for it to move.
  Concretely:
  - They remain in `cardLabels`, so `isCardLabelLine` still returns true for them.
    This is load-bearing: without it, a stray `**Context:**` line in a half-migrated card is swallowed into `Intent:`'s collect-until-next-label prose instead of terminating it.
  - `parseCardBody` routes each occurrence into a new `RetiredLabels []string` slot on `Card` rather than into any field, and consumes its bullets without storing them.
  - A new check, **`card-retired-label`**, reports one finding per occurrence, naming the label and its format-3 → format-4 mapping.
- **Rationale:** `parseCardBody`'s fallthrough is `default: i++` (`parse.go:388-389`), so a label that simply leaves `cardLabels` is silently discarded.
  A half-migrated card would then parse clean and validate clean while carrying instructions nobody reads — exactly the silent misparse this format's fail-loud discipline exists to prevent, and worse than the fully-stripped card `card-type-missing` already catches.
  Keeping them recognized also gives the migration a real diagnostic: an author or a stale producer writing format-3 fields into a format-4 card is told which field became what.
- **`ImpactSummary:`'s trailing lines are retained for the same reason.** `impact-summary-multiline` cannot report lines the parser has already thrown away, so the `ImpactSummary:` branch consumes trailing non-label lines into an `ImpactSummaryTrailing []string` slot and the check fires when it is non-empty.
  This is the same lenient-capture pattern `MovesRaw` uses today, retargeted.
- **Rejected:** a hard `ParsePlan` error on a retired label (fail-loud, but a single stale label would abort the whole plan parse instead of enumerating every defect in one pass, breaking `doc.go`'s lenient-card-parse decision);
  accepting the silent drop (contradicts this discussion's own "a loud validation finding, never a silent misparse" rule).
- **Not in scope:** a general `card-unknown-label` check for arbitrary unrecognized bold labels.
  That is today's pre-existing behavior and is unchanged by this migration;
  widening it here would be a new feature, not a migration.

### path-missing-rework

- **Decision:** `path-missing` stays, but becomes type-conditional and keeps two union helpers under new definitions.
  - **What it checks:** path-shaped entries in any card's `Uses:`, and path-shaped targets of `Edit`, `Delete`, `Move`, and `Prosa` cards, plus a `Rename` pair's `Old` side.
  - **What it never checks:** a `Create` card's targets (by definition they do not exist yet), a `Rename` pair's `New` side (the post-rename path does not exist yet either), and **`Custom` card targets** — a `Custom` card used to create something would otherwise produce a spurious finding for doing exactly what the escape hatch is for.

    **`Custom` is exempt from *type-specific* checks only, never from the card-generic ones.** Stated precisely so no implementer writes a blanket skip:
    - **Exempt:** `path-missing` **on its own targets only** — a `Custom` card's `Uses:` paths are checked exactly like any other card's, since `Uses:` means read-not-written for `Custom` too and the escape-hatch rationale (a `Custom` card creating something) applies only to targets.
      Also exempt from any target-shape rule (`prosa-symbol-target`'s analogue).
    - **Still binding, like every other card:** `card-type-missing`, `card-missing-field` (a `Custom` card still needs `Intent:`), `card-field-empty`, `card-field-overlap`, `card-path-malformed`, `card-retired-label`, `card-numbering`, `commit-subject-mismatch`.
    Well-formedness is not existence: a `Custom` card may name a path that does not exist yet, but it may not name a malformed one.
  - **What satisfies an otherwise-missing path:** `createTargetsUnion` — the union, across the plan, of every `Create`-type card's path-shaped targets — and `renameTargetsUnion` — the union of every `Rename` pair's `New` side.
    These are today's `createsUnion`/`movesTargetsUnion` renamed and redefined, **not deleted**;
    the Technical context's earlier claim that both "go away" was wrong, since `checkPathMissing` uses both today (`validate.go:528-530`).
- **Rationale:** without the two unions, every path a card legitimately creates or renames into place becomes a spurious `path-missing` finding on the card that later reads it — turning a useful check into noise the author learns to ignore.
  Without the type conditions, a `Create` card is flagged for creating something that does not exist yet, which is its entire purpose.
- **Known limitation, to be documented in the spec:** a `Move` card states its destination in `Intent` prose, not in a field, so there is no `moveTargetsUnion` to build.
  A later card whose path-shaped entry names a file an earlier `Move` relocated into place will produce a **false-positive** `path-missing` finding.
  This is accepted: `Move` is the rarest type, the failure is a loud finding rather than a silent misparse, and the author resolves it by reordering or by naming the pre-move path.
- **Rejected:** suppressing `path-missing` plan-wide whenever any `Move` card is present (mechanical and simple, but disables a useful check across the entire plan to serve the rarest type);
  giving `Move` a mechanical destination field to build the third union (already rejected under `rename-grammar` for departing from the design doc).

### rename-grammar

- **Decision:** bullet grammar is type-conditional.
  A `Rename` card's bullets parse against the existing `` `old` -> `new` `` regex into `[]MovePair` — the type is reused, not deleted.
  Every other type's bullets are plain refs.
  A `Rename` bullet failing the pair grammar lands in a `RenameRaw` slot, exactly as `MovesRaw` works today, and the renamed `rename-format` check reports it.
- **Decision — how `Rename` relates to `Targets`:** a `Rename` card populates **both** `Pairs []MovePair` *and* `Targets`, projecting **both endpoints** of every pair into `Targets` in pair order (`Old`, then `New`).
  `Pairs` is the structured form the rename mechanic and `rename-format` work from;
  `Targets` is what every target-list-based consumer reads, so `Rename` is not a hole in the model.
  Consequences, stated so no consumer has to guess:
  - `card-field-overlap` sees both endpoints, so a card that renames `X` and also lists `X` in its own `Uses:` is correctly flagged.
  - `card-path-malformed` and the classifier run over both endpoints.
  - `path-missing` runs over `Old` only — see `path-missing-rework`.
  - Wave 3's edge derivation gets both names, which is what it needs: a later card depending on the post-rename name (`New`) and a card depending on the pre-rename name (`Old`) both genuinely depend on this card.
  `Targets` is therefore never empty for any card type, including `Rename`.
- **Rationale:** the design doc's card-type table gives `Rename` "existing symbol(s), `old -> new` pairs" and `Move` "destination stated in `Intent`, not the list".
  That asymmetry is deliberate and has to be reflected in the parser.
  Retargeting `MovesRaw`'s lenient-capture mechanism keeps the parse-lenient discipline intact for the one type that still has a grammar to fail.
- **Rejected:** uniform plain-string bullets everywhere with `MovePair` deleted (simplest parser, but no mechanical rename checking ever);
  giving `Move` a mechanical `To:` field to restore the collision checks (departs from the design doc).

### rename-mechanic-section

- **Decision:** the plan-level `## Rename mechanic` section survives.
  `Plan.RenameMechanic` and `sections.go` are unchanged.
  `move-mechanic-missing` is renamed `rename-mechanic-missing` and fires when any card is type `Rename` and the section is absent.
- **Rationale:** this is a deliberate partial amendment to `validator-checks` — of the five `move-*` checks, **three drop and two survive renamed** (`move-format` → `rename-format`, `move-mechanic-missing` → `rename-mechanic-missing`), which is what the disposition table and the 15 − 4 arithmetic both encode.
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
- **Decision — how much each stencil restates, per the Producer Pointer-Rule Invariant:** both stencils are instruction files, so neither may duplicate or paraphrase `loom-plan-spec.md`'s format contract;
  each may only point at it.
  Concretely:
  - `loom-template-plan.md` is `Plan-Write`'s own producer prompt, so it legitimately carries the **LLM-facing subset** — what the producer must write — which the spec's own banner already names as a deliberate, pinned-elsewhere split ("so the agent's prompt never duplicates this file and the two cannot drift from being the same doc").
    That split survives this task unchanged;
    the rewrite updates the subset's field names and grammar without widening it.
    Any validator rule the stencil currently spells out that the spec also spells out is a pointer candidate, not something to re-paraphrase in the new shape.
  - `webster-body-implementer.md` is a **consumer** prompt and must restate even less: it needs the deviation-union definition (see `deviation-union`) and the fact that a card names its targets by type label, and it should point at the spec for the grammar rather than reproduce it.
    Its current text spells out `Context:`/`Edits:`/`Creates:`/`Deletes:`/`Moves:` twice;
    the rewrite is the moment to reduce that to one pointer plus the union rule.
- **Rationale for calling this out:** a mechanical find-and-replace of field names across both stencils would preserve today's duplication and quietly re-commit the Producer Pointer-Rule violation in the new vocabulary.
  The rewrite has to reduce duplication, not translate it.
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
  3. **Every stale check-count figure.** Do not work from a hand list — the card-format carriers get two re-runnable sweeps above, and this deserves its own.
     **Sweep 3:**

     ```
     grep -rn "14 checks\|fourteen checks\|14 validation checks\|14 validator checks" --include=*.go --include=*.md . | grep -v '^./.git/' | grep -v '_mill/'
     ```

     As of this discussion that hits `contracts/specs/loom-plan-spec.md:3` (the "fourteen checks below" banner), `internal/planparser/validate.go:17`, `internal/planparser/validate_test.go:89`, `internal/planparser/doc.go:58`, `internal/webstercli/validate.go:47`, `manifest/designs/plan-card-format.md:84`, and `manifest/designs/scout-plan-symbol-fields.md:64`.
     Note that a single file can carry more than one occurrence (`validate_test.go` has three, at lines 1, 9, and 89), which is why the sweep uses `-n` rather than `-l` and why no fixed site count is stated here.

     Every hit becomes **16**, the post-migration count of distinct `Check:` IDs — see `validator-checks` for why the figure counts IDs and not spec rows, and for the requirement that the rewritten spec's list carry one row per ID.

     `manifest/designs/scout-plan-symbol-fields.md:64` is a **figure-only** correction, deliberately narrow: that doc as a whole is stale and its substantive reconciliation stays out of scope (see Scope: Out).
     Correcting a number is not reconciling a document, and skipping one site out of a mechanical sweep would be arbitrary.
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
  `parseDependsOnField` and `noneSentinel` go away, but the retired **label constants stay** in `cardLabels` per `retired-label-disposition` — deleting them is the specific mistake that turns a half-migrated card into a silent misparse via `parseCardBody`'s `default: i++` fallthrough (`parse.go:388-389`).
  Note `parseFileOpField` currently returns `[]string{}` for the `none` sentinel and errors on an inline value — under `field-presence` the `none` branch is deleted and the inline-value error stays.
- `normalize.go` (60 lines) — `normalizeCardPath`, `hasWorktreeRootEscape`, `cleanPosixPath`, `normalizeCard`, `normalizePathSlice`.
  Only `normalizeCard` changes shape (it must consult the classifier);
  the rest is reusable as-is.
- `validate.go` (616 lines) — `Validate`'s fixed call sequence plus one `checkXxx` function per check, `ValidationError`, `cardID`, and the helpers `createsUnion`, `movesTargetsUnion`, `pathExistsOnDisk`, `cardPathMalformedReason`, `cardHeadingNumber`.
  `createsUnion` and `movesTargetsUnion` **survive, renamed and redefined** as `createTargetsUnion`/`renameTargetsUnion` — `checkPathMissing` uses both today (`validate.go:528-530`), so deleting them would break a kept check;
  see `path-missing-rework`.
  `pathExistsOnDisk`, `cardPathMalformedReason`, `cardHeadingNumber`, and `cardID` all survive unchanged.
  Note also that `checkPathMissing` today deliberately skips `CreatesFiles` (`validate.go:533`);
  the type-conditional rework preserves that intent by skipping `Create` targets.
- `sections.go` (69 lines) — unchanged.
- `doc.go` (62 lines) — the package doc describes the five typed file-op fields, the `none` sentinel's three-state model, and "the plan format's 14 validation checks" (a Sweep 3 site — see `stale-comments`).
  All three passages need rewriting.
- `testdata/goodplan/` — a four-card golden fixture (`00-overview.md` + `01-json-flag.md` … `04-helptree-rename.md`) that is a materialization of the spec's worked example.
  It uses `root: internal/boardcli`, a `//`-escaped path, a `Depends-on:` chain, a pinned `Commit:`, and a `Moves:` card with the `## Rename mechanic` section.

**Consumers outside `planparser` — much smaller than the roadmap entry implies.**

- `internal/loomshed/planvalidate.go` — calls `planparser.ParsePlan` then `planparser.Validate` and maps a non-empty `[]ValidationError` to `shedengine.Stuck`. **No field reads.** Needs no change;
  its test fixture does.
- `internal/batcher` — `Batch{Cards []planparser.Card}` and the identity batcher pass cards through. **No field reads.** No change.
- `internal/websterengine` — `render.go` reads only `Card.SourcePath` (in `renderCardPointers`) and `Number`/`Slug`/`Intent` in `RenderBatchIndex`.
  `RenderProgress` (`render.go:277-294`) reads `Number`/`Slug` only.
  The `Intent` → `Summary` rename therefore touches exactly one function, **`RenderBatchIndex` (`render.go:268`)**.
  `runlevel.go` calls `ParsePlan`/`Validate`. `beginbatch.go`/`recoverbatch.go`/`integration.go` hold a `*planparser.Plan` and read `Plan.Verify`.
  **No non-test file reads any file-op field.**
- `internal/loomengine/plan.go` — `PlanSpec` composes the `loom-template-plan` stencil prompt.
  No field reads;
  but `plan_test.go` has stencil-pinning tests (`TestPlanSpec_PromptStatesContextSemantics`, `..._PromptStatesMoveRedundantRule`, `..._PromptStatesMovedFileNotInEdits`, `..._PromptStatesDependsOnCriterion`, `..._PromptStatesCardCriteria`, `..._PromptStatesRootResolution`, `..._PromptStatesVerifyIsRunnable`) asserting the old rules are present in the stencil text.
  These must be retargeted alongside the stencil rewrite.

**Files carrying a literal old-format card body or a `format: 3` plan.**

Two disjoint sweeps are needed, because the two carrier kinds fail differently.

**Sweep 1 — markdown/instruction carriers (the compiler cannot see these).** Re-run before planning:

```
grep -rln '\*\*What:\*\*\|format: 3' internal/ tools/ contracts/ cmd/
```

**Sweep 2 — Go-level model carriers.** These do **not** match Sweep 1's pattern, because they construct a `Card` in Go rather than writing card markdown.
They are caught by `go build ./... && go test ./...` once the fields are renamed, so the compiler is the authority;
this grep is only to see them ahead of time:

```
grep -rln 'MovePair\|\.Moves\|ContextFiles\|EditsFiles\|CreatesFiles\|DeletesFiles\|DependsOn\|HasWhat' internal/ cmd/
```

`internal/websterengine/template_test.go` is a Sweep-2 carrier only — its sole coupling is `card.Moves = []planparser.MovePair{...}` at line 759, and it contains neither `**What:**` nor `format: 3`.
A plan writer running Sweep 1 alone will not see it.

As of this discussion Sweep 1 yields, beyond `internal/planparser`'s own sources, tests, and `testdata/goodplan/`:

- `internal/loomshed/planvalidate_test.go`
- `internal/websterengine/runlevel_test.go`
- `internal/loomrecipe/fixture_test.go`
- `internal/loomcli/validate_test.go` — the `validate-plan` verb's fixture, coupled to the Gate Self-Check Parity Invariant
- `internal/webstercli/cli_test.go`
- `contracts/specs/loom-plan-spec.md`, `contracts/stencils/loom/loom-template-plan.md`, `contracts/stencils/webster/webster-body-implementer.md` (already in scope above)
- **`tools/sandbox/SANDBOX-WEBSTER-SUITE.md`** — an **agent-facing instruction file, not a Go fixture.**
  This one will **not** be caught by the `go build ./... && go test ./...` backstop the Testing section relies on.
  It has to be found by the grep and fixed deliberately, or the sandbox suite will instruct an agent to write a format the parser rejects.

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
- **Producer Pointer-Rule Invariant** — an instruction file (a producer's prompt or a skill) must never duplicate or paraphrase another producer's format-contract content, only point at it, so that editing the one format-contract file is sufficient to change what both producer and consumers do.
  Both stencils in scope are instruction files;
  see the `doc-reach` decision for exactly how much each may restate.
  Enforced by review obligation, so a reviewer will check this — the rewrite must reduce duplication rather than translate it into the new field names.
- **Gate Self-Check Parity Invariant** — Plan-Validate's `ShedProducer` row and the `lyx loom validate-plan` verb both call `planparser.Validate`, and neither re-implements the other's checks;
  the verb's envelope distinguishes a findings failure from an I/O fault structurally, by the presence of the `findings` key.
  Enforced by `internal/loomcli/parity_test.go` (`TestGateParity_PlanValidate`).
  Changing the check set must not break that parity, and `internal/loomcli/validate_test.go`'s fixture — which carries an old-format plan — is in scope for the fixture sweep below.

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
  Cover `internal/x/y.go`, bare `list.go`, `//cmd/lyx/main.go`, `shedrecipe.Lookup`, a bare `Lookup` and a bare `Makefile` (both no-dot → symbol, pinning rule 3's default), and the documented `shedrecipe.lookup` misclassification so the limitation is pinned rather than discovered later.
  **Deliberately no fuzz test** — the classifier is a small pure function fully covered by a table, and fuzzing it would be scope beyond a bounded migration with no observed problem to solve.
- **One test per validator check.** Every surviving, reworked, and new check from the `validator-checks` table gets its own focused test.
  The five `move-*` tests and the `depends-on-order` test are **deleted, not adapted** — retrofitting a dropped check's test onto a new check produces a test that documents the wrong thing.
- **Golden all-checks-pass test**: `validate_test.go`'s existing "all checks pass simultaneously on the happy path" test, retargeted to the new count.
- **Parse-lenient scenarios**: a label present with no bullets (`card-field-empty`), a card with two type labels and a card with none (`card-type-missing`), a malformed `Rename` bullet landing in `RenameRaw` (`rename-format`), a multi-line `ImpactSummary` whose trailing lines reach `ImpactSummaryTrailing` (`impact-summary-multiline`), a `Prosa` card with a symbol target (`prosa-symbol-target`), and a **half-migrated card carrying a retired `**Context:**` label, plus one carrying the lowercase `**verify:**`** (`card-retired-label`) — the last two must also assert the retired label terminated `Intent:`'s prose collection rather than being swallowed into it, and the `**verify:**` case specifically pins that the case-sensitive match does not let it slip through as unrecognized text.
- **Fail-loud scenarios stay fail-loud**: an inline value on a field admitting only bullets must still be a `ParsePlan` error, not a finding.

**`internal/loomengine` — stencil-pinning tests.**

Retarget the seven `TestPlanSpec_PromptStates*` tests to assert the new rules.
`..._PromptStatesMoveRedundantRule`, `..._PromptStatesMovedFileNotInEdits`, and `..._PromptStatesDependsOnCriterion` pin rules that no longer exist and should be deleted;
add pins for the new type-label grammar, the `Uses:` semantics, and the `ImpactSummary` requirement.

**`internal/loomcli` — gate parity.**

`validate_test.go`'s fixture carries an old-format plan and must be rewritten to format 4.
`parity_test.go`'s `TestGateParity_PlanValidate` must still pass unchanged — the check set changes, but both the `ShedProducer` row and the `validate-plan` verb still call `planparser.Validate`, so parity is structural and should survive.
If it does not, that is a signal something re-implemented a check rather than calling through.

**Fixture-only updates** (`loomrecipe`, `loomshed`, `websterengine`, `webstercli`): rewrite each literal card body to format 4.
These are not new coverage — they must simply stop describing a format the parser rejects.
`websterengine/template_test.go:759`'s `card.Moves` assignment becomes a `Rename`-type card.

**`tools/sandbox/SANDBOX-WEBSTER-SUITE.md` — no test covers this.**
It is an agent-facing instruction file.
Rewriting it is a required step that the green-tree gate cannot verify;
whoever plans this must treat it as its own explicit item, not a trailing edit.

**Whole-tree gate:** `go build ./... && go test ./...`. The migration is only complete when the tree is green — the compiler is the real backstop for the field renames, since `Card.Intent` → `Card.Summary` and the removal of six field names will surface every missed reader.
The gate does **not** cover the two markdown instruction files (`SANDBOX-WEBSTER-SUITE.md` and the two stencils), so those need the grep sweep above as their own verification.

## Q&A log

- **Q:** How far does the doc/stencil rewrite reach? **A:** Spec rewritten in full, plus both stencils but only their mechanical field-name/grammar/validator content — the Wave-3 prompt redesign in `loom-template-plan.md` is not this task's job, while the live `webster-body-implementer.md` must name fields that exist.
- **Q:** What happens to `What:`, `Depends-on:`, `Commit:`, `verify:`, and the Card Index one-liner? **A:** `Intent:` replaces `What:`;
  `Depends-on:` dropped;
  `Commit:` and `Verify:` survive;
  `Card.Intent` renamed `Card.Summary`.
- **Q:** Which validator checks survive, and is the repo's "14" wrong? **A:** "14" is not a miscount — it is the **row count** of the spec's numbered list, whose row 1 bundles `format-unrecognized`/`plan-unapproved`, while **15** is the count of distinct `Check:` IDs the code emits.
  This task settles on the ID count as the figure and requires the rewritten spec to carry one row per ID, so the two metrics stop diverging.
  Of those 15: keep 5 unchanged, rework 3, retarget 1 (`card-missing-field`), rename 2, drop 4, add 5 — **16**.
  See the `validator-checks` table.
- **Q:** What enforces a missing `Intent:`? **A:** `card-missing-field`, which keeps its ID and its `[]cardFieldLabel` shape and simply iterates the new required set (`Intent:`, plus `ImpactSummary:` on Edit/Delete).
  `card-type-missing` stays separate because it also fires on *two* type labels, which a presence check cannot express.
- **Q:** How is a card's mixed symbol/path target list modelled in Go? **A:** One flat `Targets []string` plus a `Type` enum, classified by shape at validation time — "distinguished by shape" is verbatim from the design doc, and shape-based classification at validation keeps the parser dumb, in line with nothing being inferred or guessed at parse time.
- **Q:** How is `Rename`'s pair grammar reconciled with `Move` stating its destination in prose? **A:** Type-conditional bullet grammar — `Rename` reuses the arrow regex and `MovePair`, everything else is plain refs, and a failed `Rename` pair lands in `RenameRaw`.
- **Q:** Bold-label type key, or a separate `Type:` field? **A:** Bold label (`**Edit:**`), reusing the existing bullet machinery;
  a separate `Type:` field is explicitly banned by the design doc.
- **Q:** How is `root:` reconciled with symbol references? **A:** One shared shape classifier gates normalization — path entries get root-resolved, symbols pass through verbatim.
- **Q:** Dual-read `format: 3`? **A:** No. Bump to 4 and hard-reject 3.
- **Q:** Is the three-tier Verify model implemented here? **A:** Specified only.
  Implementing tier1 would be exactly the behavior change the roadmap entry rules out, so spec-only is the correct scope, not merely the recommended one.
- **Q:** What happens to a card that still carries a retired label like `**Context:**`? **A:** The eight retired labels — including the format-3 lowercase `**verify:**`, which is matched case-sensitively and would otherwise vanish once the card field becomes `**Verify:**` — stay recognized by the parser — otherwise a stray one is swallowed into `Intent:`'s prose — are routed to a `RetiredLabels` slot, and each produces a `card-retired-label` finding naming its format-3 → format-4 mapping.
  A hard parse error was rejected for aborting the whole plan instead of enumerating every defect in one pass.
- **Q:** How can `impact-summary-multiline` report lines the parser discards? **A:** It cannot, so it does not discard them — the `ImpactSummary:` branch captures trailing non-label lines into `ImpactSummaryTrailing`, the same lenient-capture pattern `MovesRaw` uses today.
- **Q:** How does `path-missing` survive when `Creates:` is no longer a field? **A:** It becomes type-conditional and keeps both union helpers under new names — `createTargetsUnion` (every `Create` card's path-shaped targets) and `renameTargetsUnion` (every `Rename` pair's `New` side).
  It never checks a `Create` target or a `Rename` pair's `New` side.
  A `Move` destination lives in prose, so there is no third union;
  the resulting false positive is accepted and documented rather than suppressed plan-wide.
- **Q:** Does a `Rename` card populate `Targets`, or only `Pairs`? **A:** Both — both endpoints project into `Targets` in pair order, so no target-list-based check or Wave 3 edge derivation has a hole for `Rename`.
  `path-missing` is the one consumer that looks at `Old` only.
- **Q:** How is the old-format fixture inventory established? **A:** By re-running `grep -rln '\*\*What:\*\*\|format: 3' internal/ tools/ contracts/ cmd/`, not by trusting a hand list.
  Two sweeps, not one: a markdown/instruction sweep the compiler cannot see, and a Go-model sweep the compiler catches anyway.
  `internal/websterengine/template_test.go` appears only in the second.
  The first surfaces three carriers an initial hand list missed (and the roadmap ownership of the two stale design docs is `roadmap.md:14`/`:61-62`/`:136-138`, not the Wave 3 item) — `internal/loomcli/validate_test.go`, `internal/webstercli/cli_test.go`, and `tools/sandbox/SANDBOX-WEBSTER-SUITE.md`, the last of which the green-tree gate cannot catch because it is an agent-facing markdown instruction file.
- **Q:** Does `ImpactSummary` on `Delete` need a structured shape? **A:** No — one line of prose, identical to `Edit`.
  Closes the third of the design doc's open items;
  a structured shape would need caller enumeration planparser cannot produce without the symbol lookup this task excludes.
- **Q:** Does the shape classifier follow the design doc's rule completely? **A:** No, and the deviation is deliberate.
  The doc's rule has a second clause — resolve ambiguity against ground truth via `go doc` — which is declined because a process spawn breaks the Test Tier Purity Invariant and stops `planparser` being a leaf.
  The file-existence half survives as the `path-missing` check.
  The rewritten spec must state this rather than let it read as a silent contradiction.
- **Q:** How do the stencil rewrites interact with the Producer Pointer-Rule Invariant? **A:** A mechanical field-name find-and-replace would preserve today's duplication in new vocabulary and re-commit the violation.
  `loom-template-plan.md` keeps only its pinned LLM-facing subset;
  `webster-body-implementer.md` reduces to a pointer plus the deviation-union rule.
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
- **Q:** Type-specific checks for `Prosa`, `Custom`, `Create`, `Delete`? **A:** Required `ImpactSummary:` on Edit/Delete is enforced by the retargeted `card-missing-field`, not by a separate check;
  plus `rename-format` and `prosa-symbol-target`.
  `Custom` gets none — a principled closure of that design-doc open item in the affirmative, not an oversight.
- **Q:** What about the two stale design docs? **A:** Left alone.
  The roadmap already assigns `webster-parallel-execution.md` to the Wave 3 DAG task.
- **Q:** The now-false "no symbol fields" claim in `websterengine/doc.go`? **A:** Corrected in this task, comment-only;
  the seam stays dead.
  Also fixes every stale "14 checks" figure via Sweep 3, settling the row-count-versus-ID-count ambiguity in favour of distinct IDs.
  No new `CONSTRAINTS.md` invariant.
- **Q:** Does the plan-level `## Rename mechanic` section survive? **A:** Yes, with `move-mechanic-missing` renamed `rename-mechanic-missing` and retargeted to `Rename` cards.
  This is a genuine revision of the drop-all-`move-*` answer, not another instance of it: the AST-aware requirement makes the check *more* valuable than it was, and letting it fall would open exactly the text/regex rename the design doc bans.
