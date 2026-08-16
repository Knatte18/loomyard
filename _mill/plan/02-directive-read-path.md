# Batch: directive-read-path

```yaml
task: 'PATTERN directives: move from Go constants to stencil files'
batch: directive-read-path
number: 2
cards: 7
verify: go build ./... && go test ./...
depends-on: [1]
```

## Batch Scope

This batch is the whole behaviour change, and it is one batch because it cannot be anything else: the moment `Directive` gains a parameter and an error return, all four call sites stop compiling, so the tree only builds again once every one of them, plus every test that calls `Directive`, has moved with it.
It rewrites `Directive` to read from `stencilsDir` and strip the banner, deletes the three constants and the two now-false comment blocks that describe them, amends the Pattern Leaf Invariant at all four of its statement sites, migrates ten existing `internal/pattern` tests, adds four new ones plus a package-local stamped-fixture helper, updates the two trivial call sites (loom, burler) and the two that need the call hoisted out of a map literal (webster ×2), and seeds the three new stencils into the three consumer test fixtures.
Intermediate card commits inside this batch will not compile;
that is expected and fine, because `verify:` runs once at the batch boundary, not per card.
The batch delivers no new external interface — batch 3 consumes only the facts it makes true, in prose.

Batch-local decisions, beyond the overview's Shared Decisions:

- Error wrapping follows each package's existing house prefix: `pattern: …` inside `internal/pattern`, `webster: …`, `burler: …`, `loom: PlanSpec: …` at the call sites.
- The two webster hoists also collapse the duplicated `fabricengine.StencilsDir(l.HubPath)` derivation in each function into a single local, used for both the `Directive` call and the template read that already followed the map.
  No webster signature changes.
- `internal/pattern`'s new test helper deliberately writes **stamped** fixture bytes, unlike the three existing consumer helpers, which write raw embedded defaults.
  A raw fixture would make the banner-strip test vacuous.

## Cards

### Card 3: Rewrite `Directive` to read the stencil and strip its banner

- **Context:**
  - `internal/stencilstore/reconcile.go`
  - `internal/stencil/stencil.go`
- **Edits:**
  - `internal/pattern/pattern.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Change `Directive`'s signature in `internal/pattern/pattern.go` from `Directive(l *lyxcwd.Location, role Role) string` to `Directive(l *lyxcwd.Location, stencilsDir string, role Role) (string, error)`.

  Keep the existing guard order exactly: the `l == nil` guard first, then the `!isActive(l)` guard, each returning `("", nil)` before any file is touched.
  Then keep the same three-case `switch role` with its `default`, but have each case assign a stencil *name* to a local instead of returning a constant: `RoleImplementer` yields `pattern-directive-implementer`, `RoleReviewFix` yields `pattern-directive-review-fix`, `RoleOrchestrator` yields `pattern-directive-orchestrator`, and `default` returns `("", nil)` with its existing explanatory comment about an unknown or zero `Role` preserved (reworded only where it now says "renders no directive" about a return path that also has to be nil-error).
  Declare the three names as unexported package-level string constants next to `Directive` rather than inline in the switch, so each is written exactly once.

  After the switch, and only there, call `stencilstore.Read(stencilsDir, name)`.
  On error return `("", err)` wrapped as `fmt.Errorf("pattern: directive stencil: %w", err)` — do not add the stencil name to the wrap text, because `stencilstore.Read`'s own error already names both the stencil and the base directory.
  On success return `(stencil.StripLeadingComment(string(content)), nil)`.
  Do not normalise line endings, and do not trim, pad, or otherwise post-process the stripped body.

  Delete the three constants `implementerDirective`, `reviewFixDirective`, and `orchestratorDirective` in their entirety, together with the doc comment above `implementerDirective` and the paragraph beneath it explaining why "the three directive constants below" are not built from `PathspecFile`/`PathspecDir`/`lyxdirs.LyxDirName`, and the one-line doc comments above `reviewFixDirective` and `orchestratorDirective`.
  That explanatory paragraph's *substance* survives the move and must not simply be dropped: rewrite it as a short comment above the three new stencil-name constants, stating that the literal `_lyx/PATTERN.md` and `_lyx/pattern/` pointers now live in the stencil files rather than in Go, that they are still plain fixed literals rather than anything interpolated from `PathspecFile`/`PathspecDir`, and that this is what keeps this package's own tests' and every consumer template test's fixed-string equality and substring comparisons meaningful.

  Rewrite the file-header comment on the first two lines of `internal/pattern/pattern.go`, which currently describes "the three role-specific directive constants Directive selects between" — a phrase that is false the moment the constants are gone.
  It must instead describe the active check plus the role-keyed stencil read.
  This is a separate edit from the pre-constant paragraph above and is easy to miss.

  Update `Directive`'s own doc comment to state the new contract: the `stencilsDir` it is told, the strip, the `("", nil)` results for nil layout / inactive PATTERN / unknown role with no read attempted on any of them, and the error on a read failure while PATTERN is active.

  Do not touch `isActive`, the `statFile` seam, `File`, `FileHere`, `PathspecFile`, `PathspecDir`, the `patternFileName` constant, the `Role` type, or its three members.
- **Commit:** `refactor(pattern): read directive text from stencils instead of Go constants`

### Card 4: Extend the Pattern Leaf Invariant allowlist at all four statement sites

- **Context:**
  - `internal/pattern/pattern.go`
  - `internal/stencil/stencil.go`
  - `internal/stencilstore/stencilstore.go`
  - `internal/stencilstore/reconcile.go`
- **Edits:**
  - `internal/pattern/leaf_enforcement_test.go`
  - `CONSTRAINTS.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  The Pattern Leaf Invariant's allowlist is stated in four places and all four change together, adding exactly two entries — `github.com/Knatte18/loomyard/internal/stencilstore` and `github.com/Knatte18/loomyard/internal/stencil` — and nothing else.

  In `internal/pattern/leaf_enforcement_test.go`, change all three of its statement sites: the file-header comment's list of what production code may import;
  the `allowedImports` map;
  and the parenthetical in `TestLeafInvariant_AllowlistOnly`'s failure message, currently reading "stdlib + lyxcwd + lyxdirs".
  Do not change the walk, the parser call, or any other logic in that file.

  In `CONSTRAINTS.md`, amend the **Pattern Leaf Invariant** section's opening sentence and add the two justifications.
  The two entries carry **different** justifications and conflating them would state something false.
  `internal/stencil` is a zero-import leaf — it imports no internal package at all — so it cannot participate in a cycle by construction, which is the same argument the existing text already makes for `internal/lyxdirs` and can be worded the same way.
  `internal/stencilstore` is **not** a leaf: it imports `internal/stencil` and `internal/logger`.
  Its justification is that the invariant restricts *feature* packages and `internal/stencilstore` is shared infrastructure, plus a verified-acyclic closure — nothing reachable from it imports `internal/pattern`.
  Leave the "never `websterengine`, `burlerengine`, `loomengine`, or any other feature package" clause and the "Reverse import never allowed" line intact, and leave the **Enforced by** line pointing where it already does.
- **Commit:** `docs(constraints): admit stencilstore and stencil to the Pattern Leaf Invariant`

### Card 5: Add the stamped test fixture helper and migrate the ten existing `Directive` tests

- **Context:**
  - `internal/pattern/pattern.go`
  - `internal/loomengine/prompt_test.go`
  - `internal/stencilstore/stencilstore.go`
  - `internal/stencilstore/reconcile.go`
  - `internal/stencil/stencil.go`
  - `stencils/stencils.go`
- **Edits:**
  - `internal/pattern/pattern_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add a package-local `newTestStencilsDir(t *testing.T) string` helper to `internal/pattern/pattern_test.go`, modelled on the one in `internal/loomengine/prompt_test.go`: `t.TempDir()`, a `pattern` subdirectory, and the three files seeded from the `stencils` package's embedded defaults `PatternDirectiveImplementer`, `PatternDirectiveReviewFix`, and `PatternDirectiveOrchestrator`.
  One deliberate difference from all three existing consumer helpers: write each file's bytes through `stencilstore.ApplyStamp(content, stencilstore.BodyHash(content))` rather than writing the embedded default raw, so the fixture matches what `stencilstore.Reconcile` really puts on disk in a hub.
  State that difference and its reason in the helper's own comment, so a future reader does not "fix" it into consistency with the other three: the other helpers get away with raw bytes because everything they feed passes through `stencil.Fill`, which strips either way, whereas a raw fixture here would make the banner-strip test vacuous and let a missing strip pass green.

  Migrate every existing test in the file that calls `Directive` to the new signature and assert on the error: `TestDirective_ActiveWithFile`, `TestDirective_InactiveWithoutFile`, `TestDirective_EmptyPatternFileIsActive`, `TestDirective_PatternFileAsDirectoryIsInactive`, `TestDirective_NilLayout`, `TestDirective_UnknownRole`, `TestDirective_VariantsArePairwiseDistinct`, `TestDirective_VariantsBeginWithOwnHeading`, `TestDirective_RelPathNestedSubdirectory`, and `TestDirective_NonNotExistStatErrorIsActive`.
  Each gains the `stencilsDir` argument — `newTestStencilsDir(t)` — and a non-nil-error check that fails the test loudly.

  Every substantive assertion these tests already make must keep asserting exactly the same thing, because they are what proves the prose survived the move intact and they are a far stronger relocation guard than any equality-with-the-default check.
  Specifically: `TestDirective_VariantsArePairwiseDistinct` keeps asserting that each of the three variants contains `_lyx/PATTERN.md`, contains `_lyx/pattern/`, does not contain `_pattern/`, and that the three are pairwise distinct;
  `TestDirective_VariantsBeginWithOwnHeading` keeps asserting each variant begins with its own `##` heading.
  Do not weaken, delete, or convert any of them into a comparison against the embedded default.

  Update the file-header comment, which currently declares the file untagged Tier 1 on the grounds that it uses only `os.Stat` via the `statFile` seam and `t.TempDir` and spawns nothing.
  It must now also mention the seeded stencils directory the helper writes.
  The claim itself stays true — the helper adds only `os.MkdirAll` and `os.WriteFile` inside a `t.TempDir()` and spawns nothing — so `cmd/lyx`'s tier-purity guard stays satisfied;
  it is the comment's accuracy that needs the edit.
- **Commit:** `test(pattern): migrate Directive tests to the stencil-read signature`

### Card 6: Add the four new `Directive` property tests

- **Context:**
  - `internal/pattern/pattern.go`
  - `internal/loomengine/discussion_test.go`
  - `internal/stencilstore/stencilstore.go`
  - `internal/stencilstore/reconcile.go`
  - `internal/stencil/stencil.go`
  - `stencils/stencils.go`
- **Edits:**
  - `internal/pattern/pattern_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add four new tests to `internal/pattern/pattern_test.go`, one per distinct property, each using the fixtures already in the file (`layoutAt`, `writePatternFile`, `newTestStencilsDir`).

  1. Lazy read.
  PATTERN inactive, plus a `stencilsDir` that deliberately does not exist, returns `("", nil)` — no error, no empty-but-errored result.
  This is the test that pins the decision limiting fixture churn across `burlerengine`, `websterengine`, and `loomengine`;
  without it a future eager-read refactor would break those fixtures with nothing local failing.
  Cover the nil-layout path the same way in the same test.

  2. Missing-stencil error.
  PATTERN active, plus a `stencilsDir` that exists but lacks the pattern files, returns a non-nil error whose message names the missing stencil.
  `TestDiscussionSpec_MissingStencilsDirIsHardError` in `internal/loomengine/discussion_test.go` is the existing precedent for this shape, including the names-the-missing-stencil assertion — follow it.
  Cover all three roles.

  3. Banner is stripped.
  PATTERN active against `newTestStencilsDir(t)` — whose files carry a realistic leading banner including a `lyx-stencil:` stamp line, because the helper writes them through `ApplyStamp` — returns text that begins at the `## ` heading and contains no `<!--` anywhere in it.
  Cover all three roles.
  This is the regression guard for the correctness bug that would otherwise ship, since `stencilstore.Read` does not strip and `Directive`'s value never passes through `stencil.Fill`.

  4. Stripped-body equality with the embedded default.
  Each role's returned text equals `stencil.StripLeadingComment(string(<the matching stencils package embedded default>))`.
  State the assertion in exactly those terms.
  Do not assert whole-file byte equality against the on-disk fixture and do not assert equality against the raw embedded default: the on-disk file carries a banner and a stamp that the return value never does.
  Add a comment on this test recording what it is and is not worth: it is a cheap tripwire against a future edit to a stencil file drifting from the embedded default, not proof the relocation was faithful — with the constants deleted it effectively compares the stencil against itself, and what actually pins the relocation is the migrated pairwise-distinct and begins-with-own-heading tests plus loom's end-to-end ordering assertion.
- **Commit:** `test(pattern): pin lazy read, missing-stencil error, banner strip, and default parity`

### Card 7: Update the loom call site and seed loom's fixture

- **Context:**
  - `internal/pattern/pattern.go`
  - `internal/loomengine/plan_test.go`
  - `stencils/stencils.go`
- **Edits:**
  - `internal/loomengine/plan.go`
  - `internal/loomengine/prompt_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In `internal/loomengine/plan.go`, inside `PlanSpec`, replace the single-value `pattern.Directive` assignment with the two-value form passing `PlanSpec`'s own `stencilsDir` parameter, and check the error, returning `shuttleengine.Spec{}` and an error wrapped in the file's existing `loom: PlanSpec: %w` house style.
  The call keeps `pattern.RoleImplementer`.
  Nothing else in `PlanSpec` changes, and `composePlanPrompt` keeps its current signature — it already takes the directive as a plain string.

  In `internal/loomengine/prompt_test.go`, extend `newTestStencilsDir` to also seed a `pattern` subdirectory with the three new files from the `stencils` package's embedded defaults — `stencils.PatternDirectiveImplementer` written as `pattern-directive-implementer.md`, `stencils.PatternDirectiveReviewFix` as `pattern-directive-review-fix.md`, and `stencils.PatternDirectiveOrchestrator` as `pattern-directive-orchestrator.md`, all three declared in `stencils/stencils.go` — in the same raw-bytes style the helper already uses for loom's two.
  This is required, not optional: `internal/loomengine/plan_test.go` writes a real PATTERN file and then calls `PlanSpec` with this helper's directory, so PATTERN is active there and the read fires — without the seeding that test hard-errors under the fail-loud posture.
  Update the helper's doc comment, which currently says it seeds "loom's two stencils".

  Do not change `internal/loomengine/plan_test.go` in this card.
  Its PATTERN-active ordering assertion — the injected directive's `## Constraints` preceding `## Step 1` in the rendered prompt — must keep passing unchanged, because it is the end-to-end proof that the stencil-sourced directive still lands in the right place in a real producer prompt.
- **Commit:** `refactor(loom): pass stencilsDir to pattern.Directive and check its error`

### Card 8: Update the burler call site and seed burler's fixture

- **Context:**
  - `internal/pattern/pattern.go`
  - `stencils/stencils.go`
- **Edits:**
  - `internal/burlerengine/engine.go`
  - `internal/burlerengine/prompt_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In `internal/burlerengine/engine.go`, inside `Engine.Run`, replace the single-value `pattern.Directive` assignment with the two-value form passing the already-present `e.stencilsDir` field, and check the error, returning `Result{}` and an error wrapped in the file's existing `burler: %w` house style.
  The call keeps `e.layout` and `pattern.RoleReviewFix`, and stays where it is in `Run`'s control flow — before the instruction-directory creation.
  The `directive` local continues to flow into `composePrompt` exactly as it does today.

  In `internal/burlerengine/prompt_test.go`, extend `newTestStencilsDir` to also seed a `pattern` subdirectory with the three new files from the `stencils` package's embedded defaults — `stencils.PatternDirectiveImplementer` written as `pattern-directive-implementer.md`, `stencils.PatternDirectiveReviewFix` as `pattern-directive-review-fix.md`, and `stencils.PatternDirectiveOrchestrator` as `pattern-directive-orchestrator.md`, all three declared in `stencils/stencils.go` — in the same raw-bytes style the helper already uses for burler's four, and update its doc comment, which currently says it seeds "burler's four stencils".
  This one is a consistency change, not a fix, and must not be reported as one: no burler test activates PATTERN today, so nothing here fails without it.
  It is done for symmetry with the loom and webster helpers, so that the first burler test that ever does activate PATTERN does not fail for an unrelated reason.
- **Commit:** `refactor(burler): pass stencilsDir to pattern.Directive and check its error`

### Card 9: Hoist both webster call sites out of their map literals and cover the error path at each

- **Context:**
  - `internal/pattern/pattern.go`
  - `stencils/stencils.go`
  - `internal/fabricengine/junctionnames.go`
- **Edits:**
  - `internal/websterengine/render.go`
  - `internal/websterengine/template_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Both webster call sites are inline map-literal values, and a two-return-value call cannot sit inline as a map value in Go, so both need the call hoisted above the `values` map with its own error check.
  Neither `RenderRecoveryPrompt` nor `RenderMasterPrompt` changes signature.

  In `internal/websterengine/render.go`, in `RenderRecoveryPrompt`: hoist the existing `fabricengine.StencilsDir(l.HubPath)` derivation — currently made inline in the `composeRecoveryTemplate` call on the line after the map — into a local above the map, call `pattern.Directive` with it and `pattern.RoleImplementer` above the map, check the error and return it wrapped in the file's existing `webster: <what>: %w` house style, set the map's `pattern_directive` entry to the resulting local, and pass the same local to `composeRecoveryTemplate` so the derivation is made once rather than twice.

  Apply the identical shape in `RenderMasterPrompt`, with `pattern.RoleOrchestrator` and `MasterTemplate` in place of `pattern.RoleImplementer` and `composeRecoveryTemplate`.

  In `internal/websterengine/template_test.go`, split the existing `seedHubStencils` helper so that seeding webster's five stencils and seeding the three pattern stencils are separately callable, and have `seedHubStencils` call both — `patternActiveLayout` and `testLayout` both go through it, so seeding once covers both, and `testLayout`-based tests keep asserting an empty pattern_directive because PATTERN stays inactive under that fixture regardless.
  Seed the pattern files from the `stencils` package's embedded defaults — `stencils.PatternDirectiveImplementer` written as `pattern-directive-implementer.md`, `stencils.PatternDirectiveReviewFix` as `pattern-directive-review-fix.md`, and `stencils.PatternDirectiveOrchestrator` as `pattern-directive-orchestrator.md`, all three declared in `stencils/stencils.go` — in the same raw-bytes style the helper already uses.
  Update the helper doc comments that currently say "webster's five stencils".

  Add a missing-stencil error-path test at **both** `RenderRecoveryPrompt` and `RenderMasterPrompt`: PATTERN active against a hub whose stencils directory holds webster's five but not the three pattern files, asserting a non-nil error.
  Use the webster-only seeding function the split above makes available, so the negative fixture is built from the same source as the positive one rather than by hand.
  Both sites, not one: these are the two call sites with genuinely new control flow, and a hoist that drops its error check is precisely the mistake a test at the *other* site would not catch.

  Do not weaken the existing PATTERN-active assertion in `TestRenderRecoveryPrompt_InstructsColdOrientation`, which requires the rendered prompt to contain `_lyx/PATTERN.md` and `## Constraints`;
  it now proves the stencil-sourced directive reaches the prompt.
  Do not touch the hardcoded placeholder `pattern_directive` values in the pure-template tests in this file — those never call `Directive`.
- **Commit:** `refactor(webster): hoist pattern.Directive out of both map literals and check its error`

## Batch Tests

`verify:` is the full `go build ./... && go test ./...`, per the overview's Shared Decision on verify scope.
That command is what proves this batch at all, since the change is a signature change whose blast radius is four packages plus the guards.
The measured warm baseline for it on this tree is about seven seconds, so the full suite is the cheap option here, not the expensive one.

The tests that carry this batch, and what each one is for:

- `internal/pattern/pattern_test.go` — the ten migrated tests are the relocation guard (the `_lyx/PATTERN.md` and `_lyx/pattern/` substrings present, `_pattern/` absent, the `##` heading prefix, the three variants pairwise distinct), and the four new tests pin lazy read, the missing-stencil error, the banner strip, and stripped-body parity with the embedded default.
- `internal/pattern/leaf_enforcement_test.go` — `TestLeafInvariant_AllowlistOnly` fails on the two new production imports until card 4 lands, which is exactly what makes the invariant amendment deliberate rather than incidental.
- `internal/loomengine/plan_test.go` — the PATTERN-active ordering assertion is the end-to-end proof that the stencil-sourced directive still renders in the right position inside a real producer prompt.
  It is not edited by this batch;
  it must simply keep passing.
- `internal/websterengine/template_test.go` — the PATTERN-active recovery-prompt assertion plus the two new missing-stencil error tests, one per hoisted call site.
- `cmd/lyx`'s tier-purity guard — confirms `internal/pattern`'s new test helper has not moved the package's untagged test file out of Tier 1.

No test is added for `internal/burlerengine`: its fixture change is a symmetry change with no behaviour behind it, and inventing a test for it would misrepresent it as a fix.
