# Batch: tokenvocab-module

```yaml
task: "Built-in operator console pane in mux"
batch: tokenvocab-module
number: 2
cards: 5
verify: go test ./internal/tokenvocab/...
depends-on: [1]
```

## Batch Scope

Creates `internal/tokenvocab`, the general/shared token-vocabulary module (loom will also consume
it). It owns the token registry (`repo`, `hub`) and the reusable `Render` compose over
`internal/stencil`. It is a leaf: stdlib + `internal/hubgeometry` + `internal/stencil` only,
enforced by a leaf test and a CONSTRAINTS.md entry mirroring `internal/modelspec`. External
interface consumed by batch 3: `tokenvocab.Ctx`, `tokenvocab.Build`, and `tokenvocab.Render`.

## Cards

### Card 3: tokenvocab types, registry, and Build

- **Context:**
  - `internal/hubgeometry/hubgeometry.go`
  - `internal/stencil/stencil.go`
  - `internal/modelspec/registry.go`
- **Edits:** none
- **Creates:**
  - `internal/tokenvocab/tokenvocab.go`
  - `internal/tokenvocab/doc.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** In package `tokenvocab`: define `type Ctx struct { Layout *hubgeometry.Layout }`
  (a struct, not a bare `*Layout`, so future context such as a task slug can be added without a
  signature change). Define `type Token struct { Name string; Resolve func(Ctx) string }`. Define an
  unexported `registry []Token` with two always-resolvable entries: `{Name: "repo", Resolve: func(c
  Ctx) string { return c.Layout.Repo }}` and `{Name: "hub", Resolve: func(c Ctx) string { return
  c.Layout.Hub }}`. Do NOT add a `slug` token (deferred). Export `func Build(c Ctx)
  map[string]string` that iterates `registry` and returns `{name: resolve(c)}` for every token.
  `doc.go` holds the package godoc: purpose, the leaf-invariant statement (stdlib + hubgeometry +
  stencil only), and the "one registry entry per token" extension rule.
- **Commit:** `feat(tokenvocab): add token registry and Build over hubgeometry.Layout`

### Card 4: tokenvocab.Render compose over stencil

- **Context:**
  - `internal/stencil/stencil.go`
  - `internal/tokenvocab/tokenvocab.go`
- **Edits:** none
- **Creates:**
  - `internal/tokenvocab/render.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add `func Render(template []byte, c Ctx) ([]byte, error)` that returns
  `stencil.Fill(template, Build(c))`. This is the single reusable compose every consumer (mux header,
  loom) calls; it surfaces `stencil.Fill`'s unfilled-top-level-marker error unchanged for an
  unknown/empty token. Keep `Render` in its own file so the stencil dependency is localized.
- **Commit:** `feat(tokenvocab): add Render composing the vocabulary with stencil.Fill`

### Card 5: tokenvocab unit tests

- **Context:**
  - `internal/tokenvocab/tokenvocab.go`
  - `internal/tokenvocab/render.go`
  - `internal/stencil/stencil_test.go`
- **Edits:** none
- **Creates:**
  - `internal/tokenvocab/tokenvocab_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Hermetic unit tests building `hubgeometry.Layout` struct literals directly (no
  `Resolve` — Test Tier Purity). Cover: each token's `Resolve` (`repo`, `hub`) reads the matching
  `Layout` field; `Build` returns both keys; `Render` fills a `{{.hub}}`/`{{.repo}}` template
  verbatim; `Render` propagates `stencil`'s unfilled-marker error for a template referencing an
  unknown top-level token (e.g. `{{.slug}}`); the `repo` value reflects `Layout.Repo` exactly
  (including a `Layout` whose `Repo` was set from the empty-`Prime` fallback in batch 1). Add one
  case demonstrating that adding a hypothetical token is a single registry entry (documents the
  "trivial to add" property).
- **Commit:** `test(tokenvocab): cover resolvers, Build, Render, and unknown-token error`

### Card 6: tokenvocab leaf-enforcement test

- **Context:**
  - `internal/modelspec/leaf_enforcement_test.go`
  - `internal/tokenvocab/tokenvocab.go`
- **Edits:** none
- **Creates:**
  - `internal/tokenvocab/leaf_enforcement_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Port `internal/modelspec/leaf_enforcement_test.go`'s
  `TestLeafInvariant_AllowlistOnly` to `tokenvocab`: walk the package's production imports and assert
  every import is in the allowlist `{stdlib, github.com/Knatte18/loomyard/internal/hubgeometry,
  github.com/Knatte18/loomyard/internal/stencil}`. Any other internal import fails the test. Match the
  peer test's mechanics (same import-scan approach and stdlib detection).
- **Commit:** `test(tokenvocab): enforce leaf import allowlist`

### Card 7: tokenvocab module doc, overview table, and CONSTRAINTS entry

- **Context:**
  - `docs/modules/loom.md`
  - `internal/tokenvocab/doc.go`
- **Edits:**
  - `docs/overview.md`
  - `CONSTRAINTS.md`
- **Creates:**
  - `docs/modules/tokenvocab.md`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `docs/modules/tokenvocab.md` describing the module: purpose (shared token
  vocabulary), the `Token`/`Ctx`/`Build`/`Render` API, the leaf invariant, and how to add a token
  (append one registry entry). In `docs/overview.md`, add a `tokenvocab` row to the module table
  (matching the existing table's columns/style) and, if the doc has an execution-stack/dependency
  listing, note `tokenvocab` as a shared leaf consumed by mux (and future loom). In `CONSTRAINTS.md`,
  add a "## Tokenvocab Leaf Invariant" section modeled on "## Modelspec Leaf Invariant": state the
  allowlist (stdlib + hubgeometry + stencil), the never-allowed reverse edges, and "**Enforced by**
  `internal/tokenvocab/leaf_enforcement_test.go`".
- **Commit:** `docs(tokenvocab): module doc, overview row, and leaf invariant`

## Batch Tests

`verify: go test ./internal/tokenvocab/...` runs the unit tests (card 5) and the leaf-enforcement
test (card 6). All untagged and spawn-free (Layout literals, no `Resolve`). The doc/CONSTRAINTS
changes (card 7) have no runnable surface; they are validated by the batch's reviewer and by the
existing repo-wide doc-link checks under the module-wide `go build ./...` gate.
