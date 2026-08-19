# Plan: loom: phase-machine scaffolding

```yaml
task: 'loom: phase-machine scaffolding'
slug: loom-phase-machine-scaffolding
approved: true
started: '20260819-093203'
parent: standalone-producers
root: ""
verify: null
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to schedule batches.
Every batch lives at `NN-<batch-slug>.md` in this directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: status-schema-migration
    file: 01-status-schema-migration.md
    depends-on: []
    verify: go test ./internal/loomengine/... ./internal/lyxcwd/... ./cmd/lyx/... && go test -tags integration ./internal/loomengine/...
  - number: 2
    name: loomshed-producers
    file: 02-loomshed-producers.md
    depends-on: [1]
    verify: go test ./internal/loomshed/... ./internal/lyxcwd/... ./cmd/lyx/...
  - number: 3
    name: sequence-and-integration
    file: 03-sequence-and-integration.md
    depends-on: [2]
    verify: go test ./internal/loomshed/... ./internal/lyxcwd/... ./cmd/lyx/... && go test -tags integration ./internal/loomshed/...
```

## Shared Decisions

### Decision: shed-schema-wins

- **Decision:** `_lyx/loom/status.json` becomes a `shedengine.Status`. loom's own `slug`, `parent` and `start_sha` move into the existing `product json.RawMessage` passthrough field; `narration` is dropped in favour of Shed's mechanically-composed `activity{now,last,wait}`.
- **Rationale:** `loomengine.Preflight`'s check 4 and `shedengine` read the same file under two mutually incompatible strict schemas today. One file, one writer, one schema follows from "loom = Shed + its own producer list, nothing else". Making loom's schema win is unavailable — it would require editing `internal/shedengine`, which the Shed Producer-Seam Invariant puts off-limits.
- **Applies to:** all batches

### Decision: loomshed-takes-told-paths

- **Decision:** every path `internal/loomshed` uses is a told absolute string supplied by its caller. The package has no direct production import of `internal/lyxcwd` and never writes the literals `_lyx` or `.lyx`.
- **Rationale:** the Told-Geometry Invariant plus the Lyxdirs Single-Declarer Invariant. A `seam_enforcement_test.go`-style allowlist guard in the package converts the review obligation into a machine check.
- **Applies to:** all batches

### Decision: explicit-deps-struct

- **Decision:** `loomshed.New` takes a `Deps` struct. Exactly two rows are injected — `Preflight` as a bare `shedengine.ShedProducer`, and `Webster` as `WebsterRun`+`WebsterDeps` parts. Every other row `loomshed` builds itself from told values.
- **Rationale:** those are the only two rows that would otherwise spawn git or an LLM session, so injecting exactly them makes the whole real 12-row list exercisable at Tier 1 without a second, test-only list.
- **Applies to:** loomshed-producers, sequence-and-integration

### Decision: local-cancellation-helpers

- **Decision:** `internal/loomshed` declares its own unexported `entryErr`/`cancelErr` helpers rather than reusing `internal/shedadapters`' identically-shaped ones.
- **Rationale:** `shedadapters`' versions are unexported and Scope forbids changing that package. Every real producer written here must honour the two obligations `Shed` cannot enforce — return exactly `Done` or `Stuck`, and surface context cancellation as a non-nil error, never as `Stuck`. The duplication is deliberate, not an oversight.
- **Applies to:** loomshed-producers, sequence-and-integration

### Decision: producer-names-verbatim

- **Decision:** every `ProducerDef.Name` uses `manifest/designs/loom.md`'s table strings verbatim: `Preflight`, `Discussion-Write`, `Discussion-Validate`, `Discussion-Review`, `Plan-Sweep`, `Plan-Write`, `Plan-Validate`, `Plan-Review`, `Batchifier`, `Webster`, `Webster-Review`, `Finalize`.
- **Rationale:** the name is the durable on-disk identity in `current_producer`; a later rename breaks resume for any in-flight task.
- **Applies to:** loomshed-producers, sequence-and-integration

### Decision: onstuck-routing

- **Decision:** every gate and validator bounces back to the producer whose artifact it guards; a gate whose guarded artifact is produced by no row in the list escalates instead (`OnStuck: ""`), as does every non-gate row.
- **Rationale:** `loom.md` already pins the shape (`Plan-Review` → `Plan-Write`); the rest follows mechanically. `Preflight` gates git state and `Batchifier` gates `batcher.yaml`, neither of which any row writes, so there is nothing to bounce to.
- **Applies to:** loomshed-producers, sequence-and-integration

### Decision: docs-land-in-the-falsifying-commit

- **Decision:** each doc the task falsifies is edited in the same card, and therefore the same commit, as the change that falsifies it — never batched into a trailing docs-only card.
- **Rationale:** the repo's own Documentation Lifecycle rule. A commit that ships the schema change while leaving `manifest/designs/shed.md` asserting the old shape leaves the design docs actively lying about what ships.
- **Applies to:** all batches

### Decision: go-test-tiers

- **Decision:** untagged test files stay offline — no `hubforge.NewHub`, no git spawn, temp dirs only. Anything needing real git carries `//go:build integration` and the package gains a `TestMain` calling `gitkit.HermeticGitEnv()`.
- **Rationale:** the Test Tier Purity Invariant and the Hermetic Git Test Environment Invariant, both machine-enforced from `cmd/lyx`.
- **Applies to:** all batches

## All Files Touched

- `CONSTRAINTS.md`
- `cmd/lyx/constructoranchoring_test.go`
- `cmd/lyx/notransients_test.go`
- `contracts/specs/loom-status-spec.md`
- `docs/overview.md`
- `internal/loomengine/coherence.go`
- `internal/loomengine/coherence_test.go`
- `internal/loomengine/config.go`
- `internal/loomengine/loomstatus_test.go`
- `internal/loomengine/preflight.go`
- `internal/loomengine/preflight_integration_test.go`
- `internal/loomengine/status.go`
- `internal/loomshed/batchifier.go`
- `internal/loomshed/batchifier_test.go`
- `internal/loomshed/ctx.go`
- `internal/loomshed/ctx_test.go`
- `internal/loomshed/discussionvalidate.go`
- `internal/loomshed/discussionvalidate_test.go`
- `internal/loomshed/doc.go`
- `internal/loomshed/fixture_test.go`
- `internal/loomshed/loomshed.go`
- `internal/loomshed/loomshed_test.go`
- `internal/loomshed/planvalidate.go`
- `internal/loomshed/planvalidate_test.go`
- `internal/loomshed/preflight.go`
- `internal/loomshed/preflight_integration_test.go`
- `internal/loomshed/resume_test.go`
- `internal/loomshed/seam_enforcement_test.go`
- `internal/loomshed/seed.go`
- `internal/loomshed/seed_test.go`
- `internal/loomshed/sequence_test.go`
- `internal/loomshed/stub.go`
- `internal/loomshed/stub_test.go`
- `internal/loomshed/testmain_integration_test.go`
- `internal/loomshed/webster.go`
- `internal/loomshed/webster_test.go`
- `internal/shedengine/doc.go`
- `manifest/designs/loom.md`
- `manifest/designs/shed.md`
- `manifest/roadmap.md`
