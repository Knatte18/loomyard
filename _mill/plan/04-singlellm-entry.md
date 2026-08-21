# Batch: SingleLLM entry

```yaml
task: 'Shed recipe: engine registry'
batch: 'SingleLLM entry'
number: 4
cards: 3
verify: go test ./internal/shedrecipe/...
depends-on: [3]
```

## Batch Scope

This batch delivers the `SingleLLM` registry entry — the only entry with real logic of its own, since `shedadapters.NewSingleLLMProducer` has no production caller today and nobody has yet built the spec-composition step it needs.
It is one batch because the entry, its registration, and its tests are one indivisible piece of work: the entry's whole substance is a `shedadapters.SpecSource` closure, and a closure that is never invoked is untested code.
The external interface batch 6's coverage guard consumes is the `"SingleLLM"` key in `registry.go`'s map literal.

Batch-local decisions beyond `## Shared Decisions`:

- The four reserved geometry-derived tokens are supplied to `stencil.Fill` unconditionally, whether or not the stencil names them, so the entry's `Env` requirements never depend on parsing template text.
  This is safe because `stencil.Fill` errors only on a marker the template references whose value is absent, empty, or whitespace-only, never on an extra value the template ignores.
- The stencil is read twice by design: once at construction as a fail-fast probe whose bytes are discarded, and once per `Call` inside the closure, so a stencil edited mid-run takes effect without a restart.

## Cards

### Card 11: The SingleLLM entry and its SpecSource

- **Context:**
  - `internal/shedrecipe/recipe.go`
  - `internal/shedrecipe/config.go`
  - `internal/shedrecipe/env.go`
  - `internal/shedrecipe/paths.go`
  - `internal/shedadapters/singlellm.go`
  - `internal/shedadapters/bouncer.go`
  - `internal/shuttleengine/spec.go`
  - `internal/stencil/stencil.go`
  - `internal/stencilstore/reconcile.go`
- **Edits:** none
- **Creates:**
  - `internal/shedrecipe/entries_singlellm.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Write one unexported function `singleLLMEntry` matching the `Constructor` signature.

  Recognised `Config` keys, and nothing else: required `stencil` (a `stencilstore` name, via `configString`) and `output_files` (via `configStringSlice`);
  optional `model`, `effort`, `version`, `role` (each via `configString`), `interactive` (via `configBool`), and `tokens` (via `configStringMap`).
  End the extraction with `configRejectUnknown(cfg, "stencil", "output_files", "model", "effort", "version", "role", "interactive", "tokens")`.

  `Env` validation, all unconditional: `requireAbsRoot` on `Env.WorktreeRoot`, `Env.AnchorPath`, and `Env.StencilsDir`, then `requireSeam` on `Env.Shuttle`.
  A comment states why all three roots are validated unconditionally rather than only when the stencil names the corresponding marker: making the requirement depend on template text would let a stencil edit change a recipe row's validity without the recipe changing.

  Resolve every `output_files` entry with `resolveUnderRoot(entry, "output_files", env.WorktreeRoot, value)`, collecting the absolute results in order.
  A comment states that absolute is not optional here — `SingleLLMProducer.Call` rejects a non-absolute `spec.OutputFiles` entry outright rather than resolving it, because resolution would need a worktree root that adapter is barred from reading.

  Build the `stencil.Fill` token map as the union of two closed sets.
  Declare the four reserved names as a package-level sorted `[]string` (or a `map[string]bool`) named `singleLLMReservedTokens` holding `"anchor_path"`, `"output_files"`, `"stencils_dir"`, `"worktree_root"`, so the rejection check and the fill both read one declaration.
  Their values: `worktree_root` is `env.WorktreeRoot`;
  `anchor_path` is `env.AnchorPath`;
  `stencils_dir` is `env.StencilsDir`;
  `output_files` is the resolved absolute paths joined with `"\n"`.
  Everything else comes from `Config.tokens`.

  Two construction-time rejections over `Config.tokens`, both erroring with a message naming the offending token: a token whose name is in `singleLLMReservedTokens` is rejected rather than silently overriding or being overridden;
  a token whose value is empty or whitespace-only (`strings.TrimSpace` reports empty) is rejected, because `stencil.Fill` treats an empty value exactly like a missing one, and a token that legitimately has no value must carry an explicit placeholder the way `shedadapters.Bouncer` passes the literal `"(none)"`.

  Probe the stencil at construction with `stencilstore.Read(env.StencilsDir, stencilName)`, discarding the bytes and returning a wrapped error naming the stencil on failure.
  A comment states that this mirrors `NewBouncer`'s own eager rubric probe and exists so a mistyped `stencil` name fails at construction rather than at first `Call`.

  Build the `shedadapters.SpecSource` closure.
  On each invocation it re-reads the stencil with `stencilstore.Read(env.StencilsDir, stencilName)`, calls `stencil.Fill(template, tokens)`, and returns a `shuttleengine.Spec` whose `Prompt` is the filled bytes as a string, whose `OutputFiles` is the resolved absolute slice, and whose `Model`, `Effort`, `Version`, `Role`, and `Interactive` come from `Config`.
  Do not call `stencil.StripLeadingComment` anywhere in this file — the stencil here is the template, and `stencil.Fill` strips its own stamp banner;
  state that in a comment, together with the note that a future row injecting stencil content as a token *value* would pay the strip itself.
  Leave `Spec.Timeout`, `Spec.ForkSubagents`, `Spec.KeepPane`, `Spec.Round`, `Spec.Parent`, and `Spec.Display` at their zero values, with a comment stating this is a decision rather than an oversight: they are deliberately not recipe-authorable in this task, and `Round`/`Parent`/`Display` are per-run strand-display values a recipe is the wrong place for regardless.

  Return `shedadapters.NewSingleLLMProducer(name, specSource, env.Shuttle, env.Now)`, passing `env.Now` through unchanged so a nil clock defaults inside that constructor.
- **Commit:** `feat(shedrecipe): add the SingleLLM registry entry with its SpecSource builder`

### Card 12: Register SingleLLM

- **Context:**
  - `internal/shedrecipe/entries_singlellm.go`
- **Edits:**
  - `internal/shedrecipe/registry.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add one entry to the `registry` map literal: `"SingleLLM": singleLLMEntry`.
  Change nothing else in the file — `Lookup`, `Names`, and the map's own comment stay as they are, except that the comment's forward reference to the keys batches 4 and 5 still owe should drop `SingleLLM` and now name only `Bouncer` and `BurlerRound`.
- **Commit:** `feat(shedrecipe): register the SingleLLM engine`

### Card 13: Tests for the SingleLLM entry

- **Context:**
  - `internal/shedrecipe/entries_singlellm.go`
  - `internal/shedrecipe/fixture_test.go`
  - `internal/shedrecipe/registry.go`
  - `internal/shedadapters/singlellm.go`
  - `internal/shuttleengine/spec.go`
  - `internal/stencilstore/reconcile.go`
  - `internal/stencilstore/stencilstore.go`
- **Edits:** none
- **Creates:**
  - `internal/shedrecipe/entries_singlellm_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Write tests in the `shedrecipe` package, all geometry from `newTestEnv(t)`.
  Each test that needs a readable stencil writes one into `env.StencilsDir` first, in whatever on-disk shape `stencilstore.Read` resolves a name to — `stencilstore.Read` in `internal/stencilstore/reconcile.go` delegates the layout to `stencilstore.Path`/`stencilstore.RelPath`, both declared in `internal/stencilstore/stencilstore.go`, so read that file for the actual convention rather than guessing.

  The composed-`Spec` assertions must invoke the closure, not merely construct the producer.
  The `SpecSource` is captured inside `shedadapters.SingleLLMProducer` and unreachable from outside, so drive it by calling the returned producer's `Call` with a `context.Background()` and the `fakeShuttle` from `fixture_test.go` configured to return `shuttleengine.OutcomeDone`, then assert against the `shuttleengine.Spec` that fake recorded.
  Assert the recorded `Spec` carries: the filled prompt text, the resolved absolute `OutputFiles` in the `Config` order, and the `Model`, `Effort`, `Version`, `Role`, and `Interactive` values from `Config`.
  Because `Spec.OutputFiles` entries must not exist when a run starts, do not pre-create the output files in these tests.

  Construction-failure subtests, each asserting the error names the offending thing:
  a `stencil` name with no file behind it fails at construction, before any `Call`;
  a `tokens` map naming a reserved token (`worktree_root`, `anchor_path`, `stencils_dir`, or `output_files`) fails at construction, one subtest per reserved name;
  a `tokens` value that is the empty string fails at construction;
  a `tokens` value that is whitespace-only fails at construction;
  an absolute `output_files` entry fails at construction;
  a `..`-escaping `output_files` entry fails at construction;
  a missing `stencil` key and a missing `output_files` key each fail at construction;
  a `stencil` key present with an empty-string value fails at construction;
  an empty `output_files` list fails at construction;
  an unrecognised `Config` key fails at construction;
  and, one subtest per read `Env` field, blanking `Env.WorktreeRoot`, `Env.AnchorPath`, or `Env.StencilsDir`, or nilling `Env.Shuttle`, fails at construction naming that field.

  Reserved-token value subtests: with a stencil declaring all four reserved markers, drive `Call` and assert the filled prompt contains `env.WorktreeRoot`, `env.AnchorPath`, `env.StencilsDir`, and the newline-joined resolved output paths.

  One `Call`-time subtest: a stencil naming a marker that is neither reserved nor present in `tokens` makes `Call` return an error rather than filling empty — this is the assertion that pins `stencil.Fill`'s hard-error behaviour rather than assuming it.

  One nil-clock subtest: an `Env` with `Now` nil constructs successfully.
- **Commit:** `test(shedrecipe): cover the SingleLLM entry and its composed Spec`

## Batch Tests

`verify: go test ./internal/shedrecipe/...` is correctly scoped: this batch adds one production file and one map key inside the package, and touches nothing outside it.
`entries_singlellm_test.go` is the batch's whole verification surface and covers both halves of the entry — the construction-time validations, and the closure's composed `Spec`, reached through `Call` with a recording fake because the closure is not otherwise addressable.
The mistyped-stencil subtest is the one that distinguishes a correct implementation from one that skips the constructor probe, and the missing-marker subtest is the one that pins `stencil.Fill`'s error behaviour the reserved-token design rests on.
