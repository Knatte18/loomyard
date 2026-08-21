# Batch: recipe-file-and-loomrecipe-package

```yaml
task: 'loom: convert to a Shed recipe'
batch: 'recipe-file-and-loomrecipe-package'
number: 1
cards: 4
verify: go test ./contracts/... ./internal/loomrecipe/...
depends-on: []
```

## Batch Scope

This batch ships the two new production artefacts the whole task hangs off: the recipe file itself, embedded under `contracts/`, and the `internal/loomrecipe` package that parses it, builds it against a caller-supplied `shedrecipe.Env`, and returns the assembled `*shedengine.Shed`.
Nothing existing is deleted or rewired here — `loomshed.New` still exists and `internal/loomcli` still calls it, so the tree stays green throughout.
The external interface every later batch consumes is `loomrecipe.New(env shedrecipe.Env, paths loomrecipe.ShedPaths) (*shedengine.Shed, error)` plus the exported `recipes.LoomRecipe []byte`.

Batch-local decision: `contracts/recipes` gets no `doc.go` and no test of its own, following `contracts/stencils`' precedent (a file-header comment on the single Go file doubles as the package doc).
`internal/loomrecipe` gets a `doc.go`, matching every other `internal/` package in the tree.
A `contracts/recipes` test asserting the bytes are non-empty and `Parse`-able would be redundant with `internal/loomrecipe`'s own suite (batch 2), which parses and builds the same bytes for real.

## Cards

### Card 1: Copy the proven fixture into `contracts/recipes/loom-recipe.yaml`

- **Context:**
  - `internal/shedbuild/testdata/loom-recipe.yaml`
  - `internal/shedbuild/equivalence_test.go`
  - `internal/loomshed/loomshed.go`
- **Edits:** none
- **Creates:**
  - `contracts/recipes/loom-recipe.yaml`
- **Deletes:** none
- **Moves:** none
- **Requirements:** First run `go test ./internal/shedbuild/ -run TestLoomEquivalence -count=1` and confirm it passes against the current tree.
  If it fails, stop and report — the fixture has drifted from `New`'s literal in `internal/loomshed/loomshed.go` and copying it would carry the drift into production.
  Then create `contracts/recipes/loom-recipe.yaml` as a copy of `internal/shedbuild/testdata/loom-recipe.yaml` with only its five-line "test fixture only" header comment replaced by a production header.
  Carry over verbatim: `version: 1`, `entry: Preflight`, `terminals: [Finalize]`, the thirteen rows in their existing order, every `on_done`/`on_stuck` value, no `segment` key on any row, no `max_bounces` key on any row, no `config` block on any row, and the explicit `on_done: ""` on the `Finalize` row together with its two-line load-bearing comment.
  The production header must state that this file is loom's producer graph, that it is embedded into the binary by the sibling `recipes.go` rather than read from disk, and that the thirteen row names are the durable on-disk identities pinned against `internal/loomshed`'s `Name*` constants by the coverage guard.
  Fold in the routing rationale worth preserving from `New`'s own doc comment in `internal/loomshed/loomshed.go`: that every gate and validator bounces back to the producer whose artifact it guards, and that a gate whose guarded artifact is produced by no row in the list escalates instead (`Preflight`, `Loom-Preflight`, `Batchifier`, `Publish`, `Finalize`), which is why those five rows carry no `on_stuck`.
  Do not add a `Plan-Sweep` row.
- **Commit:** `feat(recipes): add loom's producer graph as a shipped recipe file`

### Card 2: The `contracts/recipes` embed package

- **Context:**
  - `contracts/stencils/stencils.go`
  - `contracts/recipes/loom-recipe.yaml`
- **Edits:** none
- **Creates:**
  - `contracts/recipes/recipes.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `contracts/recipes/recipes.go` declaring `package recipes`, importing `_ "embed"` only, and exporting `var LoomRecipe []byte` behind a `//go:embed loom-recipe.yaml` directive.
  Give the file a header comment in the same shape `contracts/stencils/stencils.go` uses: state that `//go:embed` reaches only files at or below its own directory, so every recipe's shipped-default byte var and its directive live in this one file.
  Do not add a registry type, a `Names`/`Default` pair, or a `stencilstore.Registry` implementation — `contracts/stencils`' registry exists for stencil-vs-disk seeding completeness, which a single embedded recipe with no on-disk location does not need.
  Do not import `internal/stencilstore`.
- **Commit:** `feat(recipes): embed loom-recipe.yaml as a shipped default`

### Card 3: `internal/loomrecipe` — `New`, `ShedPaths`, and the package doc

- **Context:**
  - `contracts/recipes/recipes.go`
  - `internal/shedbuild/parse.go`
  - `internal/shedbuild/build.go`
  - `internal/shedbuild/check.go`
  - `internal/shedbuild/recipe.go`
  - `internal/shedbuild/doc.go`
  - `internal/shedrecipe/env.go`
  - `internal/shedrecipe/entries_simple.go`
  - `internal/shedengine/shed.go`
  - `internal/loomshed/loomshed.go`
  - `internal/shedcheck/doc.go`
- **Edits:** none
- **Creates:**
  - `internal/loomrecipe/doc.go`
  - `internal/loomrecipe/loomrecipe.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `internal/loomrecipe/loomrecipe.go` declaring `package loomrecipe` with exactly two exported symbols.

  `ShedPaths` is a struct carrying the four told values `shedengine.Shed` itself reads and no registry entry does: `StatusPath string`, `LockPath string`, `StatusLockPath string`, `MaxBounces int`.
  Its field docs mirror `shedengine.Shed`'s own — in particular that `MaxBounces` of 0 means "use the internal default", never "no bounces allowed", and that the budget it seeds is per-producer and episode-scoped.
  Document why these four cannot travel in `shedrecipe.Env`: `Env` holds roots and run-wide values the registry *entries* read, and no entry reads `LockPath`.
  Also document that `StatusPath` and `StatusLockPath` are deliberately told twice — once in `Env` (for `loomPreflightEntry`) and once here (for `Shed`) — and that the duplication is inherent to the split and must not be collapsed.

  `New(env shedrecipe.Env, paths ShedPaths) (*shedengine.Shed, error)` parses `recipes.LoomRecipe` via `shedbuild.Parse`, builds it via `shedbuild.Build(recipe, env)`, and returns a `*shedengine.Shed` carrying the built `[]shedengine.ProducerDef` plus `paths`' four fields.
  It must use `shedbuild.Parse` on the embedded bytes, never `shedbuild.Load` — there is no on-disk runtime location for this recipe.
  It must surface both the parse error and the build error rather than swallowing either, wrapping each with a `loomrecipe: ` prefix and nothing more: `shedbuild` already names the offending row's zero-based index and `name` in every error it raises after decode, and the decoder keeps yaml line numbers, so no further position work is needed.
  It must not call `shedbuild.Check` — `internal/shedcheck/doc.go` and `internal/shedbuild/check.go` both state that `Check` is authoring-time only, because a resumed run legitimately starts mid-graph and reachability-from-entry is the wrong production question.
  `New` performs no nil-guard of its own on any `Env` field: each registry entry validates exactly the fields it reads, and `preflightEntry`'s `requireAbsRoot("Preflight", "Cwd", …)` is what now covers the guard `loomshed.New`'s nil-`Preflight` check used to.

  Create `internal/loomrecipe/doc.go` with the package doc: `internal/loomrecipe` owns loom's recipe-backed producer-list construction and is the drop-in replacement for `loomshed.New`;
  `internal/loomcli` is its only production caller;
  it takes every absolute path from its caller and has no direct production import of `internal/lyxcwd`, per the Told-Geometry Invariant.
  State why the package sits above `internal/loomshed` rather than inside it: `internal/shedrecipe`'s registry already imports `loomshed` for six of its constructors, so a `loomshed` → `shedbuild` → `shedrecipe` → `loomshed` production import cycle would not compile.
- **Commit:** `feat(loomrecipe): build loom's Shed from the embedded recipe`

### Card 4: Told-Geometry seam enforcement for `internal/loomrecipe`

- **Context:**
  - `internal/loomshed/seam_enforcement_test.go`
  - `internal/shedrecipe/seam_enforcement_test.go`
  - `internal/loomrecipe/loomrecipe.go`
  - `internal/loomrecipe/doc.go`
  - `CONSTRAINTS.md`
- **Edits:** none
- **Creates:**
  - `internal/loomrecipe/seam_enforcement_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `internal/loomrecipe/seam_enforcement_test.go` in `package loomrecipe`, modelled on `internal/shedrecipe/seam_enforcement_test.go`: a `loomrecipeAllowedImports` membership map, a `loomrecipeDeniedLyxcwdImport` constant holding `github.com/Knatte18/loomyard/internal/lyxcwd`, and a `TestToldGeometryInvariant_AllowlistOnly` walking every non-`_test.go` `.go` file in the package directory with `parser.ParseFile(..., parser.ImportsOnly)`, failing on any non-stdlib import outside the allowlist and separately naming the denied `lyxcwd` import if found.
  The allowlist holds exactly the package's production imports and no more: `github.com/Knatte18/loomyard/contracts/recipes`, `github.com/Knatte18/loomyard/internal/shedbuild`, `github.com/Knatte18/loomyard/internal/shedrecipe`, and `github.com/Knatte18/loomyard/internal/shedengine`.
  Verify that set by grepping the package's own production files rather than trusting this list — a membership allowlist that over-permits is the failure mode the test exists to prevent.
  Note in the file header that `contracts/recipes` sits outside `internal/` and so must be allowlisted explicitly, since the stdlib test is "the first path segment contains no dot", which a full module path never satisfies.
  Use the standard `TestToldGeometryInvariant_AllowlistOnly` name — `CONSTRAINTS.md`'s Machine-enforced list is keyed off it, and batch 6 adds this package to that list.
- **Commit:** `test(loomrecipe): enforce the Told-Geometry import allowlist`

## Batch Tests

`verify: go test ./contracts/... ./internal/loomrecipe/...` compiles both new packages and runs `internal/loomrecipe/seam_enforcement_test.go`, the only test this batch adds.
`./contracts/...` has no test files and reports `[no test files]` at exit 0 while still compiling the embed directive — a broken `//go:embed` path fails there rather than surfacing later.
The recipe's own parse-and-build correctness is not asserted in this batch;
that is batch 2's `internal/loomrecipe/recipe_test.go`, which is where the assertion loop that `internal/shedbuild/equivalence_test.go` currently carries lands.
The module-wide `go build ./...` at the batch boundary catches any cross-package compile fallout, of which there should be none — this batch only adds files.
