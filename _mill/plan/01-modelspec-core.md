# Batch: modelspec-core

```yaml
task: Build modelspec - the model-spec parser + registry
batch: modelspec-core
number: 1
cards: 7
verify: go test ./internal/modelspec/...
depends-on: []
```

## Batch Scope

Creates the complete `internal/modelspec` leaf library: exported types, the strict spec
parser, the built-in registry + resolution, the models.yaml loader, the configreg seed
template, the leaf-import enforcement test, and the docs/invariant entries that describe
the package (CONSTRAINTS.md, model-spec.md built-in-list amendment, overview.md,
shared-libs README). External interface consumed by batch 2: `modelspec.ConfigTemplate()
string`. External interface consumed by future consumers (builder):
`Parse(string) (Spec, error)`, `LoadRegistry(string) (Registry, error)`,
`Registry.Resolve(Spec) (Resolved, error)`.

Batch-local decision: the package stays a leaf — production imports are ONLY the
standard library (including `embed`), `internal/hubgeometry`, and `gopkg.in/yaml.v3`.
`configreg` will import modelspec (batch 2); modelspec never imports configreg,
configengine, envsource, yamlengine, or any feature package.

## Cards

### Card 1: Package skeleton — types, vocabularies, package doc

- **Context:**
  - `docs/reference/model-spec.md`
  - `internal/shuttleengine/spec.go`
- **Edits:** none
- **Creates:**
  - `internal/modelspec/modelspec.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create package `modelspec` with the as-built API documented in the
  package doc comment (this comment is the package's module doc per the Documentation
  Lifecycle — cover: purpose, the grammar in one line, alias vs escape form, Parse/Resolve
  split, LoadRegistry fallback semantics, the leaf import discipline, and a note that
  consumers map `Resolved` onto `shuttleengine.Spec` themselves:
  `Model=Resolved.Model`, `Effort=Params["effort"]`, `Version=Params["version"]`).
  Declare:
  `type Spec struct { Alias string; Engine string; Model string; Params map[string]string }`
  (exactly one of `Alias` or the `Engine`+`Model` pair is non-empty — alias form vs
  escape form; document this on the type),
  `type Entry struct { Engine string; Model string; Defaults map[string]string }`
  (with yaml tags `engine`, `model`, `defaults`),
  `type Registry map[string]Entry`,
  `type Resolved struct { Engine string; Model string; Params map[string]string }`.
  Declare the closed vocabularies as unexported package-level sets:
  `knownParams = map[string]bool{"effort": true, "version": true}` and
  `knownEngines = map[string]bool{"claude": true}`, each with a doc comment stating they
  gate param keys / engine names ONLY and must never gate model names or aliases
  (new-model-without-recompile requirement). No logic yet beyond declarations; the file
  must compile.
- **Commit:** `feat(modelspec): package skeleton — types, vocabularies, package doc`

### Card 2: Parse — strict spec grammar

- **Context:**
  - `docs/reference/model-spec.md`
- **Edits:** none
- **Creates:**
  - `internal/modelspec/parse.go`
  - `internal/modelspec/parse_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Implement `func Parse(s string) (Spec, error)` accepting exactly four
  shapes: `alias`, `alias[k=v,...]`, `engine:modelid`, `engine:modelid[k=v,...]`.
  Strict rules (each rejection is its own error naming the offending token or character):
  empty input; ANY whitespace anywhere (error names the character and its position);
  alias, param keys, and engine charset `[a-z0-9-]+` (case-sensitive — uppercase is a
  charset error); escape-form model-id charset `[a-z0-9._-]+`; param value charset
  `[a-z0-9._-]+`, non-empty; escape form detected by the presence of `:` — exactly one
  colon allowed, empty engine or empty model-id rejected; bracket part optional but if
  present non-empty (`sonnet[]` rejected) and must end with `]` with nothing after it;
  duplicate param key in one bracket rejected; empty key or empty value (`effort=`)
  rejected; param key not in `knownParams` rejected; escape-form engine not in
  `knownEngines` rejected. On success return `Spec` with `Params` nil when no bracket,
  else the parsed map. Table-driven TDD: write the rejection and acceptance tables first
  (valid: bare alias, single param, multiple params, escape form with and without
  bracket, `version=4.5` dotted value; invalid: every rule above with exact-substring
  assertions on error text).
- **Commit:** `feat(modelspec): strict spec parser with fail-loud grammar errors`

### Card 3: Built-ins and Registry.Resolve

- **Context:**
  - `docs/reference/model-spec.md`
- **Edits:** none
- **Creates:**
  - `internal/modelspec/registry.go`
  - `internal/modelspec/registry_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Implement `func builtins() Registry` returning the pinned default-free
  fallback set: `sonnet`, `opus`, `haiku`, `fable`, each
  `Entry{Engine: "claude", Model: <same word as the alias>, Defaults: nil}` (built-ins
  carry NO effort defaults — operator defaults live in models.yaml; contract's built-in
  fallback plus discussion decision "Built-in fallback set and template values").
  Implement `func (r Registry) Resolve(s Spec) (Resolved, error)`:
  alias form — look up `s.Alias` in `r`; unknown alias is a loud error naming the alias
  and the sorted known aliases; `Resolved.Params` starts as a copy of `Entry.Defaults`
  overlaid by `s.Params` (bracket param wins per key — contract precedence "bracket
  param > registry default"); `Resolved.Engine`/`Model` come from the entry.
  Escape form — no registry lookup: `Resolved{Engine: s.Engine, Model: s.Model,
  Params: copy of s.Params}`. Resolve never mutates the input Spec or the Registry, and
  the returned Params map is never nil (empty map when no params). Table-driven TDD:
  registry default only; bracket overrides default; bracket adds param absent from
  defaults; unknown alias fails; escape form bypasses registry; zero-value Registry
  resolves nothing but errors cleanly.
- **Commit:** `feat(modelspec): built-in fallback registry and Resolve with bracket-over-default precedence`

### Card 4: LoadRegistry — models.yaml loader

- **Context:**
  - `docs/reference/model-spec.md`
  - `internal/hubgeometry/hubgeometry.go`
- **Edits:** none
- **Creates:**
  - `internal/modelspec/load.go`
  - `internal/modelspec/load_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Implement `func LoadRegistry(baseDir string) (Registry, error)`.
  Path is `hubgeometry.ConfigFile(baseDir, "models")` — never hand-joined. Absent file
  (`os.IsNotExist`) → return `builtins()` with nil error (deliberately NOT the
  configengine.Load pattern: models.yaml is optional). Any other read error → wrapped
  error naming the path. Present file: decode as `map[string]Entry` using a
  `yaml.Decoder` with `KnownFields(true)` (strict-yaml-decode Shared Decision); then
  validate every file entry, each failure a loud error naming the alias and path:
  alias key must match `[a-z0-9-]+`; `Engine` non-empty and in `knownEngines`; `Model`
  non-empty (free-form string — never checked against any model list); every `Defaults`
  key in `knownParams` with a non-empty value. Merge: start from `builtins()`, overlay
  each file entry as WHOLE-ENTRY replacement (a file `sonnet:` replaces the built-in
  entirely; discussion decision "models.yaml override granularity"). An empty or
  comments-only file yields `builtins()` unchanged. Table-driven tests with `t.TempDir()`
  and `hubgeometry.ConfigFile`-built paths: absent → builtins (all four aliases);
  file extends (new alias present alongside builtins); file overrides whole-entry
  (built-in defaults do not leak through); missing engine/model → error; unknown entry
  field → error; unknown defaults key → error; unknown engine → error; malformed YAML →
  error naming path.
- **Commit:** `feat(modelspec): models.yaml loader with built-in fallback and whole-entry override`

### Card 5: ConfigTemplate — live-keys seed template

- **Context:**
  - `internal/shuttleengine/template.go`
  - `internal/shuttleengine/template.yaml`
- **Edits:** none
- **Creates:**
  - `internal/modelspec/template.go`
  - `internal/modelspec/template.yaml`
  - `internal/modelspec/template_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Mirror the shuttleengine embed pattern: `template.go` holds
  `//go:embed template.yaml` and `func ConfigTemplate() string`. `template.yaml` is the
  SEED models.yaml (discussion decisions "configreg registration … seed-only" and
  "Built-in fallback set and template values"): header comments stating the file is
  operator-owned after materialization, reconcile never rewrites it (seed-only), aliases
  may be freely added (one example line, commented, e.g.
  `# zephyr: {engine: claude, model: claude-zephyr-1}`), and pointing at
  `docs/reference/model-spec.md`; then live entries exactly:
  `sonnet` → engine `claude`, model `sonnet`, defaults `effort: medium`;
  `opus` → engine `claude`, model `opus`, defaults `effort: high`;
  `haiku` → engine `claude`, model `haiku`, NO defaults (haiku does not support effort);
  `fable` → engine `claude`, model `fable`, defaults `effort: high`.
  `template_test.go`: write `ConfigTemplate()` to a `t.TempDir()` models.yaml path (via
  `hubgeometry.ConfigFile`) and assert `LoadRegistry` accepts it and yields those four
  entries with exactly those defaults (proves the seed always passes the loader's own
  validation).
- **Commit:** `feat(modelspec): embedded live-keys seed template for models.yaml`

### Card 6: Leaf enforcement test

- **Context:**
  - `internal/lyxtest/leaf_enforcement_test.go`
- **Edits:** none
- **Creates:**
  - `internal/modelspec/leaf_enforcement_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Enforcement test for the Modelspec Leaf Invariant. Reuse the
  `go/parser` `parser.ParseFile(..., parser.ImportsOnly)` technique from lyxtest's test,
  but invert the check shape: this is an ALLOWLIST, not a banned-list. Walk every
  non-`_test.go` `.go` file in the package directory; every import path must be either
  stdlib (no `.` in the first path segment) or exactly
  `github.com/Knatte18/loomyard/internal/hubgeometry` or `gopkg.in/yaml.v3`; anything
  else fails the test naming the file and the offending import. Test name:
  `TestLeafInvariant_AllowlistOnly`.
- **Commit:** `test(modelspec): allowlist leaf-import enforcement`

### Card 7: Docs — CONSTRAINTS invariant, contract amendment, overview, shared-libs

- **Context:**
  - `docs/roadmap.md`
  - `internal/lyxtest/leaf_enforcement_test.go`
- **Edits:**
  - `CONSTRAINTS.md`
  - `docs/reference/model-spec.md`
  - `docs/overview.md`
  - `docs/shared-libs/README.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** (1) `CONSTRAINTS.md`: add a `## Modelspec Leaf Invariant` section in
  the established style (statement: `internal/modelspec` production code imports only
  stdlib + `internal/hubgeometry` + `gopkg.in/yaml.v3`, so every future consumer
  (builder, perch/burler/loom configs) can import it without cycles; `configreg` →
  modelspec is the allowed direction, never the reverse; **Enforced by**
  `internal/modelspec/leaf_enforcement_test.go` (`TestLeafInvariant_AllowlistOnly`) on
  every `go test`). Place it after the lyxtest Leaf Invariant section.
  (2) `docs/reference/model-spec.md`: amend the "Built-in fallback" sentence to name
  `sonnet` / `opus` / `haiku` / `fable`, and note built-ins carry no parameter defaults —
  operator defaults live in the seeded models.yaml (deliberate contract amendment from
  the discussion).
  (3) `docs/overview.md`: add `internal/modelspec/` to the source-tree listing (one line,
  `model-spec parser + models.yaml registry leaf`, alongside the other library lines) and
  append `internal/modelspec` to the shared-infrastructure package list near the
  `shared-libs/README.md` reference.
  (4) `docs/shared-libs/README.md`: add `internal/modelspec` to the
  "Implementation-only libraries" list — one line: model-spec parser + models.yaml
  registry loader; the pinned contract is `docs/reference/model-spec.md`, the as-built
  API lives in the package doc.
  Do NOT touch `docs/roadmap.md` (no-roadmap-edit Shared Decision — read it only to
  confirm the builder milestone subsumes this work).
- **Commit:** `docs(modelspec): record leaf invariant, fable built-in, overview and shared-libs entries`

## Batch Tests

`verify: go test ./internal/modelspec/...` runs the full new package suite:
`parse_test.go` (grammar tables), `registry_test.go` (resolve precedence + fail-loud),
`load_test.go` (loader fallback/override/validation), `template_test.go` (seed passes
loader), `leaf_enforcement_test.go` (import allowlist). TDD cards (2, 3, 4) write their
tables before implementation. The scope is exactly the package this batch creates; no
other package is touched, so no wider scope is needed. The module-wide `go vet ./...`
overview gate catches accidental cross-package fallout.
