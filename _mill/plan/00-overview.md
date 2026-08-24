# Plan: loom: Plan-Write producer

```yaml
task: 'loom: Plan-Write producer'
slug: 'loom-plan-write-producer'
approved: false
started: '20260824-181800'
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
    name: planparser-archive-name-and-loomshed-decorator
    file: 01-planparser-archive-name-and-loomshed-decorator.md
    depends-on: []
    verify: go test ./internal/planparser/... ./internal/loomshed/...
  - number: 2
    name: shedrecipe-entry-and-recipe-row
    file: 02-shedrecipe-entry-and-recipe-row.md
    depends-on: [1]
    verify: go test ./internal/shedrecipe/... ./internal/shedbuild/... ./internal/loomrecipe/...
  - number: 3
    name: loomcli-wiring
    file: 03-loomcli-wiring.md
    depends-on: [2]
    verify: go test ./internal/loomcli/...
  - number: 4
    name: stencil-prompt-and-docs
    file: 04-stencil-prompt-and-docs.md
    depends-on: [3]
    verify: go test ./internal/loomengine/...
```

## Shared Decisions

_Cross-cutting decisions every batch inherits: naming conventions, error-handling posture, test frameworks, style/lint constraints.
One subsection per decision.
Batch-local decisions live in each batch file._

### Decision: mirror the Discussion-Write commit verbatim wherever the shape transfers

- **Decision:** every file this plan touches has a Discussion-Write sibling shipped in commit `c2638bb3`, and the sibling's file shape, doc-comment style, error-text prefix, and test topology are reproduced rather than re-invented. `planwrite.go` mirrors `discussionwrite.go`; `entries_planwrite.go` mirrors `entries_discussionwrite.go`; `planwrite_test.go` mirrors `discussionwrite_test.go`; `entries_planwrite_test.go` mirrors `entries_discussionwrite_test.go`.
- **Rationale:** `_mill/discussion.md`'s Technical context section names that commit as "the closest possible template for this task — same seam, same decorator shape, same `Env` extension, same test topology". Deviating stylistically from a one-commit-old sibling forks a convention for no gain.
- **Applies to:** all batches

### Decision: the decorator takes the anchor path, not the plan directory

- **Decision:** `loomshed.NewPlanWrite` takes `anchorPath string` and calls `planparser.PlanDir(anchorPath)` itself; `planWriteEntry` validates `Env.AnchorPath` with the existing `requireAbsRoot` helper and passes it through.
- **Rationale:** `loomshed.NewPlanValidate` already takes `anchorPath` and calls `planparser.PlanDir` internally for exactly this reason, and doing the same here keeps `internal/shedrecipe` free of any `planparser` import. `_mill/discussion.md`'s Decisions list the three seams the entry validates but does not name a root; `requireAbsRoot("PlanWrite", "AnchorPath", ...)` is the established fourth check `planValidateEntry` already performs, so this is following the sibling rather than adding a rule.
- **Applies to:** planparser-archive-name-and-loomshed-decorator, shedrecipe-entry-and-recipe-row

### Decision: rotation failure and commit failure are both returned errors, never Stuck

- **Decision:** `planWrite.Call` rotates first and returns a wrapped error on any rotation failure without ever invoking the inner producer; on a `Done`-with-nil-error inner outcome it invokes `commit` and returns a wrapped error on failure. Neither maps to `shedengine.Stuck`.
- **Rationale:** verbatim `_mill/discussion.md`'s `rotation-failure-is-an-error-never-stuck` and `commit-failure-is-an-error-never-stuck` Decisions — a filesystem or git fault is infrastructure, not plan quality, and `Stuck` persists blocked and bounces while a returned error persists failed and aborts.
- **Applies to:** planparser-archive-name-and-loomshed-decorator

### Decision: CONSTRAINTS.md is edited for a factual correction only, not for a new invariant

- **Decision:** `_mill/discussion.md`'s `no-new-invariant` Decision holds — no new invariant is recorded. But the Shed Recipe Registry Invariant's own **Enforced by** line names `TestRegistry_ShipsThirteenEntries` and "the registry's exact thirteen names", both of which this task's registry growth falsifies. That one line is corrected in the same card that renames the test.
- **Rationale:** the `no-new-invariant` Decision rules out *adding* an invariant; it does not license leaving an existing invariant's enforcement pointer naming a test symbol that no longer exists. The identical correction was made by the Discussion-Write commit (`TestRegistry_ShipsTwelveEntries` → `TestRegistry_ShipsThirteenEntries`) in its own commit, so this is the established precedent, not a scope expansion.
- **Applies to:** shedrecipe-entry-and-recipe-row

### Decision: the `Plan-never-reads-support-log` build-time assertion lands here

- **Decision:** a new `TestPlanSpec_PromptNeverNamesSupportLog` in `internal/loomengine/plan_test.go` asserts the composed plan prompt names neither the literal `support-log.md` nor `DiscussionSupportLog(layout)`'s absolute path.
- **Rationale:** `manifest/designs/loom.md`'s "The `Plan-never-reads-support-log` boundary is not a per-run check" paragraph states outright that the assertion "lands with the real `Plan-Write`", and that the reason it had not landed already was that a stub declares no input set. This task makes `Plan-Write` real, so the documented obligation comes due here; leaving it unwritten would require editing that paragraph to defer it again. `_mill/discussion.md` neither lists nor excludes it, and it costs one test.
- **Applies to:** stencil-prompt-and-docs

### Decision: the shared loomrecipe fake shuttle dispatches on `Spec.Role`

- **Decision:** `internal/loomrecipe/fixture_test.go`'s single `shedrecipe.Env.Shuttle` value now serves two real LLM rows, so its fake is generalised to branch on `spec.Role`: `"plan"` writes the whole plan-directory fixture, anything else keeps today's discussion behaviour over `spec.OutputFiles`. Both of the fixture's `SpecSource` closures set `Role` explicitly so the branch is total.
- **Rationale:** `Env` carries one `Shuttle` field, not one per row, and Plan-Write's output is a whole directory (an overview plus at least one card file) rather than the `OutputFiles` list alone — writing only `OutputFiles` would leave the Card Index referencing a card file the rotation had just archived away, and `Plan-Validate` would report `Stuck` and bounce. Branching on `Role` is the only fixture-visible signal that distinguishes the two rows without threading a second seam through `Env`.
- **Applies to:** shedrecipe-entry-and-recipe-row

### Decision: Go verify commands, no `PYTHONPATH=` prefix

- **Decision:** every `verify:` in this plan is a bare `go test` invocation scoped to the packages the batch touches.
- **Rationale:** this is a Go repository; the `PYTHONPATH= ` prefix rule applies to Python/mill projects only. The repo-wide regression net is already configured as `pipeline.done_gate` (`go test ./... && go test -tags integration ./...`), which mill-go runs from the git root before marking the task done, so per-batch scopes stay narrow.
- **Applies to:** all batches

## All Files Touched

_Full union of every `Creates:` / `Edits:` / `Moves:` **target** path across every batch, sorted alphabetically (Move **source** paths are excluded — they disappear, like `Deletes:` tokens)._

- `CONSTRAINTS.md`
- `contracts/recipes/loom-recipe.yaml`
- `contracts/stencils/loom/loom-template-plan.md`
- `internal/loomcli/wiring.go`
- `internal/loomcli/wiring_test.go`
- `internal/loomengine/plan_test.go`
- `internal/loomrecipe/coverage_guard_test.go`
- `internal/loomrecipe/fixture_test.go`
- `internal/loomrecipe/resume_test.go`
- `internal/loomrecipe/sequence_test.go`
- `internal/loomrecipe/shape_test.go`
- `internal/loomshed/doc.go`
- `internal/loomshed/planwrite.go`
- `internal/loomshed/planwrite_test.go`
- `internal/loomshed/stub.go`
- `internal/planparser/parse.go`
- `internal/planparser/planpath_test.go`
- `internal/shedbuild/build_engines_test.go`
- `internal/shedbuild/fixture_test.go`
- `internal/shedrecipe/entries_planwrite.go`
- `internal/shedrecipe/entries_planwrite_test.go`
- `internal/shedrecipe/fixture_test.go`
- `internal/shedrecipe/recipe.go`
- `internal/shedrecipe/registry.go`
- `internal/shedrecipe/registry_test.go`
- `manifest/designs/loom.md`
- `manifest/designs/shed-recipe.md`
- `manifest/roadmap.md`
- `plugins/scribe/skills/INDEX.md`
