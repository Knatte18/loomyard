# Batch: planparser-alphabet

```yaml
task: "Adopt quarry's glyph alphabet as the plan alphabet"
batch: "planparser-alphabet"
number: 2
cards: 6
verify: go test ./internal/planparser/ ./internal/loomcli/ ./internal/loomshed/ ./internal/webstercli/ ./internal/loomrecipe/ && go test -tags integration ./internal/websterengine/
depends-on: [1]
```

## Batch Scope

This batch turns `internal/planparser`'s ref alphabet from "path or bare package-qualified symbol" into "path or glyph", inside the pure tier1 leaf and with no `quarry.Repo` call anywhere.
It is one batch because the shape classifier, the `language:` key that gates it, the canonicalization that follows it, and the two checks (`prosa-symbol-target`, `path-missing`) whose predicates are built on the classifier all move together — splitting them would land intermediate merges where a check silently matches nothing.
The external interface batch 3 consumes is the post-canonicalization `planparser.Plan` model: `Card.Targets`/`Uses`/`Pairs` still typed `[]string`/`[]MovePair` and now holding canonical glyph strings, plus the new `Plan.Language` field and the new `Plan.SurfaceRefs` side map.

Batch-local decisions, beyond `## Shared Decisions`:

- **Ordering is load-bearing.** `root:`/`//` resolution runs first, while a surface ref is still path-shaped; canonicalization to a glyph runs after.
  A surface glyph is always repository-root-relative and is never `root:`-joined.
- **The classifier never calls `Resolve`.** It enforces form syntactically and stays a pure leaf; existence is `internal/planglyph`'s business in batch 4.
- **Card 6 is the one card in the whole plan gated on the quarry-side `Glyph.UnitPath()` accessor.** If the accessor has not merged when the plan reaches it, that card blocks and every other card proceeds.

## Cards

### Card 3: add the `language:` frontmatter key and its syntactic check

- **Context:**
  - `contracts/specs/loom-plan-spec.md`
- **Edits:**
  - `internal/planparser/parse.go`
  - `internal/planparser/plan.go`
  - `internal/planparser/validate.go`
  - `internal/planparser/parse_test.go`
  - `internal/planparser/validate_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Extend the overview's scalar-only frontmatter with a `language:` key.
  In `parse.go`, add `Language *string \`yaml:"language"\`` to the `overviewFrontmatter` struct — a pointer field, like the three keys beside it, so an absent key is distinguishable from an empty one — and, in `ParsePlan`, set `plan.Language` from it, defaulting to the literal `"go"` when `fm.Language` is nil.
  Note that `parseOverviewFrontmatter` decodes with `dec.KnownFields(true)`, so without this struct field a plan carrying `language:` fails the whole parse rather than reaching validation.
  In `plan.go`, add a `Language string` field to `Plan` with a doc comment stating that legal values are the alphabets the `glyph` package implements (today `go`) plus the literal `none`, and that absent defaults to `go`.
  In `validate.go`, add a new check `checkLanguageRecognized(plan *Plan) []ValidationError` emitting check ID `plan-language-unrecognized` when `plan.Language` is neither `"go"` nor `"none"`, with a detail naming the offending value and the two legal ones. No file read needed.
  Wire it into `validate`'s fixed dispatch list immediately after `checkFormatRecognized`, which places it **ahead** of the `requireApproved` `checkApproved` call.
  That displaces `plan-unapproved` from position two, so update `Validate`'s doc comment sentence naming "plan-unapproved at position two" along with the counts — leaving it would make the doc comment false about ordering while being true about totals, which is the harder kind of stale comment to notice.
  Also update this file's package comment and `ValidateFormat`'s doc comment, which currently name a seventeen/sixteen check count, to the new counts.
  This is a pure string check: it calls no `Resolve`, stats no disk, and keeps the package tier1-safe.
  Add table-driven tests covering `go` accepted, `none` accepted, absent defaulting to `go`, and an unknown value producing exactly one `plan-language-unrecognized` finding.
- **Commit:** `3: feat(planparser): add the language: frontmatter key and its recognition check`

### Card 4: rewrite the shape classifier over the glyph alphabet

- **Context:**
  - `internal/planparser/normalize.go`
  - `internal/planparser/plan.go`
  - `contracts/specs/loom-plan-spec.md`
- **Edits:**
  - `internal/planparser/classify.go`
  - `internal/planparser/classify_test.go`
  - `internal/planparser/validate.go`
  - `internal/planparser/validate_test.go`
- **Creates:**
  - `internal/planparser/glyphref.go`
  - `internal/planparser/glyphref_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Replace `classifyRef`'s three-rule shape analysis with the glyph alphabet's own.
  The current rule 1 — `strings.Contains(raw, "/")` returns `refKindPath` — sends **every** Go glyph to `refKindPath`, because every Go glyph contains a slash; a `#`-keyed rule must therefore precede it.
  Add two new `refKind` constants beside the existing `refKindPath`/`refKindSymbol`: `refKindGlyph` and `refKindHandle`.
  The new rule order in `classifyRef` is: a ref beginning with the literal prefix `plan:` is `refKindHandle`; a ref containing `#` is `refKindGlyph`; a ref containing `/` or whose final dot-segment is all-lowercase-alphanumeric (the existing `isLowerAlphanumeric` test, kept verbatim) is `refKindPath`; a ref containing **no** `.` and **no** `/` is `refKindPath` as well — an extensionless repository-root filename such as `Makefile`, `LICENSE` or `Dockerfile`; everything else is `refKindSymbol`.
  That fourth rule is required rather than tidy: today's `classifyRef` doc comment records that a dot-free, slash-free entry falls to `refKindSymbol` and names `Makefile` as its own example, so without the rule card 4's `bare-symbol-target` would make every such filename a hard finding with **no legal spelling left** — the `//` worktree-root escape does not rescue it, since `normalizeCardPath` strips the prefix and hands back the identical bare token.
  A `refKindSymbol` entry therefore always contains a `.`, which is exactly the `pkg.Symbol` shape the hard rule exists to catch.
  State the consequence rule 4 carries rather than leaving it to be discovered: a `refKindPath` entry goes through `normalizeRefIfPath`, so under a non-`.` `root:` a bare `Makefile` now resolves to `<root>/Makefile` where today it passes through verbatim.
  That is the intended behaviour — a bare filename under a declared `root:` means the file in that root, exactly as every other relative path in the plan does — but it is a behaviour change and must be written down and tested rather than inferred.
  Keep `isPathRef` as the convenience wrapper reporting `refKindPath`, and add `isGlyphRef` and `isHandleRef` wrappers beside it.
  In the new file `glyphref.go`, put the glyph-side helpers so `classify.go` stays a shape-only file: `parseGlyph(lang, raw string) (glyph.Glyph, error)` delegating to `glyph.Parse` with no local grammar, and `planLanguage(plan *Plan) (glyph.Language, bool)` mapping `Plan.Language` `"go"` to `glyph.Go` and `"none"` to the not-ok second return. No file read needed.
  Never write a `strings.TrimSuffix(raw, "#")`, never read `Glyph.Unit` as a disk path, and never add a regex over a glyph string — the glyph-conversion-chokepoint Shared Decision forbids all three, and card 15 adds a constraint test that fails the build if one appears.
  In `validate.go`, add two hard findings keyed on the new classification, as `checkBareSymbolTarget(plan *Plan) []ValidationError` and `checkDirectoryTarget(plan *Plan) []ValidationError`, wired into `validate`'s fixed dispatch list immediately after `checkCardPathMalformed` so a shape finding is reported beside the other shape findings: check ID `bare-symbol-target` for any `Targets`/`Uses` entry classifying as `refKindSymbol`, with a detail stating that a bare package-qualified symbol is the one spelling that cannot have come verbatim from a quarry answer and naming the glyph form to use instead; and check ID `directory-target` for a `refKindPath` entry that contains a `/` and carries no file extension, whose detail reads that the author should spell it as a unit glyph such as `internal/foo#` if it is a package, and list the files instead if it is not code.
  The `/` condition is what keeps the extensionless repository-root filenames the fourth classifier rule admits out of this finding; state that limit in the check's own comment rather than leaving it implicit, and note honestly that a slash-free extensionless **directory** at the repository root is therefore not caught here — it falls to `path-missing` and, on a `Prosa` group, to `prosa-symbol-target`, which is narrower coverage than the slashed case and is accepted rather than papered over.
  Both checks are skipped entirely when `plan.Language` is `none`, where a symbol-shaped ref keeps today's behaviour. No file read needed.
  A unit glyph that names a directory with no Go files is deliberately **not** a classification finding — it is a resolve finding batch 4 raises.
  Cover in tests: a member glyph, a unit self glyph, a file self glyph, a plain path, a bare filename with a lowercase extension, a `plan:` handle, a bare `pkg.Symbol`, an extensionless directory path producing `directory-target`, an extensionless slash-free filename such as `Makefile` classifying as a path and producing **neither** new finding, and the documented `shedrecipe.lookup` edge that rule 2 classifies as a path.
- **Commit:** `4: feat(planparser): rewrite the shape classifier over the glyph alphabet`

### Card 5: canonicalize path-shaped refs to glyphs at parse time

- **Context:**
  - `internal/planparser/classify.go`
  - `internal/planparser/glyphref.go`
  - `contracts/specs/loom-plan-spec.md`
- **Edits:**
  - `internal/planparser/normalize.go`
  - `internal/planparser/parse.go`
  - `internal/planparser/plan.go`
  - `internal/planparser/normalize_test.go`
  - `internal/planparser/parse_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Make the `Plan` model hold exactly one spelling per thing, canonicalized once at parse time.
  In `normalize.go`, add `canonicalizeCard(card *Card, cardKey string, lang glyph.Language, surface map[string]map[string]string)` running **after** `normalizeCard` on the same card, never before: `normalizeRefIfPath` is classifier-gated on `isPathRef`, and its own comment names the regression that gate prevents, so canonicalizing first would put every ref on the non-path side of the gate and silently switch `root:` off plan-wide.
  Canonicalize a path-shaped ref **only when it carries a file extension**; leave an extensionless path ref untouched.
  This gate is load-bearing, not an optimization: without it, canonicalization would rewrite a bare directory path such as `internal/foo` into the perfectly valid unit self glyph `internal/foo#`, so card 4's `directory-target` finding would have nothing left to classify and the `package-spelling` rule — a bare directory path as a target is a hard finding — would be silently satisfied by rewriting the defect away.
  That is the same "canonicalizing makes guessing rewarded" failure the bare-symbol rule rejects, pointed at directories instead of symbols.
  Walk exactly the field set `normalizeCard` walks, and no smaller one: the card-level `Targets`, `Uses` and both endpoints of every `Pairs` entry, **and** every `TargetGroups[i].Refs` entry and both endpoints of every `TargetGroups[i].Pairs` entry.
  The group-level half is not optional — `checkProsaSymbolTarget`, `checkPathMissing`, `checkCardFieldEmpty` and `createTargetsUnion` all read group `Refs` rather than the card-level union, so canonicalizing only the card-level fields would leave `createTargetsUnion` holding pre-canonical strings while `checkPathMissing` compared canonical ones, manufacturing false `path-missing` findings on every `Create` target.
  For each such ref that `isPathRef` classifies as a path **and** that carries a file extension, replace it with `glyph.Self(lang, ref).String()` — the one path→glyph call, per the glyph-conversion-chokepoint Shared Decision — and record the pre-canonicalization surface lexeme in `surface`, keyed on the resulting canonical string.
  A ref that already classifies as `refKindGlyph` is left byte-identical and is never `root:`-joined: a glyph is copied verbatim from a quarry answer and is already a complete repository-relative string, so prefixing `root:` onto one would corrupt it.
  A `glyph.Self` error leaves the ref untouched and is not a parse failure — the classification checks from card 4 already report a malformed entry, and `planparser` is deliberately lenient at card level. No file read needed.
  In `plan.go`, add `SurfaceRefs map[string]map[string]string` to `Plan`, keyed **card identity first, canonical model string second**, whose value is the surface lexeme that card's own file actually carries.
  The two-level shape is required rather than cosmetic: two cards may legitimately spell one canonical string differently — a plain path on one card and its file self glyph on another both canonicalize to the same glyph — and a flat one-level map would silently lose one of them, which is precisely the byte-match failure `RewriteRefs` exists to avoid.
  Key the outer level on the card's own `N-<slug>` identity, the same string `cardID` builds in `validate.go`. No file read needed.
  Document in the field's comment that `Card.Targets`/`Uses`/`Pairs` stay `[]string`/`[]MovePair` and unchanged in type so `websterengine.deriveEdges` and `refsIntersect` never see the difference, and that batch 3's `RewriteRefs` is its only consumer.
  In `ParsePlan`, derive `lang` **before** the cards loop, from `fm.Language`, exactly as `root` is already derived from `fm.Root` before that loop: the `*Plan` value is constructed only after the loop finishes, so there is no `*Plan` to hand anything inside it. No file read needed.
  Do not call `planLanguage` here — that helper takes a `*Plan` and exists for the post-parse callers in `validate.go`, which run against a fully built plan; `ParsePlan` maps `fm.Language` to a `glyph.Language` directly, applying the same absent-defaults-to-`go` rule card 3 states. No file read needed.
  Then thread the map through: allocate it once, pass it plus the card's own `N-<slug>` key into each card's `canonicalizeCard` call placed immediately after the existing `normalizeCard(&card, root)` line, and assign it onto the returned `Plan`.
  When `plan.Language` is `none`, canonicalization is a complete no-op: no `glyph.Self` call runs and `SurfaceRefs` stays empty. No file read needed.
  Cover in tests: a bare extensionless filename such as `Makefile` under a non-`.` `root:` resolving to `<root>/Makefile`, and the same token under an empty `root:` passing through verbatim; an extensionless path ref surviving canonicalization untouched, so card 4's `directory-target` still fires on it; a plain path and its file self glyph landing on the identical model string; a non-`.` `root:` still applying to a plain path and never to a surface glyph; the documented consequence that a bare-filename surface glyph such as `focus.go#` under a non-`.` `root:` names the repository-root file and is left alone; `language: none` leaving every ref byte-identical; and `SurfaceRefs` recording the original lexeme for a canonicalized path under its own card's key, with two cards spelling one canonical string differently each keeping their own entry.
- **Commit:** `5: feat(planparser): canonicalize path refs to glyphs after root: resolution`

### Card 6: re-express the disk-shaped checks over glyphs

- **Context:**
  - `internal/planparser/classify.go`
  - `internal/planparser/glyphref.go`
  - `internal/planparser/plan.go`
- **Edits:**
  - `internal/planparser/validate.go`
  - `internal/planparser/validate_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Repair the two checks card 5 leaves matching nothing, in this one card, because both are gated on `isPathRef` and the moment refs canonicalize to glyphs both stop firing.
  This card is the one card in the plan carrying the `Glyph.UnitPath()` precondition from the two-preconditions-outside-this-worktree Shared Decision: it is the only place a glyph is mapped back to a file on disk.
  If `Glyph.UnitPath` is absent from the linked quarry version, stop and report that this card blocks — do **not** implement a local `#`-trimming helper as an interim, which the chokepoint decision rejects outright rather than defers, and do **not** read `Glyph.Unit` as a path, which encodes the same Go-only assumption without the trim.
  In `validate.go`, change `checkPathMissing` and `checkCardPathMalformed` to gate on `isGlyphRef` in addition to `isPathRef`, and for a glyph-shaped entry obtain its disk path from `parseGlyph` followed by `Glyph.UnitPath()`; a not-ok second return means the glyph's unit is not path-shaped and the entry is skipped by both checks rather than reported.
  `pathExistsOnDisk` and `cardPathMalformedReason` keep their current bodies and are simply fed the mapped path instead of the raw ref.
  `createTargetsUnion` and `renameTargetsUnion` do **not**: neither takes a ref at all — both walk the plan themselves and filter on `isPathRef` — so after card 5 every glyph-shaped `Create` target silently drops out of the union while `checkPathMissing`'s own `satisfied` closure looks up a `UnitPath`-mapped path, manufacturing `path-missing` findings on exactly the `Create` targets that canonicalization exists to keep consistent.
  Give both union builders the same `isGlyphRef` branch this card adds to the two checks, and key each union on the identical mapped value `satisfied` looks up, so the two sides of the comparison are in one vocabulary.
  Cover that pairing directly: a glyph `Create` target in one card satisfying a glyph `Uses` reference in another must produce no finding.
  Under `plan.Language` `none` both checks keep exactly today's `isPathRef`-only behaviour. No file read needed.
  Cover in tests: a file self glyph whose file exists passing `path-missing`; one whose file does not exist and is not a `Create` target failing it; a member glyph resolving to its unit's directory being skipped rather than reported; a unit self glyph for a package that exists passing; and the whole `none`-language path being byte-for-byte today's behaviour.
- **Commit:** `6: fix(planparser): re-express path-missing and card-path-malformed over glyphs`

### Card 7: redefine prosa-symbol-target in glyph terms and bump the format to 5

- **Context:**
  - `internal/planparser/classify.go`
  - `internal/planparser/glyphref.go`
- **Edits:**
  - `internal/planparser/validate.go`
  - `internal/planparser/validate_test.go`
  - `internal/planparser/doc.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Redefine one check and bump the format constant, together, because the redefinition is only correct under the new format.
  `checkProsaSymbolTarget` currently flags any `Prosa` group ref where `isPathRef` is false; after card 5 every file ref is a glyph, so the check as written would fire on every `Prosa` card in a glyph-enabled plan.
  Retiring it would be an over-correction — its purpose is that a `Prosa` card documents files and packages, never individual symbols, and that purpose survives the alphabet change.
  Under a glyph-enabled `plan.Language`, a `Prosa` group ref passes when `parseGlyph` succeeds and the resulting glyph's `IsSelf()` reports true — which admits both the file self glyph and the unit self glyph, the latter being required by the package-spelling rule so a `Prosa` card can target `internal/foo#` — and is the `prosa-symbol-target` finding when `IsSelf()` reports false. No file read needed.
  Under `none`, the check keeps today's exact behaviour: path-shaped refs pass, symbol-shaped refs are the finding.
  Then change `recognizedFormat` from `4` to `5`, so `checkFormatRecognized` accepts exactly one version and a format-4 plan fails loud rather than half-parsing under the new alphabet.
  Update `validate.go`'s package comment and `doc.go`'s package documentation everywhere they say format-4 or name the old check set, including the two new check IDs from card 4 and the one from card 3.
  Cover in tests: a `Prosa` group targeting a file self glyph passing; one targeting a unit self glyph passing; one targeting a member glyph producing the finding; the `none`-language behaviour unchanged; and a plan declaring `format: 4` producing exactly one `format-unrecognized` finding.
- **Commit:** `7: feat(planparser): redefine prosa-symbol-target over self glyphs and bump format to 5`

### Card 8: rewrite the spec and stencil, and sweep every format-4 fixture to format 5

- **Context:**
  - `internal/planparser/classify.go`
  - `internal/planparser/glyphref.go`
  - `internal/planparser/validate.go`
  - `internal/planparser/plan.go`
- **Edits:**
  - `contracts/specs/loom-plan-spec.md`
  - `contracts/stencils/loom/loom-template-plan.md`
  - `internal/planparser/testdata/goodplan/00-overview.md`
  - `internal/planparser/testdata/goodplan/01-json-row-type.md`
  - `internal/planparser/testdata/goodplan/02-json-flag.md`
  - `internal/planparser/testdata/goodplan/03-json-emission.md`
  - `internal/planparser/testdata/goodplan/04-legacy-rows-delete.md`
  - `internal/planparser/testdata/goodplan/05-rowmapper-rename.md`
  - `internal/planparser/testdata/goodplan/06-helppins-move.md`
  - `internal/planparser/testdata/goodplan/07-json-docs.md`
  - `internal/planparser/approve_test.go`
  - `internal/planparser/parse_test.go`
  - `internal/planparser/sections_test.go`
  - `internal/planparser/validate_test.go`
  - `internal/loomcli/validate_test.go`
  - `internal/loomshed/planvalidate_test.go`
  - `internal/loomshed/gatefindings_test.go`
  - `internal/webstercli/cli_test.go`
  - `internal/websterengine/runlevel_test.go`
  - `internal/loomrecipe/fixture_test.go`
  - `tools/sandbox/SANDBOX-WEBSTER-SUITE.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Bring the pinned contract, the LLM-facing subset, and the golden fixture onto format 5, in the same commit as the code that changed them per this repository's task-completion rule.
  In `contracts/specs/loom-plan-spec.md`, update the `## The shape classifier` section to the four-way glyph/path/handle/symbol rules from card 4 in their exact order, add the `language:` key to the frontmatter description, restate `## Card path resolution: root: and //` to record that `root:` resolves first and canonicalization runs after and that a surface glyph is never `root:`-joined, extend the `## Validation checks (as implemented by internal/planparser)` section with `plan-language-unrecognized`, `bare-symbol-target` and `directory-target`, restate `prosa-symbol-target` in self-glyph terms, and rewrite the `## Worked example` so every symbol entry is spelled as a glyph.
  Rewrite the spec's `## Deferred / forward-compat` section too: it defines the `changes-files`/deviation union as every path-shaped target entry plus the files holding every symbol-shaped target entry, a definition batch 7 replaces with a mechanical comparison of the delta's symbol IDs against the batch's own target glyphs.
  Restate it over glyphs, name the mechanical guard as what performs the comparison, and keep the section's existing statement that a mismatch is always informational and never blocking on its own, plus its exclusion of `Uses:` from the union.
  Card 39 rewrites the corresponding paragraph in the implementer stencil; this pinned contract doc is the other half and must not ship describing the superseded mechanism.
  Write the hard rule explicitly in the spec, in the terms the decision requires so nobody later softens it into a style preference: a bare package-qualified symbol is a hard finding because it is the one spelling that cannot have come verbatim from a quarry answer, not because the form is uglier.
  In `contracts/stencils/loom/loom-template-plan.md`, change every ref example to glyph spelling and state the same hard rule.
  Leave the stencil's `### No quarry inventory exists — do the lookups yourself` section alone in this card — card 30 replaces it once the `lyx quarry` verbs exist to replace it with.
  Rewrite the eight `internal/planparser/testdata/goodplan` files so the fixture declares `format: 5`, carries a `language: go` key, and spells every symbol-shaped ref as a glyph; keep its card structure, numbering and `Commit:` subjects unchanged so the existing round-trip assertions keep their shape.
  Verify the fixture is clean under the new check set by running the package's own tests — a fixture that trips one of the three new checks is the fixture's defect, not the check's.
  Then sweep **every other** `format: 4` fixture in the repository to `format: 5`, in this same card, because card 7's `recognizedFormat` bump makes each one produce a `format-unrecognized` finding the moment it lands: `internal/planparser/approve_test.go`, `internal/planparser/parse_test.go`, `internal/planparser/sections_test.go` and `internal/planparser/validate_test.go` (the last two of which cards 3–7 already edit); `internal/loomcli/validate_test.go`; `internal/loomshed/planvalidate_test.go` and `internal/loomshed/gatefindings_test.go`; `internal/webstercli/cli_test.go`; `internal/websterengine/runlevel_test.go`; `internal/loomrecipe/fixture_test.go`; and the plan overview embedded in `tools/sandbox/SANDBOX-WEBSTER-SUITE.md`.
  Change only the `format:` value in each — no `language:` key is added here, because absent correctly defaults to `go` and batch 5 is where the fixtures that need `language: none` get it.
  Locate the full set with `grep -rn "format: 4"` across the module rather than trusting this list, and report any file it finds that is not named above.
  This sweep belongs in this batch rather than a later one: the bump is what breaks them, and `go build ./...` compiles tests without running them, so nothing else would catch the break at its own boundary.
- **Commit:** `8: docs(plan-format): move the spec, stencil and every fixture to format 5 glyph spelling`

## Batch Tests

`verify: go test ./internal/planparser/ ./internal/loomcli/ ./internal/loomshed/ ./internal/webstercli/ ./internal/loomrecipe/ && go test -tags integration ./internal/websterengine/` runs `internal/planparser` — the surface cards 3 through 7 touch — plus every other package holding a plan fixture card 8's format-5 sweep rewrites.
The wider scope is card 8's doing and is the point: `recognizedFormat` moving to 5 makes every un-swept `format: 4` fixture fail, and those fixtures live in five packages outside `planparser`, so a verify scoped to `planparser` alone would let the break escape to the hub done gate several batches later.
The chained `-tags integration` half covers `internal/websterengine/runlevel_test.go`, which carries that tag on its own first line and holds one of the swept fixtures.
The package is tier1-pure and untagged — no `gitexec.Run`, no `exec.Command`, no `gitkit.Copy*`, no `hubforge.NewHub` — and this batch adds no spawn to it, since `glyph` is stdlib-only and `parseGlyph` reads no source.
The files the command covers are `classify_test.go`, `glyphref_test.go`, `normalize_test.go`, `parse_test.go`, `validate_test.go`, `approve_test.go`, `sections_test.go` and `planpath_test.go`, plus the golden-fixture round-trip that reads `testdata/goodplan`.
The scope is deliberately the single package rather than the module: nothing outside `internal/planparser` compiles against a symbol this batch changes, and the overview's module-wide `go build ./...` catches it at this batch's boundary if that assumption is wrong.
</content>
