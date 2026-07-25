# Plan: loom: Planner producer

```yaml
task: 'loom: Planner producer'
slug: 'loom-planner'
approved: true
started: '20260725-073224'
parent: 'main'
root: ""
verify: go test ./cmd/lyx/...
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to
schedule batches. Every batch lives at `NN-<batch-slug>.md` in this
directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: plan-path-helpers
    file: 01-plan-path-helpers.md
    depends-on: []
    verify: go test ./internal/hubgeometry/...
  - number: 2
    name: planner-producer
    file: 02-planner-producer.md
    depends-on: [1]
    verify: go test ./internal/loomengine/...
```

## Shared Decisions

_Cross-cutting decisions every batch inherits._

### Decision: mirror-the-discussion-producer

- **Decision:** The Planner producer is built as the exact structural sibling of the
  already-built Discussion producer in `internal/loomengine`: a `//go:embed`-ed prompt
  template + a `stencil`-based composer + a `Spec` factory returning `shuttleengine.Spec`.
  Reuse the Discussion producer's seams (`modelspec.Parse`/`reg.Resolve`, `stencil.Fill`,
  the `Resolved→Spec` field mapping) verbatim; do not invent new abstractions.
- **Rationale:** the Discussion producer (`discussion.go`, `prompt.go`, `discussion-template.md`,
  `prompttemplate.go`, `config.go`) is the pinned pattern; matching it keeps the two producers
  parallel and reviewable.
- **Applies to:** all batches

### Decision: pure-composer-no-preflight

- **Decision:** `PlanSpec` is a pure composer. Like `DiscussionSpec` it does NOT stat its
  input (`decision-record.md`), does NOT stat/create/clean its output dir, and does NOT spawn.
  Verifying the input exists and rotating a stale `_lyx/plan/` are the future loom phase
  machine's job — out of scope here. Do not add stat/cleanup logic to `PlanSpec`.
- **Rationale:** `_mill/discussion.md` Decisions `input-decision-record-only`,
  `done-sentinel-overview-last`, and the shuttle-contract notes pin this boundary explicitly.
- **Applies to:** planner-producer

### Decision: autonomous-only-no-slug

- **Decision:** `PlanSpec(layout, cfg, reg)` takes NO `slug` and NO `autonomous` param. Its
  sole input is `_lyx/discussion/decision-record.md` (self-contained, no board read). The
  returned `Spec` always sets `Interactive: false` and `Role: "plan"`.
- **Rationale:** `_mill/discussion.md` Decisions `autonomous-only` and
  `input-decision-record-only`.
- **Applies to:** planner-producer

### Decision: producer-writes-approved-false

- **Decision:** the prompt instructs the agent to write `approved: false` in the plan's
  `00-overview.md` frontmatter. The producer never self-approves; flipping to `true` is the
  future loom orchestrator's job after `perch` returns `APPROVED` — NOT built here.
- **Rationale:** `_mill/discussion.md` Decisions `approved-false-draft` and `approved-flag-flip`.
- **Applies to:** planner-producer

### Decision: compact-format-inline

- **Decision:** `plan-template.md` carries a COMPACT plan-format-v3 spec inline — a literal
  fill-in skeleton for `00-overview.md` plus one `NN-<card>.md`, with terse per-field rules,
  in the style of the Discussion producer's `discussion-template.md`. It must NOT reproduce
  `docs/reference/plan-format-v3.md` at ~440 lines; that doc stays a development-only reference
  the producer never consumes. The compact spec must still cover every REQUIRED v3 field.
- **Rationale:** `_mill/discussion.md` Decision `compact-format-inline` and its "plan-format-v3
  essentials" list under `## Technical context`.
- **Applies to:** planner-producer

### Decision: hubgeometry-owns-plan-paths

- **Decision:** the `_lyx/plan/` directory path and the `00-overview.md` filename are
  constructed ONLY in `internal/hubgeometry` (new `Layout.PlanDir()` / `Layout.PlanOverview()`
  methods). `internal/loomengine` obtains them via those methods, never via a raw
  `filepath.Join` with a geometry token.
- **Rationale:** the Hub Geometry Invariant in `CONSTRAINTS.md`, enforced by
  `internal/hubgeometry/enforcement_test.go`.
- **Applies to:** all batches

### Decision: same-commit-docs

- **Decision:** the doc-lifecycle work (fold `loom-planner.md`'s durable design into
  `plan.go`'s header godoc, flip `loom.md`'s module-table row to built, move the roadmap item
  Planned→Done, drop every inbound link to `loom-planner.md`, refresh `docs/overview.md`, and
  DELETE `loom-planner.md`) lands in this same task, per `loom-planner.md`'s own lifecycle note
  and CLAUDE.md's Documentation-Lifecycle rule.
- **Rationale:** `_mill/discussion.md` Scope and Decision `files-mirror-discussion-producer`;
  CLAUDE.md `## Task completion`.
- **Applies to:** planner-producer

## All Files Touched

_Full union of every `Creates:` / `Edits:` / `Moves:` **target** path across every batch,
sorted alphabetically. Move sources and Deletes are excluded._

- `docs/overview.md`
- `internal/hubgeometry/hubgeometry.go`
- `internal/hubgeometry/planpath_test.go`
- `internal/loomengine/config.go`
- `internal/loomengine/config_test.go`
- `internal/loomengine/plan-template.md`
- `internal/loomengine/plan.go`
- `internal/loomengine/plan_test.go`
- `internal/loomengine/plantemplate.go`
- `internal/loomengine/template.yaml`
- `manifest/designs/loom.md`
- `manifest/designs/webster-rewrite.md`
- `manifest/roadmap.md`
