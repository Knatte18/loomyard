# Batch: standalonegeom-builders

```yaml
task: "the standalone CLI path"
batch: "standalonegeom-builders"
number: 2
cards: 5
verify: go test ./internal/standalonegeom/...
depends-on: []
```

## Batch Scope

This batch adds the told-mode geometry builders burler and perch need in standalone, plus the one exported helper that gives the `<state>/_lyx/stencils` literal a single construction site across all three CLIs.
It is one batch because the three new files, the `WebsterGeometry` repoint onto the new helper, the package doc's contract sentence, and the extended table test are one cohesive package-level change with no external dependency — it compiles and tests entirely on its own, which is why it runs parallel to batch 1 rather than after it.

**External interface batches 4 and 5 consume:** `standalonegeom.BurlerGeometry(target, stateDir string) burlerengine.Geometry`, `standalonegeom.PerchGeometry(target, stateDir string) perchengine.Geometry`, and `standalonegeom.StencilsDir(stateDir string) string`.

**Batch-local decision:** neither new builder takes a `hash8` parameter.
No value in either struct is hash-derived, and `WebsterGeometry` already set the precedent of omitting the parameter rather than carrying an unused one for symmetry with `ReedGeometry`.

## Cards

### Card 4: add `StencilsDir`, the sole construction site for the standalone stencils default

- **Context:**
  - `internal/lyxdirs/dirs.go`
  - `internal/fabricengine/junctionnames.go`
  - `internal/standalonegeom/reedgeom.go`
- **Edits:** none
- **Creates:**
  - `internal/standalonegeom/stencilsdir.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Create `internal/standalonegeom/stencilsdir.go` in `package standalonegeom`, carrying a file-header comment in the same shape as `reedgeom.go`'s and `webstergeom.go`'s.

  Add `func StencilsDir(stateDir string) string` returning `filepath.Join(stateDir, lyxdirs.LyxDirName, "stencils")`.

  Its doc comment must state: that this is the sole construction site for the standalone stencils directory across every standalone-capable CLI; that it deliberately mirrors `fabricengine.StencilsDir(hub)`, with `<state>` playing the role the hub plays in hub mode, which is why the result is `_lyx`-resident rather than a third top-level `<state>/stencils`; and that the returned value is carried by callers as a plain string, never as an engine geometry field, because neither `burlerengine.Geometry` nor `perchengine.Geometry` has a `StencilsDir` field and both engines already accept the directory as a told parameter.
  It takes only `stateDir` — like every other builder in this package it never calls `standalonestate.Derive`, never reads the environment, and never touches disk.
- **Commit:** `feat(standalonegeom): add StencilsDir, the sole standalone stencils construction site`

### Card 5: repoint `WebsterGeometry` at the new helper

- **Context:**
  - `internal/standalonegeom/stencilsdir.go`
  - `internal/websterengine/geometry.go`
- **Edits:**
  - `internal/standalonegeom/webstergeom.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In `WebsterGeometry`, replace the inline `StencilsDir: filepath.Join(stateDir, lyxdirs.LyxDirName, "stencils")` field expression with `StencilsDir: StencilsDir(stateDir)`.
  The shipped value must stay byte-identical — this is a de-duplication, not a behaviour change.

  Remove the `path/filepath` and `internal/lyxdirs` imports if and only if no other expression in the file still uses them; if `planparser.PlanDir` or another field keeps one of them live, leave that import in place.
  Update the `PlanDir and StencilsDir are the defaults only` paragraph in `WebsterGeometry`'s doc comment to name `StencilsDir(stateDir)` as where the default now comes from, so the one-construction-site property is discoverable from the call site rather than only from the helper.
- **Commit:** `refactor(standalonegeom): build WebsterGeometry's StencilsDir via the shared helper`

### Card 6: add `BurlerGeometry`

- **Context:**
  - `internal/burlerengine/geometry.go`
  - `internal/hubgeom/hubgeom.go`
  - `internal/standalonegeom/reedgeom.go`
- **Edits:** none
- **Creates:**
  - `internal/standalonegeom/burlergeom.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Create `internal/standalonegeom/burlergeom.go` in `package standalonegeom`, with a file-header comment matching `reedgeom.go`'s shape.

  Add `func BurlerGeometry(target, stateDir string) burlerengine.Geometry` returning
  `burlerengine.Geometry{WorktreeRoot: target, AnchorPath: stateDir}`.

  The doc comment must state why the two fields diverge, since that divergence is the design's load-bearing move: `WorktreeRoot` is `target` because `burlerengine`'s `(*Profile).validate` resolves `Target.Paths`, `Fasit.Paths`, `ReviewPath`, `FixerReportPath` and both prior-report lists against it, so it must be the directory the operator asked burler to review; `AnchorPath` is `stateDir` because it is the base burler's `.lyx/burler` instruction directory joins onto, and pointing it at `target` is exactly what would push a hidden `.lyx` tree into the reviewed folder.
  It must also state that no parameter is `hash8`-derived, mirroring `WebsterGeometry`'s own omission.
  Do not add a `StencilsDir` field to `burlerengine.Geometry` — the stencils directory reaches burler as `burlerengine.New`'s fourth parameter, and a geometry field would be a competing home for a value the constructor already takes.
- **Commit:** `feat(standalonegeom): add BurlerGeometry, the told-mode burler geometry builder`

### Card 7: add `PerchGeometry`

- **Context:**
  - `internal/perchengine/geometry.go`
  - `internal/hubgeom/hubgeom.go`
  - `internal/standalonegeom/reedgeom.go`
- **Edits:** none
- **Creates:**
  - `internal/standalonegeom/perchgeom.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Create `internal/standalonegeom/perchgeom.go` in `package standalonegeom`, with a file-header comment matching `reedgeom.go`'s shape.

  Add `func PerchGeometry(target, stateDir string) perchengine.Geometry` returning
  `perchengine.Geometry{GateDir: target, AnchorPath: stateDir}`.

  The doc comment must state: `GateDir` is `target` because it is the gate command's working directory, the value `Engine.Run` stamps onto `treadleengine.Profile.GateDir`, and perch's field is named `GateDir` rather than `WorktreeRoot` precisely because that is its only use of that root; `AnchorPath` is `stateDir` because it is the base `perchengine.RunsDir` and `perchengine.ScratchDir` join onto, so the durable/scratch pair relocates under `<state>` automatically rather than as two separate things a caller must remember.
  It must also state that no parameter is `hash8`-derived.
  Do not add a `StencilsDir` field to `perchengine.Geometry` — perch takes the stencils directory per call at `Engine.Run`.
- **Commit:** `feat(standalonegeom): add PerchGeometry, the told-mode perch geometry builder`

### Card 8: extend the package doc and the builder table test

- **Context:**
  - `internal/standalonegeom/burlergeom.go`
  - `internal/standalonegeom/perchgeom.go`
  - `internal/standalonegeom/stencilsdir.go`
  - `internal/standalonegeom/webstergeom.go`
  - `internal/burlerengine/geometry.go`
  - `internal/perchengine/geometry.go`
  - `internal/lyxdirs/dirs.go`
- **Edits:**
  - `internal/standalonegeom/doc.go`
  - `internal/standalonegeom/standalonegeom_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In `doc.go`, the contract sentence currently reads "standalonegeom's contract today is ReedGeometry and WebsterGeometry, converting a told target, stateDir, and (for reed) hash8 into a reedengine.Geometry or websterengine.Geometry respectively."
  Rewrite it to name all four builders plus the `StencilsDir` helper, keeping the "Neither engine imports this package back — the told direction stays one-way" property and generalising it to all four engine packages.
  Also update the "it imports reedengine and websterengine" sentence in the not-a-leaf paragraph so it names `burlerengine` and `perchengine` too — that paragraph is what keeps a future contributor from adding this package to `internal/buildinfo`'s or `internal/standalonestate`'s leaf-enforcement allowlists, so an incomplete import list weakens it.

  In `standalonegeom_test.go`, add `TestBurlerGeometry`, `TestPerchGeometry`, and `TestStencilsDir`, following the existing file's style: told literal `target`/`stateDir` values with `target` deliberately not under `stateDir`, `t.Parallel()` on every case, and one assertion per field naming which of the two roots it must equal.
  `TestBurlerGeometry` asserts `WorktreeRoot == target` and `AnchorPath == stateDir`; `TestPerchGeometry` asserts `GateDir == target` and `AnchorPath == stateDir`.
  Each must assert both fields in the same case with a comment naming the two-root split as the reason, mirroring how `TestReedGeometry` already justifies asserting `PaneCwd` and `AnchorPath` together.
  `TestStencilsDir` asserts the result equals `filepath.Join(stateDir, lyxdirs.LyxDirName, "stencils")`, and add an assertion to the existing `WebsterGeometry` coverage that its `StencilsDir` field equals `StencilsDir(stateDir)` — that equality is what pins the one-construction-site property against a future divergence.
  No test in this file may call `t.Setenv`, read an environment variable, or touch disk; the whole point of the builders' told parameters is that this file needs neither.
- **Commit:** `test(standalonegeom): pin the burler/perch builders and the StencilsDir helper`

## Batch Tests

`verify:` runs `go test ./internal/standalonegeom/...`.
The package has no `//go:build integration` file and this batch adds none, so no `-tags integration` invocation is chained — every test here is tier 1 by construction, which is the package's stated design property.

The scope is deliberately the single package: cards 4-8 touch nothing outside `internal/standalonegeom`, and the two engine geometry structs they build against (`burlerengine.Geometry`, `perchengine.Geometry`) are read-only context, unmodified by this batch.
Card 5's `WebsterGeometry` repoint is the one behaviour-preserving edit to shipped code, and the existing `WebsterGeometry` test rows plus card 8's new equality assertion are its regression coverage.
The overview's module-wide `go vet ./...` is what catches any cross-package compile fallout at this batch's boundary.
