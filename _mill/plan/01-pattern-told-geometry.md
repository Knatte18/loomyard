# Batch: pattern told geometry

```yaml
task: "pattern told-geometry"
batch: "pattern told geometry"
number: 1
cards: 5
verify: go test ./internal/pattern/... ./internal/burlerengine/... ./internal/websterengine/... ./internal/loomengine/... ./cmd/lyx/...
depends-on: []
```

## Batch Scope

This batch delivers T4 of `manifest/designs/producers-standalone.md` in full: `internal/pattern` stops taking a `*lyxcwd.Location` and is told its anchor directory as a plain string, `FileHere` is deleted, `internal/lyxcwd` leaves the package entirely (production and test files alike), the four production call sites pass `l.AnchorPath()` inline, the two call sites with no behavioural PATTERN coverage gain a transposition detector, and the Pattern Leaf Invariant plus the package godoc are narrowed in the same commit series.

It is one batch because the signature change, its four call sites and the tests that pin them are a single atomic refactor: the tree does not compile with any subset of them applied.
The card order is chosen so the compile-broken window is exactly one commit wide — card 1 leaves `internal/pattern` self-consistent and green in isolation while its four consumers still pass a `*lyxcwd.Location`, and card 2 closes that window.
Cards 3 and 4 are purely additive test cards, and card 5 is the documentation narrowing.

No batch-local decision differs from `## Shared Decisions` in the overview.

There is no external interface for a next batch to consume — this is the only batch in the plan.

## Cards

### Card 1: Convert `pattern.Directive`/`isActive` to told geometry and port the package's own tests

- **Context:**
  - `internal/pattern/doc.go`
  - `internal/lyxdirs/dirs.go`
- **Edits:**
  - `internal/pattern/pattern.go`
  - `internal/pattern/pattern_test.go`
  - `internal/pattern/patternpath_test.go`
  - `internal/pattern/leaf_enforcement_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In `internal/pattern/pattern.go`: change `Directive`'s signature from `Directive(l *lyxcwd.Location, stencilsDir string, role Role) (string, error)` to `Directive(anchorPath, stencilsDir string, role Role) (string, error)`, and replace its `if l == nil` early return with `if anchorPath == ""`, keeping that return in exactly the same position — before the `isActive` call — and keeping its `("", nil)` result byte-identical.
  Change `isActive`'s signature from `isActive(l *lyxcwd.Location) bool` to `isActive(anchorPath string) bool`, and change its body's single `statFile(FileHere(l))` call to `statFile(File(anchorPath))`.
  Every other line of `isActive`'s body — the `os.IsNotExist` branch, the non-`IsNotExist`-means-active rule, and the `info.IsDir()` check — stays byte-identical, along with all three of their explanatory comments.
  Delete `FileHere` and its four-line doc comment outright.
  Delete the `github.com/Knatte18/loomyard/internal/lyxcwd` import.
  Rewrite `Directive`'s doc-comment sentence that today reads "for a nil l" so it names an empty `anchorPath` instead, and rewrite `isActive`'s doc comment so that "an absent `FileHere(l)`" names `File(anchorPath)` instead.
  Do not change `File`, `PathspecFile`, `PathspecDir`, `patternFileName`, the three `Role` constants, the three stencil-name constants, the `statFile` seam, or the role-switch and `stencilstore.Read` body of `Directive`.
  Do not touch `internal/pattern/doc.go` in this card — card 5 owns every change to it.
  In `internal/pattern/pattern_test.go`: delete the `layoutAt` helper and the `github.com/Knatte18/loomyard/internal/lyxcwd` import, and convert every call site that today builds `l := layoutAt(root, ".")` and passes `l` so it passes the `root` string directly to `Directive`.
  The affected tests are `TestDirective_ActiveWithFile`, `TestDirective_InactiveWithoutFile`, `TestDirective_EmptyPatternFileIsActive`, `TestDirective_PatternFileAsDirectoryIsInactive`, `TestDirective_UnknownRole`, `TestDirective_VariantsArePairwiseDistinct`, `TestDirective_VariantsBeginWithOwnHeading`, `TestDirective_NonNotExistStatErrorIsActive`, `TestDirective_LazyRead`, `TestDirective_MissingStencilErrors`, `TestDirective_StripsBanner` and `TestDirective_StrippedBodyMatchesEmbeddedDefault`.
  Every one of those tests keeps its assertions exactly as written — only the fixture plumbing changes.
  An assertion that has to be weakened to make the port compile is a signal the port is wrong; report it rather than weakening it.
  Replace `TestDirective_NilLayout` with `TestDirective_EmptyAnchorPath`, written first, before `Directive`'s guard is converted.
  It calls `Directive("", newTestStencilsDir(t), RoleImplementer)`, asserts the `("", nil)` return, and additionally asserts that no stat was attempted, by assigning a recording closure to the package-level `statFile` variable that sets a bool and returns `nil, os.ErrNotExist`, restoring the original through `t.Cleanup` exactly as `TestDirective_NonNotExistStatErrorIsActive` already does.
  Its doc comment states why the return-value assertion alone would be insufficient: an inactive PATTERN returns the same pair, so only the absence of the stat distinguishes the guard from a cwd-dependent lookalike.
  In `TestDirective_LazyRead`, keep both sub-tests: convert the "nil layout, stencilsDir does not exist" sub-test to pass `""` as the anchor path and rename it to name an empty anchor path rather than a nil layout, and convert its sibling "PATTERN inactive, stencilsDir does not exist" sub-test by fixture plumbing alone.
  Do not delete either sub-test as subsumed by `TestDirective_EmptyAnchorPath` — the value is that this table keeps enumerating every inactive path in one place.
  Rename `TestDirective_RelPathNestedSubdirectory` to `TestDirective_NestedAnchorSubdirectory` and reword its six-line header comment out of the `lyxcwd` vocabulary it uses today ("a Layout whose RelPath", "`<WorktreeRoot>/<RelPath>`") into the told-geometry vocabulary of an anchor path given as a nested subdirectory.
  Its body passes `filepath.Join(root, "sub", "dir")` as the anchor path, and both of its assertions keep their logic exactly as written — the root-planted `PATTERN.md` must still not satisfy it, and the nested-planted one must.
  The reword also covers the three remaining `RelPath` spellings inside the body, which the card's own `grep -rn lyxcwd` gate does not match: the inline comment above the first assertion that today calls the resolution "RelPath-aware", the first `t.Errorf` message's "via a nested RelPath", and the second `t.Error` message's "`<WorktreeRoot>/<RelPath>/_lyx/PATTERN.md`".
  Reword each to name the told anchor path; change no condition, no fixture and no control flow.
  In `internal/pattern/patternpath_test.go`: delete `TestLocation_PatternAccessors`, `TestFileHere_EqualsFileOfAnchorPath`, the `newTestLocation` helper, and the `github.com/Knatte18/loomyard/internal/lyxcwd` import.
  Leave `TestFile_Free`, `TestPathspec_ExactStrings` and `TestPathspec_ForwardSlashedOnEveryPlatform` byte-identical.
  Amend the file-header comment so its "the `File/FileHere` constructors" phrasing names only `File`.
  In `internal/pattern/leaf_enforcement_test.go`: remove the `github.com/Knatte18/loomyard/internal/lyxcwd` entry from the `allowedImports` map, drop `internal/lyxcwd` from the file-header comment's allowlist sentence, and drop `lyxcwd` from `TestLeafInvariant_AllowlistOnly`'s failure message.
  Tighten the map before removing the import from `internal/pattern/pattern.go`, and run `go test ./internal/pattern/ -run TestLeafInvariant_AllowlistOnly` in that intermediate state to confirm it fails; a green run over an unchanged map proves nothing.
  After the whole card is applied, `grep -rn lyxcwd internal/pattern/` must return no hits at all, production and test files alike.
- **Commit:** `refactor(pattern): take a told anchor path instead of a lyxcwd.Location`

### Card 2: Pass `l.AnchorPath()` at the four call sites and rewrite the `cmd/lyx` anchoring rows

- **Context:**
  - `internal/pattern/pattern.go`
  - `internal/loomengine/plan_test.go`
  - `internal/webstercli/verbs_test.go`
- **Edits:**
  - `internal/burlerengine/engine.go`
  - `internal/websterengine/render.go`
  - `internal/loomengine/plan.go`
  - `cmd/lyx/constructoranchoring_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Change the first argument at each of the four `pattern.Directive` call sites, and nothing else about those functions.
  In `internal/burlerengine/engine.go`, the call inside `Engine.Run` becomes `pattern.Directive(e.layout.AnchorPath(), e.stencilsDir, pattern.RoleReviewFix)`.
  In `internal/websterengine/render.go`, the call inside `RenderRecoveryPrompt` becomes `pattern.Directive(l.AnchorPath(), stencilsDir, pattern.RoleImplementer)` and the call inside `RenderMasterPrompt` becomes `pattern.Directive(l.AnchorPath(), stencilsDir, pattern.RoleOrchestrator)`.
  In `internal/loomengine/plan.go`, the call inside `PlanSpec` becomes `pattern.Directive(layout.AnchorPath(), stencilsDir, pattern.RoleImplementer)`.
  Do not add an `anchorRoot` field to `burlerengine.Engine`, do not change `burlerengine.New`'s signature, and do not change the signature of `RenderRecoveryPrompt`, `RenderMasterPrompt` or `PlanSpec` — all four keep their `*lyxcwd.Location` parameters.
  Do not re-create a nil-`Location` guard at any of the four sites.
  Do not reorder, hoist or otherwise move any of the four calls relative to their surrounding statements.
  In `cmd/lyx/constructoranchoring_test.go`, rewrite the single `pattern.FileHere` row in `TestConstructorAnchoring_Unanchored` and the single `pattern.FileHere` row in `TestConstructorAnchoring_SubpathAnchored` to call `pattern.File(l.AnchorPath())`, updating each row's first argument (the constructor name string `assertPath` reports on) to `pattern.File` and leaving each row's `want` expression byte-identical.
  Amend — do not duplicate — the six-line tautology comment that precedes the `planparser` rows in each of those two tests.
  Each amended comment must name the covered rows explicitly, as the two `planparser` rows plus the `pattern.File` row, and must not restate a contiguous count such as "the three rows below": those rows are not contiguous, and the seven constructor rows sitting between them still take `l` rather than `l.AnchorPath()` and are therefore not tautological.
  Leave each comment's existing pointer list — the two cases it already names — as it stands in this card; card 3 adds its own new test to that list, in the same commit that creates it.
  Leave every other row, the `anchoringFixture` helper, the `assertPath` helper, the `.lyx`-group prefix-exclusion guard and the file-header comment untouched.
- **Commit:** `refactor(pattern): pass l.AnchorPath() at every pattern.Directive call site`

### Card 3: Relocate the PATTERN-anchoring proof into `loomengine`'s `PlanSpec` suite

- **Context:**
  - `internal/loomengine/plan.go`
  - `internal/loomengine/prompt_test.go`
  - `internal/pattern/pattern.go`
- **Edits:**
  - `internal/loomengine/plan_test.go`
  - `cmd/lyx/constructoranchoring_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add one new test, `TestPlanSpec_PatternDirectiveAnchoredUnderAnchorPath`, placed immediately after `TestPlanSpec_AnchoredUnderAnchorPathNotWorktreePath`.
  It carries the anchoring signal that card 2's `cmd/lyx` row rewrite would otherwise silently drop.
  The fixture builds a `*lyxcwd.Location` with `HubPath` set to a real `t.TempDir()`, a `WorktreeName`, and `AnchorRel: "backend"` — a non-`"."` `AnchorRel` is what makes `AnchorPath()` and `WorktreePath()` distinguishable roots, and a real temp root is mandatory rather than a preference.
  Use `newTestStencilsDir(t)` for the stencils directory and `modelspec.LoadRegistry(t.TempDir())` for the registry, matching the neighbouring tests, with `Config{Plan: "opus[effort=high]", PlanTimeoutMin: 120}`.
  The positive direction creates the directories under `layout.AnchorPath()` and writes a `PATTERN.md` file under `filepath.Join(layout.AnchorPath(), lyxdirs.LyxDirName)` and nowhere else, then asserts `PlanSpec`'s returned `Prompt` contains the directive text.
  Assert on the `"## Constraints"` heading, which is the same marker `TestPlanSpec_PatternDirectiveOptional` already uses as its directive-presence signal in both directions.
  Pair it with the negative direction, as a sub-test of the same test, on its own fresh `t.TempDir()` hub: write the `PATTERN.md` file under `filepath.Join(layout.WorktreePath(), lyxdirs.LyxDirName)` instead and assert the `Prompt` does not contain that heading.
  Without the negative half a `PlanSpec` that read both roots would pass.
  In `cmd/lyx/constructoranchoring_test.go`, add this new test's name to the pointer list in each of the two tautology comments card 2 amended, alongside the two cases each already names — in this card, the same commit that creates the test it points at.
  Change nothing else in that file: card 2 owns the two rewritten `pattern.File` rows and the rest of each comment's wording.
  Do not modify `TestPlanSpec_AnchoredUnderAnchorPathNotWorktreePath` — its root is the relative, never-created `filepath.Join("home", "user", "repo")`, so any fixture written under it would land inside the source tree rather than a temp directory.
  Its own comment explains only why it uses a non-`"."` `AnchorRel`, not why its root is uncreated; do not restate that comment as if it recorded the filesystem-free property.
  Do not extend or parameterise `TestPlanSpec_PatternDirectiveOptional` to host this proof — its subject is directive optionality and the directive's position relative to `## Step 1`, and folding an anchoring assertion into it would give one test two unrelated reasons to fail.
  Do not add a new stencils helper: `newTestStencilsDir` already seeds all three `pattern-directive-*` stencils from the embedded defaults.
- **Commit:** `test(loomengine): pin PlanSpec's PATTERN directive to AnchorPath, not WorktreePath`

### Card 4: Add the missing transposition detector for `burlerengine`'s `Directive` call site

- **Context:**
  - `internal/burlerengine/engine.go`
  - `internal/burlerengine/prompt.go`
  - `internal/burlerengine/prompt_test.go`
  - `internal/pattern/pattern.go`
- **Edits:**
  - `internal/burlerengine/engine_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add one new test with a matched active/inactive pair of sub-tests, working name `TestEngine_Run_PatternDirectiveReachesInstruction1`, closing the one `pattern.Directive` call site that has no behavioural coverage at all today.
  Reuse the existing same-package machinery: the `fakeShuttle` double, `newEngineTestProfile(t)`, `newEngineForTest(t, root, shuttle)` and `newTestStencilsDir(t)`.
  Add no new test double, no new stencils helper and no new fixture constructor — the package's stencils helper already seeds all three `pattern-directive-*` stencils from the embedded defaults.
  The active sub-test writes a `PATTERN.md` file under `filepath.Join(root, lyxdirs.LyxDirName)` before calling `Run`, where `root` is the `t.TempDir()` that `newEngineTestProfile` returns and that `newEngineForTest` roots its `*lyxcwd.Location` at.
  The inactive sub-test writes no such file.
  Both drive `Run` to `shuttleengine.OutcomeDone` with the file-writing `fakeShuttle` exactly as `TestEngine_Run_SpecConstruction` does.
  The assertion reads `instruction-1-explore.md` from disk rather than the recorded `Spec.Prompt`: `Engine.Run` passes the directive into `composePrompt` as `patternDirective`, and `composePrompt` fills it into the instruction-1 values map only, through `stencil.FillOptional` with `pattern_directive` in the optional set, so instructions 2 and 3 never receive it and the orchestrator prompt never carries it.
  Discover the per-round directory rather than naming it: `Engine.Run` creates it with `os.MkdirTemp` under `filepath.Join(e.layout.AnchorPath(), lyxdirs.DotLyxDirName, "burler")`, so its suffix is random per run.
  Read that burler directory and take its single entry, which the fixture guarantees is the only one.
  Do not hardcode a `round-` prefixed path.
  The active sub-test asserts the read instruction-1 content contains the literal relative pointer `_lyx/PATTERN.md`, which every role variant carries; the inactive sub-test asserts it does not.
  The pair is what makes the assertion meaningful — a presence-only assertion cannot distinguish a working read from a template that hardcodes the text.
  Amend the file-header comment, which today enumerates every subject the file tables — spec construction, the cluster audit policy wiring, every outcome, the review-file parse path, and the per-round instruction-file materialization step — so the PATTERN-directive detector is named among them.
  The header is an enumeration, so adding a subject without listing it there leaves it stale.
  Do not modify the `//go:build smoke` tests: they spawn live agents, are opt-in, and are not this detector.
- **Commit:** `test(burlerengine): pin the PATTERN directive reaching the round's instruction 1`

### Card 5: Narrow the Pattern Leaf Invariant and the package godoc

- **Context:**
  - `internal/pattern/pattern.go`
  - `internal/pattern/leaf_enforcement_test.go`
  - `internal/lyxdirs/dirs.go`
- **Edits:**
  - `CONSTRAINTS.md`
  - `internal/pattern/doc.go`
  - `internal/websterengine/template_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In `CONSTRAINTS.md`, edit the Pattern Leaf Invariant's opening allowlist sentence only, striking `internal/lyxcwd` from the list of packages `internal/pattern` production code may import, so it reads stdlib plus `internal/lyxdirs`, `internal/stencilstore` and `internal/stencil`.
  Leave the invariant's three following paragraphs — the ones justifying `internal/lyxdirs`, `internal/stencil` and `internal/stencilstore` — byte-identical, and leave the `- **Enforced by**` bullet byte-identical: it names the test file and nothing else and carries no allowlist, exactly as T3 left the Tokenvocab Leaf Invariant's.
  Do not touch any other invariant in the file.
  In `internal/pattern/doc.go`, rewrite the three places the package godoc goes stale with the signature.
  The "# The active check is pure existence" section's opening currently resolves the path "via this package's own `FileHere(l)` and nothing else" and explains that `FileHere` is what constructs it; rewrite it to name `File` applied to the caller-supplied anchor directory.
  The "# The stencil read path" section's closing sentence currently lists "a nil layout" among the cases where no read is attempted; rewrite it to name an empty anchor path.
  The "# Why the pointer stays relative" section currently contrasts the literal relative pointer against "an interpolated absolute path built from a `Location` field"; rewrite it to contrast against an interpolated absolute path built from the caller-supplied anchor path.
  Preserve the rest of the godoc verbatim, including the three edge-case rules, the three-roles rationale, and the `PathspecFile`/`PathspecDir` section.
  Preserve in particular the sentence recording that keeping `File` and the pathspec constants built from `lyxdirs.LyxDirName` is a review obligation this package accepts rather than something a test mechanically enforces.
  Introduce no bare `_lyx` token anywhere in the rewritten Go source or its comments.
  In `internal/websterengine/template_test.go`, correct the stale comment on the `patternActiveLayout` fixture, which today says the fixture mirrors "`pattern.isActive`'s own `PatternFileHere()` check" and points at the `writePatternFile`/`layoutAt` fixtures in the pattern package's own test file.
  `PatternFileHere` is already a stale name today and `layoutAt` no longer exists after card 1; name the told-anchor-path check instead.
  This is a comment-only change: leave the `patternActiveLayout` and `testLayout` fixture bodies, and every test in the file, byte-identical — both build `*lyxcwd.Location` values for `Render*`, which still take one.
  Do not update `docs/overview.md`: its two `internal/pattern` lines describe what the package computes rather than how it is parameterised, and stay accurate as written.
  Do not move `manifest/roadmap.md`: its "producers standalone: mid-layer" Planned entry spans both T4 and T5, so moving it on T4 alone would mark unshipped work done.
  Do not rewrite anything under `manifest/designs/`: those are historical records of what was decided, and two of them carry references this task makes stale on purpose.
- **Commit:** `docs(pattern): narrow the Pattern Leaf Invariant to drop lyxcwd`

## Batch Tests

The batch `verify:` command is the design doc's own named focused set: `go test ./internal/pattern/... ./internal/burlerengine/... ./internal/websterengine/... ./internal/loomengine/... ./cmd/lyx/...`.
Those five package trees are exactly the ones this batch's `Edits:` touch, so the scope is per-batch, not a full-repo suite.
The repo-wide regression sweep is already covered outside the batch loop by `pipeline.done_gate`, which mill-go runs from the git root before marking the task done.

What each package contributes:

- `internal/pattern` — the package's own suite carries the load. `TestDirective_EmptyAnchorPath` is the one new test that pins behaviour a naive port would silently break, and it is written before the guard is converted.
  `TestLeafInvariant_AllowlistOnly` over the tightened `allowedImports` map is what makes the leaf narrowing machine-checked, and it must be observed failing in the intermediate state where the map is tightened but the import is not yet gone.
  Every other `Directive` case — the three roles, the active/inactive matrix, the empty-file and directory-in-place edge rules, the non-`IsNotExist` stat error, the banner strip, the unknown and zero role, and the nested-anchor case — is a regression anchor whose assertions survive the port unchanged.
- `internal/loomengine` — card 3's new `TestPlanSpec_PatternDirectiveAnchoredUnderAnchorPath` is both the relocated anchoring proof for the rewritten `cmd/lyx` rows and the transposition detector for the `plan.go` call site.
- `internal/burlerengine` — card 4's new active/inactive pair is the transposition detector for the `engine.go` call site, the one site with no PATTERN coverage before this task.
- `internal/websterengine` — no new tests. Its existing suite is the regression check that the mechanical argument change compiles and behaves: `template_test.go` already carries an inactive-PATTERN fixture, an active-PATTERN fixture covering `RenderRecoveryPrompt`, and a separate missing-stencil fixture covering `RenderMasterPrompt`, and all three must stay green with byte-identical expectations.
- `cmd/lyx` — `constructoranchoring_test.go` must stay green in both its root-anchored and subpath-anchored tables with the two rewritten `pattern.File` rows.

Two checks are not expressible as a test and are review obligations on the diff: that `grep -rn lyxcwd internal/pattern/` returns nothing at all, production and test files alike; and that `leaf_enforcement_test.go`'s allowlist was tightened rather than left permissive.
