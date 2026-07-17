# Batch: webster-foundation

```yaml
task: 'Master Builder: new, parallel fork-based implementation module'
batch: 'webster-foundation'
number: 2
cards: 6
verify: go test ./internal/websterengine/... ./internal/hubgeometry/... ./internal/configreg/...
depends-on: [1]
```

## Batch Scope

The `websterengine` package skeleton and every foundation piece with no
behaviour of its own: hubgeometry path helpers for `_lyx/webster/`, the package
design doc, config (`webster.yaml` template, `Config`, `LoadConfig`), webster's
own roles + `ResolveRoles`, the state schema (with persisted digests and
transcript attribution), and configreg registration. External interface the
rest of the plan consumes: `hubgeometry.WebsterDir`/`WebsterReportsDir`/
`WebsterPromptsDir`, `websterengine.Config`/`LoadConfig`/`ConfigTemplate`,
`Role`/`ResolveRoles`, `State`/`BatchState`/`LoadState`/`SaveState`/
`AcquireStateMutation`.

## Cards

### Card 6: hubgeometry webster dirs

- **Context:**
  - `internal/hubgeometry/enforcement_test.go`
- **Edits:**
  - `internal/hubgeometry/hubgeometry.go`
- **Creates:**
  - `internal/hubgeometry/webstergeom_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add three accessors following the `BuilderDir`/
  `BuilderReportsDir` shape exactly:
  `WebsterDir(baseDir string) string` → `filepath.Join(baseDir, LyxDirName, "webster")`;
  `WebsterReportsDir(baseDir string) string` → `filepath.Join(WebsterDir(baseDir), "reports")`;
  `WebsterPromptsDir(baseDir string) string` → `filepath.Join(WebsterDir(baseDir), "prompts")`.
  Godoc each (state/reports/rendered-fork-prompt homes; prompts are
  machine-local re-renderable artifacts excluded from weft commits). The
  `"webster"` literal is legal here — hubgeometry is the sole owner of `_lyx`
  path construction and `webster` is not a geometry token, so
  `TestEnforcement_GeometryLiterals` needs no change. New test pins all three
  joins.
- **Commit:** `webster: add hubgeometry accessors for _lyx/webster`

### Card 7: websterengine package doc

- **Context:**
  - `internal/builderengine/doc.go`
  - `_mill/discussion.md`
- **Edits:** none
- **Creates:**
  - `internal/websterengine/doc.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Package doc for `websterengine` — the durable design
  statement (Documentation Lifecycle: this is where webster's design lives, no
  module doc file). Must state: fork-based sibling of `builder`, A/B
  contract-compatible (same plan-format input via `builderengine.ParsePlan`,
  compatible `outcome.yaml`, same batch-report schema); Master session forks
  one implementer per batch in-session, bracket verbs `begin-batch`/
  `record-batch` around each fork (Go gates only run when called — template
  discipline + fail-loud audit, not a security boundary); re-entrant
  `recover-batch` cold-strand escalation; idempotent per-batch model assertion
  via `Runner.Inject`; digest persistence in `BatchState.Digest`; the
  engine/cli split (engine weft-blind, all weft git in webstercli); crash/
  resume = fresh Master re-drives the first unreported batch. Mirror
  `builderengine/doc.go`'s tone and density.
- **Commit:** `webster: add websterengine package with design doc`

### Card 8: webster.yaml config

- **Context:**
  - `internal/builderengine/template.yaml`
  - `internal/builderengine/template.go`
  - `internal/builderengine/config.go`
  - `internal/configengine/config.go`
- **Edits:** none
- **Creates:**
  - `internal/websterengine/template.yaml`
  - `internal/websterengine/template.go`
  - `internal/websterengine/config.go`
  - `internal/websterengine/config_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** `template.yaml` seeds: `master: sonnet`,
  `master_oversized: opus`, `recovery: "opus[effort=high]"`,
  `self_fix_cap: 2`, `master_timeout_min: 480`, `recovery_timeout_min: 60`,
  `poll_wait_s: 480`, `batch_context_cap_tokens: 100000`,
  `batch_card_cap: 10` — each with a comment; `master_timeout_min`'s comment
  must state it is the WHOLE-RUN timeout (the `orchestrator_timeout_min`
  analog, spans every batch), never per-batch, and `master_oversized`'s that
  forks inherit the session's model so this is what oversized batches run at.
  `template.go`: `//go:embed template.yaml` + `func ConfigTemplate() string`
  (mirror builder's). `config.go`: `Config` struct mirroring the yaml,
  `LoadConfig(baseDir, module string) (Config, error)` calling
  `configengine.Load(baseDir, module, []byte(ConfigTemplate()))` and
  validating the three role strings' grammar via `modelspec.Parse`.
  `config_test.go` mirrors builder's config-template tests: parses as YAML,
  round-trips through `LoadConfig`, contains every yaml tag of `Config`.
- **Commit:** `webster: add webster.yaml config template and LoadConfig`

### Card 9: webster roles

- **Context:**
  - `internal/builderengine/roles.go`
  - `internal/modelspec/modelspec.go`
  - `internal/modelspec/registry.go`
- **Edits:** none
- **Creates:**
  - `internal/websterengine/roles.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** `type Role string` with exactly three consts:
  `RoleMaster = "master"`, `RoleMasterOversized = "master_oversized"`,
  `RoleRecovery = "recovery"`.
  `ResolveRoles(cfg Config, reg modelspec.Registry) (map[Role]modelspec.Resolved, error)`
  parsing+resolving each of the three config strings (mirror builder's
  `ResolveRoles` incl. the doc note that the Resolved→Spec field mapping —
  `Model`/`Params["effort"]`/`Params["version"]` — happens at spawn/inject
  sites). Godoc must state why there are no per-fork implementer roles: forks
  always inherit the Master session's model, so batch-level model choice is
  expressed by switching the Master (`RoleMasterOversized`), and only the cold
  recovery strand carries its own role.
- **Commit:** `webster: define master/master_oversized/recovery roles`

### Card 10: webster state schema

- **Context:**
  - `internal/builderengine/state.go`
  - `internal/state/state.go`
  - `internal/lock/lock.go`
  - `internal/builderengine/digest.go`
- **Edits:** none
- **Creates:**
  - `internal/websterengine/state.go`
  - `internal/websterengine/state_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** `State` struct: `RunGUID`, `PlanFingerprint`,
  `CurrentBatch int`, `MasterStrand string`, `MasterSessionID string`,
  `AssertedModel string` (the model last injected/launched for the Master
  session — the idempotent-assertion memory), `Batches map[int]*BatchState`,
  `ChainStartSHAs map[int]string`, `SeenForkTranscripts []string` (every
  subagent transcript path already attributed, across all batches).
  `BatchState`: `Slug`, `StartSHA`, `Kind string` (`"fork"` | `"recovery"`),
  `SpawnedAt`, `Terminal bool`, `Status string`,
  `Digest *builderengine.Digest` (persisted at terminal classification — the
  carry-forward home for `begin-batch(N+1)` prompt rendering and resume
  progress), `ForkTranscripts []string` (transcripts attributed to this
  batch), and — recovery only — `StrandGUID`, `ShuttleRunDir`, `EventsPath`.
  JSON tags mirror builder's camelCase style. `LoadState(websterDir)` /
  `SaveState(websterDir, st)` via `state.ReadJSON`/`WriteJSON` with
  `state.json` + `state.json.lock`;
  `AcquireStateMutation(websterDir) (*lock.FileLock, error)` blocking on
  `mutate.lock` (mirror builder's lease doc: held across every
  read-modify-write). Tests: round-trip, missing-file → `(nil, nil)`, digest
  persistence survives save/load.
- **Commit:** `webster: add state schema with persisted digests and attribution`

### Card 11: configreg registration

- **Context:**
  - `internal/websterengine/template.go`
- **Edits:**
  - `internal/configreg/configreg.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Import `internal/websterengine` and add
  `{Name: "webster", Template: websterengine.ConfigTemplate}` to the
  `Modules()` slice, keeping the slice's existing ordering convention. This is
  what makes `lyx config reconcile` materialize `_lyx/config/webster.yaml`.
- **Commit:** `webster: register webster.yaml in configreg`

## Batch Tests

`go test ./internal/websterengine/... ./internal/hubgeometry/... ./internal/configreg/...`
— every package this batch touches. New tests: geometry joins, config-template
round-trip/tag coverage, state round-trip with digest persistence. All untagged
and spawn-free (state tests use `t.TempDir()` plain files) — Tier 1 clean.
`configreg`'s existing tests guard the registration shape.
