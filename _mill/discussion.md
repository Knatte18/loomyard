# Discussion: Build modelspec - the model-spec parser + registry

```yaml
task: Build modelspec - the model-spec parser + registry
slug: modelspec
status: discussing
parent: main
```

## Problem

Every agent-spawning config in the stack (builder's roles, perch/burler reviewers and
judges, loom's producers) needs one precise notation to say which LLM runs a role. The
notation is already pinned as a contract in `docs/reference/model-spec.md`
(`<alias>[key=value,...]` plus the escape form `<provider>:<model-id>[...]`), but no code
implements it. Builder — the next milestone — is the first consumer, so the shared parser +
registry loader must exist before builder's config can be written. Carved out of the
builder task (2026-07-11) because it is stack-wide infrastructure deserving its own task,
plan, and review.

Two operator requirements sharpened during discussion:

1. **Deterministic effort** — the operator must be able to KNOW that `sonnet` resolves to
   the effort they decided in a config file they own, never Claude's floating default
   (which `~/.claude/settings.json` changes silently).
2. **New models without a new lyx.exe** — when Anthropic ships a brand-new model, a local
   user on an older `lyx.exe` must be able to adopt it via config alone (registry entry or
   escape form), with no recompile/redownload.

## Scope

**In:**

- New leaf library `internal/modelspec`: spec parser (`Parse`), registry type + built-in
  fallback set, `models.yaml` loader (`LoadRegistry`), resolution (`Resolve`) to
  `{engine, model, params}`.
- Register `models` in `internal/configreg` with a live-keys template (so `lyx config` /
  `lyx config reconcile` see it); update the pinned name list in `configreg_test.go`.
- Seed-only reconcile semantics for open-ended config files: a per-module flag in
  `configreg` (e.g. `Module.SeedOnly`) consumed by `internal/configsync` (and thereby
  `lyx init`, which reconciles via the same machinery): materialize the template when the
  file is absent; never rewrite when present.
- `version=` param realization in `internal/shuttleengine/claudeengine`: new
  `shuttleengine.Spec.Version` field (parallel to `Effort`), translation
  `sonnet`+`4.5` → `claude-sonnet-4-5` inside claudeengine only.
- Leaf-import enforcement test for modelspec + new `CONSTRAINTS.md` invariant entry.
- Doc updates (same commit as behaviour): `docs/reference/model-spec.md` contract
  amendments (add `fable` to the built-in set; generalize the version-translation rule to
  bare-word model values), `docs/overview.md` source-tree listing + shared-infrastructure
  line + shuttle bullet (Version knob), `docs/shared-libs/README.md` entry, as-built API in
  the package header comment.

**Out:**

- **Resolved-model-id recording in RunDir** (model-spec.md's reproducibility note) — lands
  with the first consumer (builder); requires capturing the CLI-resolved id from the
  session, a shuttle/claudeengine feature of its own. (Operator picked "include
  `version=` translation" but explicitly not this.)
- **Consumer migration** — builder consumes modelspec in its own task; perch's existing
  `judge_model`/`judge_effort` raw keys and any burler/loom config adopt the notation
  later. No call sites change in this task (beyond tests).
- **Cross-layer precedence** (whole-spec replacement between e.g. loom's config and
  builder.yaml) — the consumer's concern; modelspec resolves one spec against one registry.
- **Non-Claude engines** — gemini/openai engines are future work (explicitly not a current
  priority); modelspec only reserves the seam (engine name as dispatch key).
- **New CLI surface** — library package; no cobra command, no sandbox scenario.
- **`context` as a param** — explicitly not a parameter per the contract.

## Decisions

### Scope includes claudeengine `version=`, excludes RunDir recording

- Decision: Ship `internal/modelspec` complete AND implement the `version=` param
  end-to-end (new `shuttleengine.Spec.Version` + claudeengine translation). RunDir
  resolved-model-id recording stays out.
- Rationale: With `version=` realized, the notation is fully usable the day builder lands;
  RunDir recording is a separable observability feature needing session-event work.
- Rejected: leaf-only (notation not realizable end-to-end); full scope incl. RunDir
  (largest, drags in event-capture design).

### API shape — Parse/Resolve split

- Decision: `Parse(s string) (Spec, error)` handles grammar only. A `Registry` type with
  `Resolve(spec Spec) (Resolved, error)` handles alias lookup + default merging.
  `Resolved{Engine, Model string; Params map[string]string}`. Exported types: `Spec`
  (alias OR engine+model for escape form, plus bracket params), `Registry`, `Resolved`.
- Rationale: Consumers can grammar-check config at load time without a registry in hand;
  errors separate "bad syntax" from "unknown alias". (`modelspec.Spec` vs
  `shuttleengine.Spec` collide only in name, different packages — acceptable; naming is
  mill-plan's call if it prefers e.g. `ParsedSpec`.)
- Rejected: single-shot `Resolve(string, Registry)` only — smaller API but config-lint use
  would need a registry loaded first.

### Registry loading — direct read, not configengine.Load

- Decision: `LoadRegistry(baseDir string) (Registry, error)` reads
  `hubgeometry.ConfigFile(baseDir, "models")` directly with `gopkg.in/yaml.v3` strict
  decoding (`KnownFields`). Absent file → built-ins only (NOT an error). Present file →
  entries override/extend built-ins.
- Rationale: `configengine.Load` hard-errors on an absent file and validates against a
  fixed template key set — both wrong for models.yaml, which is optional and has
  open-ended alias keys. Direct read also keeps the leaf import set (no
  configengine/envsource/yamlengine deps). The same open-ended-keys objection would apply
  to configreg's default reconcile semantics too (template-as-schema pruning) — which is
  precisely why the registration decision below carries seed-only semantics: there the
  template is a one-time SEED, never a schema; no code path ever validates or prunes a
  present models.yaml against the template.
- Rejected: bending configengine.Load — heavier, breaks leaf discipline.

### configreg registration with a live-keys template, seed-only reconcile

- Decision: Register `models` in `configreg.Modules()` with `modelspec.ConfigTemplate`.
  The template ships LIVE entries carrying the operator-owned effort defaults (see
  built-ins decision below for values). Update the pinned list in
  `configreg_test.go:13` in the same commit (configcli's `Known modules:` help is
  generated from `Names()`, so it follows mechanically — verify help-tree tests).
  Registration carries **seed-only reconcile semantics** (new per-module flag in
  `configreg.Module`, consumed by `configsync.ReconcileAll` and thus by `lyx init`):
  reconcile **materializes the template when models.yaml is absent and never rewrites it
  when present**. After first materialization the file is operator-owned outright.
  Dry-run reporting for a present seed-only file shows it as in-sync/skipped (exact
  presentation is mill-plan's call).
- Rationale: Operator explicitly wants (a) discoverability via `lyx config` now and
  (b) deterministic, operator-owned effort defaults on every reconciled hub. A live-keys
  template delivers both. Seed-only is forced by reconcile's actual behaviour:
  `yamlengine.Reconcile` prunes every key absent from the template and
  `configsync.ReconcileAll` writes the pruned tree (pinned by
  `TestReconcileAll_DropsStaleMuxClaudeKey`) — under default semantics an operator-added
  alias (`zephyr:`) would be silently deleted by `lyx config reconcile --apply`, breaking
  the pinned new-model-without-recompile requirement. Seed-only also never resurrects a
  deliberately-deleted nested default (removing `sonnet.defaults.effort` to get the
  provider default sticks) — re-adding it would be exactly the silent config change the
  operator rejects. Trade-off accepted: new template aliases in a future lyx do not
  auto-propagate into an existing file; the operator adds them by hand or deletes the
  file and re-reconciles.
- Rejected: comments-only template (determinism only after a manual uncomment on each hub
  — forgetting silently falls back to Claude's floating default, the exact failure mode
  the operator rejects); no registration (invisible to `lyx config`, and no
  materialization on fresh hubs — the deterministic-effort file would need hand-creation
  everywhere); baking operator defaults into Go (recompile to change a default);
  default reconcile semantics (prunes operator-added aliases, see above);
  add-missing-never-remove semantics (keeps added aliases and propagates new template
  keys, but `MissingKeys`-based adding resurrects deliberately-deleted nested defaults).

### Built-in fallback set and template values

- Decision: Go built-ins are default-free pure fallback:
  `sonnet`/`opus`/`haiku`/`fable` → `{engine: claude, model: <same alias>}`, no
  `defaults`. The configreg template ships live entries:
  `sonnet: {engine: claude, model: sonnet, defaults: {effort: medium}}`,
  `opus: {..., defaults: {effort: high}}`,
  `haiku: {engine: claude, model: haiku}` (NO effort default — haiku does not support
  effort; the CLI would silently ignore it anyway),
  `fable: {engine: claude, model: fable, defaults: {effort: high}}`.
- Rationale: Built-ins must work with no file present (pinned contract) but stay
  unopinionated — effort policy belongs to the operator's file. `fable` added as a
  first-class alias (operator requirement); model value is the provider-side alias so
  newest-by-default applies. Adding fable extends the pinned built-in list in
  model-spec.md from `sonnet/opus/haiku` — a deliberate contract amendment in the same
  commit.
- Rejected: baking the doc-example efforts into Go built-ins (silent behavior change for
  every consumer vs provider defaults; recompile to change).

### models.yaml override granularity — whole-entry replacement

- Decision: A file entry for an alias replaces the built-in entry ENTIRELY (engine, model,
  defaults). A file entry missing `engine` or `model` → loud error.
- Rationale: Same philosophy as the contract's whole-spec-replacement precedence — no
  invisible field-level leakage from an overridden layer; one file states the whole truth
  for an alias it touches.
- Rejected: deep-merge onto built-ins (re-introduces "read two sources to know one value").

### Validation split — modelspec owns closed vocabularies, engines own values

- Decision: modelspec owns two closed sets as plain string sets (no imports):
  `knownParams = {effort, version}` and `knownEngines = {claude}`. It rejects unknown
  param keys (in brackets AND in registry `defaults:` maps) and unknown engine names
  (registry `engine:` fields AND the escape-form provider prefix). Param VALUES pass
  through untouched — the provider engine remains sole validator (claudeengine's existing
  `validateEffort` exact-lowercase check; its silent-ignore-on-mismatch CLI rationale).
- Rationale: Earliest possible fail-loud point for typos (`efort=high`,
  `engine: cluade`) per the contract; vocabularies are provider-invariant registry-shaped
  data, so owning them in the leaf costs no imports. Critically, these sets gate only
  engine names and param keys — NEVER model names/aliases — preserving the
  new-model-without-recompile requirement.
- Rejected: consumer-supplied vocabularies (every consumer repeats the sets, divergence
  risk); pass-through to engines (fails later, registry typos survive until launch).

### New-model-without-recompile (pinned requirement)

- Decision: An older `lyx.exe` must adopt a brand-new model via config alone. Guaranteed
  by four properties: (1) `models.yaml` EXTENDS the registry — a new alias entry
  (`zephyr: {engine: claude, model: claude-zephyr-1}`) just works because modelspec passes
  model strings through unvalidated; (2) the escape form
  (`claude:claude-zephyr-1[effort=high]`) needs zero config edits; (3) the closed
  vocabularies never gate model names (previous decision); (4) the version-translation
  rule is generic, not a closed alias list (next decision); (5) `lyx config reconcile`
  never prunes an operator-added alias — models.yaml is seed-only for reconcile (see
  configreg decision).
- Rationale: Operator requirement, stated explicitly. Known residual: a brand-new EFFORT
  tier would still require a new exe (claudeengine's `validEfforts` value set is
  hardcoded, pre-existing deliberate behavior) — accepted, out of scope.
- Rejected: any design where claudeengine or modelspec keeps a closed list of model
  aliases for translation or validation.

### shuttle channel — first-class `Spec.Version` field

- Decision: Add `Version string` to `shuttleengine.Spec`, exactly parallel to `Effort`:
  `Spec.validate` never inspects it (neither defaults nor rejects); the engine is sole
  validator/realizer. Empty = no version pin. claudeengine's `Prepare` consumes it before
  command construction — `buildLaunchCmd`'s signature/behaviour stays as-is, receiving the
  final translated model string in its existing `model` parameter.
- Rationale: Follows the proven `Effort` pattern (documented in spec.go and
  claudeengine); keeps `internal/shuttleengine` provider-invariant per the Shuttle
  Provider-Seam Invariant.
- Rejected: generic `Spec.Params map[string]string` (open-ended bag where shuttle today
  has documented knobs; YAGNI until a third param exists).

### claudeengine translation rule — generic bare-word composition

- Decision: In claudeengine, when `Version` is non-empty: if the model value is a single
  bare word containing no dash (`sonnet`, `fable`, `zephyr` — matches the alias-shaped
  grammar), compose `claude-<word>-<version with "." → "-">` (e.g. `sonnet`+`4.5` →
  `claude-sonnet-4-5`, `fable`+`5` → `claude-fable-5`). If the model value contains a dash
  (a full id, e.g. from the escape form): hard error — the id already pins the version, a
  second pin is a contradiction. If `Version` is set and model is empty: hard error
  (nothing to compose against). An ill-formed composition for a nonsense word fails loudly
  at the Claude CLI at launch.
- Rationale: A closed translation set `{sonnet, opus, haiku}` would require a recompile
  for every new alias — violating the new-model requirement. The generic rule needs no
  list. Translation lives ONLY in claudeengine per the Provider-Seam Invariant (Claude's
  id scheme is provider knowledge). model-spec.md's translation sentence is amended to
  state the generic rule.
- Rejected: closed alias set (recompile treadmill); best-effort composition with full ids
  (silently produces garbage ids, violates fail-loud).

### Strict grammar

- Decision: Case-sensitive, zero whitespace tolerance anywhere in a spec string (a space
  → error naming the offending character). Alias, param keys, engine names: `[a-z0-9-]+`
  (lowercase). Escape-form model-id position additionally allows dots and underscores:
  `[a-z0-9._-]+`. Param values: `[a-z0-9._-]+`, non-empty. Empty bracket `sonnet[]` →
  error. Duplicate param key in one bracket → error. Empty value (`effort=`) → error.
  Escape form is detected by the presence of `:` (exactly one allowed).
- Rationale: Specs are YAML scalars written by operators; strictness costs one retype and
  buys an unambiguous contract — same spirit as claudeengine's exact-lowercase effort
  check. Registry `model:` VALUES in models.yaml are free-form strings (passed through;
  not spec-grammar-constrained).
- Rejected: tolerant trimming (two spellings for every spec, fuzzier contract).

### Resolution semantics

- Decision: Alias form: look up alias in registry (unknown alias → loud error);
  `Resolved.Params` = registry `defaults` overlaid by bracket params (bracket wins per
  key — the contract's "bracket param > registry default"). Escape form: no registry
  lookup; `Engine` = prefix (must be in `knownEngines`), `Model` = id verbatim, `Params` =
  bracket only. Consumers map `Resolved` onto `shuttleengine.Spec` themselves
  (`Model=Resolved.Model`, `Effort=Params["effort"]`, `Version=Params["version"]`) — that
  mapping is documented in the package header, not wrapped in a helper (YAGNI until
  builder shows the shape).
- Rationale: Direct transcription of the pinned contract; one spec + one registry in, one
  resolved triple out.
- Rejected: modelspec importing shuttleengine to return a ready `shuttleengine.Spec`
  (breaks leaf; couples the notation to one consumer).

### Leaf discipline — enforced, recorded in CONSTRAINTS.md

- Decision: `internal/modelspec` production code imports ONLY stdlib +
  `internal/hubgeometry` + `gopkg.in/yaml.v3`. Enforced by a
  `leaf_enforcement_test.go` in the package, mirroring
  `internal/lyxtest/leaf_enforcement_test.go`. Recorded as a new "Modelspec Leaf
  Invariant" entry in `CONSTRAINTS.md` in the same commit (per the CONSTRAINTS.md
  update rule). `configreg` imports modelspec (for `ConfigTemplate`) — that direction is
  fine; modelspec never imports configreg or any feature package.
- Rationale: Task's stated import discipline ("every future consumer — perch/burler/loom —
  can import it without cycles"), enforced the way this repo already enforces lyxtest's
  identical property.
- Rejected: review-obligation-only (the repo's precedent is a machine check; it is cheap).

## Technical context

- **The pinned contract**: `docs/reference/model-spec.md` — grammar, registry shape,
  newest-by-default/pinning, whole-spec-replacement precedence, fail-loud, `context` is
  not a param, provider seam, roles list. Two amendments land with this task: built-in set
  gains `fable`; version-translation sentence generalized to the bare-word rule.
- **`shuttleengine.Spec`** (`internal/shuttleengine/spec.go`): has `Model`/`Effort`
  strings; `Effort`'s field comment documents the exact "engine is sole validator"
  pattern `Version` must copy. `validate` normalizes in place; it must NOT touch
  `Version`.
- **claudeengine** (`internal/shuttleengine/claudeengine/command.go`): `validEfforts`
  set + `validateEffort` (exact-lowercase, hard error, rationale comments);
  `buildLaunchCmd(sh, bin, promptPath, settingsPath, sessionID, model, effort, interactive)`
  appends `--model`/`--effort` only when non-empty, everything quoted via
  `internal/shell`. Version translation slots in before command construction (in
  `Prepare`, where `validateEffort` already runs) so `buildLaunchCmd` is untouched.
- **`hubgeometry.ConfigFile(baseDir, module)`** → `<baseDir>/_lyx/config/<module>.yaml`.
  The Hub Geometry Invariant requires this helper for models.yaml path construction — in
  test code too (`TestEnforcement_GeometryLiterals` machine-enforces).
- **`configreg`** (`internal/configreg/configreg.go`): `Modules()` is the ordered
  registry; `configreg_test.go:13` pins the name list
  (`board, mux, perch, shuttle, warp, weft`) — add `models` (alphabetical position:
  between `board` and `mux`). configcli help + reconcile pick it up mechanically.
  Check `cmd/lyx` help-tree/drift tests for any pinned help text containing the module
  list.
- **`configengine.Load`** hard-errors on absent file + missing template keys — the reason
  modelspec loads directly. modelspec does NOT use envsource resolution (no env
  interpolation in models.yaml — keep it dumb data; if someone needs it later that is a
  deliberate extension).
- **Reconcile behaviour (verified)**: default reconcile is NOT add-missing-only —
  `yamlengine.Reconcile` marshals the template tree with existing values and reports
  every existing leaf-path absent from the template as `removed`;
  `configsync.ReconcileAll` writes that pruned result when applying
  (`TestReconcileAll_DropsStaleMuxClaudeKey` pins the drop). This is why `models` gets
  the seed-only flag: `ReconcileAll` must skip the reconcile/prune pass entirely for a
  present seed-only file and only materialize when absent. `lyx init` reconciles via the
  same machinery, so the flag covers both entry points.
- **perchengine** (`internal/perchengine/config.go`): `judge_model`/`judge_effort` raw
  keys — the shape of the future adopter; unchanged in this task.
- **yaml dependency**: `gopkg.in/yaml.v3` already in go.mod. Strict decode via
  `yaml.Decoder.KnownFields(true)` against the entry struct.
- **Docs placement**: overview.md lists libraries in the source-tree block (~line 183)
  and the shared-infrastructure line (~line 274) referencing
  `docs/shared-libs/README.md`; the shuttle bullet (~line 236) documents Model/Effort
  knobs and gains Version. No `docs/modules/modelspec.md` — as-built API lives in the
  package header comment (per task + Documentation Lifecycle).

## Constraints

From `CONSTRAINTS.md` (read in full this session):

- **Hub Geometry Invariant** — models.yaml path only via
  `hubgeometry.ConfigFile(baseDir, "models")`, tests included; no geometry tokens in
  modelspec.
- **lyxtest Leaf Invariant** — untouched; modelspec tests may use `t.TempDir` fixtures
  directly (and may import lyxtest if useful — lyxtest's own imports are the constrained
  direction).
- **CLI / Cobra Invariant** — no new command. configreg addition may ripple into pinned
  help expectations (`Known modules:` is generated from `Names()` — mechanical, but
  re-check help-tree test fixtures). Errors-are-JSON does not apply (library returns
  `error` values; consumers wrap).
- **Shuttle Provider-Seam Invariant** — `Version` field in provider-invariant
  `shuttleengine` carries opaque provider vocabulary (like `Effort`); translation and
  rejection live only in claudeengine. `seam_enforcement_test.go` must stay green.
- **Sandbox Suite Coverage** — no new cobra module ⇒ no scenario/allowlist change
  (coverage is enumerated from cobra root).
- **Documentation Lifecycle / task-completion rule** — docs updated in the same commit as
  behaviour (see Scope/In). Roadmap: mark the modelspec milestone if listed
  (`docs/roadmap.md` carved builder/modelspec — check and mark per the roadmap rule).
- **New invariant added by this task** — "Modelspec Leaf Invariant" (stdlib + hubgeometry
  + yaml only), machine-enforced by a leaf test, recorded in CONSTRAINTS.md same commit.

## Testing

TDD candidates, table-driven throughout (Q11 → option 1):

- **`Parse` (TDD)** — tables covering every strict-grammar rule: valid alias form, valid
  bracket, valid escape form; rejections: whitespace (each position), uppercase, empty
  bracket, duplicate key, empty value, missing `]`, two colons, empty alias, unknown-shape
  characters. Error messages name the offending token/character.
- **`Resolve` (TDD)** — bracket>default precedence per key; whole-entry replacement
  (file entry overriding built-in `sonnet` drops built-in fields); unknown alias, unknown
  param key (bracket and registry defaults), unknown engine (registry and escape prefix)
  all fail loudly; escape form bypasses registry; built-ins present with zero-value
  registry.
- **`LoadRegistry`** — `t.TempDir` fixtures, paths via `hubgeometry.ConfigFile` (Hub
  Geometry Invariant applies to tests); absent file → built-ins; file extends; file
  overrides whole-entry; entry missing engine/model → error; unknown YAML keys in an
  entry → error (strict decode); malformed YAML → error naming path.
- **claudeengine version translation** — tables mirroring the `validateEffort` test
  style: bare word + version composes (dots→dashes, incl. `fable`+`5`); dashed
  model + version → hard error; version without model → hard error; empty version →
  pass-through untouched; translated value lands in `--model` (existing
  `buildLaunchCmd`/Prepare test seams).
- **`Spec.Version` passthrough** — shuttleengine test confirming `validate` ignores it
  (mirrors existing Effort-ignored test in `spec_test.go`).
- **Leaf enforcement** — `leaf_enforcement_test.go` fails on any import outside
  stdlib/hubgeometry/yaml.v3.
- **configreg/configsync** — pinned `TestNames` updated; template function returns the
  live-keys template; seed-only semantics: reconcile `--apply` materializes models.yaml
  when absent; on a present file, an operator-ADDED alias entry (`zephyr:`) AND an
  operator-REMOVED nested default (`sonnet.defaults.effort` deleted) both survive a
  re-reconcile untouched.
- **No sandbox scenario** — library package, no cobra surface.

## Q&A log

- **Q:** Does this task build the claudeengine side (version translation, RunDir
  recording)? **A:** Version translation yes (with `Spec.Version` channel); RunDir
  resolved-model-id recording no — lands with builder.
- **Q:** Parse/Resolve split or single-shot? **A:** Split; both exported.
- **Q:** Registry loading via configengine.Load? **A:** No — direct read via
  hubgeometry.ConfigFile + strict yaml.v3; absent file is not an error.
- **Q:** Register models.yaml in configreg? **A:** Yes, now ("why not right away?") —
  with a LIVE-keys template after the effort-determinism requirement emerged (comments-only
  was considered and rejected: forgetting to uncomment silently falls back to Claude's
  floating default).
- **Q:** Who validates what? **A:** modelspec owns closed sets {effort,version} /
  {claude} for param keys and engine names — never model names; engines validate values.
- **Q:** models.yaml override granularity? **A:** Whole-entry replacement per alias;
  partial entry → loud error.
- **Q:** Grammar strictness? **A:** Strict: case-sensitive, no whitespace, no empty
  bracket/value, no duplicate keys.
- **Q:** Built-in defaults? **A:** None in Go (pure fallback) — BUT operator requires
  deterministic effort from a file they own, hence live-keys template with
  sonnet=medium, opus=high, fable=high, haiku=none (haiku doesn't support effort).
- **Q:** Fable? **A:** First-class alias in built-ins AND template (`model: fable`,
  newest-by-default); model-spec.md built-in list amended.
- **Q:** Brand-new Anthropic model on an old lyx.exe? **A:** Must work via config alone —
  pinned as a requirement; registry extension + escape form + no closed model-name lists +
  generic bare-word version translation guarantee it. Residual: a new effort TIER still
  needs a new exe (pre-existing validEfforts hardcoding, accepted).
- **Q:** Where does per-provider support live (Gemini/OpenAI differ)? **A:** In provider
  engines under `internal/shuttleengine/` per the Provider-Seam Invariant — modelspec
  stays provider-invariant; no per-provider modules inside modelspec, no separate
  top-level module.
- **Q:** Confirmation of final package (incl. fable effort=high template default)?
  **A:** Confirmed (option 1).
- **Q:** (Review r1 GAP) Default reconcile prunes keys absent from the template — an
  operator-added alias would be deleted, breaking new-model-without-recompile. How do
  operator entries survive? **A:** Seed-only reconcile semantics for `models`
  (per-module flag in configreg, honored by configsync/init): materialize when absent,
  never rewrite when present; tests assert added alias and removed default survive.
  Rejected: add-missing-never-remove (resurrects deliberately-deleted defaults);
  dropping registration (no materialization on fresh hubs).
