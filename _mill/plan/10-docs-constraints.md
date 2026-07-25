# Batch: docs-constraints

```yaml
task: 'webster: rewrite for flat card list'
batch: docs-constraints
number: 10
cards: 5
verify: go build ./...
depends-on: [9]
```

## Batch Scope

Close out the Documentation Lifecycle in the same landing: fold the durable design rationale of
`manifest/designs/webster-rewrite.md` into `internal/websterengine/doc.go` and
`docs/reference/builder-contract.md`, update `docs/overview.md`'s webster/builder module bullets,
move the roadmap item Planned→Done, DELETE the now-obsolete design doc, and record the two new
cross-cutting invariants in `CONSTRAINTS.md`. This batch runs last (`depends-on: [9]`) so the docs
describe the shipped code. No runnable behavior changes; `doc.go` is the only compiled surface.

## Cards

### Card 43: fold design rationale into websterengine doc.go

- **Context:**
  - `manifest/designs/webster-rewrite.md`
  - `internal/websterengine/state.go`
  - `internal/websterengine/render.go`
  - `internal/websterengine/integration.go`
- **Edits:**
  - `internal/websterengine/doc.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Rewrite `internal/websterengine/doc.go` to fold in the DURABLE parts of `manifest/designs/webster-rewrite.md` and reflect the shipped design: the fork-per-batch model (batchifier-derived batches; identity batcher = batch ≡ card in v0); consumption of the flat card-list plan format via `internal/planparser` (sole parser) and `internal/batcher` (config-selected library); the minimal fork-return contract (OK/FAILED + head SHA + informational deviation list); per-card commit + per-card `verify:`; the declared-order scheduler with the dead `HasSymbolFields()` DAG seam reserved for future codeintel; the integration-suite fork + in-process SHA-bisect + terminal escalation; and crash/resume via `state.json`. REMOVE all prose describing a dependency on `builderengine` (the import edge is cut); state instead that webster reuses only the provider-invariant mux/shuttle/engine substrate and its own webster-local mechanism helpers. No `v2`/`v3` symbol naming in the doc (spec references may name the pinned `plan-format-v3.md`).
- **Commit:** `docs(websterengine): fold flat-card design rationale into package doc`

### Card 44: revise builder-contract.md Webster section

- **Context:**
  - `manifest/designs/webster-rewrite.md`
  - `docs/reference/builder-contract.md`
  - `internal/websterengine/report.go`
- **Edits:**
  - `docs/reference/builder-contract.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Revise the `## Webster: the fork-based sibling` section of `docs/reference/builder-contract.md` (the durable, kept contract doc): update it from the v2 framing to the shipped flat-format reality — webster no longer shares builder's plan input (`builderengine.ParsePlan`), batch-report schema (`builderengine.ParseReport`), or digest contract; it consumes the flat card list via `internal/planparser` and reports via its own minimal fork-return contract. Drop the A/B-interchangeable-with-builder framing and state that `builder` is obsolete as a plan-format consumer (its deletion is a separate later task; it stays frozen/functional meanwhile). Keep the sections that remain accurate (webster's independent `_lyx/webster/` state, its `summary.md` artifact). This is the second fold target for `webster-rewrite.md`'s durable parts (alongside `doc.go`).
- **Commit:** `docs(builder-contract): revise Webster section for flat card-list consumption`

### Card 45: update overview.md module bullets

- **Context:**
  - `docs/overview.md`
  - `docs/reference/builder-contract.md`
- **Edits:**
  - `docs/overview.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `docs/overview.md`'s `## Modules` list, update the `webster` bullet to drop the "Planned `webster: rewrite for flat card list` … v2 → v3" forward reference and describe webster as the shipped consumer of the flat card-list plan format (fork-per-batch, `planparser`+`batcher`, integration fork with SHA-bisect). Update the `builder` bullet to note builder is now obsolete as a plan-format consumer (frozen/functional, deletion tracked separately). Add brief module-list entries for the two new packages if the module list enumerates `internal/*` engines (`internal/planparser` — sole flat-plan parser; `internal/batcher` — config-selected batchifier library). Leave the execution-stack diagram unchanged (neither webster nor builder appears in it — they branch off shuttle directly). Keep `builder-contract.md` in the durable-docs "Other docs" list.
- **Commit:** `docs(overview): update webster/builder module bullets and add planparser/batcher`

### Card 46: roadmap Planned→Done + delete design doc

- **Context:**
  - `manifest/roadmap.md`
  - `internal/websterengine/doc.go`
- **Edits:**
  - `manifest/roadmap.md`
- **Creates:** none
- **Deletes:**
  - `manifest/designs/webster-rewrite.md`
- **Moves:** none
- **Requirements:** In `manifest/roadmap.md` move the `webster: rewrite for flat card list` item out of the `## Planned` section into `## Done`, with a pointer to the shipped module doc (`internal/websterengine` package doc) INSTEAD of the `See designs/webster-rewrite.md` link (which is deleted). Honor the roadmap maintenance rules: this is a completed planned item, so it moves Planned→Done; do NOT append bugfix/polish notes. Then DELETE `manifest/designs/webster-rewrite.md` — the design doc self-declares deletion on land once its durable parts fold into `doc.go` + `builder-contract.md` (cards 43–44), per the Documentation Lifecycle.
- **Commit:** `docs(roadmap): move webster rewrite to Done and delete landed design doc`

### Card 47: record new CONSTRAINTS invariants

- **Context:**
  - `CONSTRAINTS.md`
  - `internal/planparser/doc.go`
  - `internal/batcher/doc.go`
- **Edits:**
  - `CONSTRAINTS.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add two new cross-cutting invariants to `CONSTRAINTS.md` in the established index style (name; statement; how enforced — machine and/or review): (1) **Planparser Sole-Parser Invariant** — `internal/planparser` is the SOLE parser of the on-disk plan format (`_lyx/plan/`); no other package parses `00-overview.md`/`NN-<card-slug>.md`, and consumers (webster's `RenderForkPrompt`, the integration fork) read plan-level sections only from `planparser.Plan`. Note it composes with the Hub Geometry Invariant (planparser resolves `_lyx/plan/` via `hubgeometry`, never string literals). Enforcement: review obligation today (candidate future import/grep guard). (2) **Batcher Registry+Config Invariant** — webster's execution unit is the batchifier-derived batch; batching is selected by `internal/batcher`'s name-keyed registry + the `batcher:` config key (default `identity`); no plan-supplied batching exists and no batch grouping is expressed in the plan format. Enforcement: review obligation. Match the existing entries' tone and brevity.
- **Commit:** `docs(constraints): record planparser sole-parser and batcher registry+config invariants`

## Batch Tests

`verify: go build ./...` — this batch's only compiled surface is `internal/websterengine/doc.go`
(a package-doc file with no runnable logic); a whole-module build confirms it still compiles after
the rewrite. The remaining edits are Markdown (`builder-contract.md`, `overview.md`, `roadmap.md`,
`CONSTRAINTS.md`) and a design-doc deletion — prose/lifecycle changes with no test surface, as
noted here per the template's `verify: null`-justification guidance (a build gate is used instead
of null because `doc.go` is Go code). The two new CONSTRAINTS invariants are review obligations,
not new machine checks, matching several existing entries (e.g. Shell Mechanics Seam).
