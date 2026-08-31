# Plan: Surface merge-in-progress in fabric status

```yaml
task: "Surface merge-in-progress in fabric status"
slug: "fabric-status-merge-in-progress"
approved: false
started: "20260831-181430"
parent: "main"
root: ""
verify: null
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to schedule batches.
Every batch lives at `NN-<batch-slug>.md` in this directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: status-merge-in-progress
    file: 01-status-merge-in-progress.md
    depends-on: []
    verify: go test ./internal/fabriccli/... && go test -tags integration ./internal/fabriccli/...
```

## Shared Decisions

_Cross-cutting decisions every batch inherits: naming conventions, error-handling posture, test frameworks, style/lint constraints.
One subsection per decision.
Batch-local decisions live in each batch file._

### Decision: single-batch-scope

- **Decision:** the whole task is one batch of two cards — a TDD-red integration test, then the CLI field plus every doc artefact that describes `lyx fabric status`'s output.
- **Rationale:** the change is one Go edit inside one existing `RunE`, one new test file, and five prose edits, all sharing the same `Context:` set. Splitting it would produce two batches sharing well over 80% of their context, which the batch-sizing rule says to merge. It sits far under `pipeline.max_cards_per_batch` (20) and `pipeline.max_batch_context_tokens` (700000).
- **Applies to:** all batches

### Decision: docs-land-with-the-behavior-change

- **Decision:** card 2 carries the CLI change AND every doc edit AND the roadmap move in one commit. The docs are not a separate card.
- **Rationale:** `CLAUDE.md`'s "Task completion — docs land in the same commit" requires the module doc, `docs/overview.md`, and the roadmap move to land with an observable CLI behavior change, and `_mill/discussion.md`'s `docs-in-same-commit` decision names the exact artefact set. mill-plan's own principle forbids expressing a must-land-together set as two cards linked by a cross-card instruction, so it is one card with one `Commit:` message. Card 1 (the test) is deliberately outside that atom: a test that fails before the fix is its own prior commit, not part of the behavior change.
- **Applies to:** all batches

### Decision: field-is-this-pair-only

- **Decision:** `merge_in_progress` answers "does **this pair** have a fabric merge parked", and every artefact this plan writes says so in those terms. No artefact may describe it as "a merge is in progress" unqualified.
- **Rationale:** `*ErrMergeInProgress` is raised from two different predicates — the this-pair `mergeRecordExists`, which `MergeInProgress()` delegates to, and the hub-wide `mergeSourceInFlight`, which it does not. `remove` refuses on either, so `merge_in_progress: false` beside a refusing `remove` is correct behavior that an unqualified description would make read as a bug. See `_mill/discussion.md`'s `field-is-this-pair-only` decision.
- **Applies to:** all batches

### Decision: read-only-envelope-unchanged

- **Decision:** `status` stays a read-only verb. The new field goes into the existing `output.Ok` map literal and does NOT route through `okWithRecord`, so no `mutations` and no `partial` key appear.
- **Rationale:** nothing is mutated, and `internal/fabriccli/envelope.go`'s file-header comment names `status` as one of the four read-only verbs that deliberately skip the record helpers. `TestRunCLI_ReadOnlyVerbsOmitMutationsKey` pins this and must stay green **without modification** — an implementation that finds itself editing that test took the wrong path.
- **Applies to:** all batches

### Decision: engine-is-untouched

- **Decision:** no file under `internal/fabricengine` changes except `doc.go`'s merge-section prose. `MergeInProgress()`'s signature, semantics, and doc comment are used exactly as they ship.
- **Rationale:** `_mill/discussion.md`'s Scope/Out section rules out engine behavior change, the hub-wide `mergeSourceInFlight` sense, richer merge detail, a new verb, and a second foreign-merge-state field. Each of those needs new exported engine API that the roadmap item does not ask for.
- **Applies to:** all batches

### Decision: integration-build-tier

- **Decision:** the new test file carries `//go:build integration` on line 1, and verification runs both tiers over `./internal/fabriccli/...`.
- **Rationale:** the test calls `hubforge.NewHub`, which `CONSTRAINTS.md`'s Test Tier Purity Invariant bars from untagged files. An untagged `go test ./internal/fabriccli/...` compiles none of the tagged files, so without `-tags integration` the TDD red step and both scenarios would silently not run and the pass would be vacuous. The untagged tier still runs, to keep the pure-cobra regression pins honest.
- **Applies to:** all batches

### Decision: error-path-intentionally-untested

- **Decision:** the `MergeInProgress()` error branch is shipped but not covered by a test scenario, and this plan records that as deliberate.
- **Rationale:** inducing the error means building a torn or unreadable `fabric-merge.json` past the engine's own API — platform-dependent, and it would pin a test to today's internal record layout rather than to CLI behavior. The guarded branch is copied verbatim from the adjacent `fab.Status()` error path. Do not invent an engine-level seam or export a test hook to reach it; that would be new API, which Scope/Out rules out.
- **Applies to:** all batches

## All Files Touched

_Full union of every `Creates:` / `Edits:` / `Moves:` **target** path across every batch, sorted alphabetically (Move **source** paths are excluded — they disappear, like `Deletes:` tokens).
Cards are the source of truth;
this section is the input `_plan_validate.py`'s `all-files-touched-mismatch` check cross-references against the derived union of every card's `Edits:`/`Creates:`/Move-target paths, to catch drift between the hand/agent-maintained list here and that derived union._

- `docs/overview.md`
- `internal/fabriccli/status_mergeinprogress_integration_test.go`
- `internal/fabriccli/weft_verbs.go`
- `internal/fabricengine/doc.go`
- `manifest/roadmap.md`
- `tools/sandbox/SANDBOX-FABRIC-SUITE.md`
