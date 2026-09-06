# Batch: planparser-handles

```yaml
task: "Adopt quarry's glyph alphabet as the plan alphabet"
batch: "planparser-handles"
number: 3
cards: 7
verify: go test ./internal/planparser/ ./cmd/lyx/
depends-on: [2]
```

## Batch Scope

This batch adds the second half of format 5 to the pure leaf: `plan:` placeholder handles and the grammar that declares them, the two new write paths (`RewriteRefs` and the amendment append) that the Planparser Sole-Parser Invariant must now name, and the syntactic tier of the cross-granularity containment check.
It is one batch because the handle grammar, the checks that keep handles internally consistent, and the write path that later rewrites them are one mechanism seen at three moments, and because all three new `CONSTRAINTS.md` obligations this half creates land together with the code that creates them.
The external interface batch 4 consumes is `planparser.RewriteRefs`, `planparser.AppendAmendment`, the `Card`-level handle accessors, and the syntactic containment findings `planglyph` composes on top of.

Batch-local decisions, beyond `## Shared Decisions`:

- **A handle has no reality to point at.** It only has to be internally consistent, which is fully mechanically checkable, so letting the planner invent the *draft* spelling is safe in a way that letting it invent a real glyph is not. `quarry` never sees a handle.
- **`RewriteRefs`' map is keyed on canonical model strings**, not on-disk lexemes, because all three of its callers natively produce canonical glyphs.
  The surface↔model gap is bridged once here, in the parser, via `Plan.SurfaceRefs` from card 5.
- **The amendment append is a second write path, not a variant of the first.** It appends to a file `RewriteRefs` never touches and substitutes nothing.

## Cards

### Card 9: add the `plan:` handle spelling and the Create declaration grammar

- **Context:**
  - `internal/planparser/classify.go`
  - `internal/planparser/glyphref.go`
  - `contracts/specs/loom-plan-spec.md`
- **Edits:**
  - `internal/planparser/parse.go`
  - `internal/planparser/plan.go`
  - `internal/planparser/parse_test.go`
- **Creates:**
  - `internal/planparser/handle.go`
  - `internal/planparser/handle_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Extend a `**Create:**` group's sub-bullet grammar to the two-field form `` `plan:<draft-handle>` -> `<declaration head>` ``, reusing the arrow grammar `**Rename:**` already parses.
  `quarry.Name` takes a `{Unit, Decl}` pair: the unit is already inside the handle, left of the `#`, and the declaration head is nowhere in today's format, so this one bullet supplies both halves from one place and keeps the declaration adjacent to the handle it declares.
  In the new file `handle.go`, declare `const HandlePrefix = "plan:"` as this package's sole declarer of that literal, and add `splitHandleDeclaration(payload string) (handle, decl string, ok bool)` matching the arrow form with the same `` `x` -> `y` `` shape `moveLineRe` uses, plus `handleUnit(handle string) (string, bool)` returning the substring left of the first `#` after the prefix is stripped.
  `handleUnit` performs no glyph parsing and is not a glyph↔path conversion: it splits a handle, which is loomyard's own token, never a quarry answer.
  In `plan.go`, add `Declarations []CardDeclaration` to `Card` and to `TargetGroup`, and declare `type CardDeclaration struct{ Handle, Decl string }`, documented as one `Create` sub-bullet's handle and declaration head, both verbatim.
  In `parse.go`, do **not** route the arrow form through `parseRefField`: that function calls `stripBackticks` on each payload before returning it, so `` - `plan:X` -> `Decl` `` would arrive as `plan:X` -> `Decl` with the outer backtick pair already removed and could never match the two-backticked-token shape.
  Give `createLabel` its own field parser, `parseCreateField(labelLine string, lines []string, start int) (refs []string, decls []CardDeclaration, raw []string, next int, err error)`, mirroring `parseRenameField`, which already matches `moveLineRe` against the **unstripped** payload for exactly this reason.
  For each sub-bullet: try `splitHandleDeclaration` on the raw payload first; on a match, append a `CardDeclaration` and append the handle alone to `refs`; on no match, fall back to today's `stripBackticks` single-ref behaviour and append that; and capture a payload that contains the ` -> ` arrow but fails the grammar into `raw` rather than dropping it.
  Wire it into `parseTypeLabelCase`'s `createLabel` branch alongside the existing `renameLabel` branch, appending `refs` to both the group's `Refs` and the card's `Targets` so a handle participates in `Targets` exactly as any other ref does.
  Name the capture field the way `RenameRaw` is named: add `CreateRaw []string` to both `TargetGroup` and `Card` in `plan.go`, populated only for a `CardTypeCreate` group, and documented as the field card 10's `checkHandleMalformed` reads.
  Cover in tests: a well-formed handle declaration parsed into both `Declarations` and `Targets`; a plain non-arrow `Create` ref still parsing as today; a malformed arrow bullet landing in `CreateRaw` rather than being silently dropped; a bullet whose backticks are intact reaching the arrow matcher unstripped, which is the regression this parser split exists to prevent; and a handle surviving `normalizeCard` without picking up a `root:` prefix, which `classifyRef` already guarantees by classifying it `refKindHandle`.
- **Commit:** `9: feat(planparser): add plan: handles and the Create declaration-head grammar`

### Card 10: add the handle consistency checks

- **Context:**
  - `internal/planparser/parse.go`
  - `internal/planparser/plan.go`
  - `internal/planparser/classify.go`
- **Edits:**
  - `internal/planparser/handle.go`
  - `internal/planparser/handle_test.go`
  - `internal/planparser/validate.go`
  - `internal/planparser/validate_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add the plan-time checks that keep handles internally consistent, all of them pure string work over the parsed model.
  In `validate.go`, add `checkHandleConsistency(plan *Plan) []ValidationError` emitting three distinct check IDs: `handle-dangling` for any handle appearing in a `Targets` or `Uses` entry with no matching declaration on any card's `Create` group and no matching `Rename` to-side, `handle-collision` for the same handle declared by more than one `Create` sub-bullet across the plan, and `handle-unreferenced` for a declared handle no other card references.
  `handle-unreferenced` is a finding like the other two rather than a warning, because the format has no severity axis; its detail states plainly that the card declares a handle nothing uses.
  Add `checkHandleMalformed` emitting check ID `handle-malformed` for every entry of a card's `CreateRaw`, the field card 9 captures a malformed `**Create:**` arrow bullet into, and for a handle whose text after `HandlePrefix` carries no `#` and therefore names no unit.
  Wire both checks into `validate`'s fixed dispatch list after `checkRenameFormat`, and update the package comment's check enumeration and count.
  Under `plan.Language` `none` these checks still run: a handle is loomyard grammar, not glyph grammar, and its consistency is checkable without any alphabet. No file read needed.
  Put the reusable predicates in `handle.go` beside card 9's helpers — `declaredHandles(plan *Plan) map[string][]string` mapping a handle to every card ID declaring it, and `referencedHandles(plan *Plan) map[string][]string` doing the same for references — so `validate.go` stays a check-dispatch file.
  Cover in tests: each of the four check IDs firing exactly once on a minimal offending plan, and a well-formed plan with one declaration and one reference producing none of them.
- **Commit:** `10: feat(planparser): add the handle dangling, collision, unreferenced and malformed checks`

### Card 11: accept a handle on a Rename group's to-side

- **Context:**
  - `internal/planparser/handle.go`
  - `internal/planparser/classify.go`
  - `internal/planparser/plan.go`
  - `contracts/specs/loom-plan-spec.md`
- **Edits:**
  - `internal/planparser/parse.go`
  - `internal/planparser/validate.go`
  - `internal/planparser/parse_test.go`
  - `internal/planparser/validate_test.go`
  - `internal/planparser/testdata/goodplan/05-rowmapper-rename.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Make a `**Rename:**` group's two sides carry their different obligations.
  On a symbol rename the old side must be a glyph — it names something that exists and will be resolved — and the new side must be a `plan:` handle, whose content batch 4's card 19 computes and overwrites at the validation boundary rather than trusting the planner's draft spelling.
  A `Rename` card therefore needs no declaration head of its own, unlike a `Create` card, because the declaration is derived from the resolved old side.
  In `validate.go`, add `checkRenamePairShape(plan *Plan) []ValidationError` emitting check ID `rename-to-not-handle` when a pair's `New` classifies as `refKindGlyph` or `refKindSymbol` rather than `refKindHandle`, and check ID `rename-from-not-glyph` when a pair's `Old` classifies as `refKindSymbol`.
  File-rename pairs stay exempt from both: when both `Old` and `New` classify as `refKindGlyph` **and** both parse to glyphs whose `IsSelf()` reports true, the pair is a file rename, which has no declaration head to name and belongs in the same group as a plain path pair.
  Under `plan.Language` `none` neither check runs. No file read needed.
  In `parse.go`, no grammar change is needed — `moveLineRe` already accepts any two backticked tokens — but extend `parseRenameField`'s doc comment to record that the to-side is expected to be a handle, so the next reader does not mistake the permissive regex for a permissive contract.
  Cover in tests: a symbol rename with a glyph old side and a handle new side passing; a symbol rename whose new side is a glyph producing `rename-to-not-handle`; one whose old side is a bare symbol producing `rename-from-not-glyph`; a file self-glyph pair on both sides producing neither; and `language: none` producing neither.
  The golden fixture's own Rename card (`testdata/goodplan/05-rowmapper-rename.md`) predates this card's grammar and spells its symbol pair's new side as a bare glyph, which now trips `rename-to-not-handle` and breaks `TestValidate_GoldenFixture_ZeroFindings`: respell it as `plan:internal/boardengine#MapRowJSON` and update every parse_test.go/validate_test.go assertion pinning that pair's literal spelling to match.
- **Commit:** `11: feat(planparser): require a handle on a Rename group's to-side`

### Card 12: add RewriteRefs, the plan's second write path

- **Context:**
  - `internal/planparser/approve.go`
  - `internal/planparser/parse.go`
  - `internal/planparser/plan.go`
  - `internal/planparser/classify.go`
- **Edits:** none
- **Creates:**
  - `internal/planparser/rewrite.go`
  - `internal/planparser/rewrite_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add one new primitive, `RewriteRefs(planDir string, subs map[string]string) error`, as the single write path all three later rewrite occasions call: handle canonicalization, handle binding at card completion, and drift auto-repair.
  Three purpose-built writers would triple the surface that can rewrite plan bytes; a full re-render from the parsed model is rejected for a sharper reason — `planparser` is deliberately lenient at the card level, preserving malformed bullets so the validator can enumerate every defect, so a round-trip would silently normalize away exactly the defects the validator exists to report, and would additionally rewrite backup-mode paths the operator chose to allow.
  `subs` is keyed on **canonical model strings**, matching what all three callers natively produce, and each value is the canonical replacement.
  Bridge the surface↔model gap here rather than in the callers: parse the plan with `ParsePlan` to obtain `Plan.SurfaceRefs`, and rewrite **one card file at a time**, looking each key up in that card's own inner map — the outer level is keyed on the card's `N-<slug>` identity, so a canonical string two cards spell differently resolves to each card's own lexeme rather than to whichever card was parsed last.
  A key with no entry in the card being rewritten substitutes itself, which is correct for a ref that was already canonical on disk.
  Substitute only inside backtick-wrapped sub-bullet payloads, matching the same `- \`ref\`` and `- \`old\` -> \`new\`` shapes `parseRefField` and `parseRenameField` recognise, so prose that happens to contain the same string is never touched.
  Rewrite each card file whose bytes actually change and leave every other file byte-identical; an empty or fully-unmatched `subs` map writes nothing at all.
  Return a wrapped `planparser:`-prefixed error for a missing plan directory, an unreadable card file, or a failed write, matching `SetApproved`'s existing error convention in `approve.go`.
  Cover in tests: substitution across `Targets`, `Uses` and both endpoints of a `Pairs` entry; substitution finding a surface lexeme that differs from the canonical key; two cards spelling one canonical string differently each being rewritten from their own lexeme rather than one card's spelling leaking onto the other; idempotence when the same map is applied twice; a no-op map leaving every file's bytes untouched; and prose containing the key string left unmodified.
- **Commit:** `12: feat(planparser): add RewriteRefs as the plan's ref-substitution write path`

### Card 13: add the amendment append and teach index-file-mismatch about it

- **Context:**
  - `internal/planparser/approve.go`
  - `internal/planparser/parse.go`
  - `internal/planparser/rewrite.go`
- **Edits:**
  - `internal/planparser/validate.go`
  - `internal/planparser/validate_test.go`
- **Creates:**
  - `internal/planparser/amendment.go`
  - `internal/planparser/amendment_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add the plan's third write path — an append, which neither `SetApproved` nor `RewriteRefs` performs, so it must be named as its own path rather than left implicit.
  In `amendment.go`, declare `const AmendmentsFileName = "amendments.md"` as this package's sole declarer of that name, and add `AppendAmendment(planDir string, a Amendment) error` appending one entry to `filepath.Join(planDir, AmendmentsFileName)`, creating the file with a fixed heading when absent.
  Declare `type Amendment struct{ Timestamp, Card, OldGlyph, NewGlyph, Tier, SHA string }` and render each entry as one markdown list item carrying all six fields, in that order, so the log is human-readable beside the plan it amends.
  Append-only means append-only: never rewrite an existing entry, never sort, never deduplicate.
  The log lives beside the plan because the plan directory is the thing that changed, and `planparser` stays the sole writer of that tree because it owns the file.
  Commit messages were rejected as the record for a terminal reason — a squash-merge erases them, so the history would exist only where nothing can read it back.
  In `validate.go`, teach `checkIndexFileConsistency` that `AmendmentsFileName` is a **known non-card file**: extend the on-disk scan's skip condition, which today skips only `overviewFileName`, to skip it as an explicit allowlist entry rather than a heuristic over the filename's shape.
  Cover in tests: a first append creating the file with its heading; a second append leaving the first entry byte-identical and adding the second below it; every field rendered; and a plan directory containing `amendments.md` producing no `index-file-mismatch` finding.
- **Commit:** `13: feat(planparser): add the append-only amendment log and allowlist it in index-file-mismatch`

### Card 14: add the syntactic tier of the containment check

- **Context:**
  - `internal/planparser/classify.go`
  - `internal/planparser/glyphref.go`
  - `internal/planparser/plan.go`
- **Edits:**
  - `internal/planparser/validate.go`
  - `internal/planparser/validate_test.go`
- **Creates:**
  - `internal/planparser/containment.go`
  - `internal/planparser/containment_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add the pure half of the cross-granularity containment check — the hole a symbol-granular DAG would otherwise hide.
  `websterengine.deriveEdges` matches refs by exact string equality, which correctly sees that two cards touching different members do not serialize, but a card targeting a member glyph and a card targeting the file self glyph of the file that member lives in overlap **physically** with no string equality between them: no edge, blind parallel dispatch, merge conflict.
  This tier catches the half that string prefixes can see.
  In `containment.go`, add `syntacticContainment(plan *Plan, lang glyph.Language) []ValidationError` emitting check ID `containment-unit-overlap`: for every member glyph on one card, compare its `Glyph.Unit` against every **self** glyph on every other card, and report the pair when the other card's self glyph names that same unit. No file read needed.
  Compare parsed `Glyph.Unit` values rather than doing string-prefix arithmetic on the raw ref, so the rule cannot drift from the alphabet.
  This tier deliberately does **not** attempt the member→file half — a package's symbols are spread across its files, so mapping a member to its owning file needs the `Resolve` answer, and batch 4 adds that as a separately-identified finding in `internal/planglyph`.
  Wire the call into `validate`'s dispatch list after `checkCardFieldOverlap`, skipping it entirely under `plan.Language` `none`, and update the package comment's check enumeration and count. No file read needed.
  Cover in tests: a member glyph and a unit self glyph naming the same unit on two cards producing exactly one finding; the same two refs on one card producing none, since a card cannot conflict with itself; two member glyphs in one unit producing none, which is the whole point of symbol granularity; and `language: none` producing none.
- **Commit:** `14: feat(planparser): add the syntactic tier of the cross-granularity containment check`

### Card 15: record the two new invariant obligations in CONSTRAINTS.md

- **Context:**
  - `internal/planparser/rewrite.go`
  - `internal/planparser/amendment.go`
  - `internal/planparser/glyphref.go`
  - `internal/planparser/approve.go`
  - `cmd/lyx/tierpurity_test.go`
- **Edits:**
  - `CONSTRAINTS.md`
  - `internal/planparser/doc.go`
- **Creates:**
  - `cmd/lyx/constraintchokepoint_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Record both cross-cutting obligations this batch creates, in the same commit that finishes creating them.
  In `CONSTRAINTS.md`, edit the `## Planparser Sole-Parser Invariant` section's bullet, which today reads that `SetApproved` is the one write path, so it names all three: `SetApproved`, `RewriteRefs` (ref substitution across the plan), and `AppendAmendment` (the append-only amendment log) — and no others.
  Then add a new `## Glyph Conversion Chokepoint Invariant` section stating that loomyard performs no glyph↔path conversion of its own, with bullets naming `glyph.Self` as the only path→glyph call, `Glyph.UnitPath` as the only glyph→path call, and `glyph.Parse` plus `Glyph.String` as the only glyph grammar, and forbidding a `#`-trimming suffix operation, reading `Glyph.Unit` as a disk path, and a local regex over a glyph string. No file read needed.
  Place it near the other parser invariants rather than at the end, so a reader scanning for plan-format rules finds it beside them.
  In `internal/planparser/doc.go`, extend the package documentation to name the two new write paths and point at the chokepoint invariant.
  Create `cmd/lyx/constraintchokepoint_test.go` enforcing the new invariant the way this repository's sibling guards do, with one deliberate choice about how it finds its scan root: resolve the module root from `runtime.Caller(0)`, the way `cmd/lyx/registration_test.go` and `cmd/lyx/sandbox_coverage_test.go` both do, and **not** via `exec.Command("go", "env", "GOMOD")`.
  The `GOMOD` route is what forces every sibling guard using it onto `tierpurity_test.go`'s `allowedSpawners` allowlist; `runtime.Caller(0)` spawns nothing, so this guard needs no allowlist entry and stays tier1-pure on its own terms.
  Then walk every non-test `.go` file under `internal/` and `cmd/`, skipping the same directories `tierPuritySkipDirs` names, and fail any file that contains a `#`-trimming suffix call over a glyph-typed value, or that reads a `glyph.Glyph` value's `Unit` field in a `filepath`/`os` path-construction context. No file read needed.
  Follow the raw-substring approach `bannedTokens` documents for the tokens where that is sufficient, and state the guard's own limit honestly in its file comment — like `bannedTokens`, it catches a direct textual call site and not a transitive one, and it narrows the gap rather than closing it.
  Add a small table-driven sub-test proving the guard fires on a synthetic offending source string and stays quiet on a compliant one, so the guard itself is tested rather than asserted.
- **Commit:** `15: docs(constraints): name the two new planparser write paths and add the Glyph Conversion Chokepoint Invariant`

## Batch Tests

`verify: go test ./internal/planparser/ ./cmd/lyx/` covers the two packages this batch touches.
`./internal/planparser/` runs `handle_test.go`, `rewrite_test.go`, `amendment_test.go`, `containment_test.go` and `validate_test.go` alongside the batch-2 files, all still tier1-pure and untagged: `RewriteRefs` and `AppendAmendment` do ordinary `os` file I/O against a `t.TempDir()` plan directory, which is not a spawn and not on `bannedTokens`.
`./cmd/lyx/` is in scope solely for card 15's new `constraintchokepoint_test.go` and the guards that walk the module's source tree; it is a source-scanning package whose untagged tests spawn nothing.
The scope is two named packages rather than the module because no other package compiles against a symbol this batch adds — every new export is consumed first in batch 4 — and the overview's module-wide `go build ./...` is what catches that assumption failing, at this batch's own boundary.
</content>
