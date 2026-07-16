# Batch: burler-config-module

```yaml
task: "Fork-based cluster review in burler"
batch: "burler-config-module"
number: 3
cards: 2
verify: go test ./internal/burlerengine/ ./internal/configreg/
depends-on: []
```

## Batch Scope

The lens/fan configuration module: `burler.yaml` seeded per repo with the whole standard
library (card 6), registered in configreg as a **seed-only** module (card 7) — the
models.yaml precedent for an open-ended, operator-owned key set. Seed-only is
load-bearing: the default reconcile path marshals from the template tree and would
DELETE operator-added lenses/fans (`yamlengine.Reconcile` reports them as `removed`),
so burler.yaml must never be reconciled, only materialized when absent. Consequently
`LoadConfig` follows the `modelspec.LoadRegistry` direct-read pattern, NOT
`configengine.Load` (whose `MissingKeys` gate would also reject an operator who deletes
a standard lens they don't want). Root batch, parallel to batches 1–2; external
interface for batch 4: `burlerengine.Config`, `Lens`, `LoadConfig`, `ResolveFan`,
`maxClusterN`.

## Cards

### Card 6: burler config — lenses, fans, loader, resolver

- **Context:**
  - `internal/modelspec/load.go`
  - `internal/modelspec/template.go`
  - `internal/hubgeometry/hubgeometry.go`
  - `internal/burlerengine/template.go`
  - `docs/research/session-fork-spike.md`
- **Edits:** none
- **Creates:**
  - `internal/burlerengine/config.go`
  - `internal/burlerengine/config_test.go`
  - `internal/burlerengine/template.yaml`
- **Deletes:** none
- **Moves:** none
- **Requirements:** `config.go` defines: `type Config struct { Lenses map[string]string
  "yaml:\"lenses\""; Fans map[string][]string "yaml:\"fans\"" }`; `type Lens struct {
  Name, Text string }`; `func ConfigTemplate() string` returning an embedded
  `template.yaml` (new `//go:embed template.yaml` — the existing `template.go` embed of
  the prompt template is untouched); `func LoadConfig(baseDir string) (Config, error)`
  reading `hubgeometry.ConfigFile(baseDir, "burler")` directly via `os.ReadFile` with a
  strict `yaml.Decoder` + `KnownFields(true)` top-level decode — an absent file returns
  the zero Config with no error (clustering then fails at fan resolution with a message
  naming the file and `lyx config reconcile`, mirroring `modelspec.LoadRegistry`'s
  optional-file posture); `const maxClusterN = 16`; and
  `func ResolveFan(cfg Config, name string) ([]Lens, error)` — fail-loud,
  burler-prefixed errors for: unknown fan name (error text lists the defined fan names
  and mentions seeding via `lyx config reconcile` when `cfg.Fans` is empty), a fan
  entry naming an undefined lens, an empty fan, and a fan longer than `maxClusterN`.
  Resolution preserves fan order and permits repeated lens names (each repeat is its
  own `Lens` entry). `template.yaml` seeds the standard library with explanatory
  header comments (operator-owned file, seed-only — never reconciled; how to add
  repo-local lenses/fans): nine lenses — `generic`, `correctness`, `error-handling`,
  `test-gaps`, `security`, `performance`, `api-design`, `concurrency`,
  `docs-consistency` — each a YAML block scalar of 3–8 lines of emphasis-steering
  prose that (a) names its focus areas concretely and (b) closes with an
  emphasis-never-exclusion sentence such as "Report anything else you notice too —
  emphasis, never exclusion." (`generic` is the no-emphasis broad review). Fans:
  `standard: [generic, generic, correctness, error-handling, test-gaps]` and
  `full:` listing all eight non-generic lenses. `config_test.go`: template decodes
  through `LoadConfig`'s own decode path; self-consistency — every fan entry names a
  defined lens, `standard` has length 5, `full` length 8, every lens text non-empty
  and containing no hard-exclusion phrasing (assert absence of the substring
  "ignore "); `ResolveFan` table — happy path order/repeats, unknown fan, unknown
  lens, empty fan, over-cap fan, zero-Config fan lookup error mentioning reconcile.
- **Commit:** `burler: add lens/fan config module (seed-only burler.yaml)`

### Card 7: configreg registration as seed-only

- **Context:**
  - `internal/configsync/configsync.go`
- **Edits:**
  - `internal/configreg/configreg.go`
  - `internal/configreg/configreg_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add `{Name: "burler", Template: burlerengine.ConfigTemplate,
  SeedOnly: true}` to `Modules()` in alphabetical position (between `builder` and
  `models`), importing `internal/burlerengine`. Update `configreg_test.go`: the
  `TestNames` want-list gains `"burler"`; `TestModules_SeedOnly`'s want condition
  becomes `m.Name == "models" || m.Name == "burler"` and its doc comment is updated to
  name both open-ended operator-owned modules. Confirm (and state in the commit body if
  adjusted) that no other pinned set references config module names — configcli's
  `Known modules:` help is generated from `configreg.Names()` by design.
- **Commit:** `configreg: register burler config module as seed-only`

## Batch Tests

`go test ./internal/burlerengine/ ./internal/configreg/` — the new `config_test.go`
table (decode, self-consistency, resolver error taxonomy) and the updated configreg
pins. Existing burlerengine tests recompile unchanged (this batch adds files only).
