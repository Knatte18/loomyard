# Batch: geometry types and hubgeom tellers

```yaml
task: "burlerengine + perchengine told-geometry"
batch: "geometry types and hubgeom tellers"
number: 1
cards: 5
verify: go test ./internal/hubgeom/... ./internal/burlerengine/... ./internal/perchengine/...
depends-on: []
```

## Batch Scope

This batch declares the two told-geometry carrier types and the two hub-mode tellers that build them, and nothing else.
It is additive: `burlerengine.Geometry` and `perchengine.Geometry` are new exported types nobody consumes yet, and `hubgeom.BurlerGeometry`/`hubgeom.PerchGeometry` are new exported functions nobody calls yet, so the tree compiles and every existing test passes unchanged at the batch boundary.
It is one batch because the discussion names `internal/hubgeom` the TDD candidate to write first: the anchor/worktree swap is made inside these two tellers, so `hubgeom_test.go`'s deliberately-hostile three-distinct-directories fixture is the cheapest place to pin the mapping, before any call site is rewired.

The external interface batches 2 and 3 consume: `burlerengine.Geometry{WorktreeRoot, AnchorPath string}`, `perchengine.Geometry{GateDir, AnchorPath string}`, `hubgeom.BurlerGeometry(l *lyxcwd.Location) burlerengine.Geometry`, and `hubgeom.PerchGeometry(l *lyxcwd.Location) perchengine.Geometry`.

Batch-local decision beyond `## Shared Decisions`: each `Geometry` type gets its own new `geometry.go` file rather than being appended to `engine.go`/`identity.go`, copying `internal/reedengine/geometry.go`, which declares the type only and adds no constructor, validator, or default.

## Cards

### Card 1: declare `burlerengine.Geometry`

- **Context:**
  - `internal/reedengine/geometry.go`
  - `internal/burlerengine/engine.go`
- **Edits:** none
- **Creates:**
  - `internal/burlerengine/geometry.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `internal/burlerengine/geometry.go` in `package burlerengine`, modelled on `internal/reedengine/geometry.go`.
  It declares exactly one type and nothing else — no constructor, no validator, no default, no import.
  Declare `type Geometry struct` with two string fields, in this order: `WorktreeRoot` and `AnchorPath`.
  Per-field doc comments: `WorktreeRoot` is the root `Engine.Run` resolves a `Profile`'s relative paths against, via `(*Profile).validate`; `AnchorPath` is the base the per-round `.lyx/burler` instruction directory joins onto.
  The type doc comment states that `Geometry` is the set of paths burler is told, once, at construction, and never derives itself; that `burlerengine.New` validates no field of a `Geometry`; that populating every field with a usable absolute path is entirely the caller's obligation; and that `hubgeom.BurlerGeometry` is the hub-mode answer that builds one from a resolved `*lyxcwd.Location` but is deliberately not imported here, because this file states the contract, not the implementation.
  Add a two-line file header comment in the same shape as `internal/reedengine/geometry.go`'s, naming this file as the declaration site for the two-field struct burler is told its coordinates through.
- **Commit:** `feat(burlerengine): declare the told-geometry Geometry struct`

### Card 2: declare `perchengine.Geometry`

- **Context:**
  - `internal/reedengine/geometry.go`
  - `internal/perchengine/engine.go`
- **Edits:** none
- **Creates:**
  - `internal/perchengine/geometry.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `internal/perchengine/geometry.go` in `package perchengine`, modelled on `internal/reedengine/geometry.go`.
  It declares exactly one type and nothing else — no constructor, no validator, no default, no import.
  Declare `type Geometry struct` with two string fields, in this order: `GateDir` and `AnchorPath`.
  Per-field doc comments: `GateDir` is the gate command's working directory, the value `Engine.Run` stamps onto `treadleengine.Profile.GateDir`; `AnchorPath` is the base `RunsDir` and `ScratchDir` join onto, carried alongside `GateDir` so a perch call site's two roots stay visible together, and is deliberately not read by `Engine` itself today.
  The type doc comment states that `Geometry` is the set of paths perch is told, once, at construction, and never derives itself; that `perchengine.New` validates no field of a `Geometry`; and that `hubgeom.PerchGeometry` is the hub-mode answer that builds one from a resolved `*lyxcwd.Location` but is deliberately not imported here.
  The field is named `GateDir` rather than `WorktreeRoot` because perch's only use of that root is `treadleengine.Profile.GateDir` — state that in the field's doc comment.
  Add a two-line file header comment in the same shape as `internal/reedengine/geometry.go`'s.
- **Commit:** `feat(perchengine): declare the told-geometry Geometry struct`

### Card 3: add `hubgeom.BurlerGeometry` and `hubgeom.PerchGeometry`

- **Context:**
  - `internal/burlerengine/geometry.go`
  - `internal/perchengine/geometry.go`
  - `internal/lyxcwd/lyxcwd.go`
- **Edits:**
  - `internal/hubgeom/hubgeom.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add `func BurlerGeometry(l *lyxcwd.Location) burlerengine.Geometry` and `func PerchGeometry(l *lyxcwd.Location) perchengine.Geometry` to `internal/hubgeom/hubgeom.go`, beside the existing `ReedGeometry`, in that order after it.
  `BurlerGeometry` returns `burlerengine.Geometry{WorktreeRoot: l.WorktreePath(), AnchorPath: l.AnchorPath()}`.
  `PerchGeometry` returns `perchengine.Geometry{GateDir: l.WorktreePath(), AnchorPath: l.AnchorPath()}` — perch's `GateDir` is filled from `l.WorktreePath()`, which is the value `perchcli` passed to `perchengine.New` before this task, so the gate command's working directory resolves byte-identically.
  Add `github.com/Knatte18/loomyard/internal/burlerengine` and `github.com/Knatte18/loomyard/internal/perchengine` to the import block, keeping it gofmt-sorted.
  Each new function's doc comment follows `ReedGeometry`'s exactly: it states that the function reads the resolved `Location`'s paths off its accessors and passes them through untouched, and that it performs no `os.Getwd`, no git discovery, and no path resolution of its own, so `internal/lyxcwd` stays the sole owner of cwd resolution per the Cwd Resolution Invariant.
  Rewrite the two-line file header comment at the top of the file: it currently names `ReedGeometry` as the file's whole content, which this card falsifies.
  Replace it with a header stating that this file implements the hub-mode tellers that convert a resolved `*lyxcwd.Location` into each engine's own geometry struct, and that `ReedGeometry`, `BurlerGeometry`, and `PerchGeometry` are its members.
- **Commit:** `feat(hubgeom): add BurlerGeometry and PerchGeometry tellers`

### Card 4: pin both tellers against the hostile fixture

- **Context:**
  - `internal/hubgeom/hubgeom.go`
  - `internal/burlerengine/geometry.go`
  - `internal/perchengine/geometry.go`
- **Edits:**
  - `internal/hubgeom/hubgeom_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add `TestBurlerGeometry` and `TestPerchGeometry` to `internal/hubgeom/hubgeom_test.go`, after the existing `TestReedGeometry`, reusing its table-driven shape and its deliberately-hostile fixture verbatim: `hub`, `worktreeRoot`, and `anchorPath` as three distinct directories, `anchorRel` a real nested subpath built with `filepath.Join("sub", "dir")`, and `RepoName` set to a value differing from every basename.
  `TestBurlerGeometry` asserts `BurlerGeometry(l).WorktreeRoot` equals the independently computed `worktreeRoot` and `BurlerGeometry(l).AnchorPath` equals the independently computed `anchorPath`.
  `TestPerchGeometry` asserts `PerchGeometry(l).GateDir` equals `worktreeRoot` and `PerchGeometry(l).AnchorPath` equals `anchorPath`.
  Both compute their expected values by `filepath.Join` in the test itself, never by calling the function under test or the `Location` accessors, so a swapped field assignment inside either teller fails rather than passing silently.
  Error message shape copies the existing rows: `t.Errorf("BurlerGeometry(l).WorktreeRoot = %q; want %q", got.WorktreeRoot, worktreeRoot)`.
  Extend the file header comment so it names all three tellers as the guarded surface rather than `ReedGeometry` alone; the existing sentence explaining why the fixture keeps the three directories distinct stays as it is, since it is the reason these two new tests exist.
  Add the `burlerengine` and `perchengine` imports; the file stays in the internal `package hubgeom` and stays untagged pure-`filepath.Join` arithmetic per the Test Tier Purity Invariant.
- **Commit:** `test(hubgeom): pin BurlerGeometry and PerchGeometry field mapping`

### Card 5: retire the "future work" wording in hubgeom's package doc

- **Context:**
  - `internal/hubgeom/hubgeom.go`
- **Edits:**
  - `internal/hubgeom/doc.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** `internal/hubgeom/doc.go` currently states that hubgeom's whole contract today is `ReedGeometry`, and names T6 as the task that adds `BurlerGeometry` and `PerchGeometry`.
  Both statements are falsified by card 3.
  Rewrite the paragraph so it names `ReedGeometry`, `BurlerGeometry`, and `PerchGeometry` as the contract today, and leaves only `WebsterGeometry` (T7) named as the pending sibling later waves add here rather than spawning per-engine packages or re-deriving the construction inline at each call site.
  Leave the surrounding paragraphs unchanged: the one-way told-direction sentence still holds verbatim (neither `burlerengine`'s nor `perchengine`'s geometry type imports hubgeom), and so does the closing sentence that standalone CLIs do not call hubgeom.
- **Commit:** `docs(hubgeom): name BurlerGeometry and PerchGeometry as shipped`

## Batch Tests

`verify: go test ./internal/hubgeom/... ./internal/burlerengine/... ./internal/perchengine/...` covers the batch's only runnable surface, `internal/hubgeom/hubgeom_test.go`, plus the two packages gaining a new file — `internal/burlerengine` and `internal/perchengine` — so a compile error in either new `geometry.go` is caught here rather than at the next batch.
The scope is deliberately three packages and not the whole tree: this batch adds only unconsumed exported declarations, and the overview's module-wide `verify: go build ./...` runs at the batch boundary as the cross-package compile net.

The load-bearing new coverage is card 4's two tests against the three-distinct-directories fixture.
That fixture is what makes them meaningful — a swapped `WorktreeRoot`/`AnchorPath` (or `GateDir`/`AnchorPath`) assignment inside either teller compiles cleanly and would pass any fixture where the two roots coincide, which is this refactor's one silent failure mode.

No existing assertion changes in this batch, and no test changes tier.
