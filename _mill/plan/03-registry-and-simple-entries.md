# Batch: registry and value-only entries

```yaml
task: 'Shed recipe: engine registry'
batch: 'registry and value-only entries'
number: 3
cards: 4
verify: go test ./internal/shedrecipe/... ./internal/loomshed/...
depends-on: [1, 2]
```

## Batch Scope

This batch delivers the registry's public surface — `Lookup` and `Names` over the single central `map[string]Constructor` literal — together with the nine entries that take an empty `Config` and read only `Env`.
It is one batch because a registry with no entries would be an empty map nobody can test and an entry with no registry would be an unreachable function;
the nine value-only entries are also a single reviewable unit, each three to eight lines of `Env` validation plus one constructor call.
The three entries with real logic of their own (`SingleLLM`, `Bouncer`, `BurlerRound`) are deliberately left to batches 4 and 5, which each add their own key to the same map literal.
The external interface batches 4-6 consume is `Lookup`, `Names`, the `registry` map literal in `registry.go`, and the shared test scaffolding in `fixture_test.go`.

Batch-local decisions beyond `## Shared Decisions`:

- The nine entries live in one file, `entries_simple.go`, rather than one file per entry: each is small enough that nine separate files would obscure how uniform they are.
- Fakes for `websterengine.RunDeps`' four required seams are written by embedding the seam interface in an empty struct (`type fakeShuttleEngine struct{ shuttleengine.Engine }`), which yields a non-nil value satisfying the interface without implementing a single method.
  The `Webster` entry only checks these for non-nil, so a fake that would panic if called is exactly right and keeps the fixture small.

## Cards

### Card 7: The nine value-only entries

- **Context:**
  - `internal/shedrecipe/recipe.go`
  - `internal/shedrecipe/config.go`
  - `internal/shedrecipe/env.go`
  - `internal/preflightshed/preflight.go`
  - `internal/landingshed/publish.go`
  - `internal/landingshed/finalize.go`
  - `internal/landingshed/deps.go`
  - `internal/loomshed/loompreflight.go`
  - `internal/loomshed/batchifier.go`
  - `internal/loomshed/discussionvalidate.go`
  - `internal/loomshed/planvalidate.go`
  - `internal/loomshed/stub.go`
  - `internal/loomshed/webster.go`
  - `internal/websterengine/runlevel.go`
- **Edits:** none
- **Creates:**
  - `internal/shedrecipe/entries_simple.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Write nine unexported functions, each matching the `Constructor` signature exactly:
  `preflightEntry`, `publishEntry`, `finalizeEntry`, `loomPreflightEntry`, `batchifierEntry`, `discussionValidateEntry`, `planValidateEntry`, `stubEntry`, `websterEntry`.

  Every one of the nine takes an empty `Config`: each begins by calling `configRejectUnknown(cfg)` with no known key names and returns that error if non-nil, so a recipe author who puts a key on one of these rows fails loud.
  Every one validates only the `Env` fields it reads, using `requireAbsRoot` and `requireSeam` from `env.go`, and returns the first failure.

  The nine bodies:
  - `preflightEntry` validates `Env.Cwd` and returns `preflightshed.NewPreflight(name, env.Cwd)` with a nil error.
  - `publishEntry` returns `landingshed.NewPublish(env.Landing)`, validating no `Env` field: `NewPublish` already rejects the nil closures in `landingshed.Deps` itself, so the entry inherits the check.
    Its godoc states that `name` is deliberately discarded because `landingshed.Deps` carries no name field and `Publish`'s identity is the package constant `publishName`, and that the coverage-guard test in `coverage_guard_test.go` is what pins the row's name to match.
  - `finalizeEntry` is `publishEntry`'s twin over `landingshed.NewFinalize`, with the same godoc note.
  - `loomPreflightEntry` validates `Env.StatusPath` and `Env.StatusLockPath` and returns `loomshed.NewLoomPreflight(name, env.StatusPath, env.StatusLockPath)`.
  - `batchifierEntry` validates `Env.AnchorPath` and returns `loomshed.NewBatchifier(name, env.AnchorPath)`.
  - `discussionValidateEntry` validates `Env.DecisionRecordPath` and `Env.SupportLogPath` and returns `loomshed.NewDiscussionValidate(name, env.DecisionRecordPath, env.SupportLogPath)`.
  - `planValidateEntry` validates `Env.AnchorPath` and `Env.WorktreeRoot` and returns `loomshed.NewPlanValidate(name, env.AnchorPath, env.WorktreeRoot)`.
  - `stubEntry` validates no `Env` field and returns `loomshed.NewStub(name)`.
  - `websterEntry` validates `Env.AnchorPath`, then `Env.WebsterRun` via `requireSeam`, then exactly four inner fields of `Env.WebsterDeps` via `requireSeam` — `Starter`, `Reed`, `Engine`, `RefMatcher` — and returns `loomshed.NewWebsterProducer(name, env.AnchorPath, env.WebsterRun, env.WebsterDeps)`.
    It checks none of `WebsterDeps`' other nil-able fields, and its godoc says so field by field with the reason for each: `Batcher` is overwritten by the wrapper on every `Call` and its own field doc says the caller leaves it nil;
    a nil `Clock` selects the production clock by design;
    a nil `OpenBisector` is a legitimate mode meaning "no fabric in this mode", not a missing value;
    and `ShuttleCfg`, `Roles`, `Config`, and `Geom` are value and map types whose validation belongs to `websterengine.Run`, not to a wiring layer.

  Do not add a `Config` key to any of these nine entries, and do not call `resolveUnderRoot` anywhere in this file — none of the nine takes a path from `Config`.
- **Commit:** `feat(shedrecipe): add the nine value-only registry entries`

### Card 8: The registry table, Lookup, and Names

- **Context:**
  - `internal/shedrecipe/recipe.go`
  - `internal/shedrecipe/entries_simple.go`
  - `internal/batcher/registry.go`
- **Edits:** none
- **Creates:**
  - `internal/shedrecipe/registry.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Declare one package-level `var registry = map[string]Constructor{...}` literal with the nine keys this batch ships: `"Preflight"`, `"Publish"`, `"Finalize"`, `"LoomPreflight"`, `"Batchifier"`, `"DiscussionValidate"`, `"PlanValidate"`, `"Stub"`, `"Webster"`, each mapped to its `entries_simple.go` function.
  The map's own comment states that it is the single place every engine name is declared, that batches 4 and 5 add `"SingleLLM"`, `"Bouncer"`, and `"BurlerRound"` to this same literal, and that `init()` self-registration was rejected because the entries span four packages and registration would then depend on link-time blank imports.

  `Lookup(name string) (Constructor, error)` returns the registered constructor, and otherwise an error naming the unknown string in the shape `internal/batcher`'s `Select` uses.
  An empty `name` is an error too, with its own distinct message, not a default: the godoc must state that this is a deliberate departure from `batcher.Select`, which defaults to `DefaultName`, because there is no sensible default engine.

  `Names() []string` returns every registered name sorted with `sort.Strings`, in a freshly allocated slice each call, so a caller mutating the result cannot affect a later call or the map.
  Its godoc states that it exists so the coverage guard and the future recipe loader both have a stable enumeration and neither reaches into `registry` directly.
  Declare no other exported identifier in this file.
- **Commit:** `feat(shedrecipe): add the central registry table with Lookup and Names`

### Card 9: Shared test scaffolding

- **Context:**
  - `internal/shedrecipe/recipe.go`
  - `internal/shedadapters/singlellm.go`
  - `internal/shedadapters/burler.go`
  - `internal/shedadapters/webster.go`
  - `internal/websterengine/runlevel.go`
  - `internal/websterengine/audit.go`
  - `internal/shuttleengine/reed.go`
  - `internal/shuttleengine/engine.go`
  - `internal/landingshed/deps.go`
  - `internal/loomshed/fixture_test.go`
- **Edits:** none
- **Creates:**
  - `internal/shedrecipe/fixture_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Write the package-internal test scaffolding every later test file in this package reuses.

  `newTestEnv(t *testing.T) Env` builds an `Env` whose every path field is an absolute path derived from a single `t.TempDir()` — one subdirectory per field, created with `os.MkdirAll` where the field names a directory (`Cwd`, `WorktreeRoot`, `StencilsDir`, `RunRoot`) and left as a joined path where the field names a file (`StatusPath`, `StatusLockPath`, `DecisionRecordPath`, `SupportLogPath`);
  `AnchorPath` is a created directory.
  It fills `Shuttle`, `Burler`, and `WebsterRun` with the fakes below, fills `WebsterDeps` with the four required seams non-nil and every other field left zero, leaves `Landing` zero, and leaves `Now` nil.
  Its doc comment states the rule from `## Shared Decisions`: no test in this package may reference a path outside its own `t.TempDir()`, because a real repo path would mask a told-geometry violation.

  The fakes, all unexported:
  `fakeShuttle` implements `shedadapters.Shuttle` by returning a caller-settable `shuttleengine.Result` and error, recording the `shuttleengine.Spec` it was handed so a later test can assert on the composed spec;
  `fakeBurlerRunner` implements `shedadapters.BurlerRunner` by returning a zero `burlerengine.Result` and a nil error, recording the `burlerengine.Profile` and `burlerengine.RunOpts` it was handed;
  `fakeWebsterRun` is a `shedadapters.WebsterRunner` func value returning a zero `websterengine.RunResult` and a nil error.
  The four `websterengine.RunDeps` seams are satisfied by empty structs embedding their interface — `fakeMasterStarter struct{ websterengine.MasterStarter }`, `fakeReedOps struct{ shuttleengine.ReedOps }`, `fakeShuttleEngine struct{ shuttleengine.Engine }`, `fakeRefMatcher struct{ websterengine.RefMatcher }` — with a comment stating that these are non-nil placeholders for a non-nil check only and would panic if any method were called, which no test in this package does.
- **Commit:** `test(shedrecipe): add shared Env and seam test scaffolding`

### Card 10: Tests for Lookup, Names, and the nine value-only entries

- **Context:**
  - `internal/shedrecipe/registry.go`
  - `internal/shedrecipe/entries_simple.go`
  - `internal/shedrecipe/fixture_test.go`
  - `internal/shedrecipe/recipe.go`
  - `internal/shedengine/producer.go`
  - `internal/websterengine/runlevel.go`
- **Edits:** none
- **Creates:**
  - `internal/shedrecipe/registry_test.go`
  - `internal/shedrecipe/entries_simple_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  `registry_test.go` covers `Lookup` and `Names`.
  For `Lookup`: every name `Names()` returns resolves to a non-nil `Constructor`;
  an unknown name returns an error whose message contains the offending string;
  an empty name returns an error, asserted to be a distinct message from the unknown-name one rather than a silent default.
  For `Names`: the returned slice is sorted;
  it contains exactly the keys of `registry`;
  and mutating the returned slice's first element does not change what a second `Names()` call returns.
  Do not hardcode the expected count in this batch — the count assertion belongs to `coverage_guard_test.go` in batch 6, which asserts the full set of twelve.

  `entries_simple_test.go` is table-driven over the nine entries.
  The happy-path table supplies `newTestEnv(t)` and an empty `Config` per entry and asserts a non-nil `shedengine.ShedProducer` with a nil error.
  A second table asserts each entry rejects an unrecognised `Config` key, with the error naming that key.
  A third table is the under-filled-`Env` case: per entry, one subtest per `Env` field that entry reads, each blanking exactly that field on a copy of `newTestEnv(t)` and asserting construction fails with an error naming the field, plus one subtest per entry blanking a field that entry does not read and asserting construction still succeeds.
  `websterEntry` needs its own subtests beyond the table: a `WebsterDeps` with each of `Starter`, `Reed`, `Engine`, and `RefMatcher` nil in turn fails naming that field;
  a `WebsterDeps` with `Batcher`, `Clock`, and `OpenBisector` all nil constructs successfully.
  `publishEntry` and `finalizeEntry` need a failure-path subtest each: an `Env` whose `Landing` is the zero `landingshed.Deps` makes the underlying constructor reject, so the entry must surface that error rather than a nil producer with a nil error.
  Add one subtest asserting a relative (non-absolute) value in a read `Env` path field fails, using `batchifierEntry` and `Env.AnchorPath` as the representative case.
- **Commit:** `test(shedrecipe): cover Lookup, Names, and the nine value-only entries`

## Batch Tests

`verify: go test ./internal/shedrecipe/... ./internal/loomshed/...` covers both packages this batch's correctness depends on.
`internal/shedrecipe` is where every new file lands.
`internal/loomshed` is included because this is the first batch that calls batch 1's six newly exported constructors from outside the package, so a signature or return-type mistake in batch 1 surfaces here rather than at the end of the plan;
the command is still batch-scoped, not a repo-wide suite.
`registry_test.go` and `entries_simple_test.go` together exercise every line of `registry.go` and `entries_simple.go`.
