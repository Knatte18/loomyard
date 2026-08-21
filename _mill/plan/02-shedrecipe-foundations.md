# Batch: shedrecipe foundations

```yaml
task: 'Shed recipe: engine registry'
batch: 'shedrecipe foundations'
number: 2
cards: 5
verify: go test ./internal/shedrecipe/...
depends-on: []
```

## Batch Scope

This batch creates the `internal/shedrecipe` package and everything every registry entry stands on: the three public types (`Constructor`, `Config`, `Env`), the typed `Config` accessors plus the unknown-key rejector, the `Env` field validators, and the relative-`Config`-path resolver.
It is one batch because these four files are a single vocabulary — an accessor's error shape, the rejector's recognised-key contract, and the path resolver's rejection rules are decided together or not at all — and because no entry can be written until all of them exist.
It deliberately ships no registry and no entry, so the package at the end of this batch compiles and tests green with a public surface of exactly three types.
The external interface batches 3-5 consume is: `Constructor`, `Config`, `Env`, and the unexported helpers `configString`, `configStringSlice`, `configBool`, `configInt`, `configStringMap`, `configMap`, `configRejectUnknown`, `requireAbsRoot`, `requireSeam`, and `resolveUnderRoot`.

Batch-local decisions beyond `## Shared Decisions`:

- The accessors take a required/optional flag rather than shipping two functions per type, so the empty-is-absent rule is implemented once instead of six times.
- `Env` is declared here rather than alongside the first entry that reads it, because four different batches read disjoint subsets of it and a shared declaration is the only thing that keeps them consistent.

## Cards

### Card 2: Create the package doc

- **Context:**
  - `internal/loomshed/doc.go`
  - `internal/landingshed/doc.go`
  - `internal/shedadapters/doc.go`
  - `manifest/designs/shed-recipe.md`
  - `CONSTRAINTS.md`
- **Edits:** none
- **Creates:**
  - `internal/shedrecipe/doc.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Write `package shedrecipe`'s doc comment, in the same shape the three sibling `doc.go` files use.
  It must state: that the package owns the engine registry — the name to `shedengine.ShedProducer`-constructor mapping a future recipe loader resolves each row's `Engine` field against;
  that it is imported by nobody in the four producer-hosting packages it imports, so the dependency runs one way only;
  and that it holds the Told-Geometry Invariant in the precise form `CONSTRAINTS.md` pins, quoted verbatim — every root is told and none is derived, and the package's only path construction is joining a told root with a recipe-relative value.
  Name the `Env` struct as the told bundle and `Config` as the portable per-row half.
  The file contains the doc comment and the `package shedrecipe` clause only, no code.
- **Commit:** `docs(shedrecipe): add the package doc for the engine registry`

### Card 3: Declare Constructor, Config, and Env

- **Context:**
  - `internal/shedengine/producer.go`
  - `internal/shedadapters/singlellm.go`
  - `internal/shedadapters/burler.go`
  - `internal/shedadapters/webster.go`
  - `internal/websterengine/runlevel.go`
  - `internal/landingshed/deps.go`
  - `internal/shedrecipe/doc.go`
- **Edits:** none
- **Creates:**
  - `internal/shedrecipe/recipe.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Declare exactly three exported types.

  `type Constructor func(name string, cfg Config, env Env) (shedengine.ShedProducer, error)` — its godoc states that `name` is the recipe row's `Name` threaded into each producer's own name parameter, and that the two exceptions are `Publish` and `Finalize`, whose underlying constructors take no name at all.

  `type Config map[string]any` — its godoc states that the caller decodes the recipe file into it, that this package never learns the recipe file's format, that each entry extracts and validates only the keys it recognises, and that an unrecognised key is an error.

  `type Env struct` with exactly the fields, order, grouping, and per-field comments below.
  The first group is geometry — absolute paths and roots resolved by the caller — `Cwd`, `AnchorPath`, `WorktreeRoot`, `StatusPath`, `StatusLockPath`, `StencilsDir`, `RunRoot`, `DecisionRecordPath`, `SupportLogPath`, all `string`.
  The second group is injected seams and already-resolved values — `Shuttle shedadapters.Shuttle`, `Burler shedadapters.BurlerRunner`, `WebsterRun shedadapters.WebsterRunner`, `WebsterDeps websterengine.RunDeps`, `Landing landingshed.Deps`, `Now func() time.Time`.
  Each field's own comment names the entries that read it: `Cwd` -> `Preflight`;
  `AnchorPath` -> `Batchifier`, `PlanValidate`, `Webster`, and `SingleLLM`'s `anchor_path` token;
  `WorktreeRoot` -> `PlanValidate`, and the root every worktree-relative `Config` path resolves against;
  `StatusPath` and `StatusLockPath` -> `LoomPreflight`;
  `StencilsDir` -> `SingleLLM` and `Bouncer`;
  `RunRoot` -> the root every `Config` `run_subdir` resolves against, read by `Bouncer` and `BurlerRound`;
  `DecisionRecordPath` and `SupportLogPath` -> `DiscussionValidate`.

  The `Env` type's own godoc states the rule that makes the split decidable: `Env` holds roots and run-wide values only, never a value that differs between two rows, and anything per-row is a relative path or scalar in `Config` resolved against one of these roots by the entry.
  It also states that a caller fills only the fields its own recipe needs, because each entry validates only the fields it reads.
  `Now`'s own comment states that nil is legal and defaults to `time.Now` inside the underlying constructors.
  `Landing`'s own comment states it is a whole-struct passthrough handed to `landingshed.NewPublish`/`NewFinalize` unchanged, rather than flattened, because `landingshed.Deps` already carries fourteen fields told wholesale by `loomshed.Deps.Landing` today.
  Declare no functions in this file.
- **Commit:** `feat(shedrecipe): declare Constructor, Config, and Env`

### Card 4: Config accessors and the unknown-key rejector

- **Context:**
  - `internal/shedrecipe/recipe.go`
  - `internal/burlercli/run.go`
- **Edits:** none
- **Creates:**
  - `internal/shedrecipe/config.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Write six unexported typed accessors and one unexported rejector, all package-level functions, all returning an error whose text names the offending key.
  Each accessor takes `(cfg Config, key string, required bool)` and returns its typed value plus an error.

  - `configString` accepts a `string` value.
  - `configStringSlice` accepts both a `[]string` and a `[]any` whose every element is a `string`;
    an `[]any` carrying a non-string element is an error naming the key and the offending element's index.
  - `configBool` accepts a `bool` value.
  - `configInt` accepts an `int`, and a `float64` whose value is integral;
    a fractional `float64` is an error naming the key.
    The godoc states why both are accepted: a YAML decoder into `any` yields `int` while a JSON-shaped one yields `float64`, and piece 2 picks the format, so accepting both without truncating keeps this package format-agnostic.
  - `configStringMap` accepts a `map[string]any` whose every value is a `string`, returning a `map[string]string`;
    a non-string value is an error naming both the outer key and the offending inner key.
  - `configMap` accepts a `map[string]any` and returns it as a `Config`, so a nested map can be fed straight back through these same accessors.

  Absence rules, identical across all six: a key missing from `cfg`, or present with an empty-string value, or present with an empty slice or empty map, counts as absent.
  Absent and `required` is an error naming the key.
  Absent and not `required` returns the Go zero value with a nil error.
  A present value of the wrong Go type is an error naming the key, the expected shape, and the actual `%T`, whether or not the key is required.

  `configRejectUnknown(cfg Config, known ...string) error` returns an error naming the first unrecognised key in sorted order, so the message is deterministic across runs;
  a `cfg` with only recognised keys, and a nil or empty `cfg`, both return nil.
  Its godoc states that it runs at nested levels too, not only on the outer map — `BurlerRound`'s `profile`, `profile.target`, and `profile.fasit` each get their own call.
- **Commit:** `feat(shedrecipe): add typed Config accessors and the unknown-key rejector`

### Card 5: Env validators and the relative-path resolver

- **Context:**
  - `internal/shedrecipe/recipe.go`
  - `internal/shedadapters/bouncer.go`
  - `internal/shedadapters/singlellm.go`
  - `manifest/designs/shed-recipe.md`
- **Edits:** none
- **Creates:**
  - `internal/shedrecipe/env.go`
  - `internal/shedrecipe/paths.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In `internal/shedrecipe/env.go`, write two unexported helpers every entry uses to validate the `Env` fields it reads.
  `requireAbsRoot(entry, field, value string) error` errors when `value` is empty and when `filepath.IsAbs(value)` is false, naming the entry and the field in both messages.
  `requireSeam(entry, field string, seam any) error` errors when `seam` is nil, naming the entry and the field;
  it must detect a typed-nil interface value as nil too, since `Env.Shuttle` and `Env.Burler` are interfaces a caller can fill with a nil concrete pointer — use `reflect.ValueOf(seam)` and treat a `reflect.Ptr`, `reflect.Interface`, `reflect.Map`, `reflect.Slice`, or `reflect.Func` kind whose `IsNil()` reports true as nil.
  Both helpers' godoc states the rule from `## Shared Decisions`: an entry validates exactly the fields it consumes and never a field it does not read, so a caller filling only what its recipe needs is legal.

  In `internal/shedrecipe/paths.go`, write `resolveUnderRoot(entry, key, root, value string) (string, error)`.
  It errors when `value` is empty, when `filepath.IsAbs(value)` is true, and when the cleaned value escapes `root` — compute `joined := filepath.Join(root, value)` and reject unless `joined` equals `root` or is under `root` with a separator, so a `..` segment cannot climb out.
  Every message names the entry and the config key.
  On success it returns `joined`.
  Its godoc states why absolute values are rejected rather than passed through: `manifest/designs/shed-recipe.md` bars absolute paths from `Config` outright, and accepting one would make a non-portable recipe silently work on its author's machine.
  It also names the one documented exception that never calls this helper — `BurlerRound`'s `profile.target.paths` and `profile.fasit.paths`.
  Neither file declares an exported identifier.
- **Commit:** `feat(shedrecipe): add Env validators and the relative-path resolver`

### Card 6: Tests for the accessors, the rejector, the Env validators, and the path resolver

- **Context:**
  - `internal/shedrecipe/config.go`
  - `internal/shedrecipe/env.go`
  - `internal/shedrecipe/paths.go`
  - `internal/shedrecipe/recipe.go`
  - `internal/loomshed/batchifier_test.go`
- **Edits:** none
- **Creates:**
  - `internal/shedrecipe/config_test.go`
  - `internal/shedrecipe/env_test.go`
  - `internal/shedrecipe/paths_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Write table-driven tests in the `shedrecipe` package (internal tests, not `shedrecipe_test`), following `internal/loomshed/batchifier_test.go`'s subtest style.

  `config_test.go` covers, per accessor: a present correctly-typed value returns it;
  a present wrong-typed value errors and the message contains the key;
  an absent required key errors and the message contains the key;
  an absent optional key returns the zero value with a nil error;
  a present-but-empty-string required key errors identically to an absent one.
  Representation cases that need their own subtests: `configStringSlice` against a `[]string`, against an `[]any` of strings, and against an `[]any` with one non-string element;
  `configInt` against an `int`, against an integral `float64`, and against a fractional `float64` where the error message names the key;
  `configStringMap` against a `map[string]any` with a non-string value.
  For `configRejectUnknown`: a `Config` with one unrecognised key errors and the message names that key;
  a `Config` with two unrecognised keys names the sorted-first one, proving the message is deterministic;
  a `Config` with only recognised keys, an empty `Config`, and a nil `Config` all return nil.

  `env_test.go` covers `requireAbsRoot` against an empty value, a relative value, and an absolute value, asserting the entry name and field name appear in each error;
  and `requireSeam` against an untyped nil, against a nil `shedadapters.Shuttle` interface, against a non-nil value, and against a typed-nil concrete pointer stored in a `shedadapters.Shuttle` — the typed-nil case is the one that fails if the helper compares against `nil` directly instead of using reflection.

  `paths_test.go` covers `resolveUnderRoot` with a root from `t.TempDir()`: a plain relative value joins under the root;
  a nested relative value joins under the root;
  an empty value errors;
  an absolute value errors and the message names the key;
  a `..`-escaping value errors;
  a value that uses `..` but stays inside the root resolves successfully.
- **Commit:** `test(shedrecipe): cover the Config accessors, Env validators, and path resolver`

## Batch Tests

`verify: go test ./internal/shedrecipe/...` is correctly scoped: this batch creates one new package and touches nothing else in the repo, so the new package's own suite is the entire affected surface.
The three new test files cover every branch of the seven `config.go` functions, the two `env.go` helpers, and `resolveUnderRoot`, which is the whole of the batch's runnable surface — `doc.go` and `recipe.go` declare no executable code.
The typed-nil `requireSeam` case and the `..`-escape `resolveUnderRoot` case are the two that guard against the plausible-but-wrong implementations of their helpers.
