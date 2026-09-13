# Batch: specs-geometry

```yaml
task: Deploy cited spec/design docs to target repos like stencils
batch: specs-geometry
number: 2
cards: 4
verify: go test ./internal/fabricengine/... ./internal/standalonegeom/... ./internal/hubgeom/... ./internal/websterengine/... ./internal/lyxcwd/...
depends-on: []
```

## Batch Scope

This batch declares where a deployed spec lives, in both modes, and nothing else.
It adds `SpecsDir` to `internal/fabricengine` (hub) and `internal/standalonegeom` (standalone) as exact structural mirrors of the existing `StencilsDir` pair, and adds the one geometry-struct field webster needs, since webster is the only consumer of the four that carries its stencils directory as a geometry field rather than as a plain told parameter.

It is one batch because all four cards are the same one-line derivation repeated across the two modes plus the struct that carries it, and because the geometry-literal enforcement walk in `internal/lyxcwd` has to stay green across all of them at once.

The external interface batches 4 and 5 consume is: `fabricengine.SpecsDir(hub) string`, `standalonegeom.SpecsDir(stateDir) string`, and `websterengine.Geometry.SpecsDir`.

No seeding, no reconcile call, and no marker plumbing happens here — a directory path is declared, never created.
Batch-local decision: `"specs"` gets no `geometryTokenOwners` row, for the same reason `"stencils"` has none; the batch asserts that decision by keeping the enforcement walk in its own verify scope rather than assuming it.

## Cards

### Card 5: SpecsDir in fabricengine

- **Context:**
  - `internal/lyxdirs/dirs.go`
- **Edits:**
  - `internal/fabricengine/junctionnames.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add an unexported `specsDirName` const and an exported `SpecsDir` function to `internal/fabricengine/junctionnames.go`, placed immediately after the existing `StencilsDir` function so the two sit together as a declared pair.

  ```go
  const specsDirName = "specs"

  func SpecsDir(hub string) string {
  	return filepath.Join(BoardDir(hub), lyxdirs.LyxDirName, specsDirName)
  }
  ```

  Give `specsDirName` a comment stating, as `stencilsDirName`'s own comment already does for `"stencils"`, that it is unexported because `"specs"` is not a policed geometry token and needs no `geometryTokenOwners` row.
  Give `SpecsDir` a comment stating that it returns the hub-wide deployed-specs directory shared by every worktree in the hub, that `internal/stencilstore` receives this value as its baseDir and never joins the intermediate components itself, and that it deliberately mirrors `StencilsDir` — the two are siblings under the same `_lyx` tree, one holding producer prompts, the other the normative docs those prompts point at.

  Do not change `StencilsDir`, `stencilsDirName`, `BoardDir`, or any other declaration in this file.
- **Commit:** `feat(fabricengine): add SpecsDir mirroring StencilsDir`

### Card 6: SpecsDir hub-mode test

- **Context:**
  - `internal/fabricengine/stencilsdir_test.go`
  - `internal/fabricengine/junctionnames.go`
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/specsdir_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `internal/fabricengine/specsdir_test.go` in `package fabricengine`, modelled one-for-one on `internal/fabricengine/stencilsdir_test.go`.
  Three tests.

  `TestSpecsDir` asserts `SpecsDir("/h")` equals `filepath.Join("/h", "_board", "_lyx", "specs")`.
  The expected value is spelled as a literal join here, exactly as the stencils test spells its own, so the test fails on a silent change to any component of the derivation rather than restating the production expression.

  `TestSpecsDir_IsChildOfBoardDir` asserts the result has `BoardDir(hub) + string(filepath.Separator)` as a prefix, mirroring the neighbouring stencils assertion.

  Add a third test, `TestSpecsDir_IsSiblingOfStencilsDir`, asserting `filepath.Dir(SpecsDir("/h")) == filepath.Dir(StencilsDir("/h"))` and that the two paths are not equal.
  This is the assertion that pins the structural-mirror decision itself: a future edit that nests one inside the other, or collapses them onto one directory, fails here.
- **Commit:** `test(fabricengine): pin SpecsDir's derivation and its sibling relationship to StencilsDir`

### Card 7: SpecsDir in standalonegeom

- **Context:**
  - `internal/standalonegeom/stencilsdir.go`
  - `internal/lyxdirs/dirs.go`
- **Edits:**
  - `internal/standalonegeom/standalonegeom_test.go`
- **Creates:**
  - `internal/standalonegeom/specsdir.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `internal/standalonegeom/specsdir.go` in `package standalonegeom`, mirroring `internal/standalonegeom/stencilsdir.go` exactly:

  ```go
  func SpecsDir(stateDir string) string {
  	return filepath.Join(stateDir, lyxdirs.LyxDirName, "specs")
  }
  ```

  Its doc comment states the same three properties the neighbouring `StencilsDir` comment states: that it is the sole construction site for the standalone deployed-specs directory across every standalone-capable CLI, that it deliberately mirrors `fabricengine.SpecsDir(hub)` with stateDir playing hub's role, and that like every other builder in this package it takes only stateDir — never calling `standalonestate.Derive`, never reading the environment, never touching disk.
  Add one sentence the stencils sibling does not need: unlike the stencils directory, this one has no CLI flag that can override it, so this function's result is the only spelling standalone mode ever uses.

  Then add `TestSpecsDir` to `internal/standalonegeom/standalonegeom_test.go`, placed immediately after the existing `TestStencilsDir` and written in the same shape as it — a `t.Parallel()` call, the same `stateDir` fixture spelling, and an assertion against `filepath.Join(stateDir, lyxdirs.LyxDirName, "specs")`.
  Add a second assertion in the same test that `SpecsDir(stateDir) != StencilsDir(stateDir)`, pinning that the two standalone directories never converge.
- **Commit:** `feat(standalonegeom): add SpecsDir mirroring StencilsDir`

### Card 8: SpecsDir as a webster geometry field

- **Context:**
  - `internal/fabricengine/junctionnames.go`
  - `internal/standalonegeom/specsdir.go`
- **Edits:**
  - `internal/websterengine/geometry.go`
  - `internal/hubgeom/webstergeom.go`
  - `internal/standalonegeom/webstergeom.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add a `SpecsDir string` field to `websterengine.Geometry` in `internal/websterengine/geometry.go`, declared immediately after the existing `StencilsDir` field.
  Its comment states that it is the told absolute directory the deployed normative specs are read from by the agent the prompt is handed to — never by Go — and that, like every other field on this struct, populating it with a usable absolute path is entirely the caller's obligation.
  Update the struct's own doc comment where it counts its fields, so the count matches after the addition.

  Fill the new field in both constructors.
  In `internal/hubgeom/webstergeom.go`, add `SpecsDir: fabricengine.SpecsDir(l.HubPath)` beside the existing `StencilsDir: fabricengine.StencilsDir(l.HubPath)` line.
  In `internal/standalonegeom/webstergeom.go`, add `SpecsDir: SpecsDir(stateDir)` beside the existing `StencilsDir: StencilsDir(stateDir)` line, and extend that constructor's doc comment sentence about `StencilsDir` being a default the CLI argument boundary may override to state that `SpecsDir` is NOT in that class — there is no flag for it, so it is never overridden after this builder returns.

  Update `internal/websterengine/geometry.go`'s file comment where it describes the struct as eight-field, since it becomes nine.
  Make the same arithmetic correction in `internal/standalonegeom/webstergeom.go`'s doc comment, which states that none of webster's eight values is hash-derived.

  Nothing reads the new field yet — batch 5 is what threads it into a rendered prompt.
  Do not add a validator, a default, or a constructor to `websterengine.Geometry`; the type deliberately declares fields only.
- **Commit:** `feat(websterengine): carry the deployed-specs directory as told geometry`

## Batch Tests

`verify: go test ./internal/fabricengine/... ./internal/standalonegeom/... ./internal/hubgeom/... ./internal/websterengine/... ./internal/lyxcwd/...` covers every package this batch touches plus the one that polices it.

- `internal/fabricengine` — the new `specsdir_test.go` from card 6, alongside the existing geometry tests that must stay green.
- `internal/standalonegeom` — the extended `standalonegeom_test.go` from card 7, and the webster-geometry coverage affected by card 8.
- `internal/hubgeom` and `internal/websterengine` — compile-and-test coverage of card 8's struct-field addition and both constructors.
- `internal/lyxcwd` — this is the deliberate inclusion, not incidental: `TestEnforcement_GeometryLiterals` walks the repo's geometry tokens against `geometryTokenOwners`, and running it here is how the batch asserts the "`specs` is not a policed token" decision rather than assuming it.
  If that walk does flag `"specs"`, the decision is wrong and the remedy is a `geometryTokenOwners` row naming `internal/fabricengine` and `internal/standalonegeom` as its owners — not a suppression.

No integration-tagged file is touched, so no `-tags integration` invocation is needed in this batch's verify.
