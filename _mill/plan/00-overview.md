# Plan: loom: Plan-Write/Plan-Validate approval deadlock (F7)

```yaml
task: 'loom: Plan-Write/Plan-Validate approval deadlock (F7)'
slug: 'loom-plan-approval-gate'
approved: true
started: '20260826-114441'
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
    name: planparser-split-and-writer
    file: 01-planparser-split-and-writer.md
    depends-on: []
    verify: go test ./internal/planparser/...
  - number: 2
    name: shedadapters-approve-seam
    file: 02-shedadapters-approve-seam.md
    depends-on: []
    verify: go test ./internal/shedadapters/...
  - number: 3
    name: planvalidate-two-mode
    file: 03-planvalidate-two-mode.md
    depends-on: [1]
    verify: go test ./internal/loomshed/... ./internal/shedrecipe/... ./internal/loomrecipe/... ./internal/loomcli/...
  - number: 4
    name: shedrecipe-approve-seam
    file: 04-shedrecipe-approve-seam.md
    depends-on: [2]
    verify: go test ./internal/shedrecipe/...
  - number: 5
    name: loomcli-wiring-and-flag
    file: 05-loomcli-wiring-and-flag.md
    depends-on: [3, 4]
    verify: go test ./internal/loomcli/... ./cmd/lyx/...
  - number: 6
    name: recipe-wiring-and-regression
    file: 06-recipe-wiring-and-regression.md
    depends-on: [5]
    verify: go test ./internal/loomrecipe/...
  - number: 7
    name: docs-and-constraints
    file: 07-docs-and-constraints.md
    depends-on: [6]
    verify: go test ./internal/lyxcwd/...
```

## Shared Decisions

_Cross-cutting decisions every batch inherits: naming conventions, error-handling posture, test frameworks, style/lint constraints.
One subsection per decision.
Batch-local decisions live in each batch file._

### Decision: approval-write-lives-on-the-bouncer-settle

- **Decision:** the approval write is an injected `Approve func() error` closure on `shedadapters.BouncerConfig`, invoked on `settle`'s `verdictApproved` branch immediately before `b.cfg.Commit`, selected by an `approve_seam: plan` key on the `Plan-Bouncer` recipe row.
  No new recipe row, no new `Name*` constant, no second commit.
- **Rationale:** `Plan-Bouncer`'s approved settle is the only point that knows the plan passed review and the only point that can guarantee write-then-commit ordering in one step — the `approved: true` byte must land inside the commit `Commit` makes.
  It reuses the already-shipped `commit_seam` seam shape one-for-one, so the generic `Bouncer` gains no plan-specific knowledge.
- **Applies to:** all batches

### Decision: validate-splits-into-two-named-functions

- **Decision:** `internal/planparser` exposes `ValidateFormat` (fifteen check IDs) and `Validate` (those fifteen plus `plan-unapproved`, sixteen in all).
  Both are thin wrappers over one unexported `validate(plan, worktreeRoot string, requireApproved bool)`.
  `Validate` splices `plan-unapproved` at position two, preserving `contracts/specs/loom-plan-spec.md`'s fixed sixteen-row order byte-for-byte.
- **Rationale:** two named functions leave every existing `planparser.Validate` call site's signature untouched, and the spec already frames `plan-unapproved` as a consumer guard ("`approved: true`; else refuse to run"), which is exactly the split this makes explicit.
- **Applies to:** all batches

### Decision: planvalidate-row-mode

- **Decision:** `loomshed.NewPlanValidate` gains a trailing `requireApproved bool` parameter.
  `shedrecipe`'s `planValidateEntry` reads a new optional `require_approved` bool config key (absent ⇒ `false`) to supply it.
  The recipe sets `require_approved: true` on `Plan-Revalidate` and leaves the key absent on `Plan-Validate`.
- **Rationale:** the two rows deliberately share one engine and the recipe is where their difference already lives.
  `Plan-Validate` runs before review and must not demand a flag only review can produce;
  `Plan-Revalidate` runs after the segment settles and must confirm the flag is there.
- **Applies to:** batch 3, batch 6

### Decision: approve-failure-is-an-error-not-a-stuck

- **Decision:** in `Bouncer.settle`, a non-nil error from `Approve` is returned as `settle`'s own error, never routed through `degrade`, exactly as the existing `Commit` failure already is. `Approve` runs first;
  if it fails, `Commit` is not attempted.
- **Rationale:** `degrade` only ever returns `shedengine.Stuck`, so sending an approval-write failure through it would silently convert an approval into a rejection.
  A failed write would also mean the commit captures a plan still marked unapproved.
- **Applies to:** batch 2

### Decision: a-failed-settle-seam-costs-one-review-generation-and-that-is-accepted

- **Decision:** when `Approve` or `Commit` fails, `settle` returns the error, the run persists `failed`, and the operator resumes;
  the resume re-enters `Plan-Bouncer.Call`, archives, re-seeds, and spends a complete new LLM review generation.
  No retry loop is added inside `settle` and no short-circuit is added to the clear-and-re-seed trigger.
- **Rationale:** both seams are idempotent and the judge never reads the approval flag, so the state converges without intervention.
  The durable `settled`-marker fix that would remove the cost changes the generic `Bouncer`'s on-disk contract for two segments this task does not otherwise touch — filed as a follow-up instead.
- **Applies to:** batch 2

### Decision: writer-never-self-approves

- **Decision:** `Plan-Write` keeps writing `approved: false`, and the plan stencil keeps the "Always write `approved: false` — you never self-approve" rule and its Step 5 self-check block verbatim.
  Only the trailing clause promising "a future review gate flips it to `true`" is corrected to name `Plan-Bouncer`'s approved settle.
  The `Plan-Burler` fixer round is prohibited in prose from writing `approved:` at all.
- **Rationale:** the separation is what makes the review gate mean anything, and the verb's new default mode is precisely what makes the stencil's "re-run it until it exits 0" instruction satisfiable for the first time.
- **Applies to:** batch 6, batch 7

### Decision: consumers-keep-enforcing-approval

- **Decision:** `internal/websterengine`, `internal/webstercli`, and `internal/batcher` are untouched;
  they keep calling the full `planparser.Validate` and keep refusing an unapproved plan.
- **Rationale:** they run after `Plan-Bouncer` has settled, so the flag is genuinely there by then, and the refusal is the standalone-invocation guard the spec's "else refuse to run" wording was written for.
- **Applies to:** all batches

### Decision: go-native-verify-commands

- **Decision:** every batch `verify:` is a plain `go test` invocation scoped to the packages the batch touches, with no `PYTHONPATH=` prefix.
- **Rationale:** this is a Go repo;
  the `PYTHONPATH=` isolation prefix applies to Python/mill projects only.
  Repo-wide coverage is already supplied by `pipeline.done_gate` (`go test ./... && go test -tags integration ./...`), which needs no change for this task.
- **Applies to:** all batches

## All Files Touched

_Full union of every `Creates:` / `Edits:` / `Moves:` **target** path across every batch, sorted alphabetically (Move **source** paths are excluded — they disappear, like `Deletes:` tokens).
Cards are the source of truth;
this section is the input `_plan_validate.py`'s `all-files-touched-mismatch` check cross-references against the derived union of every card's `Edits:`/`Creates:`/Move-target paths, to catch drift between the hand/agent-maintained list here and that derived union._

- `CONSTRAINTS.md`
- `contracts/recipes/loom-recipe.yaml`
- `contracts/specs/loom-plan-spec.md`
- `contracts/stencils/loom/loom-rubric-plan-review.md`
- `contracts/stencils/loom/loom-template-plan.md`
- `internal/loomcli/parity_test.go`
- `internal/loomcli/validate.go`
- `internal/loomcli/validate_test.go`
- `internal/loomcli/wiring.go`
- `internal/loomengine/plan.go`
- `internal/loomrecipe/approveseam_test.go`
- `internal/loomrecipe/fixture_test.go`
- `internal/loomrecipe/revalidate_test.go`
- `internal/loomrecipe/sequence_test.go`
- `internal/loomrecipe/shape_test.go`
- `internal/loomshed/cancellation_test.go`
- `internal/loomshed/gatefindings_test.go`
- `internal/loomshed/planvalidate.go`
- `internal/loomshed/planvalidate_test.go`
- `internal/planparser/approve.go`
- `internal/planparser/approve_test.go`
- `internal/planparser/doc.go`
- `internal/planparser/validate.go`
- `internal/planparser/validate_test.go`
- `internal/shedadapters/bouncer.go`
- `internal/shedadapters/bouncer_commit_test.go`
- `internal/shedrecipe/entries_bouncer.go`
- `internal/shedrecipe/entries_bouncer_test.go`
- `internal/shedrecipe/entries_simple.go`
- `internal/shedrecipe/entries_simple_test.go`
- `internal/shedrecipe/recipe.go`
- `manifest/designs/loom.md`
