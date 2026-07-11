# Batch: shuttle-version

```yaml
task: Build modelspec - the model-spec parser + registry
batch: shuttle-version
number: 3
cards: 3
verify: go test ./internal/shuttleengine/...
depends-on: [2]
```

## Batch Scope

Realizes the `version=` param end-to-end: `shuttleengine.Spec` gains a `Version` field
(exactly the `Effort` pattern — provider vocabulary, never touched by `Spec.validate`,
engine is sole validator), and claudeengine translates bare-word model + version into a
pinned Claude model id before command construction. No `Engine` interface change:
`Prepare(runDir string, spec Spec, cfg Config)` already receives the whole Spec. No
shuttle CLI flag is added (discussion scope: the field is driven programmatically by
future consumers via modelspec; operators pin via models.yaml or a full `--model` id).
Closes with the contract amendment generalizing the translation rule and the overview
shuttle-bullet sentence. Depends on batch 2 only to serialize doc-file edits
(`docs/reference/model-spec.md`, `docs/overview.md` are also edited in batch 1; batch 2
edits `docs/overview.md`) — there is no code dependency.

## Cards

### Card 11: shuttleengine.Spec.Version

- **Context:**
  - `docs/reference/model-spec.md`
- **Edits:**
  - `internal/shuttleengine/spec.go`
  - `internal/shuttleengine/spec_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add `Version string` to `shuttleengine.Spec`, placed directly after
  `Effort`, with a doc comment written in exact parallel to the existing `Effort`
  comment: non-empty selects a version pin the provider engine realizes by translating
  (model, version) into a provider-specific model id; empty means no pin; version values
  are provider vocabulary — `validate` does NOT inspect this field at all (neither
  defaulting nor rejecting); the engine is the sole validator (see claudeengine's
  `resolveModelID`), and a (model, version) pair the engine cannot realize is a hard
  error from the engine, not from `Spec.validate`. Do not modify `validate`'s body.
  Extend `spec_test.go` with a test proving `validate` leaves an arbitrary non-empty
  `Version` (e.g. `"4.5"`, and also a nonsense value like `"weird"`) exactly as set and
  returns no error for an otherwise-valid Spec — mirroring how Effort's
  engine-is-sole-validator property is pinned.
- **Commit:** `feat(shuttle): add Spec.Version — engine-validated version pin, parallel to Effort`

### Card 12: claudeengine — resolveModelID translation

- **Context:**
  - `internal/shuttleengine/spec.go`
  - `internal/shuttleengine/engine.go`
  - `docs/reference/model-spec.md`
- **Edits:**
  - `internal/shuttleengine/claudeengine/claudeengine.go`
  - `internal/shuttleengine/claudeengine/command.go`
  - `internal/shuttleengine/claudeengine/command_test.go`
  - `internal/shuttleengine/claudeengine/prepare_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `command.go`, next to `validateEffort`, add
  `func resolveModelID(model, version string) (string, error)` implementing the generic
  bare-word rule (discussion decision "claudeengine translation rule"; deliberately NO
  closed alias list — a new provider alias must translate on an old binary):
  (1) `version == ""` → return `model` unchanged (including empty);
  (2) `version != "" && model == ""` → error: version pin with no model to compose
  against;
  (3) `model` contains `-` (a full model id, e.g. from the escape form) → error: the id
  already pins the version, a second pin is a contradiction;
  (4) otherwise → return `"claude-" + model + "-" + strings.ReplaceAll(version, ".", "-")`
  (so `sonnet`+`4.5` → `claude-sonnet-4-5`, `fable`+`5` → `claude-fable-5`). Doc comment
  states the rule, the no-closed-list rationale, and that a nonsense composition fails
  loudly at the Claude CLI launch (fail-loud is preserved downstream; quoting in
  `buildLaunchCmd` already prevents injection). In `claudeengine.go`'s `Prepare`, call
  `resolveModelID(spec.Model, spec.Version)` alongside the existing `validateEffort`
  pre-flight (hard error aborts Prepare before any launch artifact matters) and pass the
  resolved id as the `model` argument to `buildLaunchCmd` — `buildLaunchCmd` itself is
  UNCHANGED (it receives the final string; resume path takes no model and is untouched).
  Tests, mirroring the `validateEffort`/`buildLaunchCmd` table style: `command_test.go`
  tables for `resolveModelID` (dotted version, dotless version, empty version
  passthrough, empty model + version error, dashed model + version error);
  `prepare_test.go` cases proving Prepare with `Spec{Model: "sonnet", Version: "4.5"}`
  produces a launch Cmd containing `--model` with `claude-sonnet-4-5`, and Prepare with
  a dashed model + version fails with no run artifacts consumed.
- **Commit:** `feat(claudeengine): translate bare-word model + version into pinned model id`

### Card 13: Docs — translation-rule amendment and shuttle bullet

- **Context:**
  - `internal/shuttleengine/claudeengine/command.go`
- **Edits:**
  - `docs/reference/model-spec.md`
  - `docs/overview.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** (1) `docs/reference/model-spec.md`, "Newest by default; pinning is
  deliberate" section: generalize the per-spec pinning sentence — the provider engine
  translates the generic `version` param to its own id scheme; claudeengine's rule is
  generic over any bare single-word model value (`sonnet`+`4.5` → `claude-sonnet-4-5`,
  `fable`+`5` → `claude-fable-5`; not a closed alias list, so operator-added aliases
  translate on old binaries), and combining `version=` with a full model id (contains a
  dash, e.g. the escape form) is a hard error because the id already pins the version.
  (2) `docs/overview.md` shuttle bullet: after the existing Model/Effort knob sentence,
  add one sentence: `Spec.Version` is a programmatic engine-validated version pin
  (claudeengine composes the pinned model id; no CLI flag — consumers drive it via the
  model-spec notation's `version=` param). Keep both edits at the surrounding prose
  density.
- **Commit:** `docs(shuttle): pin generic version-translation rule in contract and overview`

## Batch Tests

`verify: go test ./internal/shuttleengine/...` runs the shuttleengine suite (Spec
validate untouched-Version test) and the claudeengine suite (`resolveModelID` tables,
Prepare wiring cases) plus the existing seam-enforcement test
(`TestProviderSeamImportRule`), which proves the provider-invariant packages gained no
Claude specifics from this batch. Scope matches the only package tree touched; docs
edits in card 13 have no runnable surface.
