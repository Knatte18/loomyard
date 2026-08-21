# Batch: shedbuild-builder

```yaml
task: 'Shed recipe: loader/builder'
batch: shedbuild-builder
number: 2
cards: 6
verify: go test ./internal/shedbuild/...
depends-on: [1]
```

## Batch Scope

This batch delivers the assembly half of `internal/shedbuild`: `Build`, which resolves each row's engine through the shipped registry and calls the returned constructor, and `Check`, the two-line authoring-time forward to `internal/shedcheck`.
It is one batch because `Build` and `Check` are the only two remaining exported functions, they share one test fixture (the filled `shedrecipe.Env` and its fakes), and neither can be written without the `Recipe` shape batch 1 delivered.
The external interface batch 3 consumes is `Build`'s `([]shedengine.ProducerDef, error)` return plus the shared fixture file this batch creates.
One batch-local decision beyond the overview's `## Shared Decisions`: the twelve-engine coverage case lives in its own test file with its own filesystem-backed fixture, kept apart from the in-memory `Build` table so the pure cases stay pure — see `## Batch Tests`.

## Cards

### Card 6: `Build`

- **Context:**
  - `internal/shedrecipe/recipe.go`
  - `internal/shedrecipe/registry.go`
  - `internal/shedengine/producer.go`
  - `internal/shedbuild/recipe.go`
  - `internal/shedbuild/doc.go`
- **Edits:** none
- **Creates:**
  - `internal/shedbuild/build.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Create `internal/shedbuild/build.go` declaring exactly one exported function, `Build(r Recipe, env shedrecipe.Env) ([]shedengine.ProducerDef, error)`.
  `Build` returns `shedbuild: recipe has no producers` when `r.Producers` is empty — defence in depth, since `Parse` already rejects that but `Build` also accepts a hand-built `Recipe`.
  It then allocates a result slice of `len(r.Producers)` and walks the rows in list order, so build order matches recipe order.
  Per row at zero-based index `i`: call `shedrecipe.Lookup(row.Engine)` and, on error, return `fmt.Errorf("shedbuild: producer %d %q: %w", i, row.Name, err)`;
  call the returned constructor as `ctor(row.Name, shedrecipe.Config(row.Config), env)` and, on error, return the same wrap.
  The `shedrecipe.Config(row.Config)` conversion is a free type conversion, not a copy or a key walk, because both types have the identical underlying `map[string]any`.
  Assign the built row as `shedengine.ProducerDef{Name: row.Name, Producer: producer, OnDone: row.OnDone, OnStuck: row.OnStuck, Segment: row.Segment, MaxBounces: row.MaxBounces}`, copying all five data fields straight through with no defaulting of any kind.
  `Build` runs no reachability, cycle, blind-gate, dangling-target, or segment analysis, and never calls into `internal/shedcheck`: a legitimately resumable graph starts mid-graph, so reachability-from-entry is the wrong question at build time, and `shedengine`'s own validation already rejects dangling targets, duplicate names, and cross-segment `OnStuck` before a run touches anything.
  `Build`'s doc comment states plainly that it is not filesystem-free: it is a pass-through for the construction-time filesystem effects some registry constructors have of their own accord, and this package neither suppresses nor wraps them.
  It must not restate which constructor produces which effect — point at the package doc in `internal/shedbuild/doc.go`, which card 1 already makes the single site enumerating that, so a future change to those constructors' construction-time behaviour has one doc site to update rather than two.
- **Commit:** `feat(shedbuild): add Build`

### Card 7: the `Check` helper

- **Context:**
  - `internal/shedcheck/doc.go`
  - `internal/shedcheck/finding.go`
  - `internal/shedcheck/check.go`
  - `internal/shedbuild/recipe.go`
- **Edits:** none
- **Creates:**
  - `internal/shedbuild/check.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Create `internal/shedbuild/check.go` declaring exactly one exported function, `Check(r Recipe, producers []shedengine.ProducerDef) []shedcheck.Finding`, whose whole body is `return shedcheck.Check(producers, r.Entry, r.Terminals)`.
  Its doc comment states that nothing in production calls it: it exists for a caller's own test suite, at authoring time, before an assembled list ever reaches a shed, and it exists at all only so the caller need not re-derive the argument order and so `Recipe`'s `Entry` and `Terminals` have a documented consumer in the package that declares them.
  Say explicitly that `Build` deliberately does not call this, and why — the same reason `internal/shedcheck/doc.go` gives for keeping it out of every production constructor.
- **Commit:** `feat(shedbuild): add the Check authoring-time helper`

### Card 8: the shared test fixture

- **Context:**
  - `internal/shedrecipe/fixture_test.go`
  - `internal/shedrecipe/coverage_guard_test.go`
  - `internal/shedrecipe/entries_singlellm_test.go`
  - `internal/loomshed/fixture_test.go`
  - `internal/stencilstore/stencilstore.go`
  - `internal/shedrecipe/recipe.go`
- **Edits:** none
- **Creates:**
  - `internal/shedbuild/fixture_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Create `internal/shedbuild/fixture_test.go` in `package shedbuild`, holding every fake and builder the rest of the package's tests share.
  It is a fresh copy rather than an import, because the two files it is modelled on live in `package shedrecipe` and are not reachable from here.
  Declare `newTestEnv(t *testing.T) shedrecipe.Env`, modelled on the builder of the same name in `internal/shedrecipe/fixture_test.go`: one `t.TempDir()`, a local `mustMkdir` helper creating a subdirectory per directory-valued field, and joined-but-uncreated paths for the file-valued fields.
  Fill all nine told roots — `Cwd`, `AnchorPath`, `WorktreeRoot`, `StatusPath`, `StatusLockPath`, `StencilsDir`, `RunRoot`, `DecisionRecordPath`, `SupportLogPath` — plus `Shuttle`, `Burler`, `WebsterRun`, and the four required `WebsterDeps` seams `Starter`, `Reed`, `Engine`, and `RefMatcher`.
  Leave the other seven `WebsterDeps` fields zero and leave `Now` nil, matching the sibling exactly.
  Unlike the sibling, also fill `Env.Landing`, because two of the twelve engines need it.
  Declare that value's builder as its own reusable function, `testLandingDeps(dir string) landingshed.Deps`, which `newTestEnv` calls with its own temp dir and which batch 3's equivalence test calls again for its own paired fixture — do not inline the literal inside `newTestEnv`.
  Build it the way `coverageGuardLandingDeps` in `internal/shedrecipe/coverage_guard_test.go` does — told absolute paths derived from the passed-in dir, a non-nil push-branch closure, a typed-nil fabric-opener closure used for both openers, and a minimal merge-resolve shuttle fake — which is the same shape the function of the same name in `internal/loomshed/fixture_test.go` uses.
  Declare the fakes those fields need, each with a doc comment saying it is never called because no test in this package invokes a producer: a shuttle fake, a burler-runner fake, a webster-runner func value, the four webster seams as empty structs embedding their own seam interface (the sibling's trick, which yields a non-nil value satisfying the interface without implementing a method), a merge-resolve shuttle fake, and the fabric-opener function returning a typed-nil handle with a nil error.
  Declare `writeStencilFile(t *testing.T, dir, name, content string)`, copied from the helper of the same name in `internal/shedrecipe/entries_singlellm_test.go`: it must `os.MkdirAll` the parent of `stencilstore.Path(dir, name)` before writing, because `stencilstore.RelPath` splits a hyphenated stencil name on its first hyphen into a family subdirectory, so writing straight into a fresh temp directory fails with a missing-parent error.
  Every path this file produces lies inside its own `t.TempDir()`;
  no test in this package may reference a real repository path, since that would mask exactly the told-geometry violation this package's guard exists to catch.
- **Commit:** `test(shedbuild): add the shared Env fixture and fakes`

### Card 9: `Build` table tests

- **Context:**
  - `internal/shedbuild/build.go`
  - `internal/shedbuild/recipe.go`
  - `internal/shedbuild/fixture_test.go`
  - `internal/shedrecipe/entries_simple.go`
  - `internal/shedrecipe/entries_singlellm.go`
- **Edits:** none
- **Creates:**
  - `internal/shedbuild/build_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Create `internal/shedbuild/build_test.go` in `package shedbuild`, table-driven over `Recipe` values constructed in Go rather than parsed, so a `Build` failure can never be a `Parse` failure in disguise.
  Cover, as separate cases: a single `Stub` row building to one producer definition with `Name`, `OnDone`, `OnStuck`, `Segment`, and `MaxBounces` copied through and a non-nil producer;
  an unknown engine name erroring with the row index, the row name, and the offending engine all present in the message;
  a `Preflight` row built against an `Env` whose `Cwd` is empty, surfacing that constructor's own error wrapped with the row index and name;
  a `SingleLLM` row missing its required stencil key, surfacing that constructor's own error the same way;
  a non-empty `config` block on one of the nine engines that accept no config keys, erroring — which is what proves the block is forwarded to the constructor rather than dropped;
  a multi-row recipe of `Stub` rows asserting build order matches recipe order by name;
  and a `Recipe` whose `Producers` slice is empty, erroring.
  Every case in this file uses either a zero `shedrecipe.Env` or `newTestEnv(t)` and touches no file of its own — the filesystem-backed coverage lives in the separate file card 10 creates.
- **Commit:** `test(shedbuild): cover Build routing and constructor errors`

### Card 10: every registered engine is buildable

- **Context:**
  - `internal/shedbuild/build.go`
  - `internal/shedbuild/recipe.go`
  - `internal/shedbuild/fixture_test.go`
  - `internal/shedrecipe/registry.go`
  - `internal/shedrecipe/entries_bouncer.go`
  - `internal/shedrecipe/entries_burler.go`
  - `internal/shedrecipe/entries_singlellm.go`
  - `internal/shedrecipe/entries_burler_test.go`
- **Edits:** none
- **Creates:**
  - `internal/shedbuild/build_engines_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Create `internal/shedbuild/build_engines_test.go` in `package shedbuild`, holding one test that asserts every name `shedrecipe.Names()` returns is buildable from a recipe given a sufficiently filled environment.
  Drive the assertion off `shedrecipe.Names()` rather than a local list, so a thirteenth registered engine fails this test until the fixture covers it.
  Build the environment as `newTestEnv(t)` and then seed the two stencil files the fixture needs with `writeStencilFile`, using arbitrary marker-free content: the plain files those two engines eagerly read at construction, no stamp banner and no reconcile pass involved.
  Hold a local map from engine name to that engine's minimal config, covering only the three engines that need one — the single-LLM engine needs a `stencil` naming a written stencil plus an `output_files` list of worktree-relative paths;
  the bouncer engine needs a `run_subdir`, an `artifact_paths` list of worktree-relative paths, and a `rubric_stencil` naming the other written stencil;
  the burler-round engine needs a `run_subdir` and a `profile` map carrying at least a non-empty `rubric`, which is the minimal shape `internal/shedrecipe/entries_burler_test.go` already uses.
  The other nine engines take no config at all and must be given none, since a non-empty config block on any of them is an error from the constructor.
  Assert per engine that `Build` returns a nil error and one definition with a non-nil producer, failing with the engine's name so a regression names the offending engine directly.
  This test is deliberately not filesystem-free, and its doc comment must say why: the bouncer and burler-round engines create the run directory they resolve, and the bouncer and single-LLM engines eagerly read their configured stencil.
  Restricting the assertion to the nine effect-free engines was rejected — it would drop exactly the three engines whose config shapes are the most complex, which is the coverage most worth having.
- **Commit:** `test(shedbuild): assert every registered engine builds from a recipe`

### Card 11: `Check` helper tests

- **Context:**
  - `internal/shedbuild/check.go`
  - `internal/shedbuild/build.go`
  - `internal/shedbuild/recipe.go`
  - `internal/shedbuild/fixture_test.go`
  - `internal/shedcheck/finding.go`
- **Edits:** none
- **Creates:**
  - `internal/shedbuild/check_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Create `internal/shedbuild/check_test.go` in `package shedbuild`, holding exactly two cases and no more.
  The first builds a small clean graph of stub rows whose told entry and terminal name real rows, and asserts `Check` returns no findings — asserting the length is zero rather than comparing against an empty non-nil slice, because a clean graph yields a nil slice.
  The second builds the same graph with one row's `on_done` retargeted at a name no row carries, and asserts the returned findings contain the dangling-target kind for that row.
  The file's header comment states that `internal/shedcheck`'s own behaviour is exhaustively tested in its own package and is deliberately not re-tested here — these two cases pin only that this package forwards the recipe's told entry and terminals in the right argument positions.
- **Commit:** `test(shedbuild): cover the Check forwarding helper`

## Batch Tests

`verify: go test ./internal/shedbuild/...` runs the whole package, which after this batch adds `build_test.go`, `build_engines_test.go`, and `check_test.go` to batch 1's three files, all sharing `fixture_test.go`.
The scope is exactly the batch's own `Creates:` set — no file outside `internal/shedbuild/` is touched.
This is a Go project, so the command uses the native runner directly with no `PYTHONPATH=` prefix.
The twelve-engine case is a separate file from the `Build` table on purpose: it is the only test in the package that needs a writable run root and real on-disk stencil files, and keeping it apart means the table's cases stay pure in-memory ones that cannot fail for a fixture-shaped reason.
