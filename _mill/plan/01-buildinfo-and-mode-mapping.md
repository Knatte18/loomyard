# Batch: buildinfo-and-mode-mapping

```yaml
task: "lift the orchestrator preflight out of loomengine, plus the shared standalone-CLI foundations"
batch: "buildinfo-and-mode-mapping"
number: 1
cards: 4
verify: go test ./internal/buildinfo/... ./internal/stencilstore/... ./internal/lyxcwd/...
depends-on: []
```

## Batch Scope

This batch lands the whole "which build channel am I" surface: the new stdlib-only `internal/buildinfo` leaf that owns the ldflags-stamped `Channel` variable, and the single mapping site `stencilstore.ModeFor(dev bool) Mode` that turns "is this a dev build" into a `stencilstore.Mode`.
They belong in one batch because the split between them is the whole design decision — `buildinfo` must not return a `stencilstore.Mode` (that would break the stdlib-only leaf property T7 and T8 depend on), so the mapping has to live in the package that owns `Mode`.
The external interface batch 4 consumes is exactly `buildinfo.IsDev()` and `stencilstore.ModeFor(...)`; batch 4 also repoints the ldflags path at `internal/buildinfo.Channel` and deletes `cmd/lyx/stencilseed.go`'s own `buildChannel` variable.
No batch-local decisions differ from `## Shared Decisions` in the overview.

## Cards

### Card 1: create the internal/buildinfo leaf

- **Context:**
  - `internal/lyxdirs/doc.go`
  - `internal/hubgeom/doc.go`
  - `cmd/lyx/stencilseed.go`
- **Edits:** none
- **Creates:**
  - `internal/buildinfo/buildinfo.go`
  - `internal/buildinfo/doc.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Declare `package buildinfo` in both new files, importing nothing at all — not even the standard library.
  `internal/buildinfo/buildinfo.go` declares exactly two exported symbols:
  `var Channel string`, documented as the value stamped by `tools/deploy -dev` via `-ldflags "-X github.com/Knatte18/loomyard/internal/buildinfo.Channel=dev"`, with an unstamped binary (a plain `go build`, a `go install`, or a `go test` binary) leaving it empty and empty meaning production;
  and `func IsDev() bool` returning `Channel == "dev"` — an exact comparison, never a prefix match and never case-insensitive.
  Carry across the "production is the conservative default because it keeps shipped defaults converging; dev is the exception and must opt in explicitly" reasoning from the doc comment currently on `buildChannel` in `cmd/lyx/stencilseed.go`.
  `internal/buildinfo/doc.go` carries the package doc comment, matching the vocabulary and tone of `internal/lyxdirs/doc.go` and `internal/hubgeom/doc.go`.
  It must state three things so a later reader does not "fix" them back:
  that the package is a zero-import leaf specifically so `cmd/lyx` and every future standalone CLI package can read the build channel with no cycle risk;
  that the accessor is `IsDev()` rather than the `StencilMode()` the producers-standalone design doc names, because `stencilstore.Mode` is a non-stdlib type whose package imports `internal/logger` and `internal/stencil`, and returning it would destroy the stdlib-only leaf property that same paragraph of the brief requires;
  and that the mapping from `IsDev()` to a stencil mode therefore lives in `stencilstore.ModeFor`, the single mapping site.
  Do not use the tokens `weft` or `warp` anywhere in either file.
- **Commit:** `feat(buildinfo): add zero-import build-channel leaf`

### Card 2: pin IsDev's exact-match semantics

- **Context:**
  - `internal/buildinfo/buildinfo.go`
- **Edits:** none
- **Creates:**
  - `internal/buildinfo/buildinfo_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  An untagged, in-package (`package buildinfo`) test file — no build constraint, no spawns of any kind, so it stays tier 1.
  Table-drive `IsDev()` over at least these `Channel` values: the empty string, `"dev"`, `"prod"`, `"Dev"`, `"DEV"`, `" dev"`, `"dev "`, and `"development"`.
  Only the exact `"dev"` row expects `true`; every other row expects `false`, which is what stops the comparison drifting to a prefix or case-insensitive match later.
  Save and restore the package-level `Channel` around each case (assign the previous value back via `t.Cleanup`) so the tests do not leak state into each other.
- **Commit:** `test(buildinfo): pin IsDev exact-match semantics`

### Card 3: enforce the Buildinfo leaf invariant mechanically

- **Context:**
  - `internal/tokenvocab/leaf_enforcement_test.go`
- **Edits:** none
- **Creates:**
  - `internal/buildinfo/leaf_enforcement_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Copy the structure of `internal/tokenvocab/leaf_enforcement_test.go` into `package buildinfo`: a `TestLeafInvariant_AllowlistOnly` that locates its own package directory via `runtime.Caller(0)`, walks it with `filepath.WalkDir`, skips directories and any file that is not a non-`_test.go` `.go` file, parses each with `go/parser` using `parser.ImportsOnly`, and fails on any import that is neither stdlib (no `.` in the first path segment) nor present in an `allowedImports` map.
  Declare `allowedImports` as an **empty** `map[string]bool{}` — `internal/buildinfo` has no permitted non-stdlib import — and word the failure message so it names the Buildinfo Leaf Invariant and reports the offending relative paths.
  Untagged: `go/parser` reads source from disk, which is not a spawn, so this file must carry no build constraint.
- **Commit:** `test(buildinfo): enforce zero-import leaf invariant`

### Card 4: add the single dev-to-Mode mapping site

- **Context:**
  - `internal/stencilstore/doc.go`
  - `internal/buildinfo/buildinfo.go`
  - `cmd/lyx/stencilseed.go`
- **Edits:**
  - `internal/stencilstore/stencilstore.go`
- **Creates:**
  - `internal/stencilstore/modefor_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In `internal/stencilstore/stencilstore.go`, immediately after the existing `const ( ModeProduction Mode = iota; ModeDev )` block, add `func ModeFor(dev bool) Mode` returning `ModeDev` when `dev` is true and `ModeProduction` otherwise.
  Its doc comment must say it is the single mapping site from "is this a dev build" to a `Mode`, that callers pass `buildinfo.IsDev()` into it, and that `ModeProduction` being `iota`'s zero value is what makes an unstamped binary safely classify as production.
  Do not import `internal/buildinfo` from `internal/stencilstore` — the parameter is a plain `bool`, and the dependency direction runs the other way at the call site in batch 4.
  Add no other exported symbol and change no existing behaviour in this file.
  `internal/stencilstore/modefor_test.go` is untagged and in-package (`package stencilstore`), asserting `ModeFor(true) == ModeDev` and `ModeFor(false) == ModeProduction`.
  Those two assertions are what keep the `buildinfo` split honest once `cmd/lyx/stencilseed.go` stops doing the comparison itself.
- **Commit:** `feat(stencilstore): add ModeFor, the single dev-to-Mode mapping site`

## Batch Tests

`verify:` runs `go test ./internal/buildinfo/... ./internal/stencilstore/... ./internal/lyxcwd/...`.

`./internal/buildinfo/...` covers the two new test files created here — `buildinfo_test.go` (the `IsDev()` exact-match table) and `leaf_enforcement_test.go` (the zero-import allowlist walk).
`./internal/stencilstore/...` covers the new `modefor_test.go` plus the package's existing `stencilstore_test.go`, `reconcile_test.go`, and `validate_test.go`, which is the regression net proving the `ModeFor` addition changed nothing about `Reconcile`'s behaviour.
`./internal/lyxcwd/...` is included in every code batch of this task because that package hosts `TestEnforcement_FabricVocabulary` and `TestEnforcement_GeometryLiterals`, the repo-wide walks that are the only thing catching a `weft`/`warp` token or a raw `_lyx` literal in the newly created production files — neither package's own test run would notice.
Every test file in this batch is untagged and tier-1 pure, so there is no integration-tagged run.
