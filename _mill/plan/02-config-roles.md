# Batch: config-roles

```yaml
task: "Build builder - the batch-implementation loop"
batch: "config-roles"
number: 2
cards: 5
verify: go test ./internal/builderengine/... ./internal/configreg/...
depends-on: []
```

## Batch Scope

builder.yaml: the `Config` type with the four model-spec roles and the numeric knobs,
the configengine-backed loader, the seeded template, configreg registration, and the
role-resolution pre-flight that maps role names to `modelspec.Resolved` values. Also
carries the pinned doc rename `fixer` → `recovery` so the docs and the config vocabulary
land together. External interface consumed later: `Config`, `LoadConfig`,
`ResolveRoles`, `RoleSpec` constants.

## Cards

### Card 7: builder Config and LoadConfig

- **Context:**
  - `internal/perchengine/config.go`
  - `internal/modelspec/modelspec.go`
  - `_mill/discussion.md`
- **Creates:**
  - `internal/builderengine/config.go`
  - `internal/builderengine/config_test.go`
- **Edits:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** `Config` mirrors builder.yaml:
  `Orchestrator, Implementer, ImplementerOversized, Recovery string` (yaml keys
  `orchestrator`, `implementer`, `implementer_oversized`, `recovery` — each a
  model-spec string), `SelfFixCap int` (`self_fix_cap`), `PollWaitS int`
  (`poll_wait_s`), `BatchTimeoutMin int` (`batch_timeout_min`),
  `OrchestratorTimeoutMin int` (`orchestrator_timeout_min`),
  `BatchContextCapTokens int` (`batch_context_cap_tokens`), `BatchCardCap int`
  (`batch_card_cap`). `LoadConfig(baseDir, module string) (Config, error)` copies
  `perchengine.LoadConfig`'s shape exactly: `configengine.Load(baseDir, module,
  []byte(ConfigTemplate()))`, the `not initialized here; run "lyx init"` rewrap, then
  `yaml.Unmarshal`. After unmarshal, `modelspec.Parse` each of the four role strings
  and fail loud on a grammar error naming the offending key. Tests use
  `lyxtest.SeedConfig` for the file variants (defaults-only, overrides, bad role
  grammar).
- **Commit:** `feat(builder): builder.yaml Config and loader`

### Card 8: config template

- **Context:**
  - `internal/perchengine/template.go`
  - `internal/perchengine/template.yaml`
  - `internal/modelspec/template.yaml`
- **Creates:**
  - `internal/builderengine/template.go`
  - `internal/builderengine/template.yaml`
  - `internal/builderengine/template_test.go`
- **Edits:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** `template.yaml`: seeded defaults with one comment line per key —
  `orchestrator: sonnet`, `implementer: sonnet`, `implementer_oversized: sonnet`
  (comment: point at a large-window variant alias from models.yaml when one exists;
  context size is a model property, never a parameter — model-spec.md),
  `recovery: opus[effort=high]`, `self_fix_cap: 2`, `poll_wait_s: 480`,
  `batch_timeout_min: 60`, `orchestrator_timeout_min: 480`,
  `batch_context_cap_tokens: 100000`, `batch_card_cap: 10`. `template.go` mirrors
  `perchengine/template.go` (`//go:embed template.yaml`, `ConfigTemplate() string`).
  `template_test.go`: the template parses as YAML, round-trips through `LoadConfig`
  into the documented defaults, and every `Config` yaml tag appears in the template.
- **Commit:** `feat(builder): seeded builder.yaml template`

### Card 9: role resolution pre-flight

- **Context:**
  - `internal/modelspec/modelspec.go`
  - `internal/modelspec/load.go`
  - `internal/modelspec/registry.go`
  - `_mill/discussion.md`
- **Creates:**
  - `internal/builderengine/roles.go`
  - `internal/builderengine/roles_test.go`
- **Edits:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Define `Role` as a string enum with constants `RoleOrchestrator`,
  `RoleImplementer`, `RoleImplementerOversized`, `RoleRecovery` (values
  `"orchestrator"`, `"implementer"`, `"implementer_oversized"`, `"recovery"`).
  `ResolveRoles(cfg Config, reg modelspec.Registry) (map[Role]modelspec.Resolved,
  error)`: `modelspec.Parse` + `reg.Resolve` for all four roles, wrapping any error
  with the role name; this is the fail-pre-flight surface `run`/`spawn-batch` call at
  entry (discussion: a typo'd alias fails before any agent spawns). Add
  `SpecForRole(resolved modelspec.Resolved, ...)`? — NO: the Resolved→shuttle.Spec
  field mapping happens at spawn sites (batch 5) per modelspec's package-doc mapping;
  this card only resolves and returns. Tests: unknown alias fails naming the role;
  escape-form specs pass without a registry entry; bracket params survive into
  `Resolved.Params`.
- **Commit:** `feat(builder): role resolution pre-flight via modelspec`

### Card 10: configreg registration

- **Context:**
  - `internal/builderengine/template.go`
- **Edits:**
  - `internal/configreg/configreg.go`
  - `internal/configreg/configreg_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add `{Name: "builder", Template: builderengine.ConfigTemplate}` to
  the module slice in `configreg.go` (alphabetical position: after `board`, before
  `models`) with the matching import. Update `configreg_test.go`'s pinned `want` list
  to `[]string{"board", "builder", "models", "mux", "perch", "shuttle", "warp",
  "weft"}`.
- **Commit:** `feat(builder): register builder config template in configreg`

### Card 11: doc rename fixer -> recovery

- **Context:**
  - `_mill/discussion.md`
- **Edits:**
  - `docs/reference/model-spec.md`
  - `docs/modules/plan-format.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Rename the builder role `fixer` to `recovery` in both docs: in
  `model-spec.md`, the "Roles that use this notation" section's role list; in
  `plan-format.md`, the "Roles and models" section's role list. Both places keep the
  parenthetical role descriptions intact and gain no other wording changes. Rationale
  (recorded in the discussion): `recovery` names the escalated recovery spawn;
  `fixer` collided with burler's B-phase fixer.
- **Commit:** `docs(builder): rename builder role fixer to recovery`

## Batch Tests

`verify:` runs builderengine (config load/template/role-resolution tests) plus
configreg's pinned-names test, which fails until card 10's registration is consistent.
