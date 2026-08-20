# Plan: shedengine: per-producer bounce budget + explicit OnDone routing

```yaml
task: 'shedengine: per-producer bounce budget + explicit OnDone routing'
slug: 'shedengine-segments-bounce-budget'
approved: false
started: '20260820-090958'
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
    name: engine-fields-and-validation
    file: 01-engine-fields-and-validation.md
    depends-on: []
    verify: go test ./internal/shedengine/...
  - number: 2
    name: run-routing-and-budget
    file: 02-run-routing-and-budget.md
    depends-on: [1]
    verify: go test ./internal/shedengine/...
  - number: 3
    name: loomshed-migration
    file: 03-loomshed-migration.md
    depends-on: [2]
    verify: go test ./internal/loomshed/...
  - number: 4
    name: docs-sweep
    file: 04-docs-sweep.md
    depends-on: [3]
    verify: go build ./... && go test ./internal/lyxcwd/...
```

## Shared Decisions

_Cross-cutting decisions every batch inherits: naming conventions, error-handling posture, test frameworks, style/lint constraints.
One subsection per decision.
Batch-local decisions live in each batch file._

### Decision: `OnDone` replaces sequential routing entirely, no fallback

- **Decision:** `ProducerDef.OnDone string` is the only thing that routes a `Done` verdict.
  Set → `Done` jumps to the named producer, forward or backward.
  Empty → `Done` here finishes the whole `Shed` (`state: "done"`, `Result.Outcome = RunDone`), regardless of the producer's list position.
  `run.go`'s `indexAfter` helper and its `def.Name == s.Producers[len(s.Producers)-1].Name` check are deleted outright.
  On the terminal path `current_producer` keeps the just-finished producer's own name, never the empty string.
- **Rationale:** a hybrid where some producers route by physical position and others by an explicit field forces a reader to work out which mode a given `ProducerDef` is in before list order can be trusted.
  Going fully explicit means one `ProducerDef` read in isolation tells the whole story.
- **Applies to:** all batches

### Decision: the bounce budget is per-producer and episode-scoped, derived from `Status.History`

- **Decision:** producer X's budget counts the `history[]` entries where `producer == X.Name && outcome == "stuck"` that appear **after X's most recent entry with `producer == X.Name && outcome == "done"`**; all of X's `Stuck` entries when X has no `Done` entry anywhere.
  The count is read from `st.History` as it stood at step 1 — the **pre-append** slice — never from `nextHistory`.
  The run-wide in-memory `bouncesRemaining` counter is deleted.
- **Rationale:** the count spans invocations, crashes, and human resumes, so a crash-restart loop is bounded, while a producer that genuinely succeeds gets its budget back.
  Reading the pre-append slice is what pins "a budget of three performs three bounce-backs and blocks on the fourth `Stuck`"; a post-append read shifts the boundary by one.
- **Applies to:** all batches

### Decision: `MaxBounces: 0` means inherit, at both levels

- **Decision:** `ProducerDef.MaxBounces` of `0` inherits `Shed.MaxBounces`, which in turn falls back to `defaultMaxBounces = 10` when itself `0`.
  `0` never means "no bounces allowed" at either level.
  `Shed.MaxBounces` keeps its name; only its doc comment changes.
- **Rationale:** it is the convention `Shed.MaxBounces` already uses, and renaming would ripple through `loomshed.Deps`, `internal/loomcli/wiring.go`, and three test files for a doc-comment-sized gain.
- **Applies to:** all batches

### Decision: `Segment` is a grouping label enforced only through `OnStuck`

- **Decision:** `ProducerDef.Segment string`, empty meaning standalone.
  `validate()` requires a producer with a non-empty `OnStuck` to name a target whose `Segment` is string-equal to its own; `""` compares equal to `""`, so every existing `loomshed` row passes as one implicit standalone group.
  `Segment` has no other mechanical meaning in this task — it does not scope the budget, does not affect `OnDone`, and is not otherwise validated.
- **Rationale:** one plain equality rule with no special case, biting exactly where segments exist.
  `OnDone` gets no such restriction, because crossing out of a segment on approval is the point.
- **Applies to:** batch 1, batch 3

### Decision: no run-wide cap survives this task, and that is deliberate

- **Decision:** after this task there is no run-wide bounce cap.
  Within one set of episodes the aggregate is bounded by the **sum** of the participating producers' effective `MaxBounces`, so an A↔B cycle costs `2×budget` — a deliberate price, not an oversight.
  Across episodes the lifetime total is unbounded, because a reset is earned by a producer succeeding, or granted once after a hard failure a human had to resolve.
  A `Done` cycle is unbounded by design, stopped only by pause or cancellation.
- **Rationale:** the thing worth bounding is wasted spend without progress, and one run-wide counter cannot tell that from progress — the same conflation, at run scope, that this task removes at producer scope.
- **Applies to:** all batches

### Decision: docs land alongside the code, and the inventory is grep-backed

- **Decision:** every doc statement this task falsifies is rewritten in batch 4, and batch 4 runs a grep sweep over `internal/**/*.go` doc comments, `manifest/*.md`, `manifest/designs/*.md`, and `contracts/specs/*.md` for `in what order`, `next entry`, `next, separate producer`, `next producer in the list`, `bounce budget`, `MaxBounces`, `last entry`, `bounces back`, committing a disposition for every hit.
  New prose about the budget must state the **inversion explicitly** — naming the old per-`Run`-call reset, saying it was deliberate, and saying why it was overturned.
- **Rationale:** describing only the new behavior leaves the old reset looking like an accidental omission, which is exactly how a future reader reintroduces it as a bugfix.
  A hand-enumerated doc inventory is exactly what goes stale.
- **Applies to:** batch 4

### Decision: Go style and seam discipline are unchanged

- **Decision:** no new import is added to `internal/shedengine` production code — the Shed Producer-Seam Invariant allows only the standard library, `internal/state`, and `internal/lock`, and nothing here needs more.
  Every new `validate()` rule returns its own distinct `"shedengine: "`-prefixed message sharing wording with no other rule.
  Both `blocked` reason strings (`"stuck with no OnStuck target"`, `"bounce budget exhausted"`) keep their exact current text, still written identically to the persisted `error` field and `Result.Reason`.
  Markdown edits follow the repo's semantic-line-break rule: one sentence per line, plain newlines.
- **Rationale:** the invariants are machine-enforced by `internal/shedengine/seam_enforcement_test.go` and `internal/loomshed/seam_enforcement_test.go`; the reason strings are pinned verbatim by `manifest/designs/shed.md` and by assertions in two packages.
- **Applies to:** all batches

## All Files Touched

_Full union of every `Creates:` / `Edits:` / `Moves:` **target** path across every batch, sorted alphabetically (Move **source** paths are excluded — they disappear, like `Deletes:` tokens).
Cards are the source of truth;
this section is the input `_plan_validate.py`'s `all-files-touched-mismatch` check cross-references against the derived union of every card's `Edits:`/`Creates:`/Move-target paths, to catch drift between the hand/agent-maintained list here and that derived union._

- `contracts/specs/loom-status-spec.md`
- `internal/loomcli/wiring.go`
- `internal/loomshed/loomshed.go`
- `internal/loomshed/loomshed_test.go`
- `internal/loomshed/resume_test.go`
- `internal/shedengine/doc.go`
- `internal/shedengine/producer.go`
- `internal/shedengine/run.go`
- `internal/shedengine/run_pause_test.go`
- `internal/shedengine/run_persist_test.go`
- `internal/shedengine/run_routing_test.go`
- `internal/shedengine/shed.go`
- `internal/shedengine/testsupport_test.go`
- `internal/shedengine/validate.go`
- `internal/shedengine/validate_test.go`
- `manifest/designs/loom.md`
- `manifest/designs/shed.md`
- `manifest/parallel-work.md`
- `manifest/roadmap.md`
