# Batch: burler told-geometry

```yaml
task: "burlerengine + perchengine told-geometry"
batch: "burler told-geometry"
number: 2
cards: 6
verify: go test ./internal/hubgeom/... ./internal/burlerengine/... ./internal/burlercli/... ./internal/perchcli/... && go vet -tags smoke ./internal/burlerengine/...
depends-on: [1]
```

## Batch Scope

This batch converts `internal/burlerengine` to told geometry: the `layout *lyxcwd.Location` field becomes `geom Geometry`, `New` takes a `Geometry` in the same argument position, `Engine.Run` reads the two told roots, and `internal/lyxcwd` leaves the package's production imports entirely.
It is one batch because changing `burlerengine.New`'s signature breaks every caller at once — `internal/burlercli/cli.go`, `internal/perchcli/cli.go`, `internal/burlerengine/engine_test.go`, and the two `//go:build smoke` files — so splitting it would leave the tree uncompilable at a batch boundary.

It touches `internal/perchcli/cli.go` for the `burlerengine.New` construction line only.
Batch 3 touches the same file for perch's own construction sites; that is why batch 3 depends on this one rather than running in parallel with it.

The `perchengine` side is deliberately absent: `perchengine` reaches `burlerengine` only through its own `Burler` interface (`Run`), never through `New`, so this batch does not touch it and the tree compiles at the boundary.

Batch-local decision beyond `## Shared Decisions`: the smoke-tagged tests are compiled but not run.
`go vet -tags smoke ./internal/burlerengine/...` is in `verify:` because `smoke_round_test.go` and `smoke_cluster_test.go` construct `burlerengine.New` directly, and no untagged or `integration`-tagged command compiles them — a broken call there would otherwise be invisible.
Running them is out of scope; they spawn real agents.

## Cards

### Card 6: convert `burlerengine.Engine` to told geometry

- **Context:**
  - `internal/burlerengine/geometry.go`
  - `internal/burlerengine/profile.go`
  - `internal/burlerengine/doc.go`
  - `internal/pattern/pattern.go`
- **Edits:**
  - `internal/burlerengine/engine.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/burlerengine/engine.go`, replace the `layout *lyxcwd.Location` field on `Engine` with `geom Geometry`, keeping its position between `shuttle` and `cfg`.
  Change `New`'s signature from `New(shuttle Shuttle, layout *lyxcwd.Location, cfg Config, stencilsDir string) *Engine` to `New(shuttle Shuttle, geom Geometry, cfg Config, stencilsDir string) *Engine`, and its body's struct literal from `layout: layout` to `geom: geom`.
  In `Engine.Run`, replace `e.layout.WorktreePath()` with `e.geom.WorktreeRoot` in the `p.validate` call, and both `e.layout.AnchorPath()` occurrences with `e.geom.AnchorPath` — the first in the `pattern.Directive` call, the second in the `burlerDir` `filepath.Join`.
  Nothing else in `Run` changes: `pattern.Directive` already takes `(anchorPath, stencilsDir string, role Role)`, so only the argument is swapped, not the call shape.
  Remove `github.com/Knatte18/loomyard/internal/lyxcwd` from the import block; it becomes unused.
  Update the two doc comments this falsifies: `Engine`'s, which says it resolves `Profile` paths against `layout`'s worktree root — say `geom.WorktreeRoot` instead; and `New`'s, which says it resolves relative `Profile` paths against `layout.WorktreePath()` — say `geom.WorktreeRoot`, and state that `geom` is the told geometry the caller supplies (`hubgeom.BurlerGeometry` in hub mode).
  Leave the `burlerDir` inline comment about `AnchorPath`-anchoring as it is — it explains the anchoring choice, not the carrier type, and stays true.
  `internal/burlerengine/doc.go` was checked during planning and names no `layout` field and no `*lyxcwd.Location`; read it to confirm, and leave it unedited.
- **Commit:** `refactor(burlerengine): take a told Geometry instead of a Location`

### Card 7: correct `prompt.go`'s file-header claim

- **Context:**
  - `internal/burlerengine/engine.go`
- **Edits:**
  - `internal/burlerengine/prompt.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** `internal/burlerengine/prompt.go`'s file header states that `composePrompt` takes the three instruction paths as plain string parameters rather than a `*lyxcwd.Location`, so it never gains geometry awareness of its own.
  After card 6 the package holds no `*lyxcwd.Location` at all, so the contrast the sentence draws no longer names anything real.
  Rewrite that clause so it contrasts against the engine's told `Geometry` instead: `composePrompt` takes the three instruction paths as plain string parameters rather than the engine's `Geometry`, so it never gains geometry awareness of its own, and the caller (`Engine.Run`) computes the directive, the stencils directory, and the three paths.
  Change nothing else in the file — no code, no other comment.
- **Commit:** `docs(burlerengine): retarget prompt.go's header at the told Geometry`

### Card 8: convert `engine_test.go` and strengthen the swap guard

- **Context:**
  - `internal/burlerengine/engine.go`
  - `internal/burlerengine/geometry.go`
  - `internal/burlerengine/profile.go`
  - `internal/burlerengine/profile_test.go`
  - `internal/lyxdirs/dirs.go`
- **Edits:**
  - `internal/burlerengine/engine_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Convert all eight `New(...)` call sites in `internal/burlerengine/engine_test.go` from a `&lyxcwd.Location{...}` literal to a `Geometry` literal, and remove the now-unused `github.com/Knatte18/loomyard/internal/lyxcwd` import.
  Seven of the eight — `newEngineForTest` plus the five inline sites inside sub-tests plus the final site — currently build `&lyxcwd.Location{HubPath: filepath.Dir(root), WorktreeName: filepath.Base(root)}`, whose `WorktreePath()` and `AnchorPath()` both resolve to `root`.
  Each becomes `Geometry{WorktreeRoot: root, AnchorPath: root}`, which is byte-identical.
  Keep the two roots collapsed at these seven sites — those tests are not about anchoring, and collapsing them is what the current fixture already does.
  The eighth site is `TestEngine_Run_MaterializesInstructionFiles`, which builds a `Location` with `AnchorRel: filepath.Join("sub", "dir")` precisely so the instruction dir's `AnchorPath` anchoring is observable.
  Rewrite it as the explicit swap guard the discussion calls for: build `worktreeRoot` and `anchorPath` as two distinct directories (`anchorPath` under `worktreeRoot` at the same `sub/dir` subpath, so the fixture's geometry is unchanged), construct via `Geometry{WorktreeRoot: worktreeRoot, AnchorPath: anchorPath}`, and keep every existing assertion intact — the three materialized instruction files, their paths appearing in `shuttle.spec.Prompt`, and instruction-1's content carrying the profile's rubric.
  Add two assertions on top: the round directory must be found under `filepath.Join(anchorPath, lyxdirs.DotLyxDirName, "burler")` and must NOT be found under `filepath.Join(worktreeRoot, lyxdirs.DotLyxDirName, "burler")`, so a swapped constructor fails here directly rather than surfacing downstream as a file-not-found.
  The profile's relative target/fasit paths must keep resolving under `WorktreeRoot` — the existing `newEngineTestProfile` writes them at `root`, so `worktreeRoot` must be that same directory for `(*Profile).validate` to keep passing.
  One further site outside the eight constructors also breaks and must be fixed in this card: inside `TestEngine_Run_PatternDirectiveReachesInstruction1`, the line building `burlerDir` reads `e.layout.AnchorPath()` directly off the constructed `*Engine`.
  After card 6 that field is `e.geom`, and `AnchorPath` is a plain string field rather than a method, so rewrite the expression as `e.geom.AnchorPath` — field access, no call parens.
  This is the only `.layout` reference left in the package once cards 6 and 8 are done; the rest of that test is unchanged.
  Update the file header comment: it currently says every `*lyxcwd.Location` built here sets `HubPath` and `WorktreeName` to a test temp dir so `WorktreePath()` resolves materialization there.
  Restate it in terms of the told `Geometry` — every `Geometry` built here points `WorktreeRoot` at a test temp dir so materialization lands there rather than in the real package source tree — and name `TestEngine_Run_MaterializesInstructionFiles` as the one site that keeps `WorktreeRoot` and `AnchorPath` distinct on purpose.
  `internal/burlerengine/profile_test.go` calls `(*Profile).validate(root, cfg)` with a plain string already and needs no change; read it to confirm it still exercises the told root, and do not edit it.
- **Commit:** `test(burlerengine): convert to Geometry and pin the anchor/worktree swap`

### Card 9: repoint the smoke-tagged construction sites

- **Context:**
  - `internal/burlerengine/engine.go`
  - `internal/hubgeom/hubgeom.go`
  - `internal/hubforge/hub.go`
- **Edits:**
  - `internal/burlerengine/smoke_round_test.go`
  - `internal/burlerengine/smoke_cluster_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Both files are `package burlerengine_test` and already import `github.com/Knatte18/loomyard/internal/hubgeom` for their `hubgeom.ReedGeometry(h.Location)` line, so no import changes.
  In `internal/burlerengine/smoke_round_test.go`, change `burlerengine.New(runner, h.Location, burlerengine.Config{}, fabricengine.StencilsDir(h.Location.HubPath))` to pass `hubgeom.BurlerGeometry(h.Location)` in `h.Location`'s position, leaving the other three arguments exactly as they are.
  In `internal/burlerengine/smoke_cluster_test.go`, make the identical swap on its `burlerengine.New(runner, h.Location, cfg, fabricengine.StencilsDir(h.Location.HubPath))` line.
  An external `_test` package importing `hubgeom` creates no import cycle even though `hubgeom` imports `burlerengine` — that is why these two sites may call the teller directly rather than building a `Geometry` literal by hand.
  Both files keep their `//go:build smoke` constraint; nothing else in either file changes, and no test changes tier.
- **Commit:** `test(burlerengine): repoint smoke construction through hubgeom.BurlerGeometry`

### Card 10: repoint `burlercli`'s construction site

- **Context:**
  - `internal/burlerengine/engine.go`
  - `internal/hubgeom/hubgeom.go`
- **Edits:**
  - `internal/burlercli/cli.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/burlercli/cli.go`'s `PersistentPreRunE`, change `c.engine = burlerengine.New(runner, layout, burlerCfg, fabricengine.StencilsDir(layout.HubPath))` to pass `hubgeom.BurlerGeometry(layout)` in `layout`'s position.
  `hubgeom` is already imported for the `hubgeom.ReedGeometry(layout)` line a few lines above, so no import changes and no new dependency.
  `layout` stays in scope and stays used — by `fabricengine.StencilsDir(layout.HubPath)` on this same line and by the config loads above it, which is why the `lyxcwd` import stays.
  This is the whole card: no other line in the file changes, and the resolved engine geometry is byte-identical to before, since `BurlerGeometry` fills `WorktreeRoot` from `layout.WorktreePath()` and `AnchorPath` from `layout.AnchorPath()` — the two accessors `Engine` called on the `Location` itself before this task.
- **Commit:** `refactor(burlercli): tell burler its geometry via hubgeom`

### Card 11: repoint `perchcli`'s burler construction site

- **Context:**
  - `internal/burlerengine/engine.go`
  - `internal/hubgeom/hubgeom.go`
- **Edits:**
  - `internal/perchcli/cli.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/perchcli/cli.go`'s `PersistentPreRunE`, change `c.burlerEngine = burlerengine.New(runner, layout, burlerCfg, fabricengine.StencilsDir(layout.HubPath))` to pass `hubgeom.BurlerGeometry(layout)` in `layout`'s position.
  `hubgeom` is already imported for the `hubgeom.ReedGeometry(layout)` line directly above it, so no import changes.
  This card touches that one line and nothing else in the file.
  The `perchengine.RunsDir`/`ScratchDir` call sites further down, and the `c.layout` field itself, belong to batch 3 — do not change them here.
- **Commit:** `refactor(perchcli): tell burler its geometry via hubgeom`

## Batch Tests

`verify:` runs `go test` over the four packages this batch touches or breaks — `internal/burlerengine` (the converted engine and its unit suite), `internal/burlercli` and `internal/perchcli` (the two production construction sites), and `internal/hubgeom` (whose `BurlerGeometry` becomes load-bearing here for the first time) — then `go vet -tags smoke ./internal/burlerengine/...` as a compile-only step for the two smoke files.
The vet step is not optional bookkeeping: `smoke_round_test.go` and `smoke_cluster_test.go` are the only `//go:build smoke` files in the tree's burler packages, and neither `go test ./...` nor an `integration`-tagged run compiles them, so card 9's edits have no other gate.
Running the smoke suite itself is out of scope — it spawns real agents.

The batch's own regression evidence is `internal/burlerengine/engine_test.go`, which keeps every existing assertion and gains the two round-directory assertions from card 8.
Those two are what make a swapped `WorktreeRoot`/`AnchorPath` fail loudly at the construction boundary instead of surfacing as an unrelated file-not-found later in the round.

Manual check to run once at batch end, per the discussion's Verify list: `grep -rn lyxcwd internal/burlerengine/ --include=*.go` must return no hit in a production (non-`_test.go`) file.
`internal/burlerengine/prompt.go`'s header no longer mentions it after card 7, and `engine.go`'s import is dropped in card 6, so a hit means one of those two cards is incomplete.

The overview's module-wide `verify: go build ./...` runs at the batch boundary and is what catches any caller of `burlerengine.New` outside these four packages; the enumeration was re-run against the tree for this task and found none, so a failure there is new information worth reading carefully rather than patching around.
