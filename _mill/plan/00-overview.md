# Plan: webster: DAG-derived card sequencing

```yaml
task: 'webster: DAG-derived card sequencing'
slug: 'webster-dag-card-sequencing'
approved: false
started: '20260824-181726'
parent: 'main'
root: ""
verify: go build ./...
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to schedule batches.
Every batch lives at `NN-<batch-slug>.md` in this directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: sequencing core
    file: 01-sequencing-core.md
    depends-on: []
    verify: go test ./internal/websterengine/...
  - number: 2
    name: wiring, render, and master template
    file: 02-wiring-render-template.md
    depends-on: [1]
    verify: go test ./internal/websterengine/... ./internal/webstercli/... ./internal/batcher/... ./contracts/stencils/... && go test -tags integration ./internal/websterengine/... ./internal/webstercli/...
  - number: 3
    name: docs
    file: 03-docs.md
    depends-on: [2]
    verify: go test ./internal/lyxcwd/...
```

## Shared Decisions

### Decision: sequencing-is-batch-level-and-pure

- **Decision:** The whole mechanism is one exported, pure function in `internal/websterengine/sequence.go` — `SequenceBatches([]batcher.Batch) ([]batcher.Batch, []Cycle)` — operating over the batches a batchifier already produced.
  It takes no path, no config, no `*planparser.Plan`, and no clock.
  It never adds, drops, or merges a batch: the returned slice holds exactly the input's batches, reordered.
- **Rationale:** `discussion.md`'s `sequencing-lives-in-websterengine` decision.
  The **Batcher Registry+Config Invariant** reserves *grouping* for `internal/batcher`;
  sequencing the batches a batchifier returned is webster's own execution policy, which `contracts/specs/loom-plan-spec.md`'s "Plan vs. schedule" section assigns to the executor.
  No new package, no new batchifier, no `batcher.yaml` change.
  Length-preservation is load-bearing: `Run`'s zero-batch refusal and `verifyEveryBatchDone` both depend on the full set surviving.
- **Applies to:** all batches

### Decision: determinism-is-a-correctness-requirement

- **Decision:** Every ordering decision keys on batch number with the original slice index as the final tie-break, and never on Go map iteration order.
  Sequencing the same input twice returns an identical order, and sequencing a shuffled copy of the same input returns the same order too.
- **Rationale:** `discussion.md`'s `stable-kahn-tiebreak` decision.
  `internal/webstercli` recomputes the batch list independently in `begin-batch`, `await-batch`, `record-batch`, and `recover-batch`, and `Run` computes it a fifth time.
  Two call sites that disagreed on order would desynchronize Master's loop from the verbs mid-run, and the failure would be very hard to diagnose.
  Keying the tie-break on declared number additionally guarantees the no-op property: an already dependency-correct plan sequences to exactly its declared order.
- **Applies to:** all batches

### Decision: batch-identity-is-untouched

- **Decision:** `batchIdentity` still returns the first card's `Number`/`Slug`;
  `ReportFileName(number, slug)` is unchanged;
  `State.Batches` stays keyed by that number;
  the reserved `-1` integration key is unaffected.
  Only the *order of the slice* changes.
- **Rationale:** `discussion.md`'s `batch-identity-unchanged` decision.
  Renumbering to execution position would invalidate every on-disk report and state file and break crash-resume.
- **Applies to:** all batches

### Decision: go-comment-and-markdown-style

- **Decision:** Go doc comments follow the `golang:golang-comments` conventions already in force across `internal/websterengine` — a file-level banner comment on every new `.go` file naming what the file implements, and a doc comment on every exported identifier.
  Every `.md` file this plan touches uses semantic line breaks (one sentence per line, plus a break at an internal independent-clause boundary), never fixed-column hard-wrap, per `CLAUDE.md`.
- **Rationale:** `CLAUDE.md`'s markdown rule binds every `.md` file in the repo, not only newly-written ones;
  matching the surrounding Go comment density is the house style `internal/websterengine` already sets.
- **Applies to:** all batches

### Decision: no-planparser-and-no-batcher-change

- **Decision:** `internal/planparser` and `internal/batcher` are not modified by any card in this plan — no new parse rule, no new validation check, no new `Card` field, no exported ref classifier, no new registry entry.
  Ref matching is exact string equality over the refs `planparser` already hands back.
- **Rationale:** `discussion.md`'s `no-planparser-change` decision plus the **Planparser Sole-Parser Invariant** and the **Batcher Registry+Config Invariant**.
  `normalizeCard` runs once per card inside `ParsePlan`, so string equality across two cards' ref lists is well-defined without touching either package.
- **Applies to:** all batches

## All Files Touched

- `contracts/stencils/webster/webster-template-master.md`
- `contracts/specs/loom-plan-spec.md`
- `docs/overview.md`
- `internal/websterengine/beginbatch.go`
- `internal/websterengine/beginbatch_test.go`
- `internal/websterengine/doc.go`
- `internal/websterengine/recoverbatch.go`
- `internal/websterengine/recoverbatch_test.go`
- `internal/websterengine/render.go`
- `internal/websterengine/runlevel.go`
- `internal/websterengine/runlevel_test.go`
- `internal/websterengine/sequence.go`
- `internal/websterengine/sequence_test.go`
- `internal/websterengine/template_test.go`
- `internal/webstercli/awaitbatch.go`
- `internal/webstercli/beginbatch.go`
- `internal/webstercli/recordbatch.go`
- `internal/webstercli/recoverbatch.go`
- `internal/webstercli/run.go`
- `manifest/roadmap.md`
