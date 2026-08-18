# Batch: perch told-geometry

```yaml
task: "burlerengine + perchengine told-geometry"
batch: "perch told-geometry"
number: 3
cards: 10
verify: go test ./internal/hubgeom/... ./internal/perchengine/... ./internal/perchcli/... ./internal/shedadapters/... ./cmd/lyx/... && go test -tags integration ./internal/perchcli/...
depends-on: [2]
```

## Batch Scope

This batch converts `internal/perchengine` to told geometry: the `layout *lyxcwd.Location` field becomes `geom Geometry`, `New` takes a `Geometry` in the same argument position, `Engine.Run` stamps `treadleengine.Profile.GateDir` from the told `GateDir`, `RunsDir`/`ScratchDir` take a told `anchorPath string`, and `internal/lyxcwd` leaves the package's production imports entirely.

It is one batch because two signature changes land together and each breaks its callers immediately: `perchengine.New` breaks `internal/perchcli/run.go` and `internal/perchengine/run_test.go`, and the `RunsDir`/`ScratchDir` parameter change breaks `internal/perchcli/cli.go`, both perchcli integration test files, and both `cmd/lyx` anchoring tables.
Splitting them would leave the tree uncompilable at the boundary.

It depends on batch 2 rather than running beside it because both batches edit `internal/perchcli/cli.go` — batch 2 for the `burlerengine.New` line, this batch for the perch construction sites and the new `perchGeom` field.

Batch-local decisions beyond `## Shared Decisions`:

- `perchCLI` keeps its `layout *lyxcwd.Location` field and gains `perchGeom perchengine.Geometry` alongside it.
  `c.layout` survives for the three fabric call sites only, which genuinely need the `Location` and are genuinely hub-mode-only.
  Making the CLI hub-blind is task T8, not this task.
- The four `perchengine` rows in `cmd/lyx/constructoranchoring_test.go` and the five in `cmd/lyx/notransients_test.go` go tautological with respect to anchoring, deliberately, exactly as the `planparser.PlanDir` and `pattern.File` rows did in waves 1 and 2.
  They are rewritten in place and annotated, never deleted: they still pin the join arithmetic and the `_lyx`-vs-`.lyx` group placement, and `notransients_test.go`'s mirrored-pair equality check is load-bearing for the Durable-vs-Ephemeral State Invariant and has nothing to do with anchoring.

## Cards

### Card 12: convert `perchengine.Engine` to told geometry

- **Context:**
  - `internal/perchengine/geometry.go`
  - `internal/treadleengine/profile.go`
- **Edits:**
  - `internal/perchengine/engine.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/perchengine/engine.go`, replace the `layout *lyxcwd.Location` field on `Engine` with `geom Geometry`, keeping its position after `cfg`.
  Change `New`'s signature from `New(burler Burler, shuttle Shuttle, cfg Config, layout *lyxcwd.Location, opts Options) *Engine` to `New(burler Burler, shuttle Shuttle, cfg Config, geom Geometry, opts Options) *Engine`, and its body's struct literal from `layout: layout` to `geom: geom`.
  In `Engine.Run`, replace `GateDir: e.layout.WorktreePath()` with `GateDir: e.geom.GateDir` in the `treadleengine.Profile` literal.
  That is `Engine`'s only remaining geometry use; `e.geom.AnchorPath` is carried but not read here, per the shared decision that the engine stores the whole `Geometry` value.
  Remove `github.com/Knatte18/loomyard/internal/lyxcwd` from the import block; it becomes unused.
  Update the two comments this falsifies.
  The file header currently says the `*lyxcwd.Location` it holds is used only to resolve the gate command's working directory, `layout.WorktreePath()`, which becomes `treadleengine.Profile.GateDir` — restate it as: the `Geometry` it holds carries the gate command's working directory, `geom.GateDir`, which becomes `treadleengine.Profile.GateDir`, alongside an `AnchorPath` the engine itself never reads.
  `Run`'s doc comment says it builds a `treadleengine.Profile` with `GateDir: e.layout.WorktreePath()` — say `GateDir: e.geom.GateDir`.
  The same doc comment's closing sentence says `stencilsDir` is resolved by perchcli, the caller that already holds the `*lyxcwd.Location`, via `fabricengine.StencilsDir` — that stays true (perchcli does still hold one, for fabric), so reword it only to say "the caller that holds the hub path" and keep the `fabricengine.StencilsDir` reference, avoiding a claim about a type this package no longer names.
  Do not change `Engine.Run`'s signature or any other field.
- **Commit:** `refactor(perchengine): take a told Geometry instead of a Location`

### Card 13: `RunsDir` and `ScratchDir` take a told anchor path

- **Context:**
  - `internal/lyxdirs/dirs.go`
  - `internal/planparser/parse.go`
- **Edits:**
  - `internal/perchengine/identity.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/perchengine/identity.go`, change `func RunsDir(l *lyxcwd.Location) string` to `func RunsDir(anchorPath string) string`, with body `return filepath.Join(anchorPath, lyxdirs.LyxDirName, perchDirName)`.
  Change `func ScratchDir(l *lyxcwd.Location) string` to `func ScratchDir(anchorPath string) string`, with body `return filepath.Join(anchorPath, lyxdirs.DotLyxDirName, perchDirName)`.
  This mirrors `planparser.PlanDir(anchorPath string)` and `pattern.File(baseDir string)` from earlier waves.
  Both doc comments keep their existing "Per the Cwd Resolution Invariant, no other package may construct this path" line verbatim — still true, and still the point.
  Keep `ScratchDir`'s existing sentence about being `RunsDir`'s mirrored sibling under `.lyx` and about a caller joining a block's run-id onto this base.
  Remove `github.com/Knatte18/loomyard/internal/lyxcwd` from the import block; it becomes unused.
  `perchDirName` stays exactly as it is — this module's own private relative-path constant, which is what keeps the Cwd Resolution Invariant satisfied.
  Change nothing else in the file: `ProfileHash`, `DeriveRunID`, `ValidRunID`, `sanitizeSlug`, `TerminalOutcome`, `PauseFlagPath`, and the re-exported treadleengine vocabulary are untouched.
- **Commit:** `refactor(perchengine): RunsDir and ScratchDir take a told anchor path`

### Card 14: correct perchengine's package doc

- **Context:**
  - `internal/perchengine/engine.go`
  - `internal/perchengine/geometry.go`
- **Edits:**
  - `internal/perchengine/doc.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** `internal/perchengine/doc.go`'s "Fabric-blindness and geometry-blindness" section states that `Engine` operates on a caller-supplied absolute `runDir` and that layout is held only to resolve the gate command's working directory, `layout.WorktreePath()`.
  Cards 12 and 13 falsify the second half.
  Rewrite it to say that `Engine` operates on a caller-supplied absolute `runDir` and is told its geometry as a `Geometry` value, whose `GateDir` resolves the gate command's working directory and whose `AnchorPath` is carried for the caller's `RunsDir`/`ScratchDir` base but never read by `Engine` itself.
  Keep the rest of the section verbatim — the sentence that committing the run dir's artifacts to fabric is the loop owner's job, the `Fabric Git Invariant` reference, and the closing note that this is the identical split `burlerengine` enforces one layer down all stay true.
  Change nothing else in the file.
- **Commit:** `docs(perchengine): describe the told Geometry in the package doc`

### Card 15: convert `run_test.go` to a told geometry fixture

- **Context:**
  - `internal/perchengine/engine.go`
  - `internal/perchengine/geometry.go`
  - `internal/treadleengine/profile.go`
- **Edits:**
  - `internal/perchengine/run_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/perchengine/run_test.go`, replace the `newTestLayout(t *testing.T) *lyxcwd.Location` helper with `newTestGeometry(t *testing.T) Geometry`, returning a `Geometry` whose `GateDir` and `AnchorPath` are two distinct fresh temp directories rather than one collapsed root — the gate command's working directory and the runs/scratch base are separate things and the fixture should say so.
  Its doc comment states that it returns the told geometry standing in for the worktree root a command gate's cwd resolves against, and that the two fields are deliberately distinct so a swapped constructor argument is observable.
  Rename every local variable currently holding the helper's result from `layout` to `geom`, and update all thirty-three call sites of the helper accordingly.
  Update all thirty-nine `New(fb, ...)` construction sites to pass `geom` in the former `layout` argument position; the argument count and order are otherwise unchanged.
  The two assertion lines currently comparing the recorded gate-command dir against `layout.WorktreePath()` become a comparison against `geom.GateDir`, with the failure message reworded to name `geom.GateDir`.
  That pair is perch's direct `treadleengine.Profile.GateDir` assertion — it is what the discussion asks be confirmed rather than newly written, and it becomes load-bearing once `GateDir` and `AnchorPath` are distinct, since a swapped constructor would now make it fail.
  Remove `github.com/Knatte18/loomyard/internal/lyxcwd` from the import block; it becomes unused.
  Keep every existing assertion in the file intact — this is a shape change, not a behaviour change.
- **Commit:** `test(perchengine): convert run_test to a told Geometry fixture`

### Card 16: convert `identity_test.go` to told anchor paths

- **Context:**
  - `internal/perchengine/identity.go`
  - `internal/lyxdirs/dirs.go`
- **Edits:**
  - `internal/perchengine/identity_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/perchengine/identity_test.go`, rewrite `TestScratchDir`'s two-case table so each case carries an `anchorPath string` instead of a `layout *lyxcwd.Location`.
  Keep both cases and their intent: one unanchored (a plain temp directory, standing in for `AnchorRel == "."`) and one subpath-anchored (a temp directory joined with a real nested subpath, mirroring the current `filepath.Join("nested", "sub")` fixture).
  Case names should stop referring to `AnchorRel` and a synthetic `Location`, since the function now takes the resolved anchor directly — name them for the shape of the anchor path instead.
  Keep all three assertions unchanged in substance: `ScratchDir(anchorPath)` equals `filepath.Join(anchorPath, ".lyx", "perch")`; it differs from `RunsDir(anchorPath)`; and rewriting `RunsDir`'s own result's `_lyx` segment to `.lyx` reproduces it exactly.
  That third assertion is the package-local half of the Durable-vs-Ephemeral State Invariant guard and must keep firing.
  Update the `TestScratchDir` doc comment to describe told anchor-path inputs rather than synthetic `*lyxcwd.Location` values.
  Remove the `github.com/Knatte18/loomyard/internal/lyxcwd` import; it becomes unused.
  `TestProfileHash`, `TestDeriveRunID`, and `TestValidRunID` are untouched.
- **Commit:** `test(perchengine): table ScratchDir against told anchor paths`

### Card 17: give `perchCLI` a told perch geometry

- **Context:**
  - `internal/perchengine/geometry.go`
  - `internal/perchengine/identity.go`
  - `internal/hubgeom/hubgeom.go`
- **Edits:**
  - `internal/perchcli/cli.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add a `perchGeom perchengine.Geometry` field to the `perchCLI` struct in `internal/perchcli/cli.go`, beside the existing `layout *lyxcwd.Location` field, which stays.
  In `PersistentPreRunE`, set `c.perchGeom = hubgeom.PerchGeometry(layout)` alongside the existing `c.layout = layout` assignment.
  Change `c.runDirBase = perchengine.RunsDir(layout)` to `perchengine.RunsDir(c.perchGeom.AnchorPath)` and `c.scratchDirBase = perchengine.ScratchDir(layout)` to `perchengine.ScratchDir(c.perchGeom.AnchorPath)`.
  Both resolve byte-identically: `hubgeom.PerchGeometry` fills `AnchorPath` from `layout.AnchorPath()`, which is the accessor both functions called internally before this task.
  The two long comments above those assignments — the one explaining why the runs base is anchored at `layout.AnchorPath()` rather than the worktree root, and the one explaining that the scratch base is anchored at the same place so a block's two directories can never disagree — must survive the rewrite in full.
  They are the record of a real bug class (a nested-init repo stranding artifacts outside fabric), not decoration.
  Update only their references to `layout.AnchorPath()` so they name the told `perchGeom.AnchorPath` and where it came from; leave the reasoning intact.
  Update the `perchCLI` struct's field-level and the file header's description of what `PersistentPreRunE` resolves so the new field is accounted for.
  Do not change the `burlerengine.New` line — batch 2 already repointed it.
- **Commit:** `refactor(perchcli): resolve a told perch geometry in PersistentPreRunE`

### Card 18: pass the told geometry to `perchengine.New`

- **Context:**
  - `internal/perchengine/engine.go`
  - `internal/perchcli/cli.go`
- **Edits:**
  - `internal/perchcli/run.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/perchcli/run.go`, change the per-invocation `perchengine.New(c.burlerEngine, c.runner, c.perchCfg, c.layout, perchengine.Options{...})` call to pass `c.perchGeom` in `c.layout`'s position.
  Every other argument, and the `Options` literal with its `PauseRequested` closure, is unchanged.
  The three remaining `c.layout` uses in this file stay exactly as they are: `fabricengine.StencilsDir(c.layout.HubPath)` in the `engine.Run` call, `fabricengine.ScopedPathspec(c.layout.AnchorRel, ...)`, and `fabricengine.Open(c.layout)`.
  Fabric sync is hub-mode-only and genuinely needs the `Location`; rethreading those sites is task T8's work, not this task's.
  Keep the comment above the construction explaining that the engine is built per-invocation because its pause seam closes over this call's concrete `scratchDir`; it is unaffected.
- **Commit:** `refactor(perchcli): construct perchengine with the told geometry`

### Card 19: convert the perchcli integration call sites

- **Context:**
  - `internal/perchengine/identity.go`
  - `internal/hubforge/hub.go`
- **Edits:**
  - `internal/perchcli/cli_integration_test.go`
  - `internal/perchcli/run_integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/perchcli/cli_integration_test.go`, change all six `perchengine.RunsDir(h.Location)` and `perchengine.ScratchDir(h.Location)` calls to pass `h.Location.AnchorPath()`.
  In `internal/perchcli/run_integration_test.go`, make the same swap at its two call sites.
  These are mechanical argument swaps and resolve identically: both functions called `l.AnchorPath()` internally before this task.
  `TestRunCLI_Pause_NestedInitAnchorsRunDirsAtCwd` in `internal/perchcli/cli_integration_test.go` is the production-call-site anchoring proof this task relies on — it builds a real nested hub, drives the real `PersistentPreRunE`, and asserts the pause verb finds a run dir under the anchored `_lyx/perch`, so a production regression to the worktree root makes its lookup miss.
  Keep it passing unmodified in substance: change only its `RunsDir`/`ScratchDir` argument, and never rewrite it to compute its expectation from the CLI's own value.
  Both files keep their `//go:build integration` constraint; no test changes tier, and the `hubforge`-backed fixtures keep their hermetic git environment.
- **Commit:** `test(perchcli): pass told anchor paths to RunsDir and ScratchDir`

### Card 20: rewrite the `cmd/lyx` anchoring rows

- **Context:**
  - `internal/perchengine/identity.go`
- **Edits:**
  - `cmd/lyx/constructoranchoring_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `cmd/lyx/constructoranchoring_test.go`, change the four `perchengine.RunsDir(l)` / `perchengine.ScratchDir(l)` rows — two in `TestConstructorAnchoring_Unanchored`, two in `TestConstructorAnchoring_SubpathAnchored` — to pass `l.AnchorPath()`, and make the same swap on the `perchengine.ScratchDir` entry in the `dotLyxConstructors` map inside `TestConstructorAnchoring_SubpathAnchored`.
  Every expectation stays as it is; the rows resolve identically.
  These rows now go tautological with respect to anchoring, deliberately: once the function takes the anchor as a parameter, a row that passes `l.AnchorPath()` in and compares against an anchor-derived expectation can no longer catch a production call site passing the wrong root.
  Extend the existing inline comment in `TestConstructorAnchoring_SubpathAnchored` that already carries this note for the two `planparser` rows and the `pattern.File` row, so it names the `perchengine` rows alongside them and points at the perch production-call-site proof, `TestRunCLI_Pause_NestedInitAnchorsRunDirsAtCwd` in the perchcli integration suite, as the place that proof now lives.
  Extend the file header comment the same way — it describes the two anchoring groups and which constructors take a plain anchor string rather than a `*lyxcwd.Location`; `perchengine.RunsDir`/`ScratchDir` now belong in the second category.
  Do not delete any row: they still pin the join arithmetic and the `_lyx`-vs-`.lyx` group placement, and the prefix-exclusion guard below the map is what fails the moment any worktree-level consumer is left behind.
  The file stays untagged pure-`filepath.Join` arithmetic per the Test Tier Purity Invariant.
- **Commit:** `test(cmd/lyx): pass told anchor paths in the constructor anchoring table`

### Card 21: rewrite the no-transients rows

- **Context:**
  - `internal/perchengine/identity.go`
  - `internal/lyxdirs/dirs.go`
  - `cmd/lyx/constructoranchoring_test.go`
- **Edits:**
  - `cmd/lyx/notransients_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `cmd/lyx/notransients_test.go`, change all five `perchengine.RunsDir(l)` / `perchengine.ScratchDir(l)` call sites to pass `l.AnchorPath()`: the two rows in `durableSet`, the `perchengine.ScratchDir` row and the two nested uses inside the `PauseFlagPath` rows in `transientSet`, and the `perchengine.RunsDir/ScratchDir` entry in the `mirroredPairs` table.
  Every expectation and every assertion stays as it is.
  Extend the comment above `durableSet` that already annotates the two `planparser` rows as tautological with respect to anchoring so it names the `perchengine` rows alongside them, pointing at the same perch production-call-site proof named in `cmd/lyx/constructoranchoring_test.go`.
  Two guards in this file must keep firing and must not be weakened: the mirrored-pair equality check, which rewrites the durable path's `_lyx` segment to `.lyx` and requires the result to equal the scratch path — load-bearing for the Durable-vs-Ephemeral State Invariant and unrelated to anchoring — and the non-empty-table sanity guard.
  This file is not in the design doc's own file list for this task; it was missed there and is a confirmed omission, so its rows are edited in place here rather than left to a later sweep.
  The file stays untagged; no test changes tier.
- **Commit:** `test(cmd/lyx): pass told anchor paths in the no-transients table`

## Batch Tests

`verify:` runs `go test` over every package this batch touches or breaks — `internal/perchengine` (the converted engine, its run suite, and its identity suite), `internal/perchcli` (both production call sites and its untagged unit tests), `cmd/lyx` (the two anchoring tables), `internal/hubgeom` (whose `PerchGeometry` becomes load-bearing here), and `internal/shedadapters`, which is deliberately not edited but consumes `perchengine.Profile`, `perchengine.ValidRunID`, and the `*perchengine.Engine`-satisfies-`PerchRunner` compile-time proof, so it is the cheapest check that no seam moved underneath it.
The `&& go test -tags integration ./internal/perchcli/...` half is not optional: card 19 edits two `//go:build integration` files, and no untagged command compiles them.

The load-bearing evidence here is existing coverage kept intact, not new assertions:

- `TestRunCLI_Pause_NestedInitAnchorsRunDirsAtCwd` in the perchcli integration suite is the production-call-site anchoring proof for the whole perch half of this task, and it survives with only its `RunsDir`/`ScratchDir` argument changed.
- `TestScratchDir`'s mirrored-segment assertion and `cmd/lyx/notransients_test.go`'s mirrored-pair equality check are the two halves of the Durable-vs-Ephemeral State Invariant guard, and both keep firing across the parameter change.
- The gate-command-dir assertion in `internal/perchengine/run_test.go` becomes a real `Geometry.GateDir` guard once card 15's fixture makes `GateDir` and `AnchorPath` distinct.

Manual check to run once at batch end, per the discussion's Verify list: `grep -rn lyxcwd internal/perchengine/ --include=*.go` must return no hit in a production (non-`_test.go`) file.
Cards 12, 13, and 14 are the three places a hit could survive.

The overview's module-wide `verify: go build ./...` runs at the batch boundary and covers any consumer of the two changed signatures outside the packages listed above; the enumeration was re-run against the tree for this task and found none.
`pipeline.done_gate` (`go test ./... && go test -tags integration ./...`) is the task-wide net mill-go runs before marking the task done, which is where a regression in a package no batch verify names would surface.
