# Batch: loom-paths-and-seed-sentinel

```yaml
task: 'loom: session bootstrap'
batch: loom-paths-and-seed-sentinel
number: 3
cards: 4
verify: go test ./internal/loomengine/ ./internal/loomshed/ ./cmd/lyx/
depends-on: []
```

## Batch Scope

This batch prepares loom's own side of the bootstrap without adding any CLI: the two new never-tracked path accessors the driver spawn needs (`driver.log` and `bootstrap.lock`), the anchor-relative form of the status file that the weft commit pathspec is built from, and the `ErrSeedExists` sentinel that lets a re-run tolerate an already-seeded pair.
It is one batch because all four cards are small, additive, and jointly proved by three package test runs; nothing here has a caller yet.
It depends on nothing, so it can land alongside batch 1.

The external interface batches 4 and 5 consume: `loomengine.LoomStatusRel`, `loomengine.LoomDriverLog`, `loomengine.LoomBootstrapLock`, and `loomshed.ErrSeedExists`.

Batch-local decisions beyond `## Shared Decisions` in the overview:

- The two repeated path segments in `internal/loomengine`'s existing accessors are lifted into named constants in the same card that adds the third and fourth accessors, so the segments are declared once rather than five times.
- `Seed`'s refusal keeps its existing human-readable text and gains the sentinel by wrapping, so the existing refusal test that only asserts a non-nil error keeps passing unchanged.

## Cards

### Card 11: loomengine gains the loom-directory constants and three new accessors

- **Context:**
  - `internal/lyxdirs/dirs.go`
  - `internal/lyxcwd/lyxcwd.go`
  - `CONSTRAINTS.md`
- **Edits:**
  - `internal/loomengine/config.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Declare two unexported constants beside the existing `discussionDirName`: `loomDirName = "loom"` and `loomStatusFileName = "status.json"`, with a comment stating `loomengine` is their sole declarer for the same reason `discussionDirName` gives.
  Add `func LoomStatusRel() string`, returning the join of `lyxdirs.LyxDirName`, `loomDirName` and `loomStatusFileName` — the worktree-anchor-relative form of the status file, documented as existing so a caller building a weft commit pathspec never has to name a directory segment loom owns.
  Rewrite `LoomStatusFile` to return `filepath.Join(l.AnchorPath(), LoomStatusRel())` so the segments are joined in exactly one place; its doc comment, its anchoring rationale, and its product-collision rationale all stay as they are.
  Rewrite `LoomStatusLock` and `LoomRunLock` to use `loomDirName` in place of their repeated literal segment, leaving their return values byte-identical and their doc comments unchanged.
  Add `func LoomDriverLog(l *lyxcwd.Location) string`, returning the ephemeral-directory-anchored join of `l.AnchorPath()`, `lyxdirs.DotLyxDirName`, `loomDirName` and `"driver.log"`.
  Its doc comment must state that it is the detached driver's captured stdout and stderr, that it lives under the ephemeral tree at the mirrored subpath of the durable status file per the Durable-vs-Ephemeral State Invariant, and that it exists as an accessor rather than an inline path because `cmd/lyx`'s transient guard walks constructors, not call sites.
  Add `func LoomBootstrapLock(l *lyxcwd.Location) string`, returning the same shape with `"bootstrap.lock"`.
  Its doc comment must state that it serialises the session bootstrap's probe-and-spawn sequence, that it is a third lock distinct from both the per-persist status lock and the whole-run lock, and that it is released before the terminal handover because that handover blocks for the operator's entire session.
- **Commit:** `feat(loomengine): add the driver-log, bootstrap-lock, and status-relative path accessors`

### Card 12: loomshed exports an ErrSeedExists sentinel

- **Context:**
  - `internal/state/state.go`
  - `internal/shedengine/status.go`
- **Edits:**
  - `internal/loomshed/seed.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Declare an exported sentinel `ErrSeedExists` at package level, documented as marking `Seed`'s refusal to overwrite an existing status file so a caller that is deliberately re-entrant — the session bootstrap, which calls `Seed` unconditionally on every invocation — can recognise exactly that one case with `errors.Is` and treat it as success, while every other `Seed` error still propagates.
  In the mutate closure's `found` branch, wrap the sentinel with the existing refusal text using the `%w` verb so `errors.Is` matches through `state.UpdateJSON`, which returns a mutate error verbatim.
  Keep the message's existing wording about not destroying an in-flight run's history and keep it naming the status path.
  Extend `Seed`'s own doc comment with one sentence stating that the refusal is reported as `ErrSeedExists` and naming the re-entrancy use it exists for.
  Import `errors` for the declaration.
- **Commit:** `feat(loomshed): export ErrSeedExists so a re-entrant seeder can tolerate the refusal`

### Card 13: the two cmd/lyx path-guard tables learn the new transients

- **Context:**
  - `internal/loomengine/config.go`
  - `internal/lyxdirs/dirs.go`
- **Edits:**
  - `cmd/lyx/notransients_test.go`
  - `cmd/lyx/constructoranchoring_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `notransients_test.go`, add two rows to the transient set beside the existing `loomengine.LoomStatusLock` and `loomengine.LoomRunLock` rows — one for `loomengine.LoomDriverLog` and one for `loomengine.LoomBootstrapLock` — so both are asserted to resolve under the ephemeral tree at the mirrored subpath of the durable content they relate to, at both of that file's fixtures.
  Do not add a durable-set row for either; neither is durable.
  In `constructoranchoring_test.go`, add an `assertPath` row for each of the two new accessors in both the unanchored and the subpath-anchored test, expecting the ephemeral base joined with the loom subdirectory and the respective filename, and add both to the collision map that file already builds from the ephemeral group.
  Update that file's package-doc enumeration of the ephemeral group so it names the two new accessors alongside the existing loom lock.
- **Commit:** `test(lyx): pin the new loom transient paths in the two constructor guards`

### Card 14: unit coverage for the new loomengine accessors and the sentinel

- **Context:**
  - `internal/loomengine/config.go`
  - `internal/loomengine/testmain_test.go`
  - `internal/lyxdirs/dirs.go`
- **Edits:**
  - `internal/loomengine/config_test.go`
  - `internal/loomshed/seed_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `config_test.go`, add a test asserting `LoomStatusRel()` is exactly the durable directory name joined with the loom subdirectory and the status filename, and a second asserting `LoomStatusFile` for a hand-built `*lyxcwd.Location` equals `l.AnchorPath()` joined with `LoomStatusRel()` — the regression guard that the refactor in card 11 left the value byte-identical.
  Add a table-driven test over both anchor fixtures asserting `LoomDriverLog` and `LoomBootstrapLock`, and asserting `LoomBootstrapLock` differs from both `LoomStatusLock` and `LoomRunLock` — three distinct lock files is a correctness property, not a coincidence.
  Build every `*lyxcwd.Location` by hand; spawn nothing.
  In `seed_test.go`, extend the existing second-`Seed`-refuses test so it additionally asserts `errors.Is` against the sentinel, and add a sibling assertion that a `Seed` failure caused by something other than an existing file does not match the sentinel — drive that second case by pointing the status path at a location whose parent cannot be created.
- **Commit:** `test(loom): cover the new path accessors and the seed-exists sentinel`

## Batch Tests

`verify: go test ./internal/loomengine/ ./internal/loomshed/ ./cmd/lyx/` runs the three packages this batch edits and no others.
`internal/loomengine` covers cards 11 and 14's accessor assertions; `internal/loomshed` covers card 12's sentinel and its extended refusal test; `cmd/lyx` covers card 13's two guard tables plus the untouched-but-adjacent transient and anchoring walks that would catch a mistake in card 11's refactor.
All three are untagged tier-1 suites, so the run is fast and spawns nothing.
