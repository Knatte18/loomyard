# Batch: modelspec-migration

```yaml
task: 'Treadle: shared round-loop engine + perch rewrite'
batch: modelspec-migration
number: 4
cards: 2
verify: go test ./internal/treadleengine/... ./internal/perchengine/... ./internal/perchcli/... ./internal/configreg/... ./cmd/lyx/...
depends-on: [3]
```

## Batch Scope

Migrates perch's operator config surface to the repo's established
model-spec notation (`docs/reference/model-spec.md`), fixing the
acknowledged split-key oversight: `perch.yaml`'s `judge_model` becomes a
model-spec string and `judge_effort` is REMOVED; profile keys
`judge-model:`/`model:` become model-spec strings and
`judge-effort:`/`effort:` are REMOVED. This is a deliberate fail-loud
breaking change to config FILES only — old files fail strict validation
with a clear message, no deprecated-fallback dual schema — while the
`perchengine.Profile`/`Config` Go structs and the engine-facing API stay
byte-identical. Grammar is validated at load (`modelspec.Parse`);
resolution runs once at block creation (`modelspec.LoadRegistry` +
`Registry.Resolve`), unpacking resolved (model, effort) pairs into the
unchanged struct fields. A bare alias picks up the operator-configured
default effort from the seeded `models.yaml`; bracket params other than
`effort` are rejected by an explicit perch-layer check on
`Resolved.Params` (NOT a modelspec guarantee — `version` is in modelspec's
known params). Treadle is untouched: it keeps receiving resolved plain
strings. Resume across the migration follows the accepted fail-loud
posture (perch-api-and-identity-stability decision): a block whose
resolved values change hashes differently and the existing
"started with a different profile; use a fresh --run-id" error fires.

## Cards

### Card 12: perch.yaml judge_model becomes a model-spec string

- **Context:**
  - `internal/modelspec/modelspec.go`
  - `internal/modelspec/parse.go`
  - `internal/modelspec/registry.go`
  - `internal/modelspec/load.go`
  - `internal/modelspec/template.yaml`
  - `internal/configreg/configreg.go`
  - `internal/perchengine/profile.go`
  - `docs/reference/model-spec.md`
  - `_mill/discussion.md`
- **Edits:**
  - `internal/perchengine/template.yaml`
  - `internal/perchengine/config.go`
  - `internal/perchengine/config_test.go`
  - `internal/perchengine/doc.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  `template.yaml`: DELETE the `judge_effort` line; reword the `judge_model`
  comment to document the model-spec notation (alias with optional
  `[effort=...]` bracket, escape form, bare alias inheriting the
  `models.yaml` default effort, pointer to `docs/reference/model-spec.md`);
  keep the default value `haiku` and the `round_caps` line unchanged. The
  strict template is what makes old `perch.yaml` files carrying
  `judge_effort` fail `configengine.Load` loud — that is the intended
  migration behavior; `configreg`'s registration picks the template up via
  `perchengine.ConfigTemplate` with no configreg source change (the
  `verify:` scope includes `internal/configreg` to prove it).
  `config.go`: `Config` struct keeps both exported fields (`JudgeModel`,
  `JudgeEffort` — byte-identical Go API); `JudgeEffort`'s yaml binding is
  removed (tag `yaml:"-"`) since the key no longer exists in the file.
  After unmarshal, `LoadConfig` resolves the spec: `modelspec.Parse` the
  `judge_model` string (grammar fail-loud), `modelspec.LoadRegistry(baseDir)`
  (absent `models.yaml` falls back to built-ins), `Registry.Resolve`, then
  the explicit perch-layer params check — every key in `Resolved.Params`
  other than `effort` is a loud error naming the offending key and the
  spec string (this is where `version` is rejected; perch has no Version
  field to thread it into). Unpack `Config.JudgeModel = Resolved.Model`,
  `Config.JudgeEffort = Resolved.Params["effort"]` (absent key → empty
  string → provider default, exactly today's semantics for the built-in
  default-free `haiku`). `Profile.validate`'s `profile > cfg > built-in`
  chain in `profile.go` is untouched — it now receives already-resolved
  cfg values.
  `config_test.go`: new/updated cases — (a) template-default `haiku`
  resolves to model `haiku`, empty effort (no models.yaml present);
  (b) a seeded `models.yaml` giving `sonnet` `defaults: {effort: medium}`
  makes `judge_model: sonnet` resolve to effort `medium` (write the
  models.yaml into the test dir via `hubgeometry.ConfigFile`-resolved
  path, honoring the Hub Geometry Invariant's in-test-code rule);
  (c) `judge_model: "sonnet[effort=high]"` — bracket beats registry
  default; (d) `judge_model: "sonnet[version=x]"` fails loud naming
  `version`; (e) an old-format file with a `judge_effort` key fails
  strict validation loud; (f) unknown alias fails loud.
  `doc.go`: update the configuration section — `perch.yaml` keys are now
  `judge_model` (model-spec string) and `round_caps`; describe the
  resolve-once-at-block-creation rule and the effort-only bracket-param
  restriction.
- **Commit:** `perch: migrate perch.yaml judge_model to model-spec notation`

### Card 13: profile files and CLI move to model-spec strings

- **Context:**
  - `internal/modelspec/parse.go`
  - `internal/modelspec/registry.go`
  - `internal/modelspec/load.go`
  - `internal/perchengine/config.go`
  - `internal/perchengine/profile.go`
  - `internal/perchengine/identity.go`
  - `internal/perchcli/cli_test.go`
  - `internal/perchcli/run_integration_test.go`
  - `internal/perchcli/cli_integration_test.go`
  - `docs/reference/model-spec.md`
- **Edits:**
  - `internal/perchcli/cli.go`
  - `internal/perchcli/run.go`
  - `internal/perchcli/run_test.go`
  - `tools/sandbox/SANDBOX-PERCH-SUITE.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  `run.go`: `profileYAML` DROPS the `JudgeEffort` (`judge-effort`) and
  `Effort` (`effort`) fields — strict `KnownFields(true)` decoding then
  makes old profiles carrying them fail loud with yaml's unknown-field
  error (the intended migration failure mode; no bespoke error text
  needed). `judge-model:` and `model:` stay string keys but now hold
  model-spec strings. `decodeProfile` gains a `modelspec.Registry`
  parameter and, for each NON-EMPTY spec string, runs Parse → Resolve →
  the same effort-only params check as card 12 (identical error shape),
  unpacking `Profile.JudgeModel/JudgeEffort` and `Profile.Model/Effort`
  from the resolved values; an empty string stays empty (defer to
  config/built-in resolution, unchanged semantics). This runs BEFORE
  `ProfileHash` is taken in `deriveBlockRunID`/`resolveRunTarget` (both
  untouched), so the hash covers resolved values — the accepted fail-loud
  resume consequence documented in the batch scope.
  `cli.go`: load the registry once in `PersistentPreRunE` via
  `modelspec.LoadRegistry(layout.Cwd)` (same anchor as the config loads),
  store it on `perchCLI`, thread it to `decodeProfile`.
  The `--model`/`--effort`/`--timeout` flags are UNCHANGED (plain provider
  overrides overlaid after hashing — out of scope per the discussion).
  Help text (`runCmd`'s `Long`): update the example profile — replace the
  `judge-model: haiku` + `judge-effort: ""` + `model: ""` + `effort: ""`
  lines with model-spec examples (e.g. `judge-model: haiku` and a
  commented `judge-model: "sonnet[effort=medium]"` variant, `model: ""`
  with a spec-string comment), and add a short paragraph: model fields use
  the model-spec notation; a bare alias picks up the `models.yaml` default
  effort; only `effort` is legal in brackets. The sandbox suite derives
  profiles from this help text (S0 ethos), so help accuracy is the
  review-obligation gate here (CLI/Cobra Invariant).
  `run_test.go`: migrate the two fixture profiles (`judge-model: haiku` +
  `judge-effort: low` becomes `judge-model: "haiku[effort=low]"` — the
  resolved Profile field values are identical, so the existing assertions
  stand); add cases — old split-key profile fails loud (unknown field
  `judge-effort`), unknown alias fails loud, `version` bracket param fails
  loud, bare `sonnet` with a seeded test models.yaml resolves the default
  effort into `Profile.JudgeEffort`.
  `SANDBOX-PERCH-SUITE.md`: update the one prose line describing perch's
  config ("judge model/effort" wording) to the model-spec reality; scan the
  suite's scenario steps for any literal `judge-effort`/`effort:` profile
  keys and migrate them (current scan says the profiles are derived from
  `--help`, so prose is the expected extent).
  `internal/perchcli/cli_test.go`, `run_integration_test.go`, and
  `cli_integration_test.go` are Context because the card must CONFIRM they
  carry no split-key fixtures or stale help pins (current scan says none);
  if the confirmation finds any, migrating them is in-scope for this card.
- **Commit:** `perch: migrate profile model keys to model-spec notation`

## Batch Tests

`verify:` adds `internal/configreg` to the standard scope: the strict
template change must keep configreg's registration/template tests green.
`internal/perchcli` covers the profile-schema migration and help text;
`internal/perchengine` covers config resolution; the treadle tree proves
the engine needed no change at all.
