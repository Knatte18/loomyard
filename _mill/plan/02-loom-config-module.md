# Batch: loom-config-module

```yaml
task: 'loom: Discussion producer (interactive interview, auto-mode capable)'
batch: loom-config-module
number: 2
cards: 5
verify: go test ./internal/loomengine/ ./internal/configreg/ ./internal/initengine/ ./internal/configcli/
depends-on: []
```

## Batch Scope

Introduce `loom.yaml` as a new config module, mirroring `internal/builderengine`'s
config shape exactly. Delivers: a `template.yaml` seed with the `discussion` role
model-spec and the `discussion_timeout_min` knob; a `ConfigTemplate()` embed
accessor; a `Config` type + `LoadConfig` that validates the role model-spec
grammar via `modelspec.Parse` at load; and registration in `configreg` so
`lyx init` / `lyx config reconcile` materialize `loom.yaml`. External interface
batch 3 consumes: `loomengine.Config` (its `Discussion` and `DiscussionTimeoutMin`
fields) and `loomengine.LoadConfig`. All new code is in `package loomengine`; the
package already exists (Preflight). This batch is independent of batch 1.

## Cards

### Card 3: loom.yaml seed template + embed accessor

- **Context:**
  - `internal/builderengine/template.yaml`
  - `internal/builderengine/template.go`
  - `docs/reference/model-spec.md`
- **Edits:** none
- **Creates:**
  - `internal/loomengine/template.yaml`
  - `internal/loomengine/configtemplate.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  - `template.yaml`: two keys with trailing inline comments, mirroring
    `builderengine/template.yaml`'s style —
    `discussion: opus[effort=high]  # model-spec for the discussion-phase interview agent (see docs/reference/model-spec.md)`
    and
    `discussion_timeout_min: 480  # minutes the discussion agent's shuttle run is allowed to run (interactive interviews run long)`.
  - `configtemplate.go`: `package loomengine`, a file-level doc comment like
    `builderengine/template.go`'s banner (scoped to just the config template),
    `import _ "embed"`, `//go:embed template.yaml` bound to a package var
    `var configTemplate string`, and an exported `func ConfigTemplate() string`
    returning it. Do NOT embed `discussion-template.md` here — that asset does not
    exist until batch 3 and embedding it now would fail the build; the prompt
    embed gets its own file in batch 3.
- **Commit:** `feat(loom): add loom.yaml config template and embed accessor`

### Card 4: loomengine Config type + LoadConfig

- **Context:**
  - `internal/builderengine/config.go`
  - `internal/loomengine/template.yaml`
  - `internal/configengine/configengine.go`
  - `internal/modelspec/parse.go`
- **Edits:** none
- **Creates:**
  - `internal/loomengine/config.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** `package loomengine`, a file-level doc comment mirroring
  `builderengine/config.go`'s banner. Define
  `type Config struct { Discussion string \`yaml:"discussion"\`; DiscussionTimeoutMin int \`yaml:"discussion_timeout_min"\` }`
  with a godoc comment per field. Define
  `func LoadConfig(baseDir, module string) (Config, error)` copying
  `builderengine.LoadConfig`'s body shape exactly: call
  `configengine.Load(baseDir, module, []byte(ConfigTemplate()))`; on an error
  containing `"not initialized"` return
  `fmt.Errorf("not initialized here; run \"lyx init\"")`; `yaml.Unmarshal` the
  resolved bytes (wrap failure as `unmarshal loom config: %w`); then validate the
  `Discussion` role's grammar via `modelspec.Parse(cfg.Discussion)`, wrapping a
  failure as `fmt.Errorf("loom config key %q: %w", "discussion", err)`. Do NOT
  resolve the spec against a registry here (that is batch 3's factory job, mirroring
  `builderengine`'s Parse-at-load / Resolve-at-spawn split). Imports: `fmt`,
  `strings`, `configengine`, `modelspec`, `gopkg.in/yaml.v3`.
- **Commit:** `feat(loom): add loomengine Config and LoadConfig`

### Card 5: Register loom in configreg + fix module-list test

- **Context:**
  - `internal/loomengine/configtemplate.go`
- **Edits:**
  - `internal/configreg/configreg.go`
  - `internal/configreg/configreg_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  - `configreg.go`: add `"github.com/Knatte18/loomyard/internal/loomengine"` to the
    import block (alphabetical position: after `internal/burlerengine`, before
    `internal/modelspec`) and add
    `{Name: "loom", Template: loomengine.ConfigTemplate},` to the `Modules()`
    slice in its alphabetical slot — between the `burler` entry and the `models`
    entry. Do NOT set `SeedOnly` (loom.yaml is a normal reconciled module, not
    operator-owned like models/burler).
  - `configreg_test.go`: update `TestNames`'s `want` slice to
    `[]string{"board", "builder", "burler", "loom", "models", "mux", "perch", "shuttle", "warp", "weft"}`
    (insert `"loom"` after `"burler"`). Leave `TestModules_SeedOnly` unchanged —
    its `want := m.Name == "models" || m.Name == "burler"` already yields the
    correct `false` for `loom`.
- **Commit:** `feat(configreg): register loom config module`

### Card 6: LoadConfig unit tests

- **Context:**
  - `internal/loomengine/config.go`
  - `internal/loomengine/configtemplate.go`
  - `internal/loomengine/testmain_test.go`
  - `internal/builderengine/config_test.go`
- **Edits:** none
- **Creates:**
  - `internal/loomengine/config_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** New `package loomengine` test file mirroring
  `builderengine/config_test.go`'s approach. Cover: (a) a well-formed
  `_lyx/config/loom.yaml` (materialized from `ConfigTemplate()` into a temp
  `baseDir` with the `_lyx/config/` layout `configengine.Load` expects) loads and
  yields `Discussion == "opus[effort=high]"` and `DiscussionTimeoutMin == 480`;
  (b) a `loom.yaml` whose `discussion:` value is a malformed model-spec (e.g.
  `discussion: "opus[effort"` — unclosed bracket) makes `LoadConfig` fail with an
  error naming the `"discussion"` key; (c) a missing `_lyx/` directory yields the
  `not initialized here; run "lyx init"` error. Reuse whatever temp-dir /
  config-seeding helper `builderengine/config_test.go` uses (read it to match the
  exact `configengine.Load` baseDir layout — `_lyx/config/<module>.yaml`); if that
  helper is package-private to builderengine, replicate the minimal file-writing
  inline. Do not depend on a live hub.
- **Commit:** `test(loom): cover loomengine LoadConfig`

### Card 7: Document the loom config module

- **Context:**
  - `internal/loomengine/template.yaml`
- **Edits:**
  - `docs/overview.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `docs/overview.md`'s "## Modules" list, update the
  **loom** bullet (currently "🚧 Design — not built") to note that loom's config
  module (`loom.yaml`) now exists carrying the `discussion` role model-spec and
  `discussion_timeout_min`, even though the `lyx loom` command itself is still
  unbuilt — keep the bullet accurate (do not claim the phase machine is built).
  Keep the edit surgical: one or two sentences appended to the existing loom
  bullet; do not restructure the module list. This satisfies the
  documentation-lifecycle rule (config-module change documented in the same
  batch).
- **Commit:** `docs(overview): note loom.yaml config module`

## Batch Tests

`verify: go test ./internal/loomengine/ ./internal/configreg/ ./internal/initengine/ ./internal/configcli/` — scoped to every consumer of the new
config module: `loomengine` (the new Config/LoadConfig + its tests),
`configreg` (the registration + the updated `TestNames`), and `initengine` /
`configcli` whose tests iterate `configreg.Modules()` and reconcile every
module's template — they are the real guard that `loom.yaml`'s template is
well-formed and reconciles cleanly (neither hardcodes a module count; they read
`len(configreg.Modules())`). This is a deliberately slightly-wider scope than a
single package because adding a config module has exactly these four blast-radius
packages; the justification is the cross-package `configreg.Modules()` fan-out,
not a cross-cutting helper.
