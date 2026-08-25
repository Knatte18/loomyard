# Batch: shedrecipe-entries

```yaml
task: 'loom: Discussion-Review producer'
batch: 'shedrecipe-entries'
number: 2
cards: 5
verify: go test ./internal/shedrecipe/...
depends-on: []
```

## Batch Scope

This batch extends the two generic registry entries the perch consumes, without touching `internal/shedadapters` itself.
It adds four run-wide review fields to `shedrecipe.Env`, makes `bouncerEntry` and `burlerRoundEntry` fall back to them when their per-row Config keys are absent, and gives `burlerRoundEntry`'s `profile:` map a new `rubric_stencil` key that resolves a stencilstore name through `Env.StencilsDir`, mutually exclusive with the map's existing literal `rubric` key.
The external interface batches 4 and 5 consume is the four `Env.Review*` fields and the `profile.rubric_stencil` key.
Nothing here reads a recipe row, so this batch is independent of batch 1 and batch 3.

## Cards

### Card 4: add the four run-wide review fields to Env

- **Context:** none
- **Edits:**
  - `internal/shedrecipe/recipe.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add four fields to the `Env` struct: `ReviewModel`, `ReviewEffort`, `ReviewVersion` (all `string`) and `ReviewTimeout` (`time.Duration`).
  Place them together, immediately after the existing `SupportLogPath` field and before the blank line separating the told-path block from the injected-seam block, since they are told values rather than seams.
  Give the group a doc comment stating that these are run-wide review defaults read by the `Bouncer` and `BurlerRound` entries, that each is used only when the corresponding per-row Config key is absent, and that `ReviewTimeout` is read by `BurlerRound` alone because `shedadapters.BouncerConfig` carries no timeout field.
  Name the contractual reason they are legal on `Env` at all: `Env` carries roots and run-wide values only, and one review model shared by every review segment is exactly such a value — a per-row value would have to be a Config key instead.
  The `time` package is already imported by this file for the `Now` field, so no import change is needed.
- **Commit:** `feat(shedrecipe): add run-wide Review* fields to Env`

### Card 5: fall back to Env.Review* in bouncerEntry

- **Context:**
  - `internal/shedrecipe/recipe.go`
  - `internal/shedadapters/bouncer.go`
- **Edits:**
  - `internal/shedrecipe/entries_bouncer.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  After the three `configString` reads for `model`, `effort`, and `version` in `bouncerEntry`, fall back to `env.ReviewModel`, `env.ReviewEffort`, and `env.ReviewVersion` respectively whenever the Config value is the empty string.
  An empty Config value and an absent key are the same thing here — `configString` with `required` false returns `""` for both — and that is deliberate: there is no meaningful "explicitly empty" model.
  Add a short comment above the fallback block stating that a row setting the key overrides the `Env` value, that a row omitting it takes the `Env` value, and that both absent leaves the provider default.
  Do not add a `timeout_s` key or any timeout fallback to this entry: `shedadapters.BouncerConfig` has no timeout field, so there is nothing to thread it into.
  Do not change the `configRejectUnknown` call — no new key is introduced by this card.
- **Commit:** `feat(shedrecipe): fall back to Env.Review* in bouncerEntry`

### Card 6: add profile.rubric_stencil and Env fallback to burlerRoundEntry

- **Context:**
  - `internal/shedrecipe/recipe.go`
  - `internal/shedrecipe/entries_bouncer.go`
  - `internal/shedrecipe/entries_singlellm.go`
  - `internal/shedrecipe/env.go`
  - `internal/shedrecipe/config.go`
  - `internal/burlerengine/profile.go`
- **Edits:**
  - `internal/shedrecipe/entries_burler.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Change `burlerRoundProfile`'s signature from `burlerRoundProfile(cfg Config) (burlerengine.Profile, error)` to `burlerRoundProfile(cfg Config, stencilsDir string) (burlerengine.Profile, error)` and update its single call site in `burlerRoundEntry` to pass `env.StencilsDir`.
  Inside `burlerRoundProfile`, read a new optional `rubric_stencil` string key alongside the existing `rubric` key, and add `"rubric_stencil"` to the `configRejectUnknown` call's known-key list so the profile map's strict-unknown-key rejection keeps working.
  Update that function's own doc comment in the same edit: it currently states the function recognises "exactly six keys" and enumerates them, which the seventh key makes false.
  Make it seven, add `rubric_stencil` to the enumeration, and record the `rubric`/`rubric_stencil` mutual-exclusivity rule there — the comment already explains which burler-profile keys are deliberately absent and why, so it is the right place for the one rule a recipe author cannot infer from the key list alone.
  Enforce mutual exclusivity: exactly one of `rubric` and `rubric_stencil` must be non-empty.
  Both non-empty is an error naming both keys;
  both empty is an error naming both keys and stating that one is required.
  Do not delegate the both-empty case to `burlerengine.Profile.validate`'s own empty-`Rubric` rejection — that error names neither key and cannot say which of the two the author meant.
  When `rubric_stencil` is set, and only then, guard the stencils root with `requireAbsRoot("BurlerRound", "StencilsDir", stencilsDir)` before reading, so a caller that fills only what its own recipe needs still constructs cleanly on the literal-`rubric` path.
  Then read the stencil with `stencilstore.Read(stencilsDir, rubricStencil)`, wrapping any error so it names the offending stencil name, and set `Profile.Rubric` to `stencil.StripLeadingComment(string(raw))`.
  The strip is load-bearing: `internal/stencilstore` stamps a `<!-- lyx-stencil: sha256=... -->` banner onto every seeded file, and `internal/stencil`'s `Fill` strips a banner from the template it parses but never from a marker value, so unstripped bytes would inject the banner into the middle of the round prompt.
  This mirrors what `internal/shedadapters`' own Bouncer already does with its rubric.
  Add the two imports the reads need: `internal/stencil` and `internal/stencilstore`, both already used by `entries_singlellm.go` in this same package.
  Separately, in `burlerRoundEntry` itself, fall back to `env.ReviewModel` and `env.ReviewEffort` when the `model` and `effort` Config values are empty, and to `env.ReviewTimeout` when the `timeout_s` Config value is `0`, building `burlerengine.RunOpts` from the resolved values rather than from the raw Config reads.
  Note that `configInt` with `required` false returns `0` for an absent key, so `0` is the absent sentinel here — the same "no meaningful explicit zero" reasoning card 5 applies to the empty string.
- **Commit:** `feat(shedrecipe): add profile.rubric_stencil and Env.Review* fallback to burlerRoundEntry`

### Card 7: cover bouncerEntry's Env.Review* fallback

- **Context:**
  - `internal/shedrecipe/entries_bouncer.go`
  - `internal/shedrecipe/fixture_test.go`
  - `internal/shedadapters/bouncer.go`
  - `internal/shedadapters/bouncer_config_test.go`
- **Edits:**
  - `internal/shedrecipe/entries_bouncer_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add a test covering the three fallback outcomes for `bouncerEntry`: a row omitting `model`/`effort`/`version` takes `env.ReviewModel`/`env.ReviewEffort`/`env.ReviewVersion`; a row setting all three overrides the `Env` values; both absent leaves all three empty (the provider default).
  Build the `Env` with the existing `newTestEnv(t)` helper and set the `Review*` fields on the returned value directly rather than changing `newTestEnv` — every other test in the package expects that helper's current shape.
  `shedadapters.BouncerConfig` is not reachable from a constructed `*shedadapters.Bouncer` (its `cfg` field is unexported and this is a different package), so assert the resolved triple through the behaviour instead: drive one `Call` against the entry's producer with the `fakeShuttle` already on `newTestEnv`'s `Env`, and assert the recorded `shuttleengine.Spec`'s `Model`, `Effort`, and `Version`.
  The first `Call` on a fresh run directory is the seed pass, which spawns unconditionally, so seeding `bouncer-template-seed` into `env.StencilsDir` alongside the rubric that `minimalBouncerConfig` already writes is what makes the spawn happen at all — without it `runSeedSpawn` logs and returns before ever reaching the shuttle.
  Extend the existing `fakeShuttle` in `internal/shedrecipe/fixture_test.go` only if it does not already record the `Spec` it was handed;
  if it does, read it there rather than adding a second fake.
- **Commit:** `test(shedrecipe): cover bouncerEntry's Env.Review* fallback`

### Card 8: cover burlerRoundEntry's rubric_stencil and Env.Review* fallback

- **Context:**
  - `internal/shedrecipe/entries_burler.go`
  - `internal/shedrecipe/entries_bouncer_test.go`
  - `internal/shedrecipe/fixture_test.go`
  - `internal/burlerengine/profile.go`
- **Edits:**
  - `internal/shedrecipe/entries_burler_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add tests for the new `profile.rubric_stencil` key, using the existing `callAndCaptureProfile` helper to read the resolved `burlerengine.Profile` back off the `fakeBurlerRunner`, and the existing `writeStencil` helper (declared in `entries_bouncer_test.go`, same package) to seed stencils.
  Cover: a `rubric_stencil` naming a seeded stencil resolves to that stencil's content in `Profile.Rubric`;
  a seeded stencil whose bytes carry a leading stamp banner has that banner stripped from `Profile.Rubric`;
  supplying both `rubric` and `rubric_stencil` is a construction error naming both keys;
  supplying neither is a construction error naming both keys;
  a `rubric_stencil` naming an unseeded stencil is a construction error naming that stencil name;
  and an empty `Env.StencilsDir` is a construction error on the `rubric_stencil` path only — the same config with a literal `rubric` and an empty `Env.StencilsDir` must still construct cleanly.
  Use the existing `assertErrContains` helper for every error assertion.
  Add a second test covering the `Env.Review*` fallback for this entry: a row omitting `model`/`effort`/`timeout_s` yields a `burlerengine.RunOpts` carrying `env.ReviewModel`/`env.ReviewEffort`/`env.ReviewTimeout`; a row setting all three overrides them; all three absent with an empty `Env` leaves the zero values.
  Read the `RunOpts` back through `callAndCaptureProfile`'s second return value.
- **Commit:** `test(shedrecipe): cover burlerRoundEntry's rubric_stencil and Env.Review* fallback`

## Batch Tests

`verify: go test ./internal/shedrecipe/...` runs the whole `internal/shedrecipe` package.
That is the exact scope of this batch's edits — all five cards touch only files in that one package — and the package's existing entry tests (`entries_bouncer_test.go`, `entries_burler_test.go`, `entries_simple_test.go`, `entries_singlellm_test.go`, plus the seam-enforcement and config-helper tests) are also the regression surface for card 4's `Env` widening and card 6's `burlerRoundProfile` signature change.
Cards 7 and 8 are the new coverage;
the rest of the package proves nothing else broke.
