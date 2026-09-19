# Batch: shedbuild-shedpaths-hoist

```yaml
task: "Shed-generic watchdog for ly-drive and loom's CLI verbs"
batch: "shedbuild-shedpaths-hoist"
number: 1
cards: 5
verify: go test ./internal/shedbuild/... ./internal/loomrecipe/... ./internal/lifecyclerecipe/... ./internal/loomcli/... ./internal/lifecyclecli/...
depends-on: []
```

## Batch Scope

This batch removes the first of the two duplications the task exists to remove: `internal/loomrecipe` and `internal/lifecyclerecipe` each declare a field-identical five-field `ShedPaths` struct and a near-identical eight-line `New` body.
Both move into `internal/shedbuild`, which already owns `Parse` and `Build` and already imports `shedengine` and `shedrecipe`, as a hoisted `ShedPaths` type plus a `NewShed(recipe []byte, env, paths)` assembler.
The two `New` functions shrink to their own package-specific guards plus a delegation, and every caller of the removed `loomrecipe.ShedPaths`/`lifecyclerecipe.ShedPaths` types switches to `shedbuild.ShedPaths`.

The external interface batch 4 consumes is `shedbuild.ShedPaths` and `shedbuild.NewShed`: `loomcli.Arm` and `lifecyclecli.Arm` both fill a `shedbuild.ShedPaths` and both hand a `BuildShed` closure to `shedverbs`.

Batch-local decision, differing from nothing in the overview but worth stating: `loomrecipe.New` and `lifecyclerecipe.New` keep their existing two-argument signature *shape* and their own `"loomrecipe: "`/`"lifecyclerecipe: "` error prefixes, so neither package's existing error-text assertions change.
`NewShed` takes the recipe bytes as its first argument rather than reading an embedded var of its own, because `contracts/recipes`' `//go:embed` vars are named per-recipe and `shedbuild` must stay recipe-agnostic.

## Cards

### Card 1: hoist ShedPaths and NewShed into shedbuild

- **Context:**
  - `internal/shedbuild/build.go`
  - `internal/shedbuild/parse.go`
  - `internal/shedbuild/recipe.go`
  - `internal/shedbuild/doc.go`
  - `internal/shedengine/shed.go`
  - `internal/loomrecipe/loomrecipe.go`
  - `internal/lifecyclerecipe/lifecyclerecipe.go`
- **Edits:** none
- **Creates:**
  - `internal/shedbuild/newshed.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** declare `type ShedPaths struct` in the new file with exactly the five fields the two removed copies carry, in the same order and with the same types: `StatusPath string`, `LockPath string`, `StatusLockPath string`, `MaxBounces int`, `CommitStatus func(producer, state string) error`.
  Carry each field's existing doc comment across verbatim from `internal/loomrecipe/loomrecipe.go`'s copy, minus the loom-specific "StatusPath and StatusLockPath are deliberately told twice" paragraph, which belongs to `loomrecipe.New`'s own guard and is restated on that function in card 2.
  Declare `func NewShed(recipe []byte, env shedrecipe.Env, paths ShedPaths) (*shedengine.Shed, error)`: it calls `shedbuild.Parse(recipe)`, then `shedbuild.Build(parsed, env)`, and returns `&shedengine.Shed{Producers: producers, StatusPath: paths.StatusPath, LockPath: paths.LockPath, StatusLockPath: paths.StatusLockPath, MaxBounces: paths.MaxBounces, CommitStatus: paths.CommitStatus}`.
  Return the parse error and the build error **unwrapped**, adding no prefix of its own.
  This is the single-prefix rule for the hoist: the two delegating `New` bodies keep their existing `"loomrecipe: "`/`"lifecyclerecipe: "` wrap, so a `NewShed` prefix on top would turn today's `loomrecipe: <err>` on `lyx loom run`'s error envelope into `loomrecipe: shedbuild: <err>` — a user-visible rewording of a shipped envelope.
  `shedbuild` already names the offending row's zero-based index and name in every error it raises after decode, and the decoder keeps yaml line numbers, so the unwrapped error is self-locating without a prefix.
  `NewShed` never calls `shedbuild.Check` — state in its doc comment that `Check` is authoring-time only, because a resumed run legitimately starts mid-graph, carrying the reason the two removed bodies already record.
  `NewShed` performs no nil-guard or absolute-path check of its own on any `Env` field: each registry entry validates the fields it reads.
- **Commit:** `feat(shedbuild): hoist ShedPaths and add the NewShed assembler`

### Card 2: shrink loomrecipe.New to guard-plus-delegation

- **Context:**
  - `internal/shedbuild/newshed.go`
  - `internal/shedrecipe/recipe.go`
  - `contracts/recipes/recipes.go`
- **Edits:**
  - `internal/loomrecipe/loomrecipe.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** remove the `ShedPaths` type declaration from this file entirely and change `New`'s signature to `func New(env shedrecipe.Env, paths shedbuild.ShedPaths) (*shedengine.Shed, error)`.
  Keep both coherence checks exactly as they are, as `New`'s first act, ahead of the delegation, with their error texts unchanged byte-for-byte: `fmt.Errorf("loomrecipe: env.StatusPath %q != paths.StatusPath %q", env.StatusPath, paths.StatusPath)` and `fmt.Errorf("loomrecipe: env.StatusLockPath %q != paths.StatusLockPath %q", env.StatusLockPath, paths.StatusLockPath)`.
  These do not hoist — `_mill/discussion.md`'s `loomrecipe-keeps-its-coherence-guard` Decision settles that the guard exists for `loomPreflightEntry`'s own `Env.StatusPath` read, which is loom's coupling, and `lifecyclerecipe` deliberately carries no such check.
  Replace the `Parse`/`Build`/`&shedengine.Shed{…}` body that follows with a call to `shedbuild.NewShed(recipes.LoomRecipe, env, paths)`, wrapping a non-nil returned error with `"loomrecipe: "` and nothing more, so this package's existing error-prefix assertions keep passing.
  `NewShed` returns its own errors unwrapped (card 1), so this is the only prefix layer and today's `loomrecipe: <err>` text is preserved exactly.
  Preserve the existing doc comment's explanation of the two coherence checks and of why `Check` is never called, trimming only the sentences describing the parse/build body that no longer lives here.
  The removed `ShedPaths` doc comment's "deliberately told twice" paragraph moves onto `New`'s own doc comment, since it explains this guard and nothing else.
- **Commit:** `refactor(loomrecipe): delegate New to shedbuild.NewShed, keeping the coherence guard`

### Card 3: shrink lifecyclerecipe.New to a bare delegation

- **Context:**
  - `internal/shedbuild/newshed.go`
  - `internal/shedrecipe/recipe.go`
  - `contracts/recipes/recipes.go`
  - `internal/loomrecipe/loomrecipe.go`
- **Edits:**
  - `internal/lifecyclerecipe/lifecyclerecipe.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** remove the `ShedPaths` type declaration from this file entirely and change `New`'s signature to `func New(env shedrecipe.Env, paths shedbuild.ShedPaths) (*shedengine.Shed, error)`.
  Replace the whole body with a call to `shedbuild.NewShed(recipes.LifecycleRecipe, env, paths)`, wrapping a non-nil returned error with `"lifecyclerecipe: "` and nothing more, so this package's existing error-prefix assertions keep passing.
  `NewShed` returns its own errors unwrapped (card 1), so this is the only prefix layer.
  Keep the existing doc comment's sentence explaining that, unlike `loomrecipe.New`'s two, this `New` performs no coherence check across its two arguments because no lifecycle registry entry reads `Env.StatusPath` or `Env.StatusLockPath` — it stays true and is the reason the two `New` bodies are not identical even after the hoist.
- **Commit:** `refactor(lifecyclerecipe): delegate New to shedbuild.NewShed`

### Card 4: retarget every ShedPaths caller onto the hoisted type

- **Context:**
  - `internal/shedbuild/newshed.go`
  - `internal/loomrecipe/loomrecipe.go`
  - `internal/lifecyclerecipe/lifecyclerecipe.go`
  - `internal/lifecycleshed/deps.go`
  - `internal/loomengine/config.go`
  - `internal/lifecyclecli/paths.go`
- **Edits:**
  - `internal/loomcli/cli.go`
  - `internal/loomcli/wiring.go`
  - `internal/lifecyclecli/cli.go`
  - `internal/lifecyclecli/wire.go`
  - `internal/loomcli/wiring_test.go`
  - `internal/lifecyclecli/wire_test.go`
  - `internal/loomcli/cli_test.go`
  - `internal/loomcli/status_test.go`
  - `internal/loomcli/step_test.go`
  - `internal/loomcli/sharedbootstrap_test.go`
  - `internal/lifecyclecli/run_test.go`
  - `internal/loomrecipe/fixture_test.go`
  - `internal/loomrecipe/shape_test.go`
  - `internal/lifecyclerecipe/fixture_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** change the `shedPaths` field's declared type on `loomCLI` (in `internal/loomcli/cli.go`) and on `lifecycleCLI` (in `internal/lifecyclecli/cli.go`) from `loomrecipe.ShedPaths`/`lifecyclerecipe.ShedPaths` to `shedbuild.ShedPaths`, and retarget the two struct literals in `internal/loomcli/wiring.go` (inside `wireLightweight` and inside `wire`, both currently written as `loomrecipe.ShedPaths{…}`) and the one in `internal/lifecyclecli/wire.go`'s `wire` (currently `lifecyclerecipe.ShedPaths{…}`) onto the same type.
  Every field value assigned in those three literals stays exactly as it is — this card changes the type name and the import set only, never a path, never `CommitStatus`, never the deliberately-zero `MaxBounces`.
  Drop the now-unused `loomrecipe`/`lifecyclerecipe` imports from any of these files where the retarget leaves no other reference, and add the `internal/shedbuild` import where it is now needed.
  Find every remaining construction site with a repo-wide grep rather than the hand list below, which is this plan's own inventory at authoring time and is the floor, not the ceiling: grep for `loomrecipe.ShedPaths`, `lifecyclerecipe.ShedPaths`, and the bare `ShedPaths{` composite literal, the last of which matters because the two recipe packages' own tests construct the type unqualified and stop compiling the moment cards 2 and 3 delete it.
  The sites this plan found are `internal/loomcli/cli_test.go`, `internal/loomcli/status_test.go`, `internal/loomcli/step_test.go`, `internal/loomcli/sharedbootstrap_test.go` (two literals), `internal/lifecyclecli/run_test.go`, `internal/loomcli/wiring_test.go`, which names the type only in two doc comments, for the qualified form, plus `internal/loomrecipe/fixture_test.go`, `internal/loomrecipe/shape_test.go` and `internal/lifecyclerecipe/fixture_test.go` for the bare in-package form.
  Retarget each onto `shedbuild.ShedPaths`, adding the `internal/shedbuild` import to the in-package ones, which never needed an import before.
  No assertion changes anywhere, since neither the field set nor any value changed;
  if any site turns out to need one, stop and report it rather than editing it.
  Do not touch `internal/loomcli/start.go` beyond whatever the type rename itself forces — `_mill/discussion.md`'s Scope puts `start` and its bootstrap helpers out of scope apart from exactly this forced signature change.
- **Commit:** `refactor(cli): retarget ShedPaths callers onto the hoisted shedbuild type`

### Card 5: cover NewShed and confirm the seam allowlists still hold

- **Context:**
  - `internal/shedbuild/newshed.go`
  - `internal/shedbuild/build_test.go`
  - `internal/shedbuild/parse_test.go`
  - `internal/shedbuild/fixture_test.go`
  - `internal/shedbuild/seam_enforcement_test.go`
  - `internal/shedengine/shed.go`
- **Edits:**
  - `internal/shedbuild/seam_enforcement_test.go`
  - `internal/shedbuild/fixture_test.go`
- **Creates:**
  - `internal/shedbuild/newshed_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** add a test asserting `NewShed` returns a `*shedengine.Shed` whose `Producers` length matches the parsed recipe's row count and whose `StatusPath`, `LockPath`, `StatusLockPath`, `MaxBounces` and `CommitStatus` fields are copied verbatim from the passed `ShedPaths` — the same five fields the two removed `New` bodies set, so the hoist is proved field-complete rather than assumed.
  Assert `CommitStatus` identity by calling the returned Shed's own `CommitStatus` and observing the passed closure's side effect, since a func value is not comparable with `==` against anything but `nil`.
  Add a test that an empty-producer recipe still errors, and that the returned error carries **no** `"shedbuild: "` prefix — the single-prefix rule card 1 states, asserted so a later prefix addition fails here rather than silently rewording both recipe packages' shipped envelopes.
  Build both cases over the existing fixture helpers in `internal/shedbuild/fixture_test.go` rather than a new hub or a real recipe file;
  extend that file only if it has no helper producing a minimal parseable recipe byte slice.
  Re-run `internal/shedbuild/seam_enforcement_test.go`'s allowlist against the new production file: `newshed.go` imports `shedengine` and `shedrecipe`, both of which the package already imports, so the allowlist should need no entry — add one only if the test actually fails, and never widen it beyond what `newshed.go` genuinely imports.
- **Commit:** `test(shedbuild): cover NewShed's field-complete assembly and error wrapping`

## Batch Tests

`verify:` runs the five packages this batch edits: `internal/shedbuild` (the new `newshed_test.go` plus its existing `build_test.go`/`parse_test.go`/`check_test.go`/`load_test.go`/`seam_enforcement_test.go`), `internal/loomrecipe` and `internal/lifecyclerecipe` (their full existing suites — `recipe_test.go`, `shape_test.go`, `sequence_test.go`, `resume_test.go`, `coverage_guard_test.go`, `seam_enforcement_test.go` and the rest, all of which must pass unchanged, since this batch changes no recipe content and no graph shape), and `internal/loomcli`/`internal/lifecyclecli` (whose `wiring_test.go` and `wire_test.go` pin the retargeted struct literals' field values, `wire_test.go` without naming the type itself).

No `-tags integration` run is chained here: this batch's only edits under `internal/loomcli` and `internal/lifecyclecli` are the type rename in card 4, and neither package's integration suite references `loomrecipe.ShedPaths` or `lifecyclerecipe.ShedPaths` by name.
If the type rename does turn out to break either integration file's compilation, that file joins card 4's `Edits:` and the chained `go test -tags integration` invocation joins this batch's `verify:` — which the batch's own re-validation will surface, since a non-compiling tagged file fails the repo-wide done gate rather than passing silently.
