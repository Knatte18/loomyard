# Batch: effort

```yaml
task: Add Effort to shuttle's run Spec
batch: effort
number: 1
cards: 5
verify: go test ./internal/shuttleengine/... ./internal/shuttlecli/...
depends-on: []
```

## Batch Scope

This batch delivers the full per-run reasoning-effort knob: the `Spec.Effort`
field, its engine-side validation and `--effort` launch-flag realization in
`claudeengine`, the `lyx shuttle run --effort` CLI flag, and the overview doc
line. It is one batch because these are the entire effort feature and share the
same small file set (`spec.go`, `claudeengine`'s `command.go`/`claudeengine.go`,
and `shuttlecli`'s `run.go`). The next batch (ask-signal) consumes nothing from
this batch except serialized access to `docs/overview.md`. Note: card 2 widens
`buildLaunchCmd`'s arity but leaves its `Prepare` call site alone, so the
`claudeengine` package does not compile until card 3 threads `spec.Effort` into
that call — intermediate cards do not compile in isolation and the batch verifies
only at the end (same as batch 2). Batch-local decision:
effort validation lives in `claudeengine` (vocabulary-only, exact-lowercase,
hard-error in `Prepare`); `Spec.validate` is never touched — see the overview's
Shared Decisions.

## Cards

### Card 1: Add `Spec.Effort` field

- **Context:**
  - `internal/shuttleengine/claudeengine/command.go`
- **Edits:**
  - `internal/shuttleengine/spec.go`
  - `internal/shuttleengine/spec_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add an exported `Effort string` field to the `Spec` struct in
  `spec.go`, placed immediately after `Model`, with a doc comment stating: empty =
  provider default; effort *values* are provider vocabulary, so the engine (not
  `Spec.validate`) validates them; a non-empty value the engine cannot realize is a
  hard error. Do NOT add any effort handling to the `validate` method — it must
  remain untouched with respect to effort. In `spec_test.go`, add a test proving
  `validate` leaves `Effort` untouched (neither defaults nor rejects a non-empty
  `Effort`, and accepts an empty one), mirroring the existing `Model`-adjacent
  validate tests.
- **Commit:** `feat(shuttleengine): add Effort field to run Spec`

### Card 2: Effort vocabulary + validator + `--effort` in buildLaunchCmd

- **Context:**
  - `internal/shuttleengine/spec.go`
  - `internal/shuttleengine/claudeengine/claudeengine.go`
- **Edits:**
  - `internal/shuttleengine/claudeengine/command.go`
  - `internal/shuttleengine/claudeengine/command_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `command.go`: (a) add a package-level set of valid effort
  values — exactly `low`, `medium`, `high`, `xhigh`, `max` (the values verified live
  against `claude --effort`); (b) add a `validateEffort(effort string) error` helper
  that returns nil for the empty string, nil for an exact-lowercase member of that
  set, and a descriptive hard error otherwise (naming the invalid value and listing
  the valid set) — case-sensitive, so `High`/`HIGH` are rejected; (c) add an `effort`
  parameter to `buildLaunchCmd` and append `--effort <effort>` (single-quoted via
  `pwshSingleQuote`, exactly like `--model`) only when `effort != ""`, positioned
  next to the `--model` append. `buildResumeCmd` is NOT changed. In
  `command_test.go`, extend `TestBuildLaunchCmd`'s table with rows: no effort (no
  `--effort` in output), each valid value including `max`, effort together with a
  model (both flags present, correct order and quoting), and an effort value
  containing a space/quote (stays one single-quoted argument — mirror the existing
  model-quoting row); add a `TestValidateEffort` covering empty (ok), each valid
  value (ok), and `bogus`/`High` (error).
- **Commit:** `feat(claudeengine): map Spec.Effort to the --effort launch flag`

### Card 3: `Prepare` validates effort before writing artifacts

- **Context:**
  - `internal/shuttleengine/claudeengine/command.go`
  - `internal/shuttleengine/spec.go`
  - `internal/shuttleengine/config.go`
- **Edits:**
  - `internal/shuttleengine/claudeengine/claudeengine.go`
- **Creates:**
  - `internal/shuttleengine/claudeengine/prepare_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `Prepare` (`claudeengine.go`), call `validateEffort(spec.Effort)`
  and return its error BEFORE any artifact is written — place the check next to the
  existing `maxLaunchPromptBytes` guard, so an unrealizable effort fails before
  `prompt.md`/`settings.json` exist. Thread `spec.Effort` into the `buildLaunchCmd`
  call. In the new `prepare_test.go` (same `claudeengine` package): a test that
  `Prepare` with a bad effort (`bogus`) returns an error AND writes no `prompt.md` /
  `settings.json` into the temp run dir; a test that `Prepare` with a valid effort
  (`high`) succeeds and the returned `Launch.Cmd` contains `--effort 'high'`; a test
  that empty effort succeeds and emits no `--effort`.
- **Commit:** `feat(claudeengine): hard-error unrealizable effort in Prepare`

### Card 4: `--effort` flag on `lyx shuttle run`

- **Context:**
  - `internal/shuttleengine/spec.go`
- **Edits:**
  - `internal/shuttlecli/run.go`
  - `internal/shuttlecli/cli_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `run.go`, add an `effort` string variable, register
  `cmd.Flags().StringVar(&effort, "effort", "", ...)` with a description matching the
  `--model` flag's style (e.g. "reasoning-effort override; empty defers to the
  provider default"), and set `Effort: effort` in the `shuttleengine.Spec` literal —
  mirroring the existing `--model` wiring exactly. Optionally add a one-line
  `--effort` mention to the command `Long`. In `cli_test.go`, add a test asserting
  `--effort high` lands in the constructed `Spec.Effort` (and omitting the flag
  yields `""`), following the existing `--model` flag test pattern with the same fake
  runner that captures the spec.
- **Commit:** `feat(shuttlecli): add --effort flag to shuttle run`

### Card 5: Document the effort knob in the overview

- **Context:**
  - `internal/shuttlecli/run.go`
  - `internal/shuttleengine/spec.go`
- **Edits:**
  - `docs/overview.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In the **shuttle** module entry of `docs/overview.md`, add a
  short phrase noting the per-run reasoning-effort knob (`Spec.Effort` / `lyx shuttle
  run --effort`, values `low|medium|high|xhigh|max`, empty = provider default,
  engine-validated) alongside the existing `Model` mention. One line; do not restate
  the whole entry.
- **Commit:** `docs(overview): note the per-run effort knob on shuttle`

## Batch Tests

`verify: go test ./internal/shuttleengine/... ./internal/shuttlecli/...` covers
every package this batch touches: `shuttleengine` (`spec_test.go`), its
`claudeengine` subpackage (`command_test.go`, `prepare_test.go`), and `shuttlecli`
(`cli_test.go`). Native Go runner, no `PYTHONPATH=` prefix (Go repo). The scope is
the two package trees the batch edits — not repo-wide — because nothing outside
them is touched.
