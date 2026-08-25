# Plan: loom: Plan-Review producer

```yaml
task: 'loom: Plan-Review producer'
slug: 'loom-plan-review-producer'
approved: false
started: '20260825-090716'
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
    name: plan-review-rubric-stencil
    file: 01-plan-review-rubric-stencil.md
    depends-on: []
    verify: go test ./contracts/stencils/...
  - number: 2
    name: bouncer-commit-seam
    file: 02-bouncer-commit-seam.md
    depends-on: []
    verify: go test ./internal/shedadapters/... ./internal/shedrecipe/...
  - number: 3
    name: plan-review-segment-rows
    file: 03-plan-review-segment-rows.md
    depends-on: [1, 2]
    verify: go build ./... && go test ./internal/loomshed/... ./internal/loomrecipe/... ./internal/shedbuild/...
  - number: 4
    name: docs-and-stale-text-sweep
    file: 04-docs-and-stale-text-sweep.md
    depends-on: [3]
    verify: go build ./... && go test ./internal/loomengine/... ./internal/loomcli/... && go vet -tags smoke ./internal/loomcli/...
```

## Shared Decisions

_Cross-cutting decisions every batch inherits: naming conventions, error-handling posture, test frameworks, style/lint constraints.
One subsection per decision.
Batch-local decisions live in each batch file._

### Decision: source-first card order, never a non-compiling commit

- **Decision:** within every batch, the production change lands before the test that asserts it, even where `_mill/discussion.md` names a step as a TDD candidate.
- **Rationale:** mill-go commits once per card, and a test card committed ahead of the symbol it references produces a commit where `go build ./...` fails.
  The discussion's TDD framing is about which assertions carry the most value, not about commit ordering;
  the batch's own `verify:` is what actually runs the tests, and it runs after every card in the batch has landed.
- **Applies to:** all batches

### Decision: fifteen recipe rows, still fourteen registry engines

- **Decision:** loom's recipe row list goes from fourteen rows to fifteen.
  `internal/shedrecipe`'s engine registry stays at fourteen entries, and `TestRegistry_ShipsFourteenEntries` is not touched.
  `manifest/designs/loom.md`'s own producer table also stays at fourteen entries.
- **Rationale:** the two new rows reuse the already-registered `Bouncer` and `BurlerRound` engines, so no registry entry is added — the Shed Recipe Registry Invariant is untouched.
  The design table already collapses each review segment's two recipe rows into one entry by design and says so in its own text.
  Confusing the three counts is the likeliest mistake in this task, so every "fourteen" hit is classified against which fourteen it counts before it is edited.
- **Applies to:** all batches

### Decision: an approved plan is committed by the loop owner, a blocked one is not

- **Decision:** `Plan-Burler` runs `fix-scope: overlay`, which performs no git of its own, so `Plan-Bouncer` carries `commit_seam: plan` and `shedadapters.Bouncer.settle` calls the injected `Commit` closure on the `verdictApproved` branch only.
  A non-nil error from `Commit` is returned as `settle`'s own error;
  it is never routed through `degrade` and never swallowed into a `Done`-with-warning.
  The commit is attempted even under an already-cancelled context.
- **Rationale:** the Fabric Git Invariant reserves committing weft content to Go, never to an agent, so `fix-scope: source` is doctrinally forbidden here;
  and nothing else in the segment commits, so an overlay round with no seam would leave every approved fix as uncommitted weft dirt.
  `degrade` returns `Stuck`, which would bounce an APPROVED plan into a findings-free fixer round that re-approves and re-commits every bounce until the budget is spent.
  The blocked path leaving the working tree dirty is deliberate: an unapproved plan must not be committed, and a blocked run has already escalated to a human.
- **Applies to:** bouncer-commit-seam, plan-review-segment-rows

### Decision: `commit_seam` is a selector, not a seam

- **Decision:** the new recipe key carries one of exactly two literal values, `plan` and `discussion`, resolving to `Env.CommitPlan` and `Env.CommitDiscussion`.
  An absent key leaves `BouncerConfig.Commit` nil, which means "commit nothing".
  A **present** key naming a closure `Env` does not carry is a construction error via the existing `requireSeam`, never a silent nil.
- **Rationale:** `manifest/designs/shed-recipe.md` bars live seams from a recipe outright, and this key does not break that rule — it names which of the two closures the told `Env` already carries to use, exactly as `rubric_stencil` names a stencil rather than carrying one.
  A two-value enum matching the two closures `Env` actually has is the whole vocabulary that exists.
  Guarding on presence rather than on the field is what keeps the absent key a legitimate nil while making a configured-but-missing seam loud.
- **Applies to:** bouncer-commit-seam, plan-review-segment-rows

### Decision: `pipeline.done_gate` is left unchanged

- **Decision:** `mill-config.yaml`'s existing `done_gate` (`go test ./... && go test -tags integration ./...`) is kept as-is;
  no lint command is appended and no `mill-config.yaml` edit is part of this plan.
- **Rationale:** the repo-wide test command already covers every package this plan touches, including the ones no batch `verify:` scopes.
  `golangci-lint` is not installed on this machine (checked at plan time — `command -v golangci-lint` finds nothing), so appending it would make every future task in this hub depend on a tool that is not there.
  `internal/loomcli`'s `smoke` suite is excluded from both halves of the gate by its own build tag, which is why batch 4 compiles it explicitly with `go vet -tags smoke` instead.
- **Applies to:** all batches

### Decision: `_lyx` paths resolve under `Env.WorktreeRoot`, knowingly not `AnchorPath()`

- **Decision:** both new rows' `_lyx/plan` entries resolve against `Env.WorktreeRoot`, while `Env.CommitPlan` commits the same directory anchored at `AnchorPath()`.
  The divergence is recorded in the recipe comment and left unfixed here.
- **Rationale:** the two roots are identical whenever `AnchorRel` is `"."`, which is its default, and the already-shipped Discussion pair carries the identical shape.
  Re-pointing the resolution root would silently change a shipped segment's behaviour, so the fix is folded into the same follow-up roadmap item as the `Discussion-Burler` `fix-scope` correction.
- **Applies to:** plan-review-segment-rows, docs-and-stale-text-sweep

## All Files Touched

_Full union of every `Creates:` / `Edits:` / `Moves:` **target** path across every batch, sorted alphabetically (Move **source** paths are excluded — they disappear, like `Deletes:` tokens).
Cards are the source of truth;
this section is the input `_plan_validate.py`'s `all-files-touched-mismatch` check cross-references against the derived union of every card's `Edits:`/`Creates:`/Move-target paths, to catch drift between the hand/agent-maintained list here and that derived union._

- `contracts/recipes/loom-recipe.yaml`
- `contracts/stencils/loom/loom-rubric-plan-review.md`
- `contracts/stencils/rubric_test.go`
- `contracts/stencils/stencils.go`
- `internal/loomcli/smoke_test.go`
- `internal/loomcli/wiring.go`
- `internal/loomcli/wiring_test.go`
- `internal/loomengine/config.go`
- `internal/loomrecipe/coverage_guard_test.go`
- `internal/loomrecipe/fixture_test.go`
- `internal/loomrecipe/recipe_test.go`
- `internal/loomrecipe/sequence_test.go`
- `internal/loomrecipe/shape_test.go`
- `internal/loomshed/doc.go`
- `internal/loomshed/loomshed.go`
- `internal/loomshed/stub.go`
- `internal/shedadapters/bouncer.go`
- `internal/shedadapters/bouncer_commit_test.go`
- `internal/shedadapters/bouncer_seed_test.go`
- `internal/shedrecipe/entries_bouncer.go`
- `internal/shedrecipe/entries_bouncer_test.go`
- `manifest/designs/loom.md`
- `manifest/designs/review-finding-classification.md`
- `manifest/designs/shed-recipe.md`
- `manifest/designs/shed.md`
- `manifest/roadmap.md`
