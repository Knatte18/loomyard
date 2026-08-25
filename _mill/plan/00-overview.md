# Plan: loom: Webster-Review producer

```yaml
task: 'loom: Webster-Review producer'
slug: 'loom-webster-review-producer'
approved: true
started: '20260825-121952'
parent: 'main'
root: ""
verify: null
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to schedule batches.
Every batch lives at `NN-<batch-slug>.md` in this directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: rubric-stencil
    file: 01-rubric-stencil.md
    depends-on: []
    verify: go build ./... && go test ./contracts/stencils/... ./internal/lyxcwd/...
  - number: 2
    name: perch-wiring
    file: 02-perch-wiring.md
    depends-on: [1]
    verify: go build ./... && go test ./internal/loomshed/... ./internal/loomrecipe/... ./internal/shedrecipe/... ./internal/shedbuild/...
  - number: 3
    name: docs-and-roadmap
    file: 03-docs-and-roadmap.md
    depends-on: [2]
    verify: go build ./... && go vet -tags smoke ./internal/loomcli/... && go test ./internal/lyxcwd/...
```

## Shared Decisions

### Decision: the-perch-is-hand-wired-from-shipped-config-keys-only

- **Decision:** the `Webster-Review` segment is built entirely out of config keys `internal/shedrecipe`'s `Bouncer` and `BurlerRound` entries already recognise.
  No Go production code under `internal/shedadapters`, `internal/burlerengine`, `internal/shedrecipe`, or `internal/websterengine` changes in this task.
  The only Go production edits are `internal/loomshed`'s row-name constants and comment text.
- **Rationale:** `bouncerEntry` recognises exactly `run_subdir`, `artifact_paths`, `rubric_stencil`, `model`, `effort`, `version`, `commit_seam`, and `burlerRoundProfile` recognises exactly `target`, `fasit`, `rubric`, `rubric_stencil`, `fix-scope`, `tool-use`, `cluster-fan`.
  Both shipped perches (`Discussion-Review`, `Plan-Review`) are built from that same key set, so this segment needs no new seam.
- **Applies to:** all batches

### Decision: the-diff-derivation-lives-only-in-the-rubric-stencil

- **Decision:** the review range (`git diff $(git merge-base <product.parent> HEAD)..HEAD`, with `product.parent` read from `_lyx/loom/status.json`) is written down exactly once, under its own named heading in `contracts/stencils/loom/loom-rubric-webster-review.md`.
  `Webster-Burler`'s `profile.target.instructions` points at that heading and restates none of the derivation.
- **Rationale:** the Producer Pointer-Rule Invariant is the rule against an instruction file duplicating content it could point at, and both rows of the perch already read the rubric, so nothing would pin a recipe copy against a stencil copy.
- **Applies to:** all batches

### Decision: no-cluster-fan-and-no-commit-seam

- **Decision:** `Webster-Burler`'s profile sets no `cluster-fan` key, and `Webster-Bouncer` sets no `commit_seam` key.
  `Webster-Burler` runs `fix-scope: source`.
- **Rationale:** fork reviewers are read-only and may never run git (`internal/burlerengine/prompt.go`, audit-enforced), so under a fan the forks cannot reach a subject that exists only through git.
  With `fix-scope: source` the fixer commits its own work, so there is no artifact left for a loop-owner commit seam — the shipped no-seam configuration `bouncerEntry` documents as "a legitimate configuration and never an error".
- **Applies to:** all batches

### Decision: the-row-count-criterion-not-the-enumeration-is-authoritative

- **Decision:** every count of loom's **producer rows** moves from sixteen to seventeen.
  Every other "sixteen" in the repo stays: `internal/planparser`'s sixteen validation check IDs (`validate.go`, `validate_test.go`, `contracts/specs/loom-plan-spec.md`, `contracts/stencils/loom/loom-rubric-plan-review.md`, `manifest/designs/plan-card-format.md`, `manifest/designs/loom.md`'s check-ID mention) and `internal/fabricengine/doc.go`'s sixteen destruction kinds.
- **Rationale:** `manifest/designs/loom.md` carries both kinds, so a blind sweep of the token would corrupt the check-ID count in the same file whose row counts must move.
  The criterion, not the per-file enumeration, is what a later reader applies.
- **Applies to:** all batches

### Decision: stub-stays-registered-and-becomes-an-allowed-unreachable-engine

- **Decision:** `internal/loomshed/stub.go`, `internal/loomshed/stub_test.go`, `internal/shedrecipe`'s `stubEntry` and `"Stub"` registry row, and `internal/shedrecipe/registry_test.go`'s `TestRegistry_ShipsFourteenEntries` are all left untouched.
  `"Stub"` joins `coverageGuardAllowedUnreachableEngines` in `internal/loomrecipe/coverage_guard_test.go`.
- **Rationale:** `shedrecipe`'s registry is generic `Shed` machinery shared by reference with `Hardener`'s future producer list, not loom's private property.
  Deleting the engine is a decision about that registry's surface, not a consequence of loom finishing its own list.
- **Applies to:** batch 2

### Decision: done-gate-left-as-configured

- **Decision:** `pipeline.done_gate` is left at its existing value, `go test ./... && go test -tags integration ./...`.
  No `mill-config.yaml` change is in this plan.
- **Rationale:** the hub already configures a repo-wide gate that covers every package outside the batch verify scopes, which is exactly what the batch-scoped `verify:` commands below do not reach.
- **Applies to:** all batches

## All Files Touched

- `contracts/recipes/loom-recipe.yaml`
- `contracts/specs/loom-status-spec.md`
- `contracts/stencils/loom/loom-rubric-webster-review.md`
- `contracts/stencils/rubric_test.go`
- `contracts/stencils/stencils.go`
- `internal/loomcli/smoke_test.go`
- `internal/loomrecipe/coverage_guard_test.go`
- `internal/loomrecipe/fixture_test.go`
- `internal/loomrecipe/recipe_test.go`
- `internal/loomrecipe/sequence_test.go`
- `internal/loomrecipe/shape_test.go`
- `internal/loomshed/doc.go`
- `internal/loomshed/loomshed.go`
- `internal/loomshed/stub.go`
- `manifest/designs/loom.md`
- `manifest/designs/shed-recipe.md`
- `manifest/roadmap.md`
