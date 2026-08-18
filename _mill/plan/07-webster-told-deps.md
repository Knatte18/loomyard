# Batch: websterengine told Deps and engine-owned fabric seams

```yaml
task: websterengine + webstercli told-geometry, and Webster standalone entry
batch: websterengine told Deps and engine-owned fabric seams
number: 7
cards: 12
verify: go test ./internal/websterengine/... ./internal/webstercli/... ./cmd/lyx/... && go test -tags integration ./internal/websterengine/... ./internal/webstercli/...
depends-on: [5, 6]
```

## Batch Scope

This is the migration's centre of gravity: `internal/websterengine` stops holding a `*lyxcwd.Location` and stops importing `internal/fabricengine` altogether.
The four `Deps` structs trade `Layout` and their flat path fields for one `Geom Geometry`;
`CheckFork` and `CheckParent` take the `RefMatcher` seam batch 4 declared;
the bisect repo becomes a caller-supplied `OpenBisector` closure with the `fabricengine.Open` fallback deleted;
and `RunResult` gains the `Warnings` channel `run`'s integration stage has never had.
It is one batch because a Go package is one compile unit — the signature changes, their internal call sites, and the seven test files holding `Location` fixtures cannot land separately and still build.
The batch also carries the *hub-mode* half of `internal/webstercli`'s adaptation (building `c.geom` from `hubgeom.WebsterGeometry` and feeding the two new seams), so the repository stays green at this batch's boundary;
batch 8 then deletes `c.layout` and adds standalone mode on top of a tree that already compiles.
Two behaviour changes ride along and are deliberate: the fork-audit workdir becomes `Geom.WorktreeRoot` rather than `Geom.AnchorRoot`, and a nil `OpenBisector` records an unlocalized integration failure instead of panicking.
Both are provably no-ops in hub mode, where `AnchorRoot == WorktreeRoot` and the opener is always non-nil.

## Cards

### Card 22: `CheckFork` and `CheckParent` take a `RefMatcher`

- **Context:**
  - `internal/fabricengine/refscanner.go`
  - `internal/websterengine/geometry.go`
- **Edits:**
  - `internal/websterengine/audit.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Change `CheckFork`'s and `CheckParent`'s final parameter from `fabricRef *fabricengine.RefScanner` to `fabricRef RefMatcher`, keeping the parameter name and every line of both function bodies unchanged — `*fabricengine.RefScanner` already satisfies the interface, so both call sites keep compiling on the real scanner.
  Remove the `internal/fabricengine` import from `internal/websterengine/audit.go`;
  after this card no production file in this package may import that module.
  Update both function doc comments to name the injected matcher rather than the concrete scanner, and add a sentence to each saying the matcher is never nil in either mode — `Matches` is called unguarded here, so a nil interface is a panic, which is why `NeverMatches` exists as the pinned no-fabric supplier.
- **Commit:** `refactor(websterengine): take the RefMatcher seam in CheckFork and CheckParent`

### Card 23: The three renderers take told strings

- **Context:**
  - `internal/pattern/pattern.go`
  - `internal/stencilstore/reconcile.go`
  - `internal/websterengine/geometry.go`
  - `contracts/stencils/webster/webster-body-implementer.md`
  - `contracts/stencils/webster/webster-template-master.md`
- **Edits:**
  - `internal/websterengine/render.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Replace the `l *lyxcwd.Location` parameter on the three renderers with told strings.
  They deliberately do **not** share one signature — each takes exactly the roots it uses.
  `RenderForkPrompt` takes `promptWorktreeRoot, stencilsDir`: it uses the Location only for the `worktree_root` value and the stencils directory, and makes no `pattern.Directive` call.
  `RenderRecoveryPrompt` takes `anchorRoot, promptWorktreeRoot, stencilsDir`: it needs all three, since it fills `worktree_root`, reads its templates from the stencils directory, and calls `pattern.Directive` with the anchor root.
  `RenderMasterPrompt` takes `anchorRoot, stencilsDir` only: it calls `pattern.Directive` and reads its template, and fills no `worktree_root` key at all — do not add one, and do not add the token to `webster-template-master`.
  Every internal `fabricengine.StencilsDir(l.HubPath)` derivation in this file disappears, replaced by the told `stencilsDir` parameter.
  The parameter is named `promptWorktreeRoot`, not `anchorRoot`, on purpose: the two are the same value in hub mode and different in standalone, and reusing the anchor name is what would re-encode the confusion this split exists to remove.
  Remove the `internal/fabricengine` and `internal/lyxcwd` imports from this file.
  `RenderIntegrationPrompt` already takes told strings and does not change.
  Update the file-header comment's claim that assets are read "from the hub's stencils directory (fabricengine.StencilsDir)" so it names a told stencils directory instead, and update `RenderForkPrompt`'s and `RenderRecoveryPrompt`'s doc comments, which currently say `{{.worktree_root}}` is filled from the Location's anchor path, to say it is filled from the caller-supplied `promptWorktreeRoot`.
- **Commit:** `refactor(websterengine): render prompts from told roots and a told stencils dir`

### Card 24: `BeginDeps` carries a `Geometry`

- **Context:**
  - `internal/websterengine/geometry.go`
  - `internal/websterengine/render.go`
- **Edits:**
  - `internal/websterengine/beginbatch.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Replace `BeginDeps`' `Layout`, `WorktreeRoot`, `WebsterDir`, `ReportsDir`, `PromptsDir` and `ScratchDir` fields with a single `Geom Geometry` field.
  Every use in this file becomes the matching `deps.Geom` accessor: the pause probe and the report/prompt directory creations, the `headSHA` capture, and the report and prompt path joins.
  The `RenderForkPrompt` call passes `deps.Geom.WorktreeRoot` as `promptWorktreeRoot` and `deps.Geom.StencilsDir` as `stencilsDir`.
  Passing `WorktreeRoot` rather than `AnchorRoot` is correct in both modes and is not a hub-mode behaviour change: hub mode's `WorktreeRoot` is the anchor path, the exact value this call renders today.
  Drop the `internal/lyxcwd` import if nothing else in the file uses it.
  Rewrite the `BeginDeps` doc comment's field descriptions accordingly, deleting the sentence that describes `Layout` as the resolved Location `RenderForkPrompt` uses.
- **Commit:** `refactor(websterengine): give BeginDeps a told Geometry`

### Card 25: `RecordDeps` carries a `Geometry` and a `RefMatcher`

- **Context:**
  - `internal/websterengine/geometry.go`
  - `internal/websterengine/audit.go`
- **Edits:**
  - `internal/websterengine/recordbatch.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Replace `RecordDeps`' `Layout`, `WorktreeRoot` and `ReportsDir` fields with `Geom Geometry`, and add a `RefMatcher RefMatcher` field carrying the injected matcher.
  Delete the `fabricRef := fabricengine.NewRefScanner(deps.Layout)` construction and the `internal/fabricengine` and `internal/lyxcwd` imports;
  use `deps.RefMatcher` at both `CheckParent` and `CheckFork` call sites.
  The three workdir arguments — `AuditForksIncremental`'s workdir and `CheckParent`'s and `CheckFork`'s `workdir` — currently pass `deps.Layout.AnchorPath()` and must become `deps.Geom.WorktreeRoot`, **not** `deps.Geom.AnchorRoot`.
  This is the card's one real behaviour decision and it must not be converted mechanically: the audit resolves transcript-relative recorded write paths against this directory, so it must be the directory the fork was actually running in.
  In hub mode the two values are identical, so nothing changes there;
  in standalone, `AnchorRoot` is the hidden state directory and joining a fork's relative write path onto it would mis-attribute or silently exonerate every file the audit exists to police.
  The dirty-worktree warning and the head-SHA cross-check read `deps.Geom.WorktreeRoot`, and the report path joins `deps.Geom.ReportsDir`.
  Rewrite the `RecordDeps` doc comment's `Layout` sentence to describe `Geom` and the injected `RefMatcher`, stating explicitly that the audit workdir is `Geom.WorktreeRoot` and why.
- **Commit:** `refactor(websterengine): give RecordDeps a told Geometry and an injected RefMatcher`

### Card 26: `RecoverDeps` carries a `Geometry`

- **Context:**
  - `internal/websterengine/geometry.go`
  - `internal/websterengine/render.go`
  - `internal/shuttleengine/rundir.go`
- **Edits:**
  - `internal/websterengine/recoverbatch.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Replace `RecoverDeps`' `Layout`, `WorktreeRoot`, `WebsterDir` and `ReportsDir` fields with `Geom Geometry`, and update every use in the file to the matching `deps.Geom` accessor.
  The `RenderRecoveryPrompt` call passes `deps.Geom.AnchorRoot` as `anchorRoot`, `deps.Geom.WorktreeRoot` as `promptWorktreeRoot`, and `deps.Geom.StencilsDir` as `stencilsDir`.
  The `shuttleengine.FindRun` call currently passes `deps.Layout.AnchorPath()` as its anchor argument;
  pass `deps.Geom.AnchorRoot`, which is the anchor-shaped value in both modes.
  The two `headSHA` captures read `deps.Geom.WorktreeRoot`.
  Drop the `internal/lyxcwd` import if nothing else in the file uses it, and update the `RecoverDeps` doc comment's field list.
- **Commit:** `refactor(websterengine): give RecoverDeps a told Geometry`

### Card 27: `RunDeps` carries a `Geometry`, a `RefMatcher` and an opener

- **Context:**
  - `internal/websterengine/geometry.go`
  - `internal/websterengine/render.go`
  - `internal/websterengine/audit.go`
  - `internal/websterengine/integration.go`
  - `internal/websterengine/recoverbatch.go`
  - `internal/planparser/parse.go`
  - `internal/planparser/validate.go`
- **Edits:**
  - `internal/websterengine/runlevel.go`
  - `internal/shedadapters/webster.go`
  - `internal/shedadapters/webster_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Replace `RunDeps`' `Layout`, `PlanDir`, `WebsterDir`, `ReportsDir`, `PromptsDir`, `ScratchDir` and `WorktreeRoot` fields with one `Geom Geometry` field.
  Replace the `Bisector FabricBisector` field with `OpenBisector func() (FabricBisector, error)`, and add a `RefMatcher RefMatcher` field.
  Update every consumer in the file: `planparser.ParsePlan` and `fingerprint` take `deps.Geom.PlanDir`;
  `planparser.Validate`'s second argument takes `deps.Geom.WorktreeRoot`;
  the zero-batch refusal message names `deps.Geom.PlanDir`;
  `LoadState`/`SaveState`, the archive helpers, `AcquireStateMutation`, `ClearPause`, `OutcomePath`/`SummaryPath`, `AwaitIntegration`, `ParseReport`, `IntegrationReportPath` and `verifyEveryBatchDone` take the matching `deps.Geom` directories.
  The `RenderIntegrationPrompt` call passes `deps.Geom.WorktreeRoot` and `deps.Geom.StencilsDir`, replacing the inline `fabricengine.StencilsDir(deps.Layout.HubPath)` derivation.
  The `RenderMasterPrompt` call passes `deps.Geom.AnchorRoot` and `deps.Geom.StencilsDir`.
  The single `shuttleengine.FindRun` call in this file passes `deps.Geom.AnchorRoot`, mirroring card 26's identical change to the other call site, which lives in `internal/websterengine/recoverbatch.go` and is already converted by then.
  In `runExitAuditCrossCheck`, delete the `fabricengine.NewRefScanner(deps.Layout)` construction, use `deps.RefMatcher`, and pass `deps.Geom.WorktreeRoot` as the workdir to `CheckParent` and `CheckFork` — the same audit-workdir reasoning card 25 records applies verbatim here.
  Remove the `internal/fabricengine` and `internal/lyxcwd` imports from this file.
  Rewrite the `RunDeps` doc comment and the `OpenBisector` field comment: the opener is caller-supplied rather than defaulted, it stays lazy so a run that never reaches a failing integration suite never opens a fabric handle, tests inject a fake by supplying an opener that returns it, and a nil opener means "no fabric in this mode" rather than "construct the production default".
  `internal/shedadapters/webster.go`'s `WebsterProducer.Call` reads `p.deps.WebsterDir` to build its `shedengine.OutputPointer`;
  convert that one read to `p.deps.Geom.WebsterDir`, and convert every `websterengine.RunDeps{WebsterDir: dir}` (and the one `WebsterDir: websterDir, ScratchDir: scratchDir` literal) in `internal/shedadapters/webster_test.go` to the matching `Geom: Geometry{WebsterDir: dir}` (or `{WebsterDir: websterDir, ScratchDir: scratchDir}`) shape — this task's own `go build ./...` union, not just the batch's own `verify:` package set, so a stale flat-field read here breaks the whole-repo build.
- **Commit:** `refactor(websterengine): give RunDeps a told Geometry, a RefMatcher and a bisector opener`

### Card 28: Record an unlocalized integration failure when there is no bisector

- **Context:**
  - `internal/websterengine/integration.go`
  - `internal/websterengine/summary.go`
  - `internal/websterengine/geometry.go`
  - `internal/websterengine/recordbatch.go`
- **Edits:**
  - `internal/websterengine/runlevel.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add a `Warnings []string` field to `RunResult`, copying `RecordResult.Warnings`' shape and contract verbatim — non-fatal observations, never a failure.
  Change `runIntegrationStage` to return `([]string, error)` instead of `error`, and have `Run` accumulate the returned slice into the `RunResult` it hands back on the `OutcomeDone` path.
  Delete the `if bisector == nil { fabricengine.Open(deps.Layout) }` fallback entirely.
  In its place: when `deps.OpenBisector` is nil, **bypass `BisectAndEscalate` and `bisect` completely** and call `RecordIntegrationFailure(st, "unknown", "unknown")` followed by `AppendIntegrationFailure(deps.Geom.WebsterDir, "unknown", "unknown")` directly, then append one warning explaining that the integration suite failed and the offending card could not be localized because this mode has no fabric repo to bisect against.
  The bypass must be at the call site and must not be pushed down into `BisectAndEscalate` or `bisect`:
  that function's empty-SHA fallback is unreachable here because card SHAs accumulate normally, a single accumulated SHA would make it record a *real* SHA under a "cannot localize" claim, and two or more reach `repo.CurrentBranch()`, which nil-pointer panics on a nil bisector.
  When `deps.OpenBisector` is non-nil, call it, propagate any error, and pass the returned bisector to `BisectAndEscalate` exactly as today.
  Both branches keep the existing `SaveState` and the existing loud return when the master outcome was done.
  Do not add a distinct sentinel in place of `"unknown"` — the state and summary vocabulary already understands it, and the warning is what says why localization is absent.
- **Commit:** `feat(websterengine): record an unlocalized integration failure when no bisector is supplied`

### Card 29: Update `websterengine`'s package doc

- **Context:**
  - `internal/websterengine/geometry.go`
  - `internal/websterengine/runlevel.go`
  - `internal/websterengine/audit.go`
  - `internal/websterengine/render.go`
- **Edits:**
  - `internal/websterengine/doc.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Extend the "engine/cli split: webster is fabric-blind" section so the claim is now literally true rather than aspirational: `internal/websterengine` imports `internal/fabricengine` from no production file after this batch, and the two seams that used to reach it are engine-declared interfaces the caller supplies — `RefMatcher` for the fork-audit fabric-reference class, and `FabricBisector` through `RunDeps.OpenBisector` for the integration bisect.
  State that `internal/lyxcwd` is likewise absent from this package's production imports and that every path arrives through `Geometry`.
  Add a short paragraph naming `hubgeom.WebsterGeometry` and `internal/standalonegeom` as the two tellers, and stating the one-way dependency direction.
  Do not restate the eight field meanings here — `geometry.go` owns those, and duplicating them is exactly how the two drift.
  Keep every other section of the file unchanged.
- **Commit:** `docs(websterengine): record the told-geometry and injected-fabric-seam contract`

### Card 30: Convert the render tests

- **Context:**
  - `internal/websterengine/render.go`
  - `internal/websterengine/geometry.go`
- **Edits:**
  - `internal/websterengine/template_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** `internal/websterengine/template_test.go` owns `testLayout`, `patternActiveLayout` and `patternActiveMissingPatternStencilsLayout`, and is the main consumer of the render functions' Location parameter.
  Convert each helper to return the told strings its callers need instead of a `*lyxcwd.Location` — keep the same on-disk fixture seeding each one performs, since the stencil reads and the `pattern.Directive` probe still need real files, and return the anchor root and the stencils directory it just seeded.
  Update every `RenderForkPrompt`, `RenderRecoveryPrompt` and `RenderMasterPrompt` call in the file to the new parameter lists.
  Every existing expected value stays as it is: in these fixtures the anchor root and the prompt worktree root are the same directory, so no rendered byte changes.
  Do not add the standalone divergence case here — card 32 owns it, and it needs a fixture where the two roots differ.
- **Commit:** `test(websterengine): drive the renderers from told roots`

### Card 31: Convert the remaining `Location` fixtures

- **Context:**
  - `internal/websterengine/geometry.go`
  - `internal/websterengine/beginbatch.go`
  - `internal/websterengine/recordbatch.go`
  - `internal/websterengine/recoverbatch.go`
  - `internal/websterengine/runlevel.go`
  - `internal/websterengine/audit.go`
- **Edits:**
  - `internal/websterengine/beginbatch_test.go`
  - `internal/websterengine/recordbatch_test.go`
  - `internal/websterengine/recoverbatch_test.go`
  - `internal/websterengine/runlevel_test.go`
  - `internal/websterengine/audit_test.go`
  - `internal/websterengine/integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Convert every `Deps`-construction site in these five files from the flat path fields plus `Layout` to a `Geom: Geometry{...}` literal carrying the same directories each fixture already computes, so no fixture's on-disk layout changes.
  Where a fixture built a `*lyxcwd.Location` solely to hand to a `Deps` struct, delete it;
  where one is still needed to construct a real `*fabricengine.RefScanner`, keep it.
  `RecordDeps` and `RunDeps` sites additionally supply `RefMatcher` — use `NeverMatches{}` in fixtures that assert no fabric-reference violation, and a real `fabricengine.NewRefScanner` or a purpose-built fake in the cases that do.
  `RunDeps` sites that previously injected `Bisector: <fake>` now supply `OpenBisector: func() (FabricBisector, error) { return <fake>, nil }`;
  sites that left `Bisector` nil and expected the production `fabricengine.Open` path must be re-examined rather than mechanically converted, since nil now means "no fabric" instead of "open the default".
  In `internal/websterengine/audit_test.go`, keep the existing `TestRefScannerMatches` cases unchanged in substance: they build a real scanner from `fakeLayout`, `*fabricengine.RefScanner` still satisfies the new interface, and that real-scanner coverage is what the Fabric Git Invariant names as its machine check.
  Set each fixture's `Geom.AnchorRoot` and `Geom.WorktreeRoot` to the same directory unless the test is specifically about their divergence — that keeps every existing expectation valid.
- **Commit:** `test(websterengine): convert every Deps fixture to a told Geometry`

### Card 32: Pin the three behaviours a hub-only fixture cannot see

- **Context:**
  - `internal/websterengine/render.go`
  - `internal/websterengine/audit.go`
  - `internal/websterengine/runlevel.go`
  - `internal/websterengine/integration.go`
  - `internal/websterengine/state.go`
  - `internal/websterengine/geometry.go`
- **Edits:**
  - `internal/websterengine/template_test.go`
  - `internal/websterengine/audit_test.go`
  - `internal/websterengine/runlevel_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add three tests, each covering a case that passes under both the correct and the broken implementation when the anchor root and the worktree root coincide.
  First, in `internal/websterengine/template_test.go`: drive `RenderForkPrompt` and `RenderRecoveryPrompt` twice — once with `anchorRoot == promptWorktreeRoot` asserting `{{.worktree_root}}` renders that value, and once with the two deliberately different asserting it renders `promptWorktreeRoot` and never `anchorRoot`.
  Add a case asserting `RenderMasterPrompt`'s output contains no `worktree_root` value at all, since that renderer must not gain the token.
  Second, in `internal/websterengine/audit_test.go`: drive `CheckFork` and `CheckParent` with a workdir that differs from the anchor-shaped directory and a *relative* recorded write path, and assert the contract-file violation is judged against the workdir-joined path.
  Add the mirror case proving a relative path that resolves under the anchor root instead is *not* flagged, so the test fails if the two are swapped.
  Third, in `internal/websterengine/runlevel_test.go`: drive `runIntegrationStage` through `Run` with a nil `OpenBisector`, a FAILED integration report, and **at least two accumulated card SHAs**.
  Two is the minimum that matters: a zero-SHA fixture takes `bisect`'s empty early return and a one-SHA fixture takes its sole-candidate return, so both pass under the broken implementation, and only two or more reach the branch call that nil-panics.
  Assert three things: the call does not panic;
  `state.json` and `summary.md` carry the failure with `"unknown"` for both the offending SHA and the offending card;
  and `RunResult.Warnings` carries the standalone-mode explanation.
- **Commit:** `test(websterengine): pin the told-root, audit-workdir and nil-bisector behaviours`

### Card 33: Wire `webstercli` onto the told Deps in hub mode

- **Context:**
  - `internal/hubgeom/webstergeom.go`
  - `internal/websterengine/geometry.go`
  - `internal/websterengine/runlevel.go`
  - `internal/websterengine/beginbatch.go`
  - `internal/websterengine/recordbatch.go`
  - `internal/websterengine/recoverbatch.go`
  - `internal/websterengine/integration.go`
  - `internal/fabricengine/refscanner.go`
  - `internal/fabricengine/open.go`
- **Edits:**
  - `internal/webstercli/cli.go`
  - `internal/webstercli/run.go`
  - `internal/webstercli/beginbatch.go`
  - `internal/webstercli/recordbatch.go`
  - `internal/webstercli/recoverbatch.go`
  - `internal/webstercli/verbs_test.go`
  - `internal/webstercli/cli_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add three fields to `websterCLI`: `geom websterengine.Geometry`, `refMatcher websterengine.RefMatcher`, and `openFabric func() (*fabricengine.Fabric, error)`.
  In `PersistentPreRunE`, set `c.geom = hubgeom.WebsterGeometry(layout)`, `c.refMatcher = fabricengine.NewRefScanner(layout)`, and `c.openFabric` to a closure capturing `layout` and calling `fabricengine.Open(layout)`.
  The matcher is built eagerly because `NewRefScanner` only compiles a regexp and cannot fail;
  the fabric handle is a closure and must **not** be opened here, because `fabricengine.Open` stat-checks the weft sibling and would fail the pre-run in the three healthy-but-unwired locations that run `validate` and `status` today.
  Keep the existing `layout`, `planDir`, `websterDir`, `reportsDir`, `promptsDir` and `websterScratchDir` fields populated as they are — batch 8 removes them, and keeping them here holds the diff to one concern.
  Update the four verb `Deps` constructions to pass `Geom: c.geom` in place of `Layout` and the flat path fields.
  `RecordDeps` additionally passes `RefMatcher: c.refMatcher`;
  `RunDeps` passes `RefMatcher: c.refMatcher` and an `OpenBisector` closure that calls `c.openFabric` and returns its result as a `websterengine.FabricBisector`.
  In `internal/webstercli/run.go`, add a `"warnings"` key to the success envelope carrying `result.Warnings` — the field is new on both sides, and a test that stops at the struct proves nothing an operator can see.
  Update the fixtures in `internal/webstercli/verbs_test.go` and `internal/webstercli/cli_test.go` to populate the three new fields, and add an assertion in `verbs_test.go` that the pre-run leaves `c.openFabric` non-nil but **uninvoked** — no fabric handle is opened during wiring.
- **Commit:** `refactor(webstercli): build websterengine Deps from a told Geometry in hub mode`

## Batch Tests

`verify:` runs the untagged suites of `./internal/websterengine/...`, `./internal/webstercli/...` and `./cmd/lyx/...`, which is every package this batch's signature changes reach, plus a chained `go test -tags integration ./internal/websterengine/... ./internal/webstercli/...`.
The tagged half carries much of this batch's weight rather than being a formality.
Cards 31 and 32 touch six `internal/websterengine` fixture files, and four of them — `beginbatch_test.go`, `recordbatch_test.go`, `recoverbatch_test.go` and `runlevel_test.go` — are `//go:build integration`, so the untagged run does not compile them at all;
the remaining two, `audit_test.go` and `template_test.go`, are untagged and do run there.
Separately, `internal/webstercli/verbs_test.go`, which card 33 edits in the other package, is also `//go:build integration`.
Card 32's nil-bisector test lives in `runlevel_test.go` and is therefore reachable only through the tagged invocation.
The regression net for the mechanical two-thirds of the batch is that the existing `websterengine` and `webstercli` suites keep passing with unchanged expected values — cards 30 and 31 convert fixtures without moving a single directory or expectation, so any drift shows up as a failure rather than as a silently different path.
Card 32 carries the batch's genuinely new coverage, and each of its three tests exists because the corresponding hub-only fixture cannot distinguish correct from broken:
with `anchorRoot == promptWorktreeRoot` the prompt renders identically either way;
with the audit workdir equal to the anchor root a relative recorded path resolves the same either way;
and below two accumulated card SHAs the nil-bisector path never reaches the call that panics.
Card 33's uninvoked-opener assertion is the executable form of the laziness argument — it is what keeps the three healthy-but-unwired hub locations working, and nothing else in the suite would catch an eager `Open` creeping back into the pre-run.
